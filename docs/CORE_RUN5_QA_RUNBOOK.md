# CORE-RUN-5 — REAL disposable browser E2E PASS (2026-10-18)

**Evidence:** [PR #98](https://github.com/dinhvansh/dataflow-engine/pull/98) merged main `1d952719`; exact PR-head `bb69664d`. [GitHub-hosted Real Operator E2E run #39131614227](https://github.com/dinhvansh/dataflow-engine/actions/runs/39131614227) **PASS**, `1 passed in 79.97s`. Browser used actual Web, Go API, SQL Server 10,000-row fixture, PostgreSQL, ClickHouse and real dbt; asserted persisted job/run state, incremental 0 new rows on run two, and exact UUID rollback. Retained 14-day artifact: `core-run5-live-operator-evidence` (JSON, screenshot, Playwright trace). This is **CI-hosted actual integration**, not an operator-local machine sign-off. The separate production acceptance remains NO-GO.

The opt-in runner launches an **actual** Go API and compiled React console alongside the
existing disposable SQL Server 2022, PostgreSQL 16, ClickHouse 25.3 and real dbt worker.
It uses the independently supplied and explicitly known synthetic browser bearer only.
All ports bind to 127.0.0.1; Docker ownership and source SELECT-only are checked **before**
the API and browser run. This is **not** production TLS/IdP/Vault acceptance.

## Operator-run acceptance (requires local Docker + Python/Playwright)

```bash
./deploy/local-qa/up.sh
python -m pip install 'pytest==8.4.2' 'playwright==1.56.0'
python -m playwright install chromium
DATAFLOW_LIVE_QA_OPT_IN=true bash deploy/local-qa/run-core-operator.sh
```

Run only on **fresh, approved, disposable** QA volumes. The Playwright suite refuses a
pre-existing `scratch.dataflow_raw_events` table to avoid disrupting any other workload.
Do **not** remove shared/production volumes or modify an actual ERP database to rerun.
Owner must archive evidence first, then reset exclusively owned disposable QA state through
a separately reviewed cleanup. Never point the runner to SRVERPTEST/ERPDBTEST.

## Actual PASS evidence required

- Playwright must navigate real Pipelines UI; authorize, select a server-saved managed
  `Erp.PartTran` stream and A→B→C Visual model chain, confirm Run, and observe the
  backend persisted `succeeded` state (not mocked routes).
- Read-only SQL Server fixture contains exactly 10,000 source rows; PostgreSQL stores real
  managed/core/flow status and source checkpoint; ClickHouse target contains one group with
  10,000 source-derived events. Refresh browser and re-inspect the UUID.
- The second operator Run must be an incremental no-op, physically publish a new immutable
  ClickHouse version, and provide a backed-up version; confirmed browser Rollback must
  restore the exact first target UUID without rewinding checkpoint.
- Evidence includes a structured redacted JSON summary and Playwright screenshot/trace.
  A skipped browser test, syntax-only check, mocked route or a green Go CI alone is **NOT**
  a CORE-RUN-5 PASS. Save observed local QA log/host/resources and sign-off separately.

### Known boundaries

- API-to-SQL Server transport `TrustServerCertificate=true` is allowed **only** for this
  owner-verified loopback disposable SQL Server fixture, not a live/production connection.
- Existing SQL DAG Canvas Run is still separate from this real core operator panel.
- Real GitHub-hosted **browser** QA is now proven, alongside separate four-service SQL QA.
  Independently owner-run local/corporate-operator sign-off and release security remain **OPEN**.
- This runner is neither worker/process SIGKILL proof nor a 7m-row performance test.
