# v1.1 Safe Table Materialization (TEST-only Developer Preview)

## What this adds
Two-phase dbt materialization for saved visual SQL models using **physical ClickHouse scratch tables**, with explicit human Publish and rollback when a prior version exists.

**NOT enabled in production.** Output target is `scratch.dft_<model_name>`. Staging table is `scratch.dft_stage_<run_uuid_without_hyphens>`. Only `scratch` with ClickHouse **Atomic** database engine is supported.

### User sequence
1. Save a **Visual** Transform Studio model from one direct source in scratch (no downstream model source in this release).
2. Choose optional unique key (example: `InvoiceNum`). Leave blank only when checking key uniqueness is not meaningful.
3. Click **Build Staging Table**. A separate dbt worker creates a physical staging table; it does not replace the current report table.
4. Worker checks table exists, has at least one row, contains at most 1,000,000 rows (TEST cap), and optional `uniqExact(key)==count()`.
5. `ready`: review row count, then explicitly click **Approve & Publish**.
6. Under ClickHouse Atomic, when a current target exists, `EXCHANGE TABLES` swaps old/new *atomically*. The old physical table remains accessible under the staging name as the rollback backup. For initial publication, a single `RENAME TABLE` makes the prepared table available.
7. If a previous table was present, click **Rollback previous table** to atomically exchange it back. Rollback only works when the current table and backup UUIDs still match the approved publication; later changes are rejected.

### Crash safety
- `table_runs` PostgreSQL rows persist queued/building/ready/publishing/published/rolling_back/rolled_back/failed.
- Durable dispatcher uses row locks, lease ownership, fencing tokens and 3-attempt cap, with a maximum 6-minute attempt and 10-minute lease.
- **Critical publish invariant:** publish *prepares* prior table UUID in PostgreSQL **before** issuing EXCHANGE. When a worker dies after EXCHANGE but before PostgreSQL update, replay compares `system.tables.uuid` of stage + target; it recognizes completed exchanges and will NOT exchange back.
- Physical table backup is preserved; no automatic DROP in the worker. Operators must manage retention with an explicit future policy; **do not run ad-hoc cleanup against unknown UUIDs**.
- A failed validation cannot advance to ready/publish; the published version is untouched.

### Guardrails
- Separate `DATAFLOW_ENABLE_SAFE_TABLE_TEST=true` alongside dbt test and scratch flags. Off by default.
- Uses a **dedicated TEST ClickHouse role** capable of necessary scratch SELECT/CREATE TABLE/RENAME/EXCHANGE/system introspection. This role is more powerful than the View-only one and MUST NOT be granted production database access. Do not reuse ERP credentials.
- Only saved Visual models, single direct source from scratch, no `dfv_*` or `dft_*` source in v1.1. Never execute raw user-authored DDL or unrestricted DML through this feature.
- The user must confirm Publish/Rollback in both UI and API; API requires literal `PUBLISH` or `ROLLBACK`.
- Detect stale SQL before accepting Publish: an edited model must be rebuilt into new staging.
- First publish has no prior table to roll back to. An operator can remove the first published target manually in isolated test, but the product does not offer a destructive rollback.
- Current dbt View Build flow remains unchanged.

### Setup
1. Back up existing PostgreSQL TEST. Apply migration `migrations/postgres/012_safe_table.sql` after 010, 011.
2. Set feature flags only in isolated Go API **and worker**: `DATAFLOW_ALLOW_TEST_LOAD=true`, `DATAFLOW_ENABLE_DBT_TEST=true`, `DATAFLOW_ENABLE_SAFE_TABLE_TEST=true`, `DATAFLOW_TEST_TARGET_DATABASE=scratch`.
3. Use a ClickHouse `scratch` database engine `Atomic`. On databases using other engines the worker refuses Publish/Rollback.
4. Use separate least-privileged staging/publish ClickHouse test credentials and verify actual grants. Restrict network and secrets.
5. Start dbt worker from the existing `dbt-preview` compose profile (after applying migration); it polls View and Table queues.
6. Navigate Transform Studio → Save Model → Build Staging Table → inspect DQ → Approve & Publish → optionally Rollback.

### CI acceptance
- Existing Go race, vet, build, web build and PostgreSQL/dbt/ClickHouse regression tests.
- ClickHouse Atomic test: first publish, replay no-op, second publish, replay no-op, preserved backup, rollback, rollback replay no-op, wrong UUID rejection, empty/duplicate key DQ rejection.
- Full PostgreSQL queue → dbt table materialization → DQ `ready` → manual state transition → ClickHouse publish → second version → rollback.
- **No live Epicor SQL Server e2e**, no load benchmark at production scale, no user RBAC or browser E2E; remaining release gates before use beyond isolated TEST.

### Important limitations
- Designed for experimental standalone scratch datasets, not multi-model atomic publishing. One table publication is atomic in the ClickHouse Atomic engine; publishing several related models is not a global transaction.
- Not an Incremental executor. Each dbt table rebuild computes the entire snapshot.
- PostgreSQL and ClickHouse are not one distributed transaction. UUID checks mitigate ambiguous ClickHouse publish; operator alerts/audit, backup retention and disaster recovery still need production hardening.
- No scheduled automatic publish; manual approval remains mandatory.
