"""REAL dbt Table SQL-in-flight SIGKILL, persisted PG lease replay and publish.

Only a disposable, locally labeled SQL Server/PG/CH/dbt stack is allowed.
The query must be OBSERVED running in ClickHouse system.processes before kill.
"""
import json
import os
import re
import time
from pathlib import Path
import pytest
import test_core_live as f

pytestmark=pytest.mark.skipif(
  os.getenv("DATAFLOW_LIVE_QA_OPT_IN")!="true" or
  os.getenv("DATAFLOW_CORE_MID_SQL_OPT_IN")!="true",
  reason="fresh disposable worker-query SIGKILL requires two opt-ins",
)
OUT=Path("qa-results/core-run6b3")

def wait(fn,label,timeout=140):
    deadline=time.monotonic()+timeout
    while time.monotonic()<deadline:
        value=fn()
        if value:
            return value
        time.sleep(.25)
    pytest.fail("Timeout: "+label)

def service(name):
    cid=f.shell("docker","compose","--project-name","dataflow-localqa",
        "--env-file",".env.qa.local","-f","deploy/local-qa/compose.qa.yml",
        "--profile","operator","ps","-q",name)
    assert cid
    f.owned(cid)
    return cid

def physical(name):
    return f.ch("SELECT toString(uuid) FROM system.tables "
        f"WHERE database='scratch' AND name='{name}' FORMAT TSV")

def test_dbt_table_actual_sql_sigkill_rebuild_and_publish():
    f.assert_environment()
    OUT.mkdir(parents=True,exist_ok=True)
    worker=service("dbt-worker")
    # SQL below is deliberately slow and entirely synthetic. Never contact
    # ERP or run arbitrary SQL supplied by a user, only disposable scratch.
    suffix=f.SUFFIX
    name="midsql"+suffix
    target="dft_"+name
    query=("SELECT number AS id, sleepEachRow(0.025) AS qa_delay "
           "FROM numbers(1600)")
    assert re.fullmatch("[a-z][a-z0-9_]+",name)
    model=f.pg("INSERT INTO transform_models(name,definition,compiled_sql) "
       f"VALUES('{name}','{{}}'::jsonb,'{query}') RETURNING id").splitlines()[0]
    assert re.fullmatch(r"[0-9a-f-]{36}",model)
    job=f.pg("SELECT gen_random_uuid()::text")
    assert re.fullmatch(r"[0-9a-f-]{36}",job)
    stage="dft_stage_"+job.replace("-","")
    assert physical(target)==""
    f.ch(f"CREATE TABLE scratch.{target} (id UInt64,qa_delay UInt8) "
         "ENGINE=MergeTree ORDER BY id")
    f.ch(f"INSERT INTO scratch.{target} VALUES (1,0)")
    original=physical(target)
    assert original
    f.pg("INSERT INTO table_runs(id,model_id,model_name,compiled_sql,"
         "source_definition,stage_name,target_name,unique_column) "
         f"VALUES('{job}'::uuid,'{model}'::uuid,'{name}',"
         f"'{query}','{{}}'::jsonb,'{stage}','{target}','id')")
    killed=False
    try:
        first=wait(lambda: (v if v[0]=="building" and len(v)==3 else None)
          if (v:=f.pg("SELECT state||'|'||attempt_count||'|'||lease_owner::text "
                     f"FROM table_runs WHERE id='{job}'").split("|")) else None,
          "dbt Table worker acquired real building lease",65)
        assert first[1]=="1"
        owner=first[2]
        assert re.fullmatch(r"[0-9a-f-]{36}",owner)
        def running():
            # Explicit self-exclusion so this monitoring query does not count itself.
            n=f.ch("SELECT count() FROM system.processes WHERE "
                   "position(query,'sleepEachRow')>0 "
                   "AND position(query,'system.processes')=0 FORMAT TSV")
            observed=f.pg("SELECT state||'|'||attempt_count||'|'||"
               "coalesce(left(error_message,180),'') FROM table_runs "
               f"WHERE id='{job}'")
            current=observed.split("|")
            if current[0] in ("failed","ready"):
                diagnostic={"dbt_table_state":current[0],"attempts":current[1],
                  "error_message":current[2] if len(current)>2 else "",
                  "system_process_count":n}
                (OUT/"mid-sql-diagnostic.json").write_text(
                    json.dumps(diagnostic,indent=2)+"\\n")
                pytest.fail("dbt job terminated before active SQL was seen: "+str(diagnostic))
            return n.isdigit() and int(n)>0
        wait(running,"actual active synthetic SQL executing in ClickHouse",95)
        assert physical(target)==original
        f.shell("docker","kill","--signal=KILL",worker)
        killed=True
        assert f.pg(f"SELECT state FROM table_runs WHERE id='{job}'")=="building"
        assert physical(target)==original
        # Shorten the PREEXISTING QA job lease only after actual SIGKILL.
        # Normal 10-minute production lease setting is NEVER modified.
        changed=f.pg("UPDATE table_runs SET lease_until=clock_timestamp()-interval '1 second' "
          f"WHERE id='{job}' AND lease_owner='{owner}'::uuid "
          "AND state='building' RETURNING id").splitlines()[0]
        assert changed==job
        f.shell("docker","start",worker)
        killed=False
        ready=wait(lambda: (r if r[0] in ("ready","failed") else None)
           if (r:=f.pg("SELECT state||'|'||attempt_count||'|'||"
              "coalesce(rows_verified,0) FROM table_runs "
              f"WHERE id='{job}'").split("|")) else None,
           "dbt rebuild and DQ after active SQL SIGKILL",200)
        assert ready==["ready","2","1600"],ready
        assert physical(target)==original, "unapproved build changed published table"
        assert f.ch(f"SELECT count(),uniqExact(id) FROM scratch.{stage} FORMAT TSV")=="1600\t1600"
        new=physical(stage)
        assert new and new!=original
        # Deliberately approved change only after dbt completed and DQ verified.
        changed=f.pg("UPDATE table_runs SET state='publishing',attempt_count=0 "
           f"WHERE id='{job}' AND state='ready' RETURNING id").splitlines()[0]
        assert changed==job
        published=wait(lambda: (r if r[0] in ("published","failed") else None)
           if (r:=f.pg("SELECT state||'|'||attempt_count FROM table_runs "
               f"WHERE id='{job}'").split("|")) else None,
           "fenced and verified atomic table publish",70)
        assert published[0]=="published",published
        assert physical(target)==new
        assert physical(stage)==original
        assert f.ch(f"SELECT count(),uniqExact(id) FROM scratch.{target} FORMAT TSV")=="1600\t1600"
        (OUT/"mid-sql-result.json").write_text(json.dumps({
          "gate":"CORE-RUN-6B-3","method":"real dbt Table build with active ClickHouse sleepEachRow SQL",
          "actual_sql_observed_in_system_processes":True,
          "signal":"SIGKILL Docker dbt-worker during active SQL",
          "post_crash_lease_state":"building","attempts":2,
          "rows_verified":1600,"old_published_uuid_unmodified_preapproval":True,
          "atomic_swap_after_DQ":True,"backup_uuid_matches_old":True,
          "source":"synthetic scratch.numbers(1600); NO ERP source writes",
        },indent=2)+"\n")
    finally:
        if killed and f.shell("docker","inspect","-f","{{.State.Running}}",worker)!="true":
            f.shell("docker","start",worker)
