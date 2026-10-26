# Core 8B Codex Local one-pass acceptance

Verdict: **PASS-LOCAL-CORE8B** (NOT production approval), Git SHA: 1200a379a11c842c1297ecbe58bee22120567903
Case totals: {'PASS': 17, 'FAIL': 0, 'BLOCKED': 0, 'NOT_RUN': 0}; 7m: NOT RUN; ERP writes: PROHIBITED

| Case | Status | Seconds | Captured log |
|---|---|---:|---|
| STATIC-SAFETY | PASS | 10.316 | logs/STATIC-SAFETY.log |
| STATIC-RESET-NEGATIVE | PASS | 0.948 | logs/STATIC-RESET-NEGATIVE.log |
| STATIC-EVIDENCE-CONTRACT | PASS | 0.209 | logs/STATIC-EVIDENCE-CONTRACT.log |
| STATIC-GO | PASS | 340.926 | logs/STATIC-GO.log |
| STATIC-WEB | PASS | 123.522 | logs/STATIC-WEB.log |
| UI-MOCK | PASS | 115.051 | logs/UI-MOCK.log |
| PREP-8B | PASS | 61.025 | logs/PREP-8B.log |
| CORE-8B-E2E | PASS | 221.321 | logs/CORE-8B-E2E.log |
| PREP-CLAIM | PASS | 55.621 | logs/PREP-CLAIM.log |
| CORE-6B-CLAIM | PASS | 87.424 | logs/CORE-6B-CLAIM.log |
| PREP-DQ | PASS | 53.814 | logs/PREP-DQ.log |
| CORE-6B-DQ | PASS | 130.324 | logs/CORE-6B-DQ.log |
| PREP-MID-SQL | PASS | 51.741 | logs/PREP-MID-SQL.log |
| CORE-6B-MID-SQL | PASS | 93.413 | logs/CORE-6B-MID-SQL.log |
| PREP-NETWORK | PASS | 50.612 | logs/PREP-NETWORK.log |
| CORE-6A-NETWORK | PASS | 164.944 | logs/CORE-6A-NETWORK.log |
| VERIFY-EVIDENCE | PASS | 0.554 | logs/VERIFY-EVIDENCE.log |

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
