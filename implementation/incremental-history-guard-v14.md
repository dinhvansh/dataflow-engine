# v1.4 — Incremental CDC historical-integrity guard (TEST only)

## Why
v1.3's `_df_version > checkpoint` selection cannot see an event which is
appended **later** carrying an older version. A finished sync could otherwise
skip it silently.

## Guarded behavior
With `DATAFLOW_ENABLE_INCREMENTAL_HISTORY_GUARD_TEST=true` on the **isolated
worker** and migration `015_incremental_history_guard.sql`, the worker:
1. Rechecks all prior events `_df_version <= cursor_lower` before staging,
   comparing count and an order-independent 128-bit sum of payload-inclusive
   row hashes against the last verified checkpoint.
2. Rechecks that historical prefix after staging, and records a fingerprint
   of all events `_df_version <= cursor_upper` with the ready run.
3. Immediately before a UUID-fenced ClickHouse publication or rollback,
   verifies that the whole captured prefix still matches.
4. Commits the witness with the target cursor in the **same PostgreSQL
   transaction**, or restores the earlier witness during rollback.
5. Refuses a new run even when max version is unchanged if an older event was
   appended; the cursor and published data are not advanced by that run.

Initial cursor **zero** is accepted only with an empty history prefix.
Existing nonzero v1.3 cursors with no prior witness are **blocked**. Do not
silently seed them from an untrusted source or reset a checkpoint.

## Non-goals and limits
- This is **detect-and-stop**, not correction/automatic repair. A failure
  requires source audit, explicit reconciliation, and a documented operator
  decision. Never DELETE or UPDATE Epicor data.
- Whole-history fingerprinting is O(total events), not performant for large
  CDC streams. The digest is a non-cryptographic integrity signal.
- A separate ClickHouse query is **not an atomic writer lock**. A writer can
  still append after final verification and before/after publish; the next
  guarded run should detect it, but immediate visibility cannot be guaranteed.
- This does not invent physical DELETE events for Epicor SysRevID, establish
  a true source transaction order, or certify production exactly-once delivery.
  Production acceptance needs an ordered ingest sequence / immutable batch
  manifest, watermark fencing and source-consistent change capture.
- TEST scratch only; leave this feature disabled until migration and review.
  Before starting the isolated worker verify dedicated credentials and
  `DATAFLOW_TEST_TARGET_DATABASE=scratch`.

## Test gates
Real disposable PostgreSQL + ClickHouse integration must demonstrate two
ordered publish windows, rollback, late append at/below checkpoint after
publish, late append between stage and publish, and unchanged checkpoint when
the guard rejects it. `INC-11` cannot be marked PASS without these logs.

## Feature-flag downgrade protection
Once a target has a stored historical witness, a future run with the guard
turned OFF is refused instead of silently reverting to version-only paging.
Likewise, a previously witnessed staging run cannot be published/rolled back
with the guard switched off. This is tested on a disposable fixture.
