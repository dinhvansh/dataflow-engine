# Managed Sync Durable Recovery v0.7 — Test-only implementation

## Why this change
Earlier HTTP and interval scheduler paths spawned Go goroutines after inserting `managed_runs.queued`, so a process restart could strand runs. The old implementation committed `sync_checkpoints` separately from marking a run successful; crashes made status ambiguous.

## New contract
- The POST /api/v1/managed/runs endpoint and interval scheduler **only enqueue** in PostgreSQL.
- A continuously polling dispatcher claims `queued` runs (or `running` runs whose lease expired). The run and stream checkpoint are leased **atomically in a single PostgreSQL transaction** using `FOR UPDATE OF r,c SKIP LOCKED`.
- Exactly one active logical run per stream is enforced by PostgreSQL's partial unique index.
- Heartbeats renew both lease records transactionally. A stolen/expired worker cannot commit checkpoint or terminal state.
- Every claim uses a new random `lease_owner` UUID as ClickHouse `_run_id`; a previous attempt's partial inserts do not corrupt the current attempt row count. Physical event repeats remain possible across attempts; `dataflow_latest` returns the latest version by stream and PK.
- After successful SNAPSHOT read and ClickHouse insert acknowledgment, `CountRunRows(attempt_uuid)` must match rows written. **Only then** PostgreSQL commits checkpoint and `managed_runs.succeeded` in the same transaction.
- Known data/IO failures mark the run `failed` without moving cursor. Shutdown/lease loss lets the run be reclaimed after expiry; at most **three attempts**, then terminal failure without checkpoint movement.
- Managed Sync remains test-only and writes exclusively to the configured `scratch.dataflow_raw_events` target; it never modifies SQL Server source.

## Deploy safely
1. Stop old DataFlow workers **before** applying migration 009. Back up PostgreSQL.
2. Review existing `managed_runs` in `queued`/`running`; ensure at most one active row per stream. Resolve legacy running states and checkpoint consistency manually before switching workers. The partial unique index intentionally refuses conflicting data.
3. Apply `migrations/postgres/009_managed_queue.sql` after migrations 001–004; the SQL DAG migration 008 is separate. Existing PostgreSQL Docker volumes do not auto-apply new migrations.
4. Keep `DATAFLOW_ALLOW_TEST_LOAD` and `DATAFLOW_ENABLE_MANAGED_TEST` disabled except in an isolated test environment. With both enabled, startup requires migration 009 and launches dispatcher automatically.
5. The existing `DATAFLOW_ENABLE_MANAGED_SCHEDULER` only schedules/enqueues; it never executes directly.
6. Test with a small test source. Confirm a no-change second run writes zero rows and checkpoint advances only on verified success.
7. Observe ClickHouse `_run_id` values: these are attempt UUIDs, not the public logical run UUID returned by the API.

## Automated tests
- `go test -race ./...` and `go vet ./...`.
- CI integration uses isolated PostgreSQL 16 and ClickHouse 25.3. It tests duplicate queue rejection; lease expiry and fresh ownership; stale worker fence rejection; per-attempt ClickHouse event counting; a single atomic checkpoint+success transaction; destination mismatch leaving checkpoint unchanged; and terminal cutoff after three crashed attempts.

## Limitations (important)
- **Real Epicor SQL Server integration still pending**. SQL Server SNAPSHOT behavior, native rowversion, 7m-row performance, data-type mapping and resource overhead must be tested on a non-production database.
- Successful HTTP ClickHouse INSERT plus count verification does not prove cross-store exactly-once or event-history correctness; retries may leave extra raw events.
- Physical deletes are not captured by SysRevID. Snapshot reconciliation is still detection-only.
- API queue has no priority scheduling, manual cancel or RBAC beyond shared bearer token. Test-only mode remains disabled by default.
- Go shutdown cancellation is best-effort: lease expiry/recovery is the guarantee, not immediate resumption.
