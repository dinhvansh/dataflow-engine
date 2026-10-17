# DataFlow v1.4 follow-up handoff — 2026-10-16

Base reviewed: `origin/main` at `61cfd7a712b5c600167a85c7ee8fd36c44f1da8d`. The worktree was clean and the prior QA branch was one commit ahead; no main divergence or conflict was found before work began. PR #20 and its latest independent review were read. No branch below has been merged.

## Pull requests

| PR | Commit | Scope | Local result |
|---|---|---|---|
| #21 | `d99775b43933c9e529fa7798cff5e0c9e8a048be` | Fail-closed live-browser preflight, trace secret scan, unique owned resources/cleanup, Docker-label checks, disk-capacity guard | PASS; GitHub CI PASS |
| #22 | `d7fbf7065e3a3684edd98a3dd5c83b8e872a764e` | Key-snapshot hard-delete candidates, two-snapshot confirmation, explicit approval, monotonic reserved versions, idempotent tombstones | Unit/race/vet/build and GitHub CI PASS; live end-to-end BLOCKED by the existing synthetic rowversion-width issue described below |
| #23 | `fe98ed75e0a712c0e02c9043d5ced074637d4c40` | Recovery-to-succeeded runner and 100k/1M resource/publish benchmark | PASS locally; GitHub CI PASS |

All three are code-only: no raw ZIP, browser trace, screenshot, generated log, result directory, credential, or sensitive source data is included.

## Hard Delete safety contract

- SQL Server access remains SELECT-only. No Epicor DML, DDL, trigger, or CDC configuration is performed.
- A key missing from one complete snapshot becomes only a candidate.
- Confirmation requires the same key to be absent from the next complete snapshot and validates the complete succeeded three-generation chain.
- Emission is separately disabled by default and requires both `DATAFLOW_ENABLE_HARD_DELETE_TEST=true` and the exact action phrase `EMIT_TOMBSTONES`.
- PostgreSQL reserves per-stream monotonic versions under an advisory lock before the ClickHouse write. A crash retry reuses the reserved version.
- ClickHouse stores the evidence in a scratch-only `ReplacingMergeTree`, keyed by stream/key/version; retries are idempotent.
- Reappearing unconfirmed keys invalidate their candidate. Confirmed/emitted evidence is immutable for operator review.

## Actual verification

- `go test -race ./...`: PASS.
- `go vet ./...`: PASS.
- `go build ./...`: PASS.
- Mock browser suite and six synthetic live-browser cases on PR #21: PASS; default live discovery skips without explicit opt-in.
- Migration 016 applied to disposable PostgreSQL: PASS.
- Recovery integration with real disposable PostgreSQL and ClickHouse: PASS. The expired first attempt was reclaimed with a new owner/fence, the stale worker was rejected, the second attempt reached `succeeded`, and the checkpoint advanced exactly once.
- Benchmark reconciliation: 18/18 samples PASS.

### Benchmark summary

| Baseline | Delta | Stage median | Stage min–max |
|---:|---:|---:|---:|
| 100,000 | 1,000 | 46.035 ms | 45.338–63.801 ms |
| 100,000 | 10,000 | 46.041 ms | 43.804–57.597 ms |
| 100,000 | 50,000 | 53.553 ms | 53.076–54.722 ms |
| 1,000,000 | 1,000 | 138.832 ms | 126.399–163.361 ms |
| 1,000,000 | 10,000 | 138.084 ms | 121.698–143.220 ms |
| 1,000,000 | 50,000 | 162.386 ms | 127.126–179.205 ms |

Atomic publish was 3.567–5.829 ms in the 100k run; rollback was 3.498–4.520 ms. Observed peaks over the complete benchmark were ClickHouse 128.79% CPU and 949,485,568 bytes RAM, PostgreSQL 2.02% CPU and 47,898,951.68 bytes RAM. ClickHouse global I/O counter deltas over the isolated run were 267,317,595 read bytes and 108,461,703 written bytes. These are container/system counter observations, not per-query attribution.

## Open bug and decisions

1. **BLOCKED — synthetic reconciliation rowversion width.** A live hard-delete reconciliation run stopped safely before evidence publication with `rowversion requires 16 hexadecimal digits, got 8`. No tombstone or source mutation occurred. The fixture declares `rowversion`; the SQL Server conversion/driver path needs an isolated regression fix before PR #22 can receive an E2E PASS.
2. Decide whether tombstone evidence stays in the dedicated scratch table or is adapted into each model's typed delta contract. PR #22 intentionally does not guess payload schemas or publish into production-facing tables.
3. Review the large formatting-only component in PR #22 before merge; behavior and tests are isolated, but the repository's older compact Go formatting makes that diff noisier than desired.
4. GitHub CI is green; independent review is still required for PRs #22 and #23. Do not merge automatically.

## Recommendation

GO for review and continued synthetic QA of PR #21 and PR #23. NO-GO for enabling Hard Delete outside disposable QA until the rowversion-width blocker is fixed, the destination delta integration decision is made, and PR #22 passes a full three-generation live test plus crash/retry emission test.
