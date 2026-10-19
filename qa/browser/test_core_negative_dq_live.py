"""CORE-RUN-6B-2: real concurrent HTTP Core Run and negative DQ no-publish."""

import concurrent.futures
import json
import os
import threading
import time
import urllib.error
from pathlib import Path

import pytest
import test_core_live as f
from qa_environment import evidence_environment

pytestmark=pytest.mark.skipif(
 os.getenv("DATAFLOW_LIVE_QA_OPT_IN")!="true" or
 os.getenv("DATAFLOW_CORE_6B_DQ_OPT_IN")!="true",
 reason="explicit disposable Core 6B negative DQ opt-ins required",
)
OUT=Path("qa-results/core-run6b-dq")

def status(run_id):
 code,item=f.api("/api/v1/core/runs?id="+run_id)
 assert code==200
 return item

def wait_done(run_id,seconds=210):
 end=time.monotonic()+seconds
 while time.monotonic()<end:
  row=status(run_id)
  if row["status"] in ("failed","succeeded"):
   return row
  time.sleep(.5)
 pytest.fail("real core run did not terminate "+run_id)

def target_uuid(name):
 return f.ch("SELECT toString(uuid) FROM system.tables "
    f"WHERE database='scratch' AND name='{name}' FORMAT TSV")

def test_real_concurrent_core_requests_and_failed_dq_never_publish():
 f.assert_environment()
 OUT.mkdir(parents=True,exist_ok=True)
 stream,model=f.build_fixture()
 request={"stream_id":stream,"model_id":model,"unique_column":"stream"}
 code,first=f.api("/api/v1/core/runs","POST",request)
 assert code==202
 first=wait_done(first["run_id"])
 assert first["status"]=="succeeded",first
 assert first["rows_verified"]==1
 assert f.ch(f"SELECT stream,events FROM scratch.{f.TARGET} FORMAT TSV")==f"{stream}\t10000"
 stable=target_uuid(f.TARGET)
 assert stable

 # Six synchronized real HTTP POSTs through the single real Go API.
 # Exactly one durable core job may be accepted; the five others MUST
 # receive PG-backed conflict 409, never silently enqueue a duplicate.
 barrier=threading.Barrier(6, timeout=20)
 def submit(_):
  barrier.wait()
  try:
   code,response=f.api("/api/v1/core/runs","POST",request)
   return code,response
  except urllib.error.HTTPError as e:
   return e.code,{"refused":e.read().decode()[:120]}
 with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
  submissions=list(pool.map(submit,range(6)))
 codes=[code for code,_ in submissions]
 assert sorted(codes)==[202,409,409,409,409,409],submissions
 second=next(d["run_id"] for code,d in submissions if code==202)
 completed=wait_done(second)
 assert completed["status"]=="succeeded",completed
 assert completed["has_backup"] is True
 assert f.pg(f"SELECT rows_written FROM managed_runs "
             f"WHERE id='{completed['managed_run_id']}'")=="0"
 assert f.ch(f"SELECT stream,events FROM scratch.{f.TARGET} FORMAT TSV")==f"{stream}\t10000"
 stable=target_uuid(f.TARGET)
 assert stable

 # Real saved Visual model deliberately produces zero rows (not a mock
 # failed status). dbt VIEW execution must succeed, DQ must fail, and the
 # coordinator must never create or exchange its target table.
 bad_name="cr6b_"+f.SUFFIX+"_empty"
 bad={"name":bad_name,"mode":"visual",
      "source":{"database":"scratch","table":"dataflow_raw_events"},
      "steps":[{"id":"deny_all","type":"filter","column":"_pk",
                "operator":"equals","value":"__never_a_fixture_pk__",
                "value_type":"text"}]}
 code,saved=f.api("/api/v1/transform/models","POST",{"model":bad})
 assert code==201,saved
 bad_model=saved["id"]
 new_target="dft_core_"+bad_name
 assert target_uuid(new_target)==""
 code,queued=f.api("/api/v1/core/runs","POST",
                   {"stream_id":stream,"model_id":bad_model,"unique_column":"_pk"})
 assert code==202,queued
 rejected=wait_done(queued["run_id"])
 assert rejected["status"]=="failed",rejected
 assert rejected["model_flow_status"]=="failed",rejected
 assert target_uuid(new_target)=="","A failed DQ physically published a table"
 assert target_uuid(f.TARGET)==stable,"DQ failure replaced an existing published table"
 assert f.ch(f"SELECT stream,events FROM scratch.{f.TARGET} FORMAT TSV")==f"{stream}\t10000"
 assert f.pg("SELECT count(*) FROM model_flow_steps "
             f"WHERE flow_id='{rejected['model_flow_id']}' "
             "AND step_index>0 AND dbt_run_id IS NOT NULL")=="0"
 (OUT/"negative-dq-concurrency.json").write_text(json.dumps({
  "gate":"CORE-RUN-6B-2","environment":evidence_environment(),
  "concurrent_http_posts":6,"accepted":1,"conflicts":5,
  "completed_second_run":second,
  "second_run_rows_written":0,"source_rows":10000,
  "negative_dq_run":queued["run_id"],"negative_dq_core_status":"failed",
  "negative_dq_model_flow_status":"failed",
  "negative_dq_new_target_never_published":True,
  "existing_target_uuid_unchanged_by_negative_dq":True,
  "producer":"real Go API, PG, SQL Server SELECT-only, ClickHouse, dbt",
  "notes":"Does not certify simultaneous in-flight ClickHouse DDL publish race",
 },indent=2)+"\n",encoding="utf-8")
