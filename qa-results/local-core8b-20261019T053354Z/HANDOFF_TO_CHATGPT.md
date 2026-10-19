# Core 8B Codex Local one-pass acceptance

Verdict: **PASS-LOCAL-CORE8B** (NOT production approval), Git SHA: 4d6544d0ca4986edcd55a3b1cca122ab4738b595
Case totals: {'PASS': 17, 'FAIL': 0, 'BLOCKED': 0, 'NOT_RUN': 0}; 7m: NOT RUN; ERP writes: PROHIBITED

| Case | Status | Seconds | Captured log |
|---|---|---:|---|
| STATIC-SAFETY | PASS | 1.067 | logs/STATIC-SAFETY.log |
| STATIC-RESET-NEGATIVE | PASS | 0.533 | logs/STATIC-RESET-NEGATIVE.log |
| STATIC-EVIDENCE-CONTRACT | PASS | 2.669 | logs/STATIC-EVIDENCE-CONTRACT.log |
| STATIC-GO | PASS | 235.051 | logs/STATIC-GO.log |
| STATIC-WEB | PASS | 129.441 | logs/STATIC-WEB.log |
| UI-MOCK | PASS | 51.877 | logs/UI-MOCK.log |
| PREP-8B | PASS | 82.006 | logs/PREP-8B.log |
| CORE-8B-E2E | PASS | 212.999 | logs/CORE-8B-E2E.log |
| PREP-CLAIM | PASS | 54.954 | logs/PREP-CLAIM.log |
| CORE-6B-CLAIM | PASS | 83.685 | logs/CORE-6B-CLAIM.log |
| PREP-DQ | PASS | 36.949 | logs/PREP-DQ.log |
| CORE-6B-DQ | PASS | 126.284 | logs/CORE-6B-DQ.log |
| PREP-MID-SQL | PASS | 42.805 | logs/PREP-MID-SQL.log |
| CORE-6B-MID-SQL | PASS | 87.416 | logs/CORE-6B-MID-SQL.log |
| PREP-NETWORK | PASS | 36.935 | logs/PREP-NETWORK.log |
| CORE-6A-NETWORK | PASS | 146.735 | logs/CORE-6A-NETWORK.log |
| VERIFY-EVIDENCE | PASS | 0.503 | logs/VERIFY-EVIDENCE.log |

## Verification
02_TEST_MATRIX.csv uses the shared CORE evidence-gate format.
Stage PASS alone is not proof: full acceptance also requires the official
verify_core_final_evidence.py --report check and original real fixture JSON.
Review original Playwright screenshots, trace, dbt/PG/ClickHouse UUIDs and checkpoints.
A failure preserves disposable volumes and logs for diagnosis.

## Deliberate exclusions
CORE-RUN-7 7m: NOT RUN (owner deferred).
8GB RAM owner hardware performance, parallel/IF-ELSE, real IdP/Vault/TLS
and production security: NOT CERTIFIED. Owner sign-off: OPEN.
