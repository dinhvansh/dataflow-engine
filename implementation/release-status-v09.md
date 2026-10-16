# DataFlow v0.9 — dbt Core View Worker (Developer Preview)

**Merged:** [PR #7](https://github.com/dinhvansh/dataflow-engine/pull/7), commit `d9c120a28d5770f9c829d266da2c78eed2daf64e`.

**Passing CI:** [GitHub Actions 38897813476](https://github.com/dinhvansh/dataflow-engine/actions/runs/38897813476), both Go and Web jobs successful.

## Implemented

1. **Transform Studio Build View (dbt)**: an explicit button after saving a *visual* transformation; unsaved or raw SQL model is not eligible.
2. API `POST /api/v1/transform/dbt/runs` queues a frozen SQL/model snapshot in PostgreSQL 011 with one active run/model; GET returns status, target, attempt count, error, last log segment, dbt run results.
3. **Separate Go dbt worker**: polls persisted queue; uses PostgreSQL FOR UPDATE SKIP LOCKED, unique lease ownership, fencing and finite crash replay (2 claims), never launches arbitrary shell commands from the web/API process.
4. Worker creates short-lived dbt 1.x project/profile/model files, invokes pinned dbt CLI to **materialize View only** in disposable `scratch.dfv_<name>`; verifies run_results.json success. Temporary files (including 0600 credential profiles) are removed.
5. **Optional Docker Compose profile** in `docker-compose.dbt.yml` and isolated `deploy/dbt-worker/Dockerfile`, non-root/read-only rootfs/capabilities dropped.
6. Worker uses dbt-core==1.11.0 and dbt-clickhouse==1.10.3, intentionally not dbt v2 beta.
7. No ERP writes, no regular table or incremental mart writes, no dbt project CLI burden placed on the UI user.

## Verified testing

| Verification | CI result |
| --- | --- |
| Go race tests, vet, Go build (API + worker) | PASS |
| TypeScript + React build | PASS |
| ClickHouse 25.3 source/SQL transformations | PASS |
| Real dbt `run` creates View and SELECT verifies result | PASS |
| PostgreSQL queue -> dbt worker -> ClickHouse View -> PostgreSQL success state | PASS |
| API authentication, duplicate active run rejection, reject Table materialization | PASS |
| PostgreSQL lease/fencing/replay cutoff tests | PASS |
| Docker worker image build with pinned dependencies | PASS |

## Operator checklist for isolated TEST only

- Back up metadata PostgreSQL and apply `migrations/postgres/011_dbt_runs.sql` after migration 010; existing Docker volumes do not auto-run new SQL.
- Set `DATAFLOW_ALLOW_TEST_LOAD=true`, `DATAFLOW_ENABLE_SQL_JOBS_TEST=true`, `DATAFLOW_ENABLE_DBT_TEST=true`, and `DATAFLOW_TEST_TARGET_DATABASE=scratch` in isolated Go API test environment.
- Give dbt worker a *separate restricted ClickHouse test user* with enough scratch SELECT/CREATE/DROP VIEW and required introspection grants, **no production DB or ERP access**.
- Configure `DATAFLOW_DBT_CH_HOST/PORT/USER/PASSWORD` and `DATAFLOW_POSTGRES_DSN`; keep secrets outside Git.
- Start worker with `docker compose --profile dbt-preview -f docker-compose.yml -f docker-compose.dbt.yml up -d --build dbt-worker` (only after verifying no port or network conflict).
- Use Transform Studio to Preview → Save → Build View → inspect queued/running/succeeded or failed → query `scratch.dfv_*`.

[Detailed setup and safety](dbt-worker-v09.md).

## Boundaries & remaining priorities

- **Only View** is materialized by dbt. Table/Incremental and production Mart publication need staging, rollback, DQ and operator approvals.
- No user-facing full model `ref()`/dbt dependency graph, even though the bundled dbt engine supports it.
- New code has no live Epicor TEST SQL Server E2E, browser drag/drop E2E, least-privilege dbt user acceptance, or long-term load benchmark.
- A worker crash can result in *at-least-once* View creation when resumed after lease expiry. Stale completion cannot override current result, but this is not an exactly-once physical execution guarantee.
- A test View may replace an existing `scratch.dfv_<name>` relation; choose disposable model names. Source/target permissions must be enforced by actual ClickHouse grants.
- CI Docker build passed; deployment on the user's infrastructure, resource limits and permissions remains to be accepted before production use.
