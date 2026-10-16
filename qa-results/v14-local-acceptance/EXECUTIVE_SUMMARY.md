# DataFlow v1.4 local QA acceptance

Date: 2026-10-16 UTC  
Base: `origin/main` at `61cfd7a712b5c600167a85c7ee8fd36c44f1da8d`  
Branch: `test/v14-full-local-acceptance-20261016`

## Verdict

**CONDITIONAL GO for the isolated QA/UAT stack. NO-GO for production Epicor.**

Matrix total: **52 PASS / 0 FAIL / 1 BLOCKED / 3 N/A** (56 checks).

All exercised synthetic functional, live-browser, benchmark and recovery cases passed after the two test-harness defects were fixed. No real Epicor host/database was contacted. SQL Server writes were limited to the disposable `DataFlowFixtureQA`; the application reader remained read-only.

The remaining safety gap is physical source DELETE detection: SQL Server `rowversion` alone cannot emit a tombstone for a row that no longer exists. Production use also needs sizing on production-like hardware and operational approval.

See `TEST_MATRIX.csv` for totals and evidence, and `HANDOFF_TO_CHATGPT.md` for independent review instructions.
