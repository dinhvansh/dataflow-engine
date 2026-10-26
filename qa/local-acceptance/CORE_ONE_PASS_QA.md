# DataFlow CORE final QA — ONE Codex Local task

Status: **prepared, NOT executed on owner's machine**. Baseline after #115, strict linear CORE-RUN-8B with confirmed hosted disposable end-to-end PASS. This document supersedes ONLY the Core 8B acceptance steps; keep the old v1.3 acceptance catalog as broader separate scope.

## Single task for Codex Local

1. Check git status, origin/main, all PRs and Actions; never overwrite local changes. Check Docker Desktop/WSL, port conflicts, CPU/RAM/disk and whether existing dataflow-localqa volumes contain anything the owner wants to keep. STOP on dirty/diverged state or unsupported environment. Current owner laptop has approximately 8 GB RAM; 16 GB RAM is NOT mandatory.
2. On a dedicated, explicitly approved disposable QA stack ONLY, start with deploy/local-qa/up.sh then verify-isolation.sh. Never modify production ERP or connect to SRVERPTEST / ERPDBTEST; synthetic DataFlowFixtureQA only, app reader SELECT-only.
3. Run Go race/unit, go vet, go build, web npm ci/npm build, and browser mock regressions. Mock-only success is not Core E2E success.
4. For EACH live test below, use a freshly initialized PostgreSQL/ClickHouse QA fixture. The tests require empty raw tables and checkpoints. Confirm the exact Docker container/network/volume labels BEFORE any reset. Ask owner permission before destroying even labeled QA volumes. NEVER reset unrelated containers, production services, company volumes or other apps. No cleanup on a failure until logs and backup UUID evidence are captured.
5. Execute the stage commands in the matrix. If resource guard fails, report BLOCKED and stop; do not disable any preflight or skip a required assertion. Capture stdout, real process exit code, source/target counts, PG run IDs, target UUID, screenshots/trace, and fault results.
6. New 8B mandatory checks: real GUI graph Add/Remove/Validate/Save; save graph A->B->C; backend rejects fork/cycle/disconnected graph, stale CAS save and stale launch; edit saved graph version 1 to version 2; already completed Run remains frozen at version 1 with unchanged graph digest/snapshot and checkpoint; stale version 1 launch MUST return 409 with no new managed run. New browser assertions reside in qa/browser/test_core_live.py.
7. No 7-million-row benchmark in this session; user explicitly deferred due resource constraints. Record NOT RUN/DEFERRED, with no made-up throughput. Parallel/IF-ELSE and production rollout are NOT covered.
8. Fix P0/P1 defects on a LOCAL feature branch only, preserve failed evidence, rerun failing test and related regressions. Do not push/merge unless user authorizes and same-head CI PASS. Deliver a single final evidence pack and link to QA report.

## Canonical real tests — each must be independently executed

| ID | Command | Acceptance |
|---|---|---|
| GIT | git status, fetch, rev-parse | Clean current main; no silent divergence |
| GO | go test -race ./... ; go vet ./... ; go build ./cmd/... | PASS, real commands |
| WEB | npm ci --prefix web ; npm run build --prefix web | PASS TypeScript+release |
| UI-MOCK | python -m pytest qa/browser/test_console.py -q | Mock only; not E2E |
| CORE8B | bash deploy/local-qa/run-core-operator.sh | Real Playwright, 10k synthetic SQL Server -> CH -> dbt A/B/C -> DQ/publish, incremental no duplicates, rollback, versioned graph edit/CAS and immutable snapshot/reload |
| FAULT-API | bash deploy/local-qa/run-core-faults.sh | Actual API SIGKILL, CH network disconnect, PG checkpoint safe recovery |
| FAULT-LEASE | bash deploy/local-qa/run-core-6b.sh | Actual worker SIGKILL after claim, stale fence rejection |
| DQ-NEGATIVE | bash deploy/local-qa/run-core-6b-dq.sh | Concurrent requests only one accepted, failed DQ never publishes |
| FAULT-MIDSQL | bash deploy/local-qa/run-core-mid-sql.sh | Observed active CH SQL SIGKILL, dbt retry/fence, verified rows and physical UUID |
| SCALE-7M | NOT RUN | Deferred by owner; not a PASS |

All live scripts require DATAFLOW_LIVE_QA_OPT_IN=true and a fresh disposable QA stack; their scripts use owned resource and read-only checks. Codex must inspect scripts/flags before execution and secure local secrets. Regenerate clean fixtures between scenarios only under explicit authorization.

## Required final evidence

Write qa-results/<UTC-RUN-ID>/02_TEST_MATRIX.csv with columns id,status,expected,actual,exit_code,elapsed_s,evidence,bug_id; statuses ONLY PASS/FAIL/BLOCKED/NOT_RUN. Keep each full log and safe screenshots from qa-results/core-run5, core-run6, core-run6b, core-run6b-dq, core-run6b3. Generate HANDOFF_TO_CHATGPT.md with exact head SHA, versions, resource envelope, 10k values, checkpoint/UUID before-after, graph_id/digest/version, failed cases, P0/P1 bugs and genuine link/path to artifacts. Also generate qa-results/<UTC-RUN-ID>/core-final-results.json and validate it via the read-only verifier qa/local-acceptance/verify_core_final_evidence.py. Scan files for credentials and machine/company identifiers before sharing. Production always NO-GO until its independent approvals.

Only report full Synthetic CORE QA PASS when every mandatory live stage really executed and has evidence; missing/blocked cases must not be silently counted. Deferred SCALE-7M must be NOT_RUN, not FAIL or PASS.
