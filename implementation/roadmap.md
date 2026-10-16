# Implementation roadmap

## Phase 0
- [ ] Repo foundations, CI, PostgreSQL migration and Compose.
- [ ] Verify SQL Server source types, indexes, isolation, permissions.
## Phase 1
- [ ] Full refresh, schema discovery, typed reader, queue, workers, ClickHouse writer.
- [ ] PostgreSQL job/run/checkpoint, API.
- [ ] Validate full PartTran logical counts and crash behavior.
## Phase 2
- [ ] Native SysRevID incremental, composite paging, watermark, fenced checkpoint.
- [ ] Transaction overlap and mid-scan update tests.
## Phase 3
- [ ] Snapshot-key reconciliation and deletion evidence.
- [ ] Quality assertions and discrepancy repair.
## Phase 4
- [ ] Cron scheduler, SQL jobs, DAG dependencies, webhooks, UI, RBAC and monitoring.
## Phase 5
- [ ] Controlled Airbyte benchmark, shadow runs, per-stream migration and rollback.

## Progress update — v0.4, 2026-10-16

- [x] Snapshot source-PK scanner under read-only SQL Server SNAPSHOT isolation.
- [x] Verified snapshot generations with ClickHouse count and distinct-PK checks.
- [x] Detection-only difference report (added/missing/changed); ClickHouse SQL validated in container CI.
- [ ] Real Epicor + ClickHouse + PostgreSQL end-to-end integration and failure injection.
- [ ] Tombstone ordering and correct ODS delete application; not enabled.
- [ ] Durable job queue, true DAG execution and SQL Job runtime.

Refer to [Release Status v0.4](release-status-v04.md) and [Reconciliation Test Guide](reconciliation-v04.md). 

## Progress update — v0.5 SQL Job + DAG Executor, 2026-10-16

- [x] Read-only SQL Job registry in PostgreSQL.
- [x] Executable DAG (ALL joins, bounded concurrency, retries, timeout, skipped downstream).
- [x] Immutable per-run DAG/SQL snapshot, run status API, CLI, MCP and shadcn-style UI.
- [x] Automated test using isolated PostgreSQL + real ClickHouse account and SELECT grants.
- [ ] True ClickHouse SQL transform jobs (INSERT INTO SELECT), permission isolation, materialization controls.
- [ ] Distributed durable queue/restart recovery, cron, alerting.
- [ ] SQL Server Epicor full 3-store integration with failure injection.

## v0.6 stability milestone — 2026-10-16

- [x] Durable PostgreSQL-dispatched SELECT-DAG queue, fenced lease ownership and restart replay.
- [x] Fail-closed lease migration check; SIGTERM/SIGINT cancellation.
- [x] Integration tests against disposable PostgreSQL 16 and ClickHouse 25.3 for expired leases, stale-worker rejection, no double claim and bounded retries.
- [ ] Managed sync and reconciliation crash-safe queued worker recovery.
- [ ] Epicor TEST + ClickHouse + PostgreSQL failure-injection E2E and performance acceptance.
- [ ] Production-safe idempotent SQL materialization and deletion repairs.
See [v0.6 status](release-status-v06.md).


## v0.7 — Managed Sync stability milestone (2026-10-16)

- [x] Durable PostgreSQL queue for Managed Sync API/scheduler with single active run per stream.
- [x] Fenced lease claim, heartbeat and bounded crash attempts.
- [x] Per-attempt ClickHouse event verification, atomic PostgreSQL checkpoint+run completion.
- [x] PostgreSQL 16 + ClickHouse 25.3 integration CI, including transient crashes and row-count mismatch; all Go and Web checks passed.
- [ ] Real SQL Server TEST -> PostgreSQL -> ClickHouse E2E, type mapping, snapshot window correctness, benchmark.
- [ ] Delete capture/reconciliation application, production RBAC and operational high availability.


## v0.8 — Visual Transform Studio (2026-10-16)

- [x] Three-pane Applied Steps visual editor with drag reordering and SQL code mode.
- [x] Safe compiled SELECT: filter, select, rename, calculate, group, sort and limit.
- [x] Read-only step Preview and ClickHouse source columns.
- [x] PostgreSQL saved transformation definitions (migration 010).
- [x] CI: Go race/vet/build, React build and PostgreSQL/ClickHouse query/model integration PASS.
- [ ] dbt Core worker and version-managed dbt-clickhouse adapter.
- [ ] Atomic View/Table/Incremental materializations, DQ gate and rollback.
- [ ] Browser drag/drop E2E and real Epicor TEST integration.
See [v0.8 release status](release-status-v08.md).


## v0.9 — dbt Core View Worker milestone (2026-10-16)

- [x] Optional real dbt Core 1.x worker container, no user CLI needed.
- [x] Transform Studio Build View button, queue/run status, error/log details.
- [x] Immutable per-run model snapshot; persisted PG queue, leases and fencing.
- [x] Scratch View materialization, verified with real dbt and ClickHouse 25.3 CI.
- [x] PostgreSQL queue-to-dbt-to-View integration and Docker image build PASS.
- [ ] dbt model dependencies/lineage visible in UI.
- [ ] Safe Table/Incremental materialization with staged publish, DQ and rollback.
- [ ] Production secrets/RBAC, browser E2E, Epicor TEST 7m-row end-to-end benchmark.
See [v0.9 release status](release-status-v09.md).


## v1.0 — Lineage / Impact Analysis (2026-10-16)

- [x] Visual dependency graph from saved model scratch View references.
- [x] React inspector with direct parents/children and transitive downstream impact.
- [x] Model-source selector in Transform Studio.
- [x] Concurrent-safe cycle rejection on model Save.
- [x] Reject dbt View Build if any ancestor is unbuilt or stale.
- [x] PostgreSQL integration test and complete Go/Web/dbt/ClickHouse CI passed.
- [ ] Automatic dbt `ref()` dependency build and snapshot-consistent multi-model execution.
- [ ] Multi-source joins / column-level lineage, curated data quality contracts.
- [ ] Approved staging/rollback-backed Table and Incremental materializations.
- [ ] Real Epicor SQL Server TEST end-to-end performance and correctness acceptance.

See [v1.0 release status](release-status-v10.md).
