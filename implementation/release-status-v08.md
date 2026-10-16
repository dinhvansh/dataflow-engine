# Release Status v0.8 — Visual Transformation Studio

**Merged into main:** [PR #6](https://github.com/dinhvansh/dataflow-engine/pull/6), merge commit `4055edc959887f596c48e85f814254065abe3ab0`.

**Verified:** [GitHub Actions run 38887366877](https://github.com/dinhvansh/dataflow-engine/actions/runs/38887366877) — both Go and Web jobs passed.

## What exists in code
- New **Transform Studio** navigation page in React: Power Query-style left operations palette, right Applied Steps, center Preview and Generated SQL.
- Native drag-and-drop step insertion/reorder, plus up/down controls, enable/disable and delete.
- Visual operations: Filter, Select Columns, Rename, Arithmetic Calculated Column, Group/Aggregate, Sort and Limit.
- Switch to an independent free-form SQL SELECT editor when desired.
- Inspect source columns from ClickHouse, preview up to 50 rows (server maximum 200) at the source or after an Applied Step, with output table and ClickHouse column metadata.
- Store/load named visual/SQL transformation definitions in PostgreSQL using migration `010_transform_models.sql`. Only metadata is saved, not transformed rows.
- Go compiler validates structured identifiers, filter/operation values, nesting and output SQL. All Preview goes through a separate SELECT-only ClickHouse account; does not write to ERP or Mart.

## Passing CI
- Go race tests, vet and build — PASS.
- React TypeScript/Vite production build — PASS.
- Live ClickHouse 25.3 SQL validation for Filter -> Calculate -> Aggregate -> Sort -> Limit and Rename — PASS.
- PostgreSQL 16 + ClickHouse API tests for compile, save/list, SQL preview, token enforcement and blocking non-scratch source — PASS.
- Existing Managed Sync recovery and SQL DAG integration tests — PASS.

## Exact ETL state
- **E:** SQL Server reader plus experimental Full/Incremental Extract implemented. Real Epicor TEST full-chain correctness and scale acceptance remain pending.
- **L:** ClickHouse writer, count-verified Managed Sync and PostgreSQL checkpoint/recovery implemented; tested with isolated PostgreSQL/ClickHouse, not Epicor TEST complete.
- **T:** Visual authoring, Go SQL compiler, SQL SELECT Preview, and save/load are implemented. dbt Core execution, materialization (View/Table/Incremental), and durable transformation publishing are **NOT implemented**.

## Install for test
1. Apply `migrations/postgres/010_transform_models.sql` to existing TEST PostgreSQL after backup.
2. Configure existing `DATAFLOW_ALLOW_TEST_LOAD=true`, `DATAFLOW_ENABLE_SQL_JOBS_TEST=true`, and restricted `DATAFLOW_CH_READONLY_USER/PASSWORD` on disposable scratch data.
3. Open Transform Studio, enter bearer token, choose scratch source, fetch columns, add steps, Preview, Save, reload.
4. Review database grants. The UI is not a SQL sandbox; untrusted SQL must be restricted via ClickHouse permissions and network policy.

## Future production gates
- Optional locked-version dbt Core + dbt-clickhouse worker, generated dbt models, explicit model selection, observability via artifacts and safe materialization approval.
- Staging/publish, rollback, idempotency, DQ and deletion repair.
- Browser-driven drag/drop E2E, visual regression, error reporting, accessibility audit.
- Real Epicor SQL Server TEST full-chain load/replay/correctness, 7m-row benchmark, resource impact and failure injection.

**Developer Preview**. Do not present this as a complete dbt integration or production ETL platform.
