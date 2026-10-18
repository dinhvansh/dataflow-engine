"""Disposable worker process SIGKILL with live claimed dbt job, re-claim fencing.

This is a crash AFTER the durable dbt lease claim but BEFORE dbt CLI SQL execution.
It must NOT be misrepresented as killing a dbt subprocess mid-query.
"""
import json
import os
import re
import time
from pathlib import Path
import pytest
import test_core_live as f

pytestmark = pytest.mark.skipif(
    os.getenv("DATAFLOW_LIVE_QA_OPT_IN") != "true" or
    os.getenv("DATAFLOW_CORE_6B_OPT_IN") != "true",
    reason="owner-verified disposable QA opt-ins required",
)
OUT=Path("qa-results/core-run6b")

def until(fn, message, seconds=160):
    deadline=time.monotonic()+seconds
    while time.monotonic()<deadline:
        result=fn()
        if result:
            return result
        time.sleep(.25)
    pytest.fail("Timeout waiting for "+message)

def status(run_id):
    return f.api("/api/v1/core/runs?id="+run_id)[1]

def pg(query):
    return f.pg(query)

def service(name):
    ident=f.shell("docker","compose","--project-name","dataflow-localqa",
        "--env-file",".env.qa.local","-f","deploy/local-qa/compose.qa.yml",
        "--profile","operator","ps","-q",name)
    assert ident, "missing disposable QA service "+name
    f.owned(ident)
    return ident

def test_crash_after_real_dbt_claim_and_fenced_reclaim():
    f.assert_environment()
    OUT.mkdir(parents=True,exist_ok=True)
    source,model_id=f.build_fixture()
    code,r=f.api("/api/v1/core/runs","POST",
        {"stream_id":source,"model_id":model_id,"unique_column":"stream"})
    assert code==202
    first=r["run_id"]
    completed=until(lambda: (v if v["status"] in ("succeeded","failed") else None)
        if (v:=status(first)) else None,"initial source→dbt→DQ→publish",210)
    assert completed["status"]=="succeeded",completed
    assert f.ch(f"SELECT sum(events) FROM scratch.{f.TARGET} FORMAT TSV")=="10000"

    name=f.A
    assert re.fullmatch(r"[a-z][a-z0-9_]+",name)
    dbt_id=pg("INSERT INTO dbt_runs(model_id,model_name,materialization,"
       "target_database,target_relation,source_definition,compiled_sql) "
       "SELECT id,name,'view','scratch','dfv_'||name,definition,compiled_sql "
       f"FROM transform_models WHERE name='{name}' RETURNING id")
    assert re.fullmatch(r"[0-9a-f-]{36}",dbt_id)
    worker=service("dbt-worker")
    # With only this service in QA claim-hold mode, it holds 12s after
    # truly transitioning PG dbt_runs to running/owner/fence.
    row=until(lambda: (x if x[0]=="running" else None)
        if (x:=pg("SELECT status||'|'||attempt_count||'|'||"
            "fence_token||'|'||lease_owner::text FROM dbt_runs "
            f"WHERE id='{dbt_id}'").split("|")) else None,"real dbt worker claim",50)
    assert row[1]=="1" and row[3]
    old_owner,old_fence=row[3],row[2]
    f.shell("docker","kill","--signal=KILL",worker)
    assert pg(f"SELECT status FROM dbt_runs WHERE id='{dbt_id}'")=="running"
    # Operator-approved disposal-only fault acceleration, so CI does not
    # sleep for the normal 10-minute lease. Never touch actual ERP/PG.
    assert pg("UPDATE dbt_runs SET lease_until=clock_timestamp()-interval '1 second' "
        f"WHERE id='{dbt_id}' AND lease_owner='{old_owner}'::uuid "
        f"AND fence_token={old_fence} RETURNING id") == dbt_id
    f.shell("docker","start",worker)
    try:
        second=until(lambda: (x if int(x[1])==2 else None)
            if (x:=pg("SELECT status||'|'||attempt_count||'|'||"
                "fence_token FROM dbt_runs "
                f"WHERE id='{dbt_id}'").split("|")) else None,
                "second real dbt claim",80)
        assert int(second[2])>int(old_fence)
        # Direct stale-owner finishing SQL must affect ZERO rows.
        # Do not call protected Go finish() with fake state.
        reject=pg("WITH attempted AS (UPDATE dbt_runs SET status='failed' "
            f"WHERE id='{dbt_id}' AND lease_owner='{old_owner}'::uuid "
            f"AND fence_token={old_fence} RETURNING id) SELECT count(*) FROM attempted")
        assert reject=="0","stale worker modified dbt job"
        succeeded=until(lambda: (x if x[0]=="succeeded" else None)
            if (x:=pg("SELECT status||'|'||attempt_count FROM dbt_runs "
                f"WHERE id='{dbt_id}'").split("|")) else None,
                "fenced dbt execution after SIGKILL",140)
        assert succeeded[1]=="2"
        assert pg(f"SELECT count(*) FROM dbt_runs WHERE id='{dbt_id}' "
                  "AND fence_token>1 AND status='succeeded'")=="1"
        assert f.ch(f"SELECT sum(events) FROM scratch.{f.TARGET} FORMAT TSV")=="10000"
        (OUT/"fencing-result.json").write_text(json.dumps({
           "gate":"CORE-RUN-6B-1","test":"real dbt worker claimed job SIGKILL, lease replay",
           "scope":"disposable QA; crash post-claim/pre-dbt CLI, not mid-SQL",
           "dbt_run":dbt_id,"attempts":2,"stale_fence_write_rows":0,
           "source":"DataFlowFixtureQA.Erp.PartTran SELECT-only 10000",
           "published_aggregate_unchanged":10000,
        },indent=2)+"\n",encoding="utf-8")
    finally:
        if f.shell("docker","inspect","-f","{{.State.Running}}",worker)!="true":
            f.shell("docker","start",worker)
