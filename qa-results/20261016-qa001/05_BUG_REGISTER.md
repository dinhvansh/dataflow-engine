# Bug register

| ID | Sev | Failure/root cause | Fix | Regression/retest | Status |
|---|---|---|---|---|---|
| BUG-001 | P1 | Migration 004 added named constraints unconditionally; rerun failed | guarded constraints with `pg_constraint` checks | new two-run integration + managed queue PASS | FIXED VERIFIED |
| BUG-002 | P2 | Integration tests used fixed unique names and failed on rerun; npm lacked lockfile | run-unique PG names; add package lock | full PG/CH/dbt suite rerun PASS; npm ci PASS | FIXED VERIFIED |
| BUG-003 | P1 | go-mssqldb rejects `TxOptions.ReadOnly`; snapshot full/rowversion could never start | use SNAPSHOT isolation without unsupported flag; source DB grants enforce read-only | real MSSQL snapshot reader + 10k E2E PASS | FIXED VERIFIED |
| BUG-004 | P1 | full reader corrupted UUID/date values; rowversion direct varchar conversion returned binary bytes | canonical UUID/date conversion and nested varbinary rowversion hex conversion | unit + real MSSQL reader + rowversion E2E PASS | FIXED VERIFIED |

Original failing evidence is retained (`go-mod-tidy.log`, initial integration logs, failed E2E responses). Product/test fixes are local commit `4eb0e79fff742e7de7836131082154aca5b8a4c8`; packaging hardening is `f37ee305957ee6eaa87b4008f733191f8216503e`. Nothing was pushed or merged.

