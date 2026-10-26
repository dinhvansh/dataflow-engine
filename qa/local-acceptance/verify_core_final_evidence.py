#!/usr/bin/env python3
"""Read-only CORE acceptance evidence gate; does not run Docker or touch DB."""
import argparse
import csv
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[2]
MANDATORY=("GIT","GO","WEB","UI-MOCK","CORE8B","FAULT-API",
           "FAULT-LEASE","DQ-NEGATIVE","FAULT-MIDSQL")
VALID={"PASS","FAIL","BLOCKED","NOT_RUN"}
LIVE=("CORE8B","FAULT-API","FAULT-LEASE","DQ-NEGATIVE","FAULT-MIDSQL")
SCRIPTS={
 "CORE8B":"deploy/local-qa/run-core-operator.sh",
 "FAULT-API":"deploy/local-qa/run-core-faults.sh",
 "FAULT-LEASE":"deploy/local-qa/run-core-6b.sh",
 "DQ-NEGATIVE":"deploy/local-qa/run-core-6b-dq.sh",
 "FAULT-MIDSQL":"deploy/local-qa/run-core-mid-sql.sh",
}
REQUIRED_JSON={
 "CORE8B":("qa-results/core-run5/operator-result.json",
            ("graph_run_id","immutable_graph_snapshot_survives_reload",
             "old_run_snapshot_survives_graph_edit","stale_revision_launch_no_managed_run",
             "interactive_editor_controls_qa")),
 "FAULT-API":("qa-results/core-run6/fault-result.json",()),
 "FAULT-LEASE":("qa-results/core-run6b/fencing-result.json",()),
 "DQ-NEGATIVE":("qa-results/core-run6b-dq/negative-dq-concurrency.json",()),
 "FAULT-MIDSQL":("qa-results/core-run6b3/mid-sql-result.json",()),
}

def contained_evidence(relative):
 if not relative or relative.startswith("/") or "://" in relative:
  raise ValueError("Evidence must be relative local file")
 file=(ROOT/relative).resolve()
 if not file.is_relative_to(ROOT.resolve()):
  raise ValueError("Evidence escapes repository")
 if not file.is_file() or file.stat().st_size==0:
  raise ValueError("Missing/empty actual evidence: "+relative)
 return file

def contract():
 for script in SCRIPTS.values():
  if not (ROOT/script).is_file():
   raise ValueError("Missing real QA script: "+script)
 browser=(ROOT/"qa/browser/test_core_live.py").read_text()
 for expected in ("old_run_snapshot_survives_graph_edit",
                  "stale_revision_launch_no_managed_run",
                  "interactive_editor_controls_qa"):
  if expected not in browser:
   raise ValueError("CORE8B graph revision lifecycle assertion absent")
 return {"contract":"PASS","note":"No QA run performed; this validates manifest only"}

def verify(report_dir):
 folder=Path(report_dir)
 if not folder.is_absolute():folder=ROOT/folder
 folder=folder.resolve()
 if not folder.is_relative_to((ROOT/"qa-results").resolve()):
  raise ValueError("Reports must be under qa-results")
 matrix=contained_evidence(str((folder/"02_TEST_MATRIX.csv").relative_to(ROOT)))
 with matrix.open(newline="",encoding="utf-8") as f:
  reader=csv.DictReader(f)
  if not {"id","status","actual","elapsed_s","evidence"}.issubset(reader.fieldnames or []):
   raise ValueError("Missing required matrix columns")
  rows=list(reader)
 ids=[r["id"] for r in rows]
 if len(set(ids))!=len(ids):raise ValueError("Duplicate test case IDs")
 indexed={r["id"]:r for r in rows}
 if not set(MANDATORY).issubset(indexed):
  raise ValueError("Missing mandatory case IDs")
 if "SCALE-7M" not in indexed or indexed["SCALE-7M"]["status"]!="NOT_RUN":
  raise ValueError("Deferred 7m must be explicitly NOT_RUN")
 for row in rows:
  if row["status"] not in VALID:raise ValueError("Invalid QA status")
  if row["status"]=="PASS":
   if not row["actual"].strip() or not row["elapsed_s"].strip():
    raise ValueError("PASS needs observed value and elapsed time: "+row["id"])
   try:
    if float(row["elapsed_s"])<=0:raise ValueError("Elapsed time must be >0")
   except ValueError as error:
    raise ValueError("PASS requires numeric elapsed time") from error
   paths=[s.strip() for s in row["evidence"].split(";") if s.strip()]
   if not paths:raise ValueError("PASS without evidence: "+row["id"])
   for item in paths:contained_evidence(item)
 for case in LIVE:
  if indexed[case]["status"]=="PASS":
   path,keys=REQUIRED_JSON[case]
   payload=json.loads(contained_evidence(path).read_text(encoding="utf-8"))
   for key in keys:
    if key not in payload or payload[key] in (None,False,""):
     raise ValueError(case+" lacks required real 8B assertion: "+key)
 # Forbid a claimed full PASS if any important case did not actually run.
 good=all(indexed[case]["status"]=="PASS" for case in MANDATORY)
 result={"status":"PASS SYNTHETIC CORE QA" if good else "INCOMPLETE OR FAILED",
         "cases":len(rows),"mandatory_pass":sum(indexed[c]["status"]=="PASS" for c in MANDATORY),
         "mandatory_total":len(MANDATORY),"scale_7m":"NOT_RUN","production":"NO-GO"}
 return result

def main():
 parser=argparse.ArgumentParser(description=__doc__)
 parser.add_argument("--contract",action="store_true",help="Static-only, NO Docker")
 parser.add_argument("--report",help="qa-results/<id> directory to verify")
 args=parser.parse_args()
 if args.contract==bool(args.report):
  parser.error("Choose exactly one of --contract or --report")
 try:
  result=contract() if args.contract else verify(args.report)
  print(json.dumps(result,sort_keys=True))
  return 0 if result.get("status","PASS") not in ("INCOMPLETE OR FAILED",) else 2
 except (ValueError,OSError,KeyError,json.JSONDecodeError) as error:
  print("QA EVIDENCE INVALID:",str(error))
  return 2

if __name__=="__main__":
 raise SystemExit(main())
