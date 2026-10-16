# DataFlow v0.9 — dbt Core Worker Integration (scratch View preview)

## New actual behavior

Transform Studio → **Save Model** → **Build View (dbt)** → PostgreSQL queued dbt run → isolated dbt Worker → dbt CLI project → ClickHouse `scratch.dfv_<model_name>` → status and dbt artifacts in DataFlow UI.

Go API is separate from the worker. No Docker socket or shell command execution in the Go API. Worker creates a temporary, restricted dbt project in a per-run directory and runs a hardcoded dbt CLI command, then deletes the directory.

### Available in v0.9

- One materialization: **View** in **scratch**, named `dfv_<model>`.
- Visual-mode saved transformations only, **no raw SQL-mode builds**.
- Model/source SQL snapshot at enqueue. Changes to saved steps after clicking Build do not change the queued run.
- PostgreSQL queue, one active run per model, `FOR UPDATE SKIP LOCKED`, 10-minute lease, two crash attempts, unique worker lease/fencing per attempt, fixed 5-minute process timeout. Stale workers cannot overwrite run results.
- Python 3.12 image with `dbt-core==1.11.0` and `dbt-clickhouse==1.10.3`, separate non-root container, read-only rootfs, no Linux capabilities, /tmp tmpfs and memory cap.
- dbt CLI `run_results.json`, last 12 KiB logs (credentials redacted), attempt count and final success/failure surfaced via API and UI.
- Only process environment supplies dbt adapter connection credentials; they are never accepted from the browser.
- Replays may recreate the same test VIEW, so this is **at-least-once model execution**, not exactly-once physical writes.

### How to test

1. Keep this entirely off on production! Create an isolated TEST ClickHouse database named `scratch` with the source table already loaded.
2. Back up PostgreSQL, apply migration `011_dbt_runs.sql` after 010 (existing Docker volumes do not auto-apply scripts).
3. Create a *dedicated ClickHouse account* with SELECT on approved scratch source objects and CREATE/DROP VIEW on scratch. Do not give it access to ERP/RAW production databases, DROP TABLE privileges or external network functions. The CI test account is broader only because it controls a disposable container.
4. Set `DATAFLOW_ALLOW_TEST_LOAD=true`, `DATAFLOW_ENABLE_SQL_JOBS_TEST=true`, `DATAFLOW_ENABLE_DBT_TEST=true`, `DATAFLOW_TEST_TARGET_DATABASE=scratch` in Go API TEST environment.
5. Configure dbt worker `DATAFLOW_POSTGRES_DSN`, `DATAFLOW_DBT_CH_HOST`, `DATAFLOW_DBT_CH_PORT`, `DATAFLOW_DBT_CH_USER`, `DATAFLOW_DBT_CH_PASSWORD` and same test flags.
6. Start worker using: `docker compose --profile dbt-preview -f docker-compose.yml -f docker-compose.dbt.yml up -d --build dbt-worker`.
7. In Transform Studio select a TEST table, create visual steps, Preview, Save, then click **Build View (dbt)**. View queue status, errors and ClickHouse relation `scratch.dfv_<model>`.
8. Run `SELECT * FROM scratch.dfv_<model> LIMIT 10` using a test account to confirm output.

### Boundaries

- View materialization modifies scratch metadata only, not ERP; **not fully non-destructive**. A rerun of the same model may replace the existing `dfv_<name>` View. Make sure the test name does not collide with manually created relations.
- PostgreSQL metadata schema enforces one active run/model. A run interrupted after database View creation but before PG acknowledgment will be replayed once after lease expiry.
- No Table or Incremental materialization, no Mart publish, no lineage graph, no automatic DAG of dbt models and no managed SQL Server integration in v0.9.
- A dbt profile is written to a private temporary file with permissions 0600 and then deleted. Python dependencies are pinned to exact published versions for reproducibility; a production release should additionally pin hashes and image digests, use container network egress restrictions and scan images.
- Some clickhouse dbt adapter operations inspect `system.*` tables; grant only necessary permissions after verifying with an isolated account.
- The current DataFlow single bearer token is insufficient for multiple untrusted users. Add per-user RBAC/approval before production transforms.

## Tests

CI must pass Go race tests/vet/build, web build, PostgreSQL queue/fencing and API tests, plus a **real `dbt run` against ClickHouse** proving the `scratch.dfv_*` View exists and returns expected grouped records.

Only merge after passing CI. Further browser E2E and internal Epicor TEST validation remain mandatory.
