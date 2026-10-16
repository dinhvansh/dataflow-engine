# Handoff to ChatGPT — DataFlow v1.4 local acceptance

## Decision

**CONDITIONAL GO for isolated QA/UAT; NO-GO for production Epicor.**

Final matrix: **52 PASS / 0 FAIL / 1 BLOCKED / 3 N/A** across 56 checks.

Review `TEST_MATRIX.csv`, then verify:

1. Base commit is `61cfd7a712b5c600167a85c7ee8fd36c44f1da8d` and PR #16–#19 merge commits are ancestors.
2. Test-only changes configure ClickHouse host/port and unique queue names; production logic is unchanged.
3. `qa/browser/test_console_live.py` uses only the disposable local endpoints and verifies PostgreSQL/ClickHouse after UI actions.
4. Raw benchmark JSONL contains exactly 9 samples with reconciliation `PASS`.
5. Recovery JSONL contains 3 trials per required scenario and published data remains intact.
6. Screenshots and trace ZIPs exist for all six live UI cases.
7. No file contains a value from `.env.qa.local`; the env file is not committed.

Functional acceptance passed for full sync, incremental checkpoint/publish/rollback, late-arrival/history guard, visual preview, dbt View, Safe Table, Auto Build and lineage. The remaining production blocker is physical SQL Server DELETE capture. CPU/RAM/query-I/O benchmark metrics are N/A because the supplied sub-second benchmark did not include a trustworthy sampler.

Do not merge automatically. Review the PR and rerun on a runner with more disk before approving the next stage.
