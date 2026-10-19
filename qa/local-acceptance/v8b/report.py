#!/usr/bin/env python3
"""Write both human and machine-verifiable one-shot QA artifacts.

Never claim a local PASS for skipped/missing tests or absent live JSON evidence.
No access to customer databases: this module reads only locally generated files.
"""
import csv
import datetime
import hashlib
import json
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[3]
REQUIRED = {
    "GIT": "STATIC-SAFETY",
    "GO": "STATIC-GO",
    "WEB": "STATIC-WEB",
    "UI-MOCK": "UI-MOCK",
    "CORE8B": "CORE-8B-E2E",
    "FAULT-LEASE": "CORE-6B-CLAIM",
    "DQ-NEGATIVE": "CORE-6B-DQ",
    "FAULT-MIDSQL": "CORE-6B-MID-SQL",
    "FAULT-API": "CORE-6A-NETWORK",
}
PROOFS = {
    "CORE8B": "qa-results/core-run5/operator-result.json",
    "FAULT-LEASE": "qa-results/core-run6b/fencing-result.json",
    "DQ-NEGATIVE": "qa-results/core-run6b-dq/negative-dq-concurrency.json",
    "FAULT-MIDSQL": "qa-results/core-run6b3/mid-sql-result.json",
    "FAULT-API": "qa-results/core-run6/fault-result.json",
}
FIELDS = ("id", "status", "expected", "actual", "exit_code",
          "elapsed_s", "evidence", "bug_id")


def generate(out: Path, code: int):
    catalog = (out / "case-catalog.txt").read_text(encoding="utf-8").splitlines()
    with (out / "matrix.tsv").open(newline="", encoding="utf-8") as handle:
        reader = csv.DictReader(handle, delimiter="\t")
        stages = {row["case"]: row for row in reader}
    status = lambda value: value if value in {"PASS", "FAIL", "BLOCKED"} else "NOT_RUN"
    rows = []
    for stage in catalog:
        recorded = stages.get(stage)
        rows.append({
            "id": stage,
            "status": status(recorded["status"]) if recorded else "NOT_RUN",
            "elapsed_s": recorded["seconds"] if recorded else None,
            "evidence": recorded["log"] if recorded else "",
        })
    counts = {kind: sum(x["status"] == kind for x in rows)
              for kind in ("PASS", "FAIL", "BLOCKED", "NOT_RUN")}
    verdict = ("PASS-LOCAL-CORE8B" if code == 0 and
               counts["PASS"] == len(rows) and len(catalog) > 0
               else "INCOMPLETE-OR-FAIL")
    proof_manifest = [{
        "file": str(file.relative_to(out)),
        "sha256": hashlib.sha256(file.read_bytes()).hexdigest(),
        "bytes": file.stat().st_size,
    } for file in sorted((out / "logs").glob("*.log"))]
    result = {
        "verdict": verdict, "exit_code": code,
        "timestamp_utc": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "git_sha": (out / "git-sha.txt").read_text(encoding="utf-8").strip(),
        "source": "ISOLATED_SYNTHETIC_QA", "counts": counts,
        "cases": rows, "log_manifest": proof_manifest,
        "seven_million_benchmark": "NOT RUN BY DESIGN",
        "real_erp_writes": "PROHIBITED", "owner_signoff": "OPEN",
        "production": "NO-GO: separate security/performance gates",
    }
    (out / "result.json").write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")

    # The shared official verifier expects exactly this normalized matrix.
    final_rows = []
    for case, stage in REQUIRED.items():
        item = stages.get(stage)
        state = status(item["status"]) if item else "NOT_RUN"
        observed = "Recorded stage completed (check original log)" if state == "PASS" else state
        evidence = []
        if item and item["log"]:
            try:
                evidence.append(str((out / item["log"]).resolve().relative_to(ROOT)))
            except ValueError:
                # CI static-contract tests deliberately use mktemp outside repo.
                evidence.append(item["log"])
        if case in PROOFS and state == "PASS":
            evidence.append(PROOFS[case])
            try:
                payload = json.loads((ROOT / PROOFS[case]).read_text(encoding="utf-8"))
                for label in ("graph_run_id", "first_run_id", "run_id", "source_rows"):
                    if label in payload:
                        observed += "; {}={}".format(label, payload[label])
            except (ValueError, OSError):
                observed += "; structured artifact missing or unreadable"
        final_rows.append({
            "id": case, "status": state,
            "expected": "Real validated synthetic QA; exact case contract in CORE_ONE_PASS_QA.md",
            "actual": observed if state == "PASS" else "Stage not verified",
            "exit_code": "0" if state == "PASS" else ("1" if state == "FAIL" else ""),
            "elapsed_s": item["seconds"] if item else "",
            "evidence": ";".join(evidence), "bug_id": "",
        })
    final_rows.append({
        "id": "SCALE-7M", "status": "NOT_RUN",
        "expected": "Deferred; separate owner benchmark", "actual": "Deferred by owner",
        "exit_code": "", "elapsed_s": "", "evidence": "", "bug_id": "",
    })
    with (out / "02_TEST_MATRIX.csv").open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=FIELDS)
        writer.writeheader()
        writer.writerows(final_rows)

    report_lines = [
        "# Core 8B Codex Local one-pass acceptance", "",
        f"Verdict: **{verdict}** (NOT production approval), Git SHA: {result['git_sha']}",
        f"Case totals: {counts}; 7m: NOT RUN; ERP writes: PROHIBITED", "",
        "| Case | Status | Seconds | Captured log |",
        "|---|---|---:|---|",
    ]
    for row in rows:
        report_lines.append("| {} | {} | {} | {} |".format(
            row["id"], row["status"], row["elapsed_s"] or "—", row["evidence"] or "—"))
    report_lines += [
        "", "## Verification",
        "02_TEST_MATRIX.csv uses the shared CORE evidence-gate format.",
        "Stage PASS alone is not proof: full acceptance also requires the official",
        "verify_core_final_evidence.py --report check and original real fixture JSON.",
        "Review original Playwright screenshots, trace, dbt/PG/ClickHouse UUIDs and checkpoints.",
        "A failure preserves disposable volumes and logs for diagnosis.", "",
        "## Deliberate exclusions",
        "CORE-RUN-7 7m: NOT RUN (owner deferred).",
        "8GB RAM owner hardware performance, parallel/IF-ELSE, real IdP/Vault/TLS",
        "and production security: NOT CERTIFIED. Owner sign-off: OPEN.",
    ]
    (out / "HANDOFF_TO_CHATGPT.md").write_text("\n".join(report_lines) + "\n", encoding="utf-8")


def main():
    if len(sys.argv) != 3:
        raise SystemExit("usage: report.py <qa-results-run-dir> <exit-code>")
    generate(Path(sys.argv[1]), int(sys.argv[2]))


if __name__ == "__main__":
    main()
