# DataFlow v1.2 — Auto Build Dependencies (Developer Preview)

## User experience

From **Data Lineage**, choose a saved Visual **Model C** and click **Run Model + Dependencies**. DataFlow snapshots the current model chain and follows it sequentially:

`scratch.raw → Model A → Model B → Model C`

- Automatically discovers `scratch.dfv_<upstream>` relationships from Visual model source metadata, no `ref()` or YAML required.
- Creates a durable parent `model_flow_runs` record and ordered `model_flow_steps` in PostgreSQL.
- An API-local coordinator advances the flow by scheduling a child **dbt View run** into the existing `dbt_runs` PostgreSQL queue (the existing dbt Worker executes the actual materialization). It does not spawn dbt/Python inside the API.
- After a dbt child reports success, performs a **read-only Data Quality check** on its output View: at least one row and at most 1m rows in disposable TEST. Only after passing DQ does the next dependent View get queued.
- A failed dbt child, invalid/missing view output, changed model SQL, missing dependency or cycle stops the flow and records an error.
- UI shows the per-model status (`pending / queued / running / succeeded / failed`) and verified row count.
- On server restart, the coordinator resumes pending parent flows from PostgreSQL and consults the durable child dbt UUID; it never schedules a second copy for a child that already has a linked run. The existing dbt worker retains its original 2-attempt crash/fencing policy.

## Installation

1. Back up isolated PostgreSQL TEST metadata and apply `migrations/postgres/013_auto_build_flows.sql` after migrations 010/011/012.
2. In Go API TEST environment set:
   - `DATAFLOW_ALLOW_TEST_LOAD=true`
   - `DATAFLOW_ENABLE_SQL_JOBS_TEST=true`
   - `DATAFLOW_ENABLE_DBT_TEST=true`
   - `DATAFLOW_ENABLE_AUTO_BUILD_TEST=true`
   - `DATAFLOW_TEST_TARGET_DATABASE=scratch`
   - `DATAFLOW_CH_READONLY_USER` and `DATAFLOW_CH_READONLY_PASSWORD` with SELECT on the disposable scratch Views.
3. Run the existing isolated dbt View worker (v0.9). No separate new worker is required. It must have access only to disposable scratch and required view DDL.
4. Save Visual `a` from `scratch.raw`; Visual `b` from `scratch.dfv_a`; Visual `c` from `scratch.dfv_b`.
5. Open Data Lineage → select `c` → **Run Model + Dependencies**. Observe progress and DQ row counts.

## API

- `POST /api/v1/transform/auto-build` with `{"model_id":"<UUID>"}` returns queued parent run ID plus order.
- `GET /api/v1/transform/auto-build?id=<RUN_UUID>` returns parent run state, ordered per-model step statuses, linked child dbt run IDs, errors and rows verified.

## Reliability and limitations

- **Only single-input saved Visual models, dbt Views, and scratch TEST.** Free-form SQL models are not parsed for dependencies. Table/Incremental auto-build is intentionally NOT enabled.
- Parent flow is persisted and can resume after an API restart; child job status is durable in the existing dbt queue. Parent execution is short database transactions rather than a monolithic in-memory goroutine.
- Only one parent flow is active at once in TEST, preventing overlapping chains from independently rebuilding upstream Views. Existing manually queued dbt model runs may still interleave. If a manual run is active for the next model, the controller waits for it to finish and then queues the flow's own snapshot.
- Before each step is enqueued, model SQL must still equal the snapshot captured at flow creation. An edited model fails the parent flow rather than mixing versions.
- DQ happens **after the View has been created by dbt**. A View is immediately readable through ClickHouse; DQ gates downstream execution and overall success, but **does not atomically hide a failed View output from external consumers**. Full isolation requires staged Table releases or versioned View pointers (future work).
- No global transaction across all A/B/C Views: a failure leaves earlier successful Views available. This is not a rollback of the entire model chain.
- DQ currently validates non-empty and a configurable hardcoded 1m test row cap, not business totals, unique key, NULL ratios or foreign keys. Add per-model DQ contract before production.
- Production user RBAC, extensive browser E2E, long-lived flow timeout/alerting, Epicor SQL Server TEST whole-chain validation, large data benchmarks and multi-model atomic publication are NOT completed.
- No automatically scheduled continuous rebuild; user explicitly clicks Run each time.

## CI acceptance

- Go race/vet/build and React/TypeScript build.
- Unit tests for lineage plan ordering and cycle/missing model rejection.
- PostgreSQL 16 + ClickHouse 25.3 + **actual dbt Core 1.x** E2E A → B → C with DQ row verification, no double-submit, injected upstream failure (B not enqueued), SQL-snapshot drift stops execution.
- Existing dbt View, safe Table, SQL DAG and Managed Sync regressions.
