# Managed incremental v0.3 — test protocol

Status: **experimental, not production release**. This is a *current-state* rowversion incremental loader, not CDC and not physical DELETE capture.

## Safety gates
- Source SQL Server account MUST be SELECT-only.
- Writes confined to the ClickHouse scratch database and table `dataflow_raw_events`.
- Both `DATAFLOW_ALLOW_TEST_LOAD=true` and `DATAFLOW_ENABLE_MANAGED_TEST=true` required. Defaults are false.
- Source SQL Server database must have SNAPSHOT isolation already enabled by DBA; DataFlow does not enable it.
- ClickHouse scratch raw-event table and `dataflow_latest` view must be installed from `deploy/clickhouse/001_raw_events.sql`.
- PostgreSQL migrations 001–004 are required. On existing PostgreSQL volume, apply 002, 003 and 004 with psql exactly once; Compose init scripts run only on brand-new volumes.

## Execution
1. Start PostgreSQL and backend with source credentials/configuration, plus the two explicit test flags.
2. `cd web && npm install && npm run dev`. Open Sync Jobs → Managed Incremental.
3. Register `Erp.PartTran` with batch size, writer workers and bounded queue.
4. Click Run. REST immediately returns `202 Accepted` and a run UUID; the Go process owns background work.
5. Check status, rows and `lower_cursor`/`upper_exclusive`. Successful completion verifies ClickHouse raw row count **for that Run ID** before committing PostgreSQL lower-inclusive cursor.
6. Run the same stream again after SQL changes. It reads from the persisted exclusive-upper boundary. A no-change run should write zero rows.
7. For terminal test, run `bash scripts/smoke-managed.sh` with environment variables and jq installed.

## Three-store failure semantics
- If ClickHouse writes but HTTP acknowledgment is ambiguous, run fails and cursor remains unchanged. Retry appends duplicate *physical* events but latest-state view groups by stream+PK and source rowversion.
- If ClickHouse count differs from expected, run fails and cursor remains unchanged.
- If PostgreSQL commit fails, raw events remain, cursor remains unchanged and the next successful run replays them.
- On Go process crash, current run may remain marked running; per-stream lease expires after 5 minutes if process is gone. A new run can resume from committed checkpoint after expiration. No automatic crash recovery scheduler yet.
- The SQL SNAPSHOT transaction persists for the duration of reading; for very large tables monitor SQL Server version-store overhead, IO and connection pressure.

## Query latest-state logically
```sql
SELECT _pk, _sysrevid, _payload
FROM scratch.dataflow_latest
WHERE _stream_id = '<stream-uuid>'
LIMIT 20;

SELECT count() FROM scratch.dataflow_raw_events
WHERE _run_id = '<run-uuid>';

SELECT count() FROM scratch.dataflow_latest
WHERE _stream_id = '<stream-uuid>';
```

## Not yet proven or provided
No database integration tests have been run against the user's Epicor/ClickHouse. No physical delete detection, rollback cleanup of orphan raw events, multi-instance durable queue, guaranteed historical change capture, production RBAC, per-stream DDL auto-creation, or exactly-once physical ingestion. `argMax` logical dedup must still be validated for real data and performance.

## Operational note
Manage stream configs on the test target only. The backend permits changing batch/worker/queue settings of an existing stream; such changes affect subsequent runs, not already-started runs. Never manually move checkpoints ahead.


## Interval scheduler (optional dev feature)
Apply PostgreSQL migration 005 before enabling. Set `DATAFLOW_ENABLE_MANAGED_SCHEDULER=true` together with the two managed-test flags, then restart the Go server. The UI provides a scheduler section under Managed Streams. This scheduler uses fixed intervals of 60–86400 seconds (not cron).

```bash
# Create/update a disabled schedule (safe default).
curl -X POST http://localhost:8080/api/v1/managed/schedules \
 -H "Authorization: Bearer $DATAFLOW_API_TOKEN" -H 'Content-Type: application/json' \
 -d '{"stream_id":"<stream-uuid>","interval_seconds":300,"enabled":false}'

# Explicitly enable by updating the same stream schedule:
curl -X POST http://localhost:8080/api/v1/managed/schedules \
 -H "Authorization: Bearer $DATAFLOW_API_TOKEN" -H 'Content-Type: application/json' \
 -d '{"stream_id":"<stream-uuid>","interval_seconds":300,"enabled":true}'

curl -H "Authorization: Bearer $DATAFLOW_API_TOKEN" \
 http://localhost:8080/api/v1/managed/schedules
```

Scheduler claims a due schedule using PostgreSQL `FOR UPDATE SKIP LOCKED`, inserts the run record in the same transaction, and starts a bounded local worker. If the Go process crashes between the transaction commit and worker start, a queued record can remain; this is **not yet** a fully durable worker queue. A schedule is delayed while a recent queued/running job exists for the same stream. Do not use this implementation for strict SLA delivery or multiple active service replicas.

### Definition of a successful test run
- API returns 202 and eventually `status=succeeded`.
- ClickHouse `count() WHERE _run_id` equals the reported rows.
- PostgreSQL `sync_checkpoints.lower_cursor` advanced to the run's `upper_exclusive` exactly once.
- A second run without source changes processes zero rows.
- After forcing one destination failure, checkpoint does **not** advance.
- After killing Go during a run, restart and wait for lease expiry (up to five minutes); re-trigger after ensuring the first process is fully stopped, then verify no logical duplicates in `scratch.dataflow_latest`.

These tests require real SQL Server/PostgreSQL/ClickHouse connections. They have not yet been run here.
