# DataFlow v0.5: SQL Jobs + Executable DAG

## What actually works in code
- Store ClickHouse `SELECT` SQL jobs (max 16 KiB each) in PostgreSQL.
- Save named, versioned-at-run DAG plans whose `job_id` is a registered SQL Job UUID.
- Run an enabled DAG asynchronously through bounded-concurrency Go worker execution.
- Enforce ALL dependency joins, retries (once), 35-second query timeout and failure propagation.
- Freeze the complete DAG + SQL texts into `dag_runs.snapshot` before a run starts. Later job edits do not affect already queued runs.
- Save final per-node attempts, statuses, failures in PostgreSQL; inspect with API, CLI and UI.
- Run query HTTP with `readonly=1`, `max_execution_time=30`, maximum 200 result rows and 512 KiB output.
- No writes to Epicor, no ClickHouse DDL/DML from SQL Jobs.

## Safety
This is a **development preview**, not a multi-tenant SQL execution service.

Both flags needed: `DATAFLOW_ALLOW_TEST_LOAD=true` and `DATAFLOW_ENABLE_SQL_JOBS_TEST=true`. A separate `DATAFLOW_CH_READONLY_USER` / `DATAFLOW_CH_READONLY_PASSWORD` must be set and **must** have read-only grants. The HTTP readonly setting and text `SELECT` validation are additional defense layers, **not substitutes for database-level permissions and network restrictions**.

Limit jobs to disposable ClickHouse database `DATAFLOW_TEST_TARGET_DATABASE=scratch`. A SELECT query might explicitly reference other databases or external table functions, so restrict grants/network on the account itself. Do not reuse the ClickHouse writer user or expose the dev API to untrusted users. DataFlow currently has a shared API token, not per-user RBAC.

SQL transformation jobs that create/insert into ODS or mart are deliberately not supported in this preview. Treat job results as bounded diagnostic queries and DAG sequencing evidence, **not** production ETL.

## Setup
1. Ensure migrations 001–007 were applied to PostgreSQL. For an existing Compose volume, migration 007 must be applied manually (back up first).
2. Create a separate restricted ClickHouse user in the test environment using your DBA-approved process. Grant SELECT only to scratch test objects, no remote/external access. No secrets committed to Git.
3. Start Go backend, then Vite UI.
4. Navigate to SQL Jobs. Save a named SELECT job, e.g. `SELECT count() AS rows FROM scratch.dataflow_raw_events`, mark enabled.
5. In Pipelines → Executable SQL DAG, load jobs, then save a DAG containing their UUIDs:
```json
{"nodes":[{"id":"source_quality","job_id":"<sql-job-uuid>","requires":[]},
{"id":"downstream_check","job_id":"<another-sql-job-uuid>","requires":["source_quality"]}]}
```
6. Enable the pipeline and click Run. Check run ID and per-node attempt/status.
7. Repeat after editing the SQL Job; verify an already queued run was executed with frozen SQL text.
8. Force a failing SQL statement and verify downstream nodes are skipped and error persists.

## CLI
```bash
dataflowctl sql-job-list
dataflowctl sql-job-save job.json
dataflowctl sql-dag-list
dataflowctl sql-dag-save dag.json
dataflowctl sql-dag-run <dag-id>
dataflowctl sql-dag-status <run-id>
```

## Current limitations
- Async jobs are in-process and their persisted run rows can remain queued/running after process crash; no durable restart reconciler for SQL DAG runs yet.
- No full PostgreSQL + ClickHouse live DAG integration was possible in the GitHub runner; CI covers Go executor unit/race tests, Go build, React build, and real ClickHouse query integration.
- No production RBAC, query plan/SQL parsing policy, per-user authorization, secure secrets, alerting, job result dataset storage or DAG visual canvas.
- A SELECT-only job result stores execution *status* and attempts, not complete returned data in metadata.
- DAG has ALL dependencies; ANY/minimum joins remain future work.
