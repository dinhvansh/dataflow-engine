# CORE-RUN-6B-1 — real dbt worker SIGKILL after durable lease claim

This covers a **real worker process SIGKILL after PostgreSQL claim but before dbt CLI execution**, with replay, new fence token, stale-owner rejection, and one successful execution. It does **not** claim interruption halfway through a ClickHouse SQL query, nor concurrent publish races or all negative DQ paths. Those remain separate CORE-RUN-6B acceptance gates.

Run only on a fresh disposable owner-verified QA stack: `bash deploy/local-qa/up.sh` then `DATAFLOW_LIVE_QA_OPT_IN=true DATAFLOW_CORE_6B_OPT_IN=true bash deploy/local-qa/run-core-6b.sh`.

A QA-only twelve-second claim hold enables deterministic crash; production default is no hold and non-scratch environments reject the override. The PostgreSQL 10-minute lease is only expired manually in the already verified **disposable QA** metadata DB, immediately after SIGKILL, to avoid wasting test duration. No source ERP writes and no production lease-time override.

Acceptance requires exact-head real GitHub workflow PASS with `fencing-result.json` and all other CI, else PR remains open. CORE-RUN-7 measured 7m hardware run and production/funding approval remain OPEN.
