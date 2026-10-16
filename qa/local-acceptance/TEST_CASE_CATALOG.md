# DataFlow v1.3 — Acceptance Case Catalog for Codex Local

**Đây là danh sách yêu cầu kiểm thử; KHÔNG phải kết quả.** Codex sinh `02_TEST_MATRIX.csv` với cột `id,layer,priority,fixture,expected,actual,status,started_at,elapsed_s,evidence,bug_id`, cập nhật sau khi THỰC THI từng case. `NOT RUN` mặc định cho toàn bộ catalog.

| ID | Priority | Case / preparation | Expected result that must be measured | Required evidence |
|---|---|---|---|---|
| ENV-01 | P0 | Git status/fetch/divergence | No silent overwrite; SHA and dirty changes captured | git status, hashes |
| ENV-02 | P0 | Create isolated compose project | QA-only labels/networks/volumes, localhost ports, no unrelated service impacted | docker inspect + ps |
| ENV-03 | P0 | SQL Server synthetic and read-only login | Go app source user can SELECT; no effective DML/DDL | grants / synthetic sentinel |
| ENV-04 | P1 | PostgreSQL migration 001..014 | Tables/indices exist; no unintended drift | migration log + schema checks |
| ENV-05 | P1 | Re-start QA with existing volumes | Data preserved; migrations safe, no duplicate schema failures | hashes + health checks |
| ENV-06 | P1 | API Go boot & real dependency health | `/healthz` 200, SQL Server/PG/CH actual query success | logs + queries |
| ENV-07 | P1 | Web install/build/open | Node build succeeds, UI loads and authenticates | npm output + screenshot |
| ENV-08 | P0 | Secrets isolation / feature flags | Disabled features rejected, no real credentials in logs/artifacts | 401/403 + redacted audit |
| EL-01 | P0 | Source metadata `Erp.PartTran` | Columns, PK, types, rowversion recognized | metadata API + SQL |
| EL-02 | P0 | `SysRevID` binary ordering/paging | Correct unsigned/effective comparison, no missing keys | query samples + cursor |
| EL-03 | P1 | 50–200 rows SMOKE | Correct count, data values, types | source-target compare |
| EL-04 | P0 | Full 10k | Exactly 10k logical rows, 0 PK/value discrepancies | oracle + diff |
| EL-05 | P0 | Full refresh twice | No duplicated logical rows, stable checksum | before/after compare |
| EL-06 | P0 | SQL Server rowversion insert window | New records captured, cursor correct | seed journal + compare |
| EL-07 | P0 | SQL Server rowversion UPDATE | Updated rows captured, no lost/duplicate latest-state | fixture keys + compares |
| EL-08 | P0 | Native rowversion cursor boundary | Inclusive/exclusive semantics correct at page boundaries | 20 boundary keys |
| EL-09 | P1 | NO-CHANGE second job | Zero new logical changes, checkpoint sane | DB count + run status |
| EL-10 | P0 | Crash before ACK / PG checkpoint | Never promote incomplete checkpoint/success | before/after run log |
| EL-11 | P0 | Physical delete of MSSQL fixture | Record limitation: rowversion alone is insufficient for DELETE | source diff + documented gap |
| EL-12 | P1 | 100k full + bounded concurrency | 0 mismatch, resource caps observed | 3 trials + oracle |
| T-01 | P1 | Visual filter/rename | Compiled preview matches client oracle | preview JSON |
| T-02 | P1 | Calculate/aggregate/grouping | Exact group sums, decimal handling | expected vs SQL result |
| T-03 | P1 | NULL/date/unicode transform | No format change or unaccounted NULL loss | 20 sample diffs |
| T-04 | P0 | SQL editor unsafe statements | DDL/DML/Jinja blocked where contract requires | rejection responses |
| T-05 | P1 | dbt View creation | Target `scratch.dfv_*` exists; SELECT expected; run_results saved | dbt logs + CH query |
| T-06 | P1 | Lineage A→B→C | Graph edges and impact exact | API response + screenshot |
| T-07 | P1 | Lineage cycle save | Cycle rejected, old model metadata unchanged | DB before/after |
| T-08 | P0 | Auto Build choose only C | A→B→C ordered, DQ per step, full success | flow steps + DB query |
| T-09 | P0 | Failure in A or change A mid-run | B/C not queued, parent failed safely | child runs + SQL snapshot |
| T-10 | P1 | Restart API during Auto Build | Parent and child IDs persist, resumes without child duplication | PG state + logs |
| TAB-01 | P0 | Build table staging | Published target does not change before approval | old UUID + row checksum |
| TAB-02 | P0 | Empty output DQ | Reject, never published | state + target UUID |
| TAB-03 | P0 | Duplicate unique key DQ | Reject, never published | DQ response |
| TAB-04 | P1 | Valid source 100 rows → table | Stage row count correct, ready | DB checks |
| TAB-05 | P0 | Approve & atomic Publish | Exact new rows, old physical table retained | UUIDs + SQL |
| TAB-06 | P0 | Crash/retry after EXCHANGE | No reverse exchange; stable new target | UUID traces + retry |
| TAB-07 | P0 | Rollback matching backup | Old rows restored; stale/wrong UUID refused | before/after data+UUID |
| INC-01 | P0 | Validate target/delta schema | Exact columns+types, key nonnull, two CDC fields | system.columns + API |
| INC-02 | P0 | Insert/update/delete window 0→4 | A latest=100, B deleted, D added exactly once | full row oracle |
| INC-03 | P0 | Stage no cursor commit | Target and checkpoint unchanged while `ready` | PG cursor, CH UUID |
| INC-04 | P0 | Explicit Publish window 0→4 | Target correct, cursor 4, stage backup retained | DB+PG proof |
| INC-05 | P0 | Second window 4→7 | Latest updates/deletes correct, cursor 7 | second oracle |
| INC-06 | P0 | Rollback 7→4 | Exact prior rows and cursor 4 | source/target/checkpoint |
| INC-07 | P0 | Replay window after rollback | No duplicate rows, correct latest state | two snapshots |
| INC-08 | P0 | Duplicate global version/bad tombstone | Reject bad delta and no cursor advance | error, PG status |
| INC-09 | P0 | Wrong baseline UUID / stale cursor | Fail closed, no publish | PG+CH state |
| INC-10 | P1 | No changes / delete all / caps | Empty final set allowed; no_change no advance; >50k/1m blocked | test run results |
| INC-11 | P1 | Late arrival with old version | Known unsupported case detected/documented, not silent PASS | source delta + gap |
| UI-01 | P1 | Source browser/filter | Data/columns render, errors actionable | screenshots/video |
| UI-02 | P1 | Visual transform drag/drop + SQL | Steps/preview reflect settings | screenshots + oracle |
| UI-03 | P1 | Save/Load model | Roundtrip without silent mutation | metadata + UI |
| UI-04 | P1 | Lineage click + impact | Descendants highlighted correctly | screenshot |
| UI-05 | P1 | Run C once + live steps | Status displayed until terminal | screenshot+run ID |
| UI-06 | P1 | Table/Incremental Publish & rollback | Buttons gated correctly, errors and counts visible | screenshot+API status |
| REC-01 | P0 | Worker crash during batch | Recovery bounded, no false success | logs and DB status |
| REC-02 | P0 | Stale worker fencing | Stale claim cannot finalize/publish | DB row/fence proof |
| REC-03 | P0 | ClickHouse outage during publish | No corrupted target/advanced checkpoint | before/after snapshots |
| REC-04 | P0 | PostgreSQL outage during publish | Recovery resolves UUID and cursor without reverse swap | UUID/PG traces |
| REC-05 | P1 | API outage mid Auto Build | Persists and resumes, or clearly terminates | restart timeline |
| REC-06 | P0 | Wrong target DB / privilege escalation | HTTP 403/422 or equivalent fail-closed | request/response |
| REC-07 | P0 | Cleanup guard | Only QA-labeled resources targeted; backups retained | docker inspect logs |
| PERF-01 | P2 | Cold full 10k | 3 trials; median/range recorded; mismatch 0 | perf.csv |
| PERF-02 | P2 | Full 100k | 3 trials; source CPU/IO/RAM measured if possible | perf.csv |
| PERF-03 | P2 | Full 1m resource-gated | 3 trials if resources allow, else BLOCKED-RESOURCE | perf.csv |
| PERF-04 | P2 | No-change startup | Startup latency measured independently | timestamps |
| PERF-05 | P2 | Delta 1k/10k/50k | Correct rows, throughput and peak resources | perf.csv |
| PERF-06 | P2 | Crash recovery elapsed | Retry-to-terminal and no data mismatch | logs + durations |
| PERF-07 | P2 | Airbyte apples-to-apples (optional) | Only compare with same hardware/data/correctness; otherwise NOT RUN | configs + trials |

## Rules for report generation

- Status vocabulary: `PASS`, `FAIL`, `BLOCKED`, `NOT RUN`, `N/A` (N/A must explain why). Never count `NOT RUN` as PASS.
- For P0/P1 failures, overall `FAIL` / production `NO-GO`; for incomplete high-risk cases, overall `CONDITIONAL PASS` or `BLOCKED`, never fully PASS.
- Each row must link to a real log, API response, SQL query, fixture oracle or screenshot. Exit code alone is not proof of data correctness.
- Cases involving SQL Server `INSERT/UPDATE/DELETE` run **only on synthetic isolated SQL Server fixture**; verify container/database identity first.
- Real Epicor TEST coverage is reported **separately** from this catalog; only read-only operations allowed, with explicit user authorization.