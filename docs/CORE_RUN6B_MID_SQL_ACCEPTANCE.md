# CORE-RUN-6B-3 — mid-active ClickHouse SQL SIGKILL and target-scoped publishing

**Status pending real exact-head CI**. This adds a PostgreSQL per-target transaction advisory lock shared by the Core coordinator and dbt Table worker, held through ClickHouse physical exchange, to prevent two unrelated table-run IDs from both exchanging against the same target UUID. The `TestRealConcurrentPublishUsesTargetScopedLock` Go integration requires real PostgreSQL + ClickHouse and starts **two independent concurrent publishers**; only one is allowed to commit, loser is rejected based on immutable old UUID, replay does not change target.

The real Docker/pytest fault test seeds synthetic scratch state, has a **real ClickHouse `SELECT ... sleepEachRow ... FROM numbers(1600)`** executed by dbt **table materialization**, and **observes the query in `system.processes` before Docker SIGKILL of worker**. It checks original published physical UUID unchanged; after owner-verified disposable-only PG lease acceleration, the restarted worker must rebuild stage, run DQ for 1600 unique rows, then swap only upon approval. Reports `qa-results/core-run6b3/mid-sql-result.json`.

```bash
bash deploy/local-qa/up.sh
DATAFLOW_LIVE_QA_OPT_IN=true DATAFLOW_CORE_MID_SQL_OPT_IN=true bash deploy/local-qa/run-core-mid-sql.sh
```

Never touch production ERP, original SQL Server remains read-only. CI must PASS all Go/browser/SQL read-only regressions + live mid-query fault before merge. A real ClickHouse publication race with **two simultaneously running Go workers** is covered by independent goroutines and PG client transactions in integration; inter-service worker/REST conflicts remain subject to full-stack lab retesting. CORE-RUN-7 owner-machine 7m measurement, CORE-RUN-8 unified Canvas, enterprise security and production release remain OPEN.
