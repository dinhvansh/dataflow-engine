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
