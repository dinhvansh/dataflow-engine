# Release Status v0.4 — Snapshot Reconciliation Preview

Updated 2026-10-16.

## Implemented
- Full SQL Server key snapshot under existing SNAPSHOT isolation, selecting only SysRowID and SysRevID.
- Read-only verification that SysRowID has a declared, unfiltered, single-key UNIQUE index (a PK also qualifies). No ERP DDL.
- ClickHouse scratch key snapshots grouped by stream UUID and run UUID.
- Count and distinct-key verification before publication. Partial/duplicate snapshots are never promoted.
- PostgreSQL stream lease fencing and transactional publication of verified baseline.
- First snapshot establishes baseline; subsequent snapshots report keys **added**, **missing** (possible physical DELETE), and **version changed**.
- Asynchronous reconciliation API, history UI, CLI and MCP tools. Start operation requires explicit dev environment flag.
- No source mutations, no ClickHouse tombstones.

## Actual automated test evidence

[PR #2](https://github.com/dinhvansh/dataflow-engine/pull/2) merged after [GitHub Actions CI run 38864021334](https://github.com/dinhvansh/dataflow-engine/actions/runs/38864021334) succeeded:
- `go test -race ./...` — PASS
- `go vet ./...` — PASS
- `go build ./cmd/...` — PASS
- `npm run build` — PASS
- **Real ClickHouse integration** using ClickHouse 25.3 container: snapshots with +1 added, -1 missing, ~1 changed; actual anti-join queries — PASS.

## Not yet validated or implemented
- Full SQL Server → PostgreSQL → ClickHouse end-to-end run. Needs an isolated test database with SNAPSHOT isolation and source permissions.
- PostgreSQL race/failure-injection integration; only Go build/unit tests have been run.
- Applying deletion tombstones/current-state repairs, source history, cross-system exactly-once, partitioned resume, production observability/RBAC.
- Real Epicor index/permissions compatibility for PartTran and CPU/version-store cost of long snapshots.
- Automatic schedule for reconciliation and controlled cleanup of unpublished snapshot data.

## Safe local setup
1. Existing PostgreSQL volumes: apply migration `migrations/postgres/006_reconciliation.sql` explicitly after backup. A fresh Compose volume auto-reads migration files.
2. In disposable ClickHouse `scratch`, apply `deploy/clickhouse/002_key_snapshots.sql`; `001_raw_events.sql` remains required for incremental.
3. Confirm SQL Server SNAPSHOT isolation already enabled and `SysRowID` has a unique single-column index.
4. Explicitly enable `DATAFLOW_ALLOW_TEST_LOAD=true`, `DATAFLOW_ENABLE_MANAGED_TEST=true`, `DATAFLOW_ENABLE_RECONCILIATION_TEST=true`.
5. In Web UI: Sync Jobs → Managed Incremental → Snapshot Reconciliation. Capture first baseline, then second snapshot to obtain differences.
6. See [Detailed test and safety protocol](reconciliation-v04.md).

**Do not use missing-key counts to delete ODS records automatically.** Verified detection must be distinguished from deletion repair.
