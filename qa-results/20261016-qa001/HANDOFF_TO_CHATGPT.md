# DataFlow local QA handoff

## Verdict

**CONDITIONAL PASS — synthetic local; Epicor TEST NOT ATTEMPTED; production NO-GO.**

- Run: `20261016-qa001`
- Base SHA: `c97d424d049673a1d045eb8c9fb7e2cc383cf9f7`
- Branch: `test/local-qa-v14-20261016`
- Local QA head: `f37ee305957ee6eaa87b4008f733191f8216503e` (fix commit `4eb0e79`; not pushed or merged)
- Matrix: PASS 58 | FAIL 0 | BLOCKED 9 | N/A 1 | total 68
- Open P0/P1 product bugs: 0; locally fixed/verified bugs: 4
- Data mismatch: 0 for measured 10k, 100k and 1m full loads

## Environment

Dedicated labeled Docker project `dataflow-localqa`: SQL Server 2022 fixture (`DataFlowFixtureQA`), PostgreSQL 16 (`dataflow_qa`, migrations 001–014), ClickHouse 25.3 (`scratch`, Atomic), pinned dbt worker, Go API. All published ports bind `127.0.0.1`. `qa_erp_reader` SELECT passed and UPDATE was denied. No real Epicor DSN/data/credentials were used.

## Core evidence

- 10k full: 10,000 source/target, 10,000 unique UUID, qty/cost/null aggregates equal, mismatch 0.
- Repeated full: atomic refresh remained 10,000.
- 100k and 1m: exact target counts, mismatch 0 across three trials each.
- Rowversion: baseline 10,000; 5 updates + 10 inserts captured; no-change 0; cursor `...0B22` → `...0B31`.
- Physical delete: not captured by SysRevID, explicitly documented known limitation.
- Auto Build A→B→C, SQL drift/failure stops, dbt View, lineage, Safe Table publish/rollback, Safe Incremental 0→4→7/rollback all passed real PG/CH/dbt integrations.
- CH outage returned 502 and retained 10,000 rows. PG outage returned 500 and retained 10,000 rows.

## Bugs fixed locally

1. BUG-001 P1: migration 004 not idempotent.
2. BUG-002 P2: integration tests not rerunnable; missing npm lockfile.
3. BUG-003 P1: unsupported `TxOptions.ReadOnly` broke all SQL Server SNAPSHOT reads.
4. BUG-004 P1: UUID/date serialization and rowversion hex conversion corrupted/rejected E/L data.

See `05_BUG_REGISTER.md`, regression logs and `artifacts/code.diff`. Changes are not merged or pushed.

## Benchmark

- 10k: 89/123/114 ms, median 114 ms, mismatch 0.
- 100k: 796/695/670 ms, median 695 ms, mismatch 0.
- 1m: 6509/6759/6605 ms, median 6605 ms, mismatch 0.
- No-change: 12 ms (one trial).

No Airbyte comparison. CPU/IO telemetry, delta 1k/10k/50k and three-trial recovery timing remain gaps.

## Blocked/gaps

UI-01..06 blocked because no callable browser automation was available. INC-11 late arrival and PERF-05/06 were not fully executed. PERF-07 is N/A without a comparable Airbyte baseline. The optional ~7m run was not attempted because the mission requires separate user consent.

## Review order

1. `02_TEST_MATRIX.csv`
2. `05_BUG_REGISTER.md`
3. `03_DATA_RECONCILIATION.md` and `perf.csv`
4. `artifacts/code.diff` and regression/integration logs
5. Reproduce with `deploy/local-qa/up.ps1`, `run-functional.ps1`, `run-faults.ps1`, `run-benchmark.ps1`.

One-line decision: **CONDITIONAL PASS synthetic; Epicor TEST not attempted; production NO-GO pending delete/late-event plan, browser E2E, remaining performance/recovery gates, RBAC/audit and DBA/IT approval.**
