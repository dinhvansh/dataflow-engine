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
