# Transform Studio v0.8 — Power Query-style Visual Prototype

This is an actual UI and API implementation, not a screenshot or dbt integration. Do not enable on production.

## User experience

- A new **Transform Studio** navigation item provides a three-panel layout: operation palette, Preview/Generated SQL, and Applied Steps.
- Click **+** or drag an operation from the palette to Applied Steps. Drag to reorder, or use up/down buttons for accessibility. Toggle and delete any step.
- Supported visual operations: **Filter Rows, Select Columns, Rename, Calculated Column** (bounded arithmetic by literal number), **Group/Aggregate**, **Sort**, and **Limit**.
- Select Source or a step to preview just that stage; preview has a 50-row UI cap and the backend enforces a maximum of 200 rows, 30-second ClickHouse execution limit, and read-only account.
- User may switch to **SQL Code** to write a single SELECT statement manually; the SQL text is independent of the visual step list.
- Fetch source schema through a 0-row ClickHouse SELECT that returns JSON metadata; a test source must already be present in `scratch`.
- Save/load named transformations as JSON step definitions and last compiled SQL in PostgreSQL. No materialized tables or dbt builds are written.

## Architecture

React TransformStudio → Go `/api/v1/transform/*` → `internal/transform` compiler → `clickhouse.QueryJob` with a **dedicated SELECT-only account**.

PostgreSQL `transform_models` stores model metadata. Compiled visual SQL uses nested SELECTs so clicking Applied Step N corresponds to a verifiable SQL stage.

## Before trying

1. **Apply `migrations/postgres/010_transform_models.sql` to PostgreSQL test database** (backup first).
2. Configure `DATAFLOW_ALLOW_TEST_LOAD=true`, `DATAFLOW_ENABLE_SQL_JOBS_TEST=true`, `DATAFLOW_CH_READONLY_USER`, `DATAFLOW_CH_READONLY_PASSWORD` in a disposable test environment. ClickHouse grants must be SELECT-only and limited to scratch. No other databases accessible to this account.
3. Ensure ClickHouse scratch has a table such as `scratch.dataflow_raw_events`; open Transform Studio and provide existing API bearer token.
4. Set source table → fetch columns → add Filter → Preview → add Group By → Preview → reorder steps → Preview → Save Model → reload.
5. SQL mode allows SELECT expressions manually. It cannot round-trip edited SQL into visual steps.

## Security and constraints

- Visual identifiers follow safe alphanumeric + underscore syntax; arbitrary expressions cannot be pasted into structured steps.
- Filter text values have quote escaping and 512-character limit. Numeric filters are syntax-checked.
- Raw SQL remains more expressive and the application uses its existing SELECT-only ClickHouse account + query limits. **This is not a full SQL sandbox**; enforce database grants and network egress policy.
- Stages are validated at compile time for shape, not inferred field types. For example SUM on a String column fails at Preview.
- No DML/DDL or data materialization from Transform Studio.
- No dbt Core worker, project generator, lineage or dependency-model inference is implemented in v0.8. Integrate an **optional isolated dbt Core worker** only after testing that ordinary users can complete transformations in the visual UI.

## CI test gates

- `go test -race ./...`, `go vet ./...`, Go build, TypeScript/Vite build.
- Real ClickHouse visual SQL test exercises filter → calculated column → aggregate → sort → limit, rename, and JSON columns metadata.
- Isolated PostgreSQL 16 + ClickHouse 25.3 test verifies authenticated save/list/compile/preview, scratch-source enforcement.
- Actual browser E2E drag-drop and Epicor TEST large-data correctness are **not** covered and remain required.

## Next milestone (no automatic enablement)

Optional dbt Integration Worker:
1. Generate dbt project artifacts from saved models in a staging directory with stable, audited names.
2. Lock dbt Core and dbt-clickhouse versions; run in an isolated container with scoped ClickHouse permissions.
3. Implement preview of the compiled SQL and materialization plan before approval.
4. Atomic staging/publish or rollback gates for Table materialization; only then provide View/Table/Incremental controls in UI.
5. Surface `manifest.json`, `run_results.json`, tests and model lineage inside DataFlow.

**ETL readiness:** Extract and Load are implemented prototypes with real Go/PostgreSQL/ClickHouse CI tests, but still lack end-to-end Epicor SQL Server TEST acceptance. Transform Studio adds the **visual Transform authoring and read-only preview** layer, not production transformation execution.
