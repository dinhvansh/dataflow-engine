# DataFlow v1.1 — Safe Table Materialization (Developer Preview)

**Merged:** [PR #9](https://github.com/dinhvansh/dataflow-engine/pull/9) on 2026-10-16; squash commit `cb5dccbe6fbd0ec7819b6d3f5a80409cb6c5e2af`.

**Verified:** [GitHub Actions run 38905418989](https://github.com/dinhvansh/dataflow-engine/actions/runs/38905418989): Go and Web both passed.

## Shipped
- **Build Staging Table:** dbt Core creates a new physical MergeTree table `scratch.dft_stage_<uuid>` from saved Visual Transformation SQL; current `scratch.dft_<model>` table is unchanged.
- **DQ gate:** reject empty staging data, output above 1,000,000 rows in disposable TEST, and duplicate keys if a unique column is configured.
- **Manual Approve & Publish:** only a verified `ready` run can be approved; API checks explicit confirmation, unchanged model SQL, and state. No automatic scheduled publishing.
- **Atomic publish:** Requires ClickHouse `scratch` database engine `Atomic`. A previous target is atomically exchanged with the staging table using `EXCHANGE TABLES`, preserving the previous physical table as a backup. Initial publication uses a single RENAME because no old table exists.
- **Crash recovery:** PostgreSQL records stage UUID and prior target UUID before external ClickHouse DDL; replays recognize when the newly built stage already occupies the published target and do **not** swap back. Fenced leases and a PostgreSQL row lock held across the ClickHouse exchange stop overlapping workers from issuing competing exchanges.
- **Rollback:** Explicitly confirm rollback to prior physical table, only if source/backup UUIDs still match. Rollback replays are also idempotent. Initial publish has no backup and cannot be rolled back via UI.
- **React UI:** Build Staging Table / Approve & Publish / Rollback with separate status and logs; View-only dbt flow preserved.
- **Opt-in migration:** `migrations/postgres/012_safe_table.sql`, `DATAFLOW_ENABLE_SAFE_TABLE_TEST=true` in isolated TEST Go API and dbt worker; default false.

## CI evidence
- Go `go test -race ./...`, `go vet ./...`, `go build ./cmd/...`: PASS.
- React TypeScript/Vite production build: PASS.
- Existing Sync, SQL DAG, Transform Studio, Data Lineage, dbt View integration: PASS.
- Real ClickHouse 25.3: first publish, second publish, idempotent replay after ambiguous exchange, preserved old table, rollback, replayed rollback, incorrect UUID rejection, empty/duplicate key DQ rejection: PASS.
- Real PostgreSQL 16 + dbt + ClickHouse: queued staging → dbt physical Table → DQ `ready` → explicit transition → publish, new version → rollback: PASS.
- PostgreSQL API: missing token, unsafe unique key, active job rejection, no approval, stale saved SQL, no-backup rollback guard: PASS.
- Docker image build of pinned dbt Core/adapter worker: PASS.

## Production blockers
- **Not Incremental materialization.** Table is rebuilt from the saved SELECT each run.
- This protects one physical TEST table, **not an atomic multi-model/data-warehouse release**.
- No tested Epicor SQL Server → ClickHouse full-chain E2E with realistic 7m row PartTran; no scale or long-running benchmark.
- No production RBAC, detailed audit, approval workflow, automatic backup retention/garbage collection, alerting, or full Disaster Recovery runbook.
- Security depends on a *separate least-privileged* scratch ClickHouse worker; never reuse ERP or production ClickHouse write credentials.
- PostgreSQL+ClickHouse are independent systems, so a failed connection can produce ambiguous states; UUID checks and replay minimize swap ambiguity, but do not prove globally exactly-once execution under arbitrary infrastructure failure.
- Native browser drag/drop E2E and production deployment acceptance are still pending.

## Operator instructions
1. Back up isolated PostgreSQL TEST and apply migration 012 after 011.
2. Use a disposable ClickHouse database named `scratch` with engine Atomic.
3. Enable the extra `DATAFLOW_ENABLE_SAFE_TABLE_TEST=true` flag on API **and** dbt worker, and keep the normal TEST/dbt flags enabled.
4. Restrict ClickHouse role to scratch-only staging, select, exchange and required system introspection.
5. Open Transform Studio, save a Visual model sourced directly from a scratch raw table, Build Staging, review DQ, Approve & Publish, optionally Rollback if a previous table existed.
6. Check the old table still exists under its staging name. Do not manually delete backups until an explicit retention policy is implemented.

[Detailed usage and safety](safe-table-v11.md).
