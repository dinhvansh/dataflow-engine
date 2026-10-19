# Copy once to Codex Local — CORE final acceptance

**Executable runner:** after verifying owned disposable QA Docker resources and obtaining explicit permission for each reset, follow `qa/local-acceptance/v8b/README.md` and run `deploy/local-qa/run-core8b-one-shot.sh`. It includes Go race/vet/build, Web build, mock Browser, full real Core E2E and independent fault scenarios, then writes `02_TEST_MATRIX.csv` and runs `verify_core_final_evidence.py` before technical PASS. GitHub CI only tests contracts; no owner-local run has occurred.

Repo: https://github.com/dinhvansh/dataflow-engine

Do the full final owner-local QA in ONE task, not piecemeal. Start by checking git main/PR/Actions/divergence, then read docs/PROJECT_HANDOFF_LATEST.md, qa/local-acceptance/CORE_ONE_PASS_QA.md, docs/CORE_RUN8B_MIXED_CANVAS_ACCEPTANCE.md, and the existing qa/local-acceptance/CODEX_LOCAL_QA_MISSION.md. Use CORE_ONE_PASS_QA.md as the CURRENT mandatory Core scope; older v1.3 performance cases are separate.

Inspect Windows/WSL2/Docker resources and all existing QA volumes BEFORE starting. Use only a freshly approved disposable dataflow-localqa SQL Server DataFlowFixtureQA / PostgreSQL / ClickHouse scratch / dbt / Go API / React UI. ERP production/TEST is strictly read-only and is NOT needed for this task. Never alter ERP or any other Docker project. If resetting even QA-labeled volumes could destroy existing work, ASK OWNER FIRST; never silently reset.

Run ALL applicable mandatory tests listed in CORE_ONE_PASS_QA.md in order, each live fault test on a fresh synthetic QA fixture: Go race/unit/vet/build, frontend build, mocked browser (reported separately), REAL Playwright/Core-8B saved graph + edit revision 1→2 + frozen old run + stale CAS/no new managed run, 10k SQL Server→ClickHouse→dbt A→B→C→DQ→publish/rollback, API+CH fault, dbt lease fencing, negative DQ and concurrency, mid-active SQL worker SIGKILL. Preserve actual exit codes and server state. On a P0/P1 failure or resource shortage STOP/FAIL/BLOCKED, do not waive assertions. Fix real bugs on a local branch with user permission, rerun failing tests and regressions; CI PASS before suggesting a merge.

Build qa-results/<UTC-RUN-ID>/02_TEST_MATRIX.csv using exactly the column names in CORE_ONE_PASS_QA.md, per-stage log/evidence paths and a concise HANDOFF_TO_CHATGPT.md. Mark SCALE-7M NOT_RUN (deferred by owner). Run: python qa/local-acceptance/verify_core_final_evidence.py --report qa-results/<UTC-RUN-ID> and fix missing evidence honestly. No production GO claims. Sanitize passwords, credentials, token values and personally identifiable file paths before sharing. Hand back one final verdict, bugs, test matrix, real evidence and a brief suggested fix list.

You may run normal CI/QA commands; do not install system software, escalate permissions, destroy shared resources or merge PRs without approval. If insufficient resources, report BLOCKED with numbers rather than test partial or simulate.
