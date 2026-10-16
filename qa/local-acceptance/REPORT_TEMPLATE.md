# DataFlow Local QA — Report Template (Codex must fill actual results)

> Copy this into `qa-results/<RUN_ID>/HANDOFF_TO_CHATGPT.md` and replace all placeholders. Never report a simulated result as measured. Remove all passwords/tokens/real ERP data before sharing.

## 1. Executive verdict

- Overall result: `PASS / CONDITIONAL PASS / FAIL / BLOCKED`
- Type: `SYNTHETIC_LOCAL` or `EPICOR_TEST_READ_ONLY` or both (report separately)
- Run ID / started / finished / elapsed: `<...>`
- Git: `main <sha>` → test branch `<branch>` → local head `<sha>`; uncommitted changes `<...>`
- Device/OS: CPU cores `<...>`, RAM available `<...>`, disk free `<...>`; Docker/WSL2 versions `<...>`
- Versions: Go `<...>`, Node `<...>`, PostgreSQL `<...>`, ClickHouse `<...>`, SQL Server `<...>`, dbt-core/adapter `<...>`
- Count: `PASS <N> | FAIL <N> | BLOCKED <N> | SKIP (with justification) <N>`
- Blocking bugs: `P0 <N> | P1 <N>`
- Release decision: `NO-GO PRODUCTION` until real Epicor validation and other production gates, even after synthetic PASS.

## 2. Environment and isolation proof

| Service | Container name/id | Image/version | Host port | QA database | Health | User/role (no secret) |
|---|---|---|---|---|---|---|
| SQL Server fixture | | | | DataFlowFixtureQA | | readonly + separate seed admin |
| PostgreSQL | | | | dataflow_qa | | metadata |
| ClickHouse | | | | scratch (Atomic) | | qa_ro + qa writer |
| dbt worker | | | internal | scratch only | | scratch writer |
| Go API | | | | n/a | | QA token |
| React UI | | | | n/a | | n/a |

- Isolation: project/network/volume labels, ports bound to loopback, no existing container stopped/modified: `<evidence>`
- Git preflight and dirty/divergence: `<evidence>`
- `SELECT`-only app account: `<evidence>`
- Secrets redacted / `.env.qa.local` ignored: `<evidence>`
- Migrations applied: `001..014`, exact details, errors, re-run result: `<evidence>`

## 3. Data sets and reference oracle

| Dataset | Rows seeded | Schema/seed hash | Expected count | Source count | Target count | PK mismatches | Decimal/amount mismatches | Verdict |
|---|---:|---|---:|---:|---:|---:|---:|---|
| SMOKE | | | | | | | | |
| CORRECTNESS | | | | | | | | |
| SCALE_S | | | | | | | | |
| SCALE_M (conditional) | | | | | | | | |
| SCALE_L (approval-required) | | | | | | | | |

- `fixtures_manifest.json`: `<path>`
- Oracle SQL / canonicalization rules: `<path>`
- 20–100 sampled key comparisons: `<path>`
- Value reconciliation incl. NULL, decimal, Unicode, dates: `<path>`
- Unsupported source CDC DELETE / late arrivals: `<proof or gap>`

## 4. Test inventory

| Category | PASS | FAIL | BLOCKED | Total | Strongest evidence |
|---|---:|---:|---:|---:|---|
| Env/Git/Security | | | | | |
| Extract/Load | | | | | |
| Transform/dbt/Lineage/Auto Build | | | | | |
| Safe Table | | | | | |
| Incremental/CDC | | | | | |
| UI browser | | | | | |
| Recovery/security | | | | | |
| Performance | | | | | |

Attach detailed `02_TEST_MATRIX.csv` with test IDs and raw artifacts. A `PASS` requires actual execution and assertions.

## 5. Important deterministic scenarios

| Scenario | Expected | Observed | Result | Evidence |
|---|---|---|---|---|
| E/L full 10k | exact PK/count/value, 0 mismatch | | | |
| Re-run full | no logical duplicate | | | |
| Native SysRevID cursor | no missing rows, correct next cursor | | | |
| Auto Build A→B→C | ordered success & DQ per step | | | |
| A fails | B/C not queued | | | |
| A changed mid-flow | flow failed, no mixed version success | | | |
| Safe Table 100→200 | old table available for rollback | | | |
| ClickHouse publish retry | no accidental reverse exchange | | | |
| Delta upsert/delete | latest A, delete B, insert D | | | |
| Incremental 0→4→7 | cursor changes after publish only | | | |
| Rollback 7→4 | both rows and cursor restored | | | |
| Crash/restart | no false success, correct checkpoint | | | |

## 6. Performance / Airbyte comparison

| Workload | Seed rows | Trial 1 (s) | Trial 2 (s) | Trial 3 (s) | Median (s) | Rows/s | Peak RAM | CPU/IO/source impact | Mismatch |
|---|---:|---:|---:|---:|---:|---:|---:|---|---:|
| 10k full | | | | | | | | | |
| 100k full | | | | | | | | | |
| 1m full (conditional) | | | | | | | | | |
| Incremental 1k | | | | | | | | | |
| No-change startup | | | | | | | | | |
| Retry/crash recovery | | | | | | | | | |
| Airbyte same environment (optional) | | | | | | | | | |

Never claim one engine faster unless same source, schema, row coverage and hardware limits were measured. Explain cold/warm setup and data reconciliation.

## 7. Bug register

| ID | Severity | Failure | Reproduction | Root cause | Fix commit | Regression test | Retest | Status |
|---|---|---|---|---|---|---|---|---|
| BUG-001 | P? | | | | | | | OPEN/FIXED/BLOCKED |

For each bug provide a minimized failing command, before/after logs, explicit file paths and actual expected/observed records. Do not erase the original failure evidence.

## 8. Security / recovery

- Verified no writes to real Epicor: `<evidence>`
- Wrong DB/role tests failed closed: `<evidence>`
- Worker crash / PG crash / CH outage and recovered run IDs: `<evidence>`
- Table UUID before/after Publish and Rollback: `<evidence>`
- Incremental cursor before/stage/after/rollback: `<evidence>`
- Test cleanup/retained backups: `<evidence>`
- Actual Epicor TEST read-only attempt: `NOT ATTEMPTED / BLOCKED / PASS`, reason/evidence.

## 9. Gaps and next actions

1. `<highest-risk blocker + owner + reproduction>`
2. `<what must change before Epicor real-test>`
3. `<what must change before production>`

### Files for ChatGPT review

- `qa-results/<RUN_ID>/HANDOFF_TO_CHATGPT.md`
- `qa-results/<RUN_ID>/02_TEST_MATRIX.csv`
- `qa-results/<RUN_ID>/03_DATA_RECONCILIATION.md`
- `qa-results/<RUN_ID>/04_PERFORMANCE.md`
- `qa-results/<RUN_ID>/05_BUG_REGISTER.md`
- `qa-results/<RUN_ID>/artifacts/`
- `DATAFLOW_LOCAL_QA_RESULTS.zip` (sanitized; no secrets/company data)

**One-line final decision:** `<PASS / CONDITIONAL PASS / FAIL / BLOCKED> — synthetic <status>; Epicor TEST <status>; production NO-GO pending <items>.`