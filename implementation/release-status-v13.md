# DataFlow v1.3 — Safe Incremental (Developer Preview)

**Merged:** [PR #11](https://github.com/dinhvansh/dataflow-engine/pull/11), commit `36ffe284a6c4e5d449bd793a30cd77485d54ac34`.

**CI green:** [GitHub Actions run 38916767766](https://github.com/dinhvansh/dataflow-engine/actions/runs/38916767766), Go **PASS**, Web **PASS**.

## Shipped

1. Separate **Incremental CDC** navigation page in the React console with target/delta configuration, immutable initial cursor registration, event/row counts, staging status, explicit Publish and Rollback.
2. New scratch-only `POST/GET /api/v1/transform/incremental/runs` and `POST /api/v1/transform/incremental/action` APIs: authenticated, disabled by default, no silent cursor reset or double active target.
3. Migration `014_safe_incremental.sql`: PostgreSQL target registration, durable pending run metadata, lower/upper version window, physical target/stage UUIDs, fenced lease and checkpoint.
4. **Change semantics:** target key non-null/unique; delta schema equals target payload columns plus `_df_version UInt64` and `_df_deleted UInt8`. Apply the latest event per key in each bounded interval. Handle **INSERT, UPDATE, DELETE/tombstones**, reject duplicated event version, invalid markers and >50k events (TEST).
5. **Copy-on-write staging:** physically clone the published target table, retain unaffected keys, insert latest non-deleted delta rows; verify unique keys and <=1m output rows. An empty result is valid if all keys are deleted.
6. **Crash-safe publish:** prepare prior UUID durably before ClickHouse Atomic exchange; hold PostgreSQL locked/fenced run and target rows during EXCHANGE. Cursor and physical target UUID advance together in a PostgreSQL transaction after publication. UUID-aware retry recognizes an already-swapped physical table and will not swap it back.
7. **Rollback:** preserve previous physical table under the stage name; only the latest matching publication with a backup may be rolled back. Revert target and persisted cursor to the lower bound. New runs may replay changes safely from the restored cursor.
8. **No-changes** runs finish without DDL or cursor movement.

## CI proof

- Go race tests, `go vet`, Go build — PASS.
- React/TypeScript build — PASS.
- ClickHouse 25.3 tests: multiple updates per key, delete marker, new insertion, stage retry without duplication, UUID publish replay no-op, physical rollback, globally duplicated event version rejection — PASS.
- PostgreSQL 16 + ClickHouse 25.3 integration: first cursor 0→4, second 4→7, row contents exact, rollback 7→4, restored dataset and replayable window — PASS.
- PostgreSQL API: bearer auth, invalid key, immutable target/key registration, duplicate active run prevention, rejects publish before DQ and with stale cursor, rejects rollback without matching previous version — PASS.
- Earlier dbt View, Safe Table, Auto Build, Managed Sync and Docker worker build regressions — PASS.

## Operator checklist (isolated TEST ONLY)

- Back up TEST PostgreSQL metadata; apply migration `migrations/postgres/014_safe_incremental.sql` **after migrations 010–013**.
- Set `DATAFLOW_ENABLE_INCREMENTAL_TEST=true` on both Go API and dbt/ClickHouse worker, plus existing `DATAFLOW_ALLOW_TEST_LOAD=true`, `DATAFLOW_ENABLE_DBT_TEST=true`, `DATAFLOW_ENABLE_SAFE_TABLE_TEST=true`, `DATAFLOW_TEST_TARGET_DATABASE=scratch`.
- Create baseline physical `scratch.dft_<name>` and append-only CDC `scratch.dfd_<name>` with payload fields in identical order/types and global unique increasing UInt64 event versions, plus `_df_deleted UInt8`.
- Do not reuse production ClickHouse or Epicor write credentials. Worker should have minimum required DDL on disposable scratch and PostgreSQL queue access.
- Open Incremental CDC → Build Delta Staging → review DQ → Approve & Publish + Cursor → optionally Rollback Data + Cursor.
- Maintain immutable input payload and prevent late events with versions at or below a previously published checkpoint.

## Critical limitations

**This is correctness-first logical incremental with copy-on-write physical publication, not yet high-throughput Incremental.** Although only changed keys are *logically reconciled*, every publish copies unchanged baseline rows into a new staging table. It is deliberately capped at 1m output rows and 50k delta events on TEST. There has been **no 7m-row PartTran speed benchmark**.

**No built-in Epicor delete detection.** Epicor `SysRevID` alone is not a complete tombstone CDC stream. A verified change source and strict version ordering are required; do not connect this feature blindly to ERP.

ClickHouse and PostgreSQL are separate systems. UUID guards make uncertain external exchanges replay-safe, but this is not a distributed transaction, automatic global multi-model publish or an exhaustive proof of exactly-once behavior under every infrastructure fault.

The existing Auto Build Dependencies v1.2 still executes **View** only. v1.3 incremental table updates are manual and are **not** automatically published by lineage flows. Production RBAC, approvals/audit, backup garbage collection, materialized partition optimization, Epicor source e2e, 7m-row load/performance testing, and browser E2E remain undone.

[Implementation and sample CDC schema](safe-incremental-v13.md).
