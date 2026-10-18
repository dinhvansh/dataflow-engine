# V4.1-1 — synthetic Data Quality release contracts

**Scope:** pure-Go contract validation/evaluation on *provided* metrics, with synthetic V4.0-1 pipeline Quality handler integration. No SQL Server/ClickHouse read query, no production publisher, no credentials or active runtime.

## Strict contracts
- `MIN_ROWS`: bounded minimum observed row count
- `REQUIRED_COLUMN`: schema-presence check for a declared column
- `MAX_NULL_RATIO`: null count divided by positive row count for a declared column; unknown metrics block
- `MAX_DUPLICATE_KEYS`: bounded duplicate-key evidence, rejects impossible counters
- `MAX_AGE_SECONDS`: freshness guard, missing age not treated as fresh
- `MAX_RECONCILIATION_DELTA`: maximum relative discrepancy, NaN/infinity/missing evidence rejects

Every check is mandatory. Contract IDs and column keys are strictly validated; type/threshold combinations are bounded. Any missing evidence, failed threshold or invalid schema **blocks the release**. Responses contain only check names and sanitized reason codes, never underlying data or SQL/credential errors.

## Synthetic acceptance
Go tests cover good metrics, invalid/missing/null/duplicate/stale/reconciliation evidence and malformed contracts. An integration test wraps evaluation as a `mixedpipeline.Quality` handler and proves **PUBLISH is skipped** after a DQ failure, then succeeds with valid synthetic evidence.

## Real gates not finished
- Reliable READ-ONLY SQL/ClickHouse metric extraction, source schema snapshots and time-zone correctness
- Durable evidence versioning/auditing and model ownership, tolerances, lineage impact and rollback
- Signed immutable job snapshot/connection revisions and resilient persistent mixed pipeline runtime
- Real Vault/IdP/HTTPS and network approval, V3.2C Test Connection, V3.3 bindings, V3.4 real synthetic E2E; no Epicor DML and no production deployment

**Status:** DQ contract engine with synthetic execution QA, not a live report validator or production data quality certification.
