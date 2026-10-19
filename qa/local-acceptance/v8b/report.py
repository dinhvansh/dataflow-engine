#!/usr/bin/env python3
"""Auditable one-shot matrix: never convert NOT RUN into PASS."""
import csv
import datetime
import hashlib
import json
from pathlib import Path
import sys

def main():
    out = Path(sys.argv[1])
    code = int(sys.argv[2])
    expected = (out / "case-catalog.txt").read_text().splitlines()
    with (out / "matrix.tsv").open(newline="") as f:
        actual = {x["case"]: x for x in csv.DictReader(f, delimiter="\t")}
    rows = [{
        "id": k,
        "status": actual[k]["status"] if k in actual else "NOT RUN",
        "elapsed_s": int(actual[k]["seconds"]) if k in actual else None,
        "evidence": actual[k]["log"] if k in actual else "",
    } for k in expected]
    counts = {s: sum(x["status"] == s for x in rows)
              for s in ("PASS", "FAIL", "BLOCKED", "NOT RUN")}
    verdict = "PASS-LOCAL-CORE8B" if code == 0 and counts["PASS"] == len(rows) else "INCOMPLETE-OR-FAIL"
    proofs = [{
        "file": str(f.relative_to(out)),
        "sha256": hashlib.sha256(f.read_bytes()).hexdigest(),
        "bytes": f.stat().st_size,
    } for f in sorted((out / "logs").glob("*.log"))]
    result = {
        "verdict": verdict, "exit_code": code,
        "timestamp_utc": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "git_sha": (out / "git-sha.txt").read_text().strip(),
        "source": "ISOLATED_SYNTHETIC_QA",
        "counts": counts, "cases": rows, "log_manifest": proofs,
        "seven_million_benchmark": "NOT RUN BY DESIGN",
        "real_erp_writes": "PROHIBITED",
        "owner_signoff": "OPEN",
        "production": "NO-GO: separate security/performance gates",
    }
    (out / "result.json").write_text(json.dumps(result, indent=2) + "\n")
    lines = [
        "# Core 8B Codex Local acceptance", "",
        f"Verdict: **{verdict}**, Git SHA {result['git_sha']}",
        f"Results: {counts}, 7M: **NOT RUN**, production: **NO-GO**",
        "", "| Case | Result | Seconds | Evidence |", "|---|---|---:|---|"
    ]
    for x in rows:
        elapsed = str(x["elapsed_s"]) if x["elapsed_s"] is not None else "—"
        lines.append(f"| {x['id']} | {x['status']} | {elapsed} | {x['evidence']} |")
    lines += [
        "", "## Manual review",
        "Review Playwright screenshots, actual run UUIDs, immutable graph revisions,",
        "ClickHouse physical UUIDs, PG checkpoints, DQ status and crash evidence.",
        "Shell exit 0 by itself is NOT semantic proof. FAIL preserves QA state.",
        "", "## Deferred",
        "7m benchmark: NOT RUN. ERP production writes: PROHIBITED.",
        "Parallel/IF-ELSE, owner signoff, Vault/IdP/TLS: NOT CERTIFIED.",
    ]
    (out / "HANDOFF_TO_CHATGPT.md").write_text("\n".join(lines) + "\n")

if __name__ == "__main__":
    main()
