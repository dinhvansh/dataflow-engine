# Airbyte-style Stream Field Selection (isolated QA)

This slice is a **working SQL Server RowVersion column projection** for the existing managed incremental stream; it is not yet a complete Airbyte connection/stream wizard.

## Workflow
1. In Managed Streams, enter the configured SQL Server schema; **Discover tables** lists read-only catalog entries.
2. Pick one table, **Inspect columns**, then tick the data fields to sync. `SysRowID` and `SysRevID` are mandatory and not removable.
3. Keep the current scratch-only destination, tune batch/workers/queue, then **Save selected stream**.
4. The server validates every selected field against current SQL Server metadata, rejects duplicates/unknown names, ensures RowVersion + unique SysRowID + SNAPSHOT isolation, and stores a selected-columns array in PostgreSQL migration 032.
5. The managed worker projects only those columns under existing SQL typed checkpoint, lease, fencing and ClickHouse event writer rules.

## Compatibility and safety
- `selected_columns = NULL` means all columns for older streams and clients. Existing streams are unchanged by the migration.
- The selected column list is immutable on subsequent stream updates, guarding against silently changing the schema under an existing checkpoint. A conflicting changed projection returns HTTP 409; future explicit versioned reprovisioning is separate work.
- Source SQL remains **SELECT-only**; column names are catalog-validated then safely quoted in the SQL reader. Mandatory cursor/key columns remain included. No source schema/index/CDC change is performed.
- Connection Manager currently uses an isolated admin-only runtime-lab profile; schema discovery itself still targets the backend-configured SQL source, not an arbitrary user-selected saved profile. Do not point this UI at production and do not infer multi-tenant readiness.
- Current managed stream supports **RowVersion incremental only**, scratch `dataflow_raw_events` only. A separate bootstrap supports one-time Full Refresh, but this release does **not** wire selectable Full Refresh, arbitrary destination mappings, or all-table multi-select to managed jobs. Those are the next phases.
- Schema column projection in ClickHouse raw JSON can affect downstream dbt expectations. Review DQ/transform model dependencies before changing what is ingested.
- Do not claim production readiness, production vault/OIDC/egress approval, or an ERP end-to-end run based on static CI.

## Acceptance gates
- Go unit rejects missing required fields, injection attempts, unknown and duplicated columns, and preserves legacy all-column projection.
- Hosted SQL Server disposable integration and Go/Web CI must pass on the PR exact head.
- Manual/Playwright disposable QA should inspect the selected table, save a subset, execute a fresh managed run and verify the raw ClickHouse event payload includes only selected fields + system metadata, plus checkpoint replay 0 new rows.
- No production ERP test required; no writes to any real ERP.
