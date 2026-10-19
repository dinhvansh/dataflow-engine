# CORE 8B owner-local acceptance matrix (not test results)

| ID | Priority | Actual check | Pass criteria |
|---|---|---|---|
| STATIC-SAFETY | P0 | Git branch/clean, owned Docker labels, volume/network scope, disk, QA credentials | No real ERP and no unknown project resource; refuse unverified reset |
| STATIC-RESET-NEGATIVE | P0 | Negative tests with stubbed Docker: clean project, unknown volume, unexpected network | Unknown QA-labeled resources are refused; tests never call live Docker or remove volumes |
| STATIC-EVIDENCE-CONTRACT | P0 | Read-only evidence schema and real QA artifact contract | Contract PASS without executing real E2E or fabricating evidence |
| STATIC-GO | P0 | Go compiler/unit/integration tests that have applicable opt-ins | No failed tests; skipped tests recorded independently |
| STATIC-WEB | P1 | npm lockfile install and TypeScript/React build | Exit 0 without lowering typesafety |
| PREP-8B | P0 | New isolated fixture PostgreSQL 001–030/SQL Server/ClickHouse | All services healthy, application reader SELECT-only |
| CORE-8B-E2E | P0 | Actual UI→Core 10k, A→B→C→DQ→publish/rollback | True DB count 10000, correct target UUID and checkpoint, 0 duplicate replay |
| CORE-8B-E2E | P0 | Valid graph save, reload, exact version run | Immutable saved graph digest persisted by PG and returned by GET |
| CORE-8B-E2E | P0 | Fork rejected; wrong graph version/unique binding rejected | 422/409; zero extra managed runs |
| CORE-8B-E2E | P0 | Edit and save graph revision 2 after run version 1 | New digest/version, old run snapshot unchanged even after reload |
| CORE-8B-E2E | P1 | Unauthenticated graph registry access | HTTP 401 |
| PREP-CLAIM / CORE-6B-CLAIM | P0 | Real dbt worker killed post-claim, fenced retry | No stale owner writes; exactly one final success |
| PREP-DQ / CORE-6B-DQ | P0 | 6 concurrent requests; negative DQ | Exactly one 202, five 409; failed DQ cannot publish |
| PREP-MID-SQL / CORE-6B-MID-SQL | P0 | Actual active ClickHouse query, real worker SIGKILL | Physical prior UUID persists until verified new publish |
| PREP-NETWORK / CORE-6A-NETWORK | P0 | Kill API / outage CH; bounded retries and explicit resume | No phantom success or checkpoint skip |
| PERF-SMOKE | P2 | Actual 10k complete E2E time | Recorded seconds; no inferred engine-only rows/s |
| OWNER-VISUAL | P1 | User verifies layout, connectors, DQ error, graph save UX | Screenshots and observations; mark NOT RUN until human review |
| CORE-RUN-7 | Deferred | 7,000,000 row measured benchmark | **NOT RUN** (owner intentionally deferred; no invented throughput) |
| PROD-GO | Independent | Real auth/secret/key/access controls, backup, owner repeatability | **NO-GO**, never inferred from synthetic test |

The runnable subset is recorded by `matrix.tsv`; the larger acceptance catalog from v1.3 stays in `qa/local-acceptance/TEST_CASE_CATALOG.md` and MUST NOT be silently marked passed. If a runner case includes several assertions, link its Playwright/PG/CH artifacts for each P0 subcase rather than treating shell exit alone as proof.

Bug severity P0/P1 blocks completion. Codex should preserve full original failure, fix on separate branch, add a regression and re-run from fresh disposable QA. For an unsupported local dependency label it `BLOCKED` with exact reason, not `PASS` or silent skip.

| UI-MOCK | P1 | Isolated mock UI pytest | PASS is reported separately, never proof of real Go/PG/SQL Server interaction |
| VERIFY-EVIDENCE | P0 | Fresh real JSON per live test, canonical 02_TEST_MATRIX and external verifier | `verify_core_final_evidence.py --report` exit 0; no NOT_RUN mandatory cases |
| API-401 | P0 | Unauthenticated GET /api/v1/core/graphs | HTTP 401 in real Core-8B browser/API E2E |

| CORE-8B-E2E | P0 | Real editor after page reload: add an invalid transform → backend Validate rejects → remove it → change DQ key → Validate + Save revision 2 using UI | Saved revision 2 is persisted in PostgreSQL, actual quality binding updated, old run's graph snapshot unchanged; screenshots and real API proof; no false green from API-only graph edits |
