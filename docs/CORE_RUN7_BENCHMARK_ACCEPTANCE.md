# CORE-RUN-7 — real 7,000,000-row performance / incremental replay acceptance

**State after CORE-RUN-6B hosted acceptance #107: executable benchmark, owner-machine execution NOT YET evidenced.** The prior 7m SQL fixture seeder and Go real managed-ingestion benchmark have been hardened to require **two verified SQL Server→ClickHouse+PostgreSQL runs**: initial 7,000,000 source rows and a second incremental run with 0 new rows, 0 duplicate rows and checkpoint generation 2. A skipped test or green PR CI does not mean the 7m performance gate passed.

## Executable opt-in on OWNED disposable QA machine

Hardware/operator preconditions: at least 16 GiB free on workspace and Docker storage, RAM to support isolated SQL Server and ClickHouse, sufficient CPU, Go/Python3/Docker, isolated volumes verified by preflight. `SRVERPTEST`, `ERPDBTEST` and company production servers are prohibited. No production source writes.

```bash
bash deploy/local-qa/up.sh
DATAFLOW_QA_ALLOW_7M=YES_I_ACCEPT_DISPOSABLE_QA_SEEDING bash deploy/local-qa/run-core-7m.sh
```

The script refuses missing opt-in, non-owned containers/network, insufficient disk, wrong source database/login, and non-7m source count. SQL seed script only creates/updates a **disposable fixture source**, then the actual managed reader uses `qa_erp_reader` SELECT-only. 7m rows are physically ingested to ClickHouse and committed to PG. A second run must transfer zero new rows with no duplicated physical rows; both runs' actual elapsed time is measured, along with host details, Go heap after run, and sampled Docker CPU/memory/IO.

`qa-results/core-seven-million-<UTC>/benchmark-result.json`, `actual_benchmark.log`, `docker_stats.csv` and seed result must all be archived before any independently authorized cleanup. No assumed throughput or invented benchmark result is accepted.

**Not covered**: full 7m-row dbt materialization with DQ, concurrent performance and crash injection at 7m, or production sizing/certification. These are separate CORE-RUN-7 follow-up tests, and funding owner-demo / Production GO remain distinct approvals.

## No premature funding/production performance claim

Even after #107 completed the hosted 6B fault matrix, the 7m SQL Server → PG → ClickHouse first-load + second 0-row incremental replay remains **unmeasured**. Do not close CORE-RUN-7 based on unit/contract green or a 10k E2E. Required output remains real `benchmark-result.json`, `actual_benchmark.log`, `docker_stats.csv` and owner QA hardware attestation. Preserve ERP source SELECT-only.
