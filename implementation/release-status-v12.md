# DataFlow v1.2 — Auto Build Dependencies (Developer Preview)

**Merged:** [PR #10](https://github.com/dinhvansh/dataflow-engine/pull/10), commit `975d87f664d078a5cfb5063b4f2886089890d0f9`.

**CI verified:** [GitHub Actions run 38911890898](https://github.com/dinhvansh/dataflow-engine/actions/runs/38911890898), Go **PASS** and Web **PASS**.

## Shipped behavior

- **One-click Auto Build** from Data Lineage: select final Visual model C, click **Run Model + Dependencies**, and DataFlow resolves/snapshots an upstream-first A → B → C chain.
- **Durable parent flow:** migration `013_auto_build_flows.sql` stores `model_flow_runs` and `model_flow_steps`, frozen SQL/model definitions, linked child dbt run IDs, statuses, DQ results and error messages.
- **Reuse existing dbt View worker:** Go coordinator queues exactly one child `dbt_runs` model at a time. Python/dbt never runs inside the Go HTTP API.
- **Data quality gating:** after successful dbt View creation, query the View using the dedicated SELECT-only account. Enforce nonempty results and a 1m row TEST limit. Only then start the next dependent model.
- **Failure and drift stops:** failed dbt child, missing View, DQ failure, missing dependency, cycle, source outside scratch, SQL editor ambiguity, changed current/ancestor SQL all prevent subsequent models running.
- **Restart recovery:** a DBT child run UUID and parent current step are committed together in PostgreSQL. API restart resumes from persistent metadata instead of scheduling duplicate child runs.
- **Conservative concurrency:** one parent auto-build flow at a time in scratch TEST; an active manual dbt run causes the next step to wait instead of taking over its lease.
- **Live visual status:** model step queue/running/succeeded/failed, verified row counts, and flow error displayed in Data Lineage.

## Test evidence

- Go `go test -race ./...`, `go vet ./...`, `go build ./cmd/...` — PASS.
- React TypeScript/Vite production build — PASS.
- PostgreSQL16 + ClickHouse25.3 + pinned dbt Core 1.x E2E:
  - Actual dbt creates A, B, C Views in dependency order, final SELECT validates values, all 3 DQ row checks PASS.
  - Simulated API process restart between A and B resumes from PostgreSQL.
  - Injected A failure causes parent failed and B not queued.
  - Model SQL changed before a step is queued rejects the flow.
  - Previously completed ancestor A SQL changed after A success but before B runs also rejects the flow.
  - A second concurrent parent flow cannot be queued.
- Existing durable Sync, SQL DAG, Transform Studio, Lineage, dbt View, Safe Table and Docker worker build regression suite — PASS.

## How to enable on isolated TEST (not production)

1. Back up PostgreSQL metadata, apply migration `migrations/postgres/013_auto_build_flows.sql` after 010–012. Existing persistent PostgreSQL volumes are not automatically migrated.
2. Set `DATAFLOW_ENABLE_AUTO_BUILD_TEST=true` in the Go API along with `DATAFLOW_ALLOW_TEST_LOAD=true`, `DATAFLOW_ENABLE_SQL_JOBS_TEST=true`, `DATAFLOW_ENABLE_DBT_TEST=true`, and `DATAFLOW_TEST_TARGET_DATABASE=scratch`. Configure dedicated `DATAFLOW_CH_READONLY_USER/PASSWORD` with scratch SELECT privileges.
3. Run the existing separate dbt View worker with its scratch-only credentials and test flags. No new worker service is needed.
4. Save A as Visual model sourced from a scratch raw table, B sourced from `scratch.dfv_a`, and C sourced from `scratch.dfv_b`.
5. Open Data Lineage, select C and click Run Model + Dependencies. The UI polls step status.
6. Review the resulting `scratch.dfv_c` only once the flow reports succeeded.

## Important boundaries

- View-only, scratch TEST. No automatic Safe Table Publish or Incremental Materialization.
- **This is not a single dbt `ref()` project invocation:** the existing worker runs each pre-ordered View model through dbt separately; dependency ordering is orchestrated durably by Go using visual model metadata.
- **View publication is not held behind DQ:** dbt updates the View first and then the coordinator checks its row count. External readers with access to the View may see it before the parent flow reports succeeded.
- The checks (nonempty, 1m row cap) are starter tests; they do not validate business totals, primary-key uniqueness, NULLs or financial reconciliation.
- Earlier successful Views are not rolled back automatically when a later model fails. Multi-model atomic publish still requires staged table releases and release coordination.
- Manual dbt builds may interleave; SQL snapshot comparisons reduce drift but do not provide database snapshot isolation across external jobs.
- No live Epicor SQL Server TEST full-chain E2E, large-data benchmark, production RBAC, alerting, or browser drag-and-drop E2E acceptance yet.

[Installation/architecture details](auto-build-v12.md).
