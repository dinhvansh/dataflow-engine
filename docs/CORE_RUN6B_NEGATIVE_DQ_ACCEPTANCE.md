# CORE-RUN-6B-2 — negative DQ and concurrent API integration

**Status PENDING until exact PR-head live GitHub Actions PASS**.

Real isolated Docker SQL Server QA (SELECT-only 10,000 fixture rows) → Go API → ClickHouse raw → PostgreSQL durable state → real dbt and DQ → versioned publish. After a valid successful first run, six synchronized HTTP POSTs to Core Run must yield exactly one 202 and five 409 responses; the accepted real job must finish with zero new incremental data and no duplicate rows.

Then save a genuine restricted Visual model that filters all source rows. Its dbt View materialization can succeed, but the actual DQ must reject its empty output: core/flow failed, **no physical ClickHouse target created**, existing published table UUID and contents unchanged, and no later dependent model runs.

This **does not** certify simultaneous in-flight ClickHouse physical DDL exchange/publish races or worker kill mid active dbt SQL query (still outstanding 6B). Full 7m owner-QA benchmark and production release still OPEN. Never write to real ERP. Executable on disposable QA with `DATAFLOW_LIVE_QA_OPT_IN=true DATAFLOW_CORE_6B_DQ_OPT_IN=true bash deploy/local-qa/run-core-6b-dq.sh`.
