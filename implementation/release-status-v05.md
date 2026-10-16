# DataFlow v0.5 — SQL Job + Executable DAG (Developer Preview)

Status: **merged to main**, GitHub Actions CI verified on 2026-10-16. **Not a production Airbyte replacement.**

## Features shipped
- ClickHouse read-only SELECT Job definitions persisted in PostgreSQL.
- Opt-in Job `enabled` and DAG `enabled` switches. User must configure a distinct `DATAFLOW_CH_READONLY_USER` with actual SELECT-only grants.
- DAG Executor: bounded parallel execution (2), ALL dependencies, one retry, timeouts, dependency failures mark downstream steps skipped.
- DAG run creates an **immutable JSON snapshot** of plan and SQL text in PostgreSQL. Live job edits do not alter queued/executing plans.
- REST endpoints `/api/v1/sql/jobs`, `/api/v1/sql/dags`, `/api/v1/sql/dag-runs`.
- React management views: **SQL Jobs** and Pipelines / Executable SQL DAG; CLI/MCP tools also added.
- SQL queries use ClickHouse HTTP `readonly=1`, explicit user and query limits, but ClickHouse grants and network policy are essential.
- SQL Jobs are deliberately restricted to diagnostic SELECT queries in `scratch` execution context (other databases may still be referenced by SQL, so enforce account grants).

## Automated test evidence

[PR #3](https://github.com/dinhvansh/dataflow-engine/pull/3) merged after [GitHub Actions run 38866264338](https://github.com/dinhvansh/dataflow-engine/actions/runs/38866264338) passed both Go and Web jobs.

| Test | Verified result |
| --- | --- |
| Go race tests `go test -race ./...` | PASS |
| Go vet `go vet ./...` | PASS |
| Go CLI/server/MCP build `go build ./cmd/...` | PASS |
| TypeScript/Vite `npm run build` | PASS |
| Live ClickHouse snapshot + bounded SELECT query tests | PASS |
| PostgreSQL 16 + ClickHouse 25.3 live DAG integration | PASS |
| Independent ClickHouse read-only account creation and SELECT grants | PASS |
| Immutable SQL snapshot test (job edited after queueing) | PASS |

## Setup for dev
1. Apply migration `migrations/postgres/007_sql_dag.sql` to the existing PostgreSQL environment after a backup; fresh Compose init uses the migration directory.
2. Configure `DATAFLOW_ALLOW_TEST_LOAD=true`, `DATAFLOW_ENABLE_SQL_JOBS_TEST=true`, `DATAFLOW_TEST_TARGET_DATABASE=scratch` **only on an isolated test installation**.
3. Set `DATAFLOW_CH_READONLY_USER` and `DATAFLOW_CH_READONLY_PASSWORD` to a dedicated, grant-limited SELECT user on ClickHouse. Never reuse the CH destination writer account or expose the API publicly.
4. Run the Go service, then Vite UI; visit **SQL Jobs** to save enabled SELECT tasks, then **Pipelines** to build and run an enabled DAG referencing SQL Job UUIDs.
5. Observe individual task state and retry count in DAG run results; test failure and restart behavior before external workflows use it.

[Detailed usage](sql-dag-v05.md).

## Remaining gates
- SQL Job DML transforms are **not** implemented; only bounded SELECT is allowed.
- DAG queue is not restart-safe; in-process worker handles runs, so a crash can leave queued/running metadata behind.
- PostgreSQL + ClickHouse test is automated, but **Epicor SQL Server + PostgreSQL + ClickHouse** integrated correctness test on user's infrastructure is still pending.
- No full Data Quality remediation, ordered DELETE tombstone application, production RBAC/secrets, scale benchmark or release packaging.
- shadcn-style React UI exists, but no browser-driven end-to-end UI test and no visual drag/drop DAG canvas.
