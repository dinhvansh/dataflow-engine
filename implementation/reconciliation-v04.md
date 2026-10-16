# Snapshot Reconciliation v0.4 — Developer preview

## Purpose
Detect source-key additions, missing keys (possible physical DELETE), and version changes **without SQL Server CDC or source mutations**.

Snapshots are read under SQL Server SNAPSHOT isolation and written into ClickHouse scratch.key snapshots as per-stream/run IDs. PostgreSQL publishes the latest verified snapshot generation. The *first run only establishes a baseline*. Later successful runs compare against the preceding published full snapshot.

This implementation is **detection-only**: it never deletes records from the ODS or creates tombstones. It does not provide live delete capture.

## Prerequisites
1. PostgreSQL migrations 001–006, in order. Existing Compose volume does not auto-apply newly introduced migrations.
2. ClickHouse scratch DDL: `deploy/clickhouse/001_raw_events.sql` and `deploy/clickhouse/002_key_snapshots.sql`. Update the database name in the DDL if `DATAFLOW_TEST_TARGET_DATABASE` is not `scratch`.
3. A registered `sync_streams` row for source `Erp.<table>`. Source must have `SysRowID` as a declared primary key of SQL Server type `uniqueidentifier` plus `SysRevID` as `rowversion/timestamp`.
4. SQL Server SNAPSHOT isolation already ON, enforced by DBA. No DDL or CDC action is issued by DataFlow.
5. Set all three flags in a **disposable test environment**:
   - `DATAFLOW_ALLOW_TEST_LOAD=true`
   - `DATAFLOW_ENABLE_MANAGED_TEST=true`
   - `DATAFLOW_ENABLE_RECONCILIATION_TEST=true`
6. Provide protected API token and ensure the ClickHouse target permits INSERT, SELECT on `scratch.dataflow_key_snapshots`.

## API
```bash
curl -X POST http://localhost:8080/api/v1/reconcile/runs \
  -H "Authorization: Bearer $DATAFLOW_API_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"stream_id":"<registered-stream-uuid>"}'

curl -H "Authorization: Bearer $DATAFLOW_API_TOKEN" \
  'http://localhost:8080/api/v1/reconcile/runs?id=<run-uuid>'

curl -H "Authorization: Bearer $DATAFLOW_API_TOKEN" \
  'http://localhost:8080/api/v1/reconcile/runs'
```

CLI: `dataflowctl reconcile-run <stream-id>`, `dataflowctl reconcile-status <run-id>`, `dataflowctl reconcile-list`.

UI: Sync Jobs → Managed Incremental → Snapshot Reconciliation. The MCP server offers list comparisons by default; snapshot capture requires both server flags and MCP write opt-in plus explicit human approval.

## Safety invariants
- Uses a **single SQL Server SNAPSHOT transaction** for the entire source-key stream.
- No published baseline if reader fails, any batch fails, source transaction fails, saved count differs from emitted rows, or distinct PK count differs.
- Reconciler claims the same fenced `sync_checkpoints` stream lease as managed incremental, preventing concurrent ownership. Heartbeat expires/aborts failed jobs.
- A single PostgreSQL transaction atomically publishes baseline pointer and marks the reconciliation run complete.
- No source DML, no source schema changes, no ClickHouse DELETE mutations.
- Failed snapshot rows may remain in scratch for debugging; do not drop a published generation without updating the metadata baseline.

## Verification sequence
1. Run first snapshot, expect succeeded with added/deleted/changed = null (baseline only).
2. Run second without any source changes, expect all differences zero.
3. On an independent test source, add/modify/remove one PK and take next snapshot; expect +1 added, +1 changed, +1 missing respectively.
4. Intentionally interrupt the source reader or ClickHouse insert; expect failed run and **unchanged** published baseline.
5. Compare aggregate results with independent SQL Server SELECTs from two isolated consistent test snapshots.
6. Measure Epicor source IO/version-store impact before testing millions of keys.

## Remaining limitations
- Build/test CI alone does not prove ClickHouse SQL syntax or real SQL Server SNAPSHOT query correctness. Full database integration is still required.
- The full source-key scan is potentially expensive, and no throttled/resumable partition scan is implemented.
- Snapshot comparisons are across two distinct transaction times, so changed keys are net changes, not all intermediate events.
- No automatic ODS tombstones or deletion repair until precedence and concurrency invariants are validated.
- No durable worker queue, tenant-level RBAC, or production scale benchmark.
