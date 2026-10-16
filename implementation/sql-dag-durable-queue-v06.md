# SQL DAG durable queue — stability milestone v0.6

## Purpose
Prevent SELECT-only DAG runs from being permanently stranded in `queued` after an API process crash. This is a **PostgreSQL-backed lease queue**, replacing the direct API goroutine. Never interpret this as a general exactly-once ETL queue.

## Operational semantics
- API validates enabled SQL Jobs and freezes an immutable plan+SQL snapshot in `dag_runs`; returns HTTP 202.
- The SQL DAG dispatcher (enabled only with `DATAFLOW_ALLOW_TEST_LOAD=true` and `DATAFLOW_ENABLE_SQL_JOBS_TEST=true` and a dedicated `DATAFLOW_CH_READONLY_USER`) polls every 2 seconds, claims up to 2 jobs using `FOR UPDATE SKIP LOCKED`.
- Claim increments `fence_token` and `dispatch_attempts`, sets a new owner UUID and a 120-second PostgreSQL-side lease.
- Worker heartbeats every 20 seconds; loss of ownership cancels the run. Completion updates the row only if owner/fence and lease still match.
- On restart, any `queued` run or expired `running` run is claimable. At most 3 claims. Exhausted expired runs become `failed`, never silently marked `succeeded`.
- Jobs are **read-only SELECT**, so full-DAG replay from the frozen snapshot after crash is acceptable for this preview. Completed prior SELECT nodes may run again. Result persistence is at-most-once per fenced owner, not exactly-once query execution.
- A 30-minute DAG timeout, per query 30-second ClickHouse timeout and bounded 200-row responses remain enforced.
- When the service receives SIGTERM/SIGINT, it cancels the dispatcher and requests HTTP graceful shutdown. It intentionally does not finalize cancelled DAG work; after lease expiry another process may resume.

## Migration
Apply `migrations/postgres/008_dag_leases.sql` after 007. **Do not enable the dispatcher without migration 008.** On an existing PostgreSQL volume, init SQL scripts are not automatically replayed. Back up first, apply the migration once and restart DataFlow.

## Test evidence
The isolated GitHub CI database test creates the SQL DAG metadata schema, queues a run, simulates a stale worker lease and validates:
1. A fresh worker increments the fence.
2. The previous worker cannot commit after expiry.
3. A simultaneous second claim is refused.
4. The recovered owner runs the original frozen DAG successfully and stores one terminal result.

## Remaining work
- Not safe for DML/transformation tasks; these require separate idempotent stage/publish mechanics before retries.
- Automatic recovery of **managed sync jobs** remains separate and is not implemented here.
- No real Epicor SQL Server integration test or production-grade RBAC.
- No task-level resume checkpoints; the entire SELECT DAG may replay after interruption.
- Monitor PostgreSQL connection count and ClickHouse query load before increasing worker capacity.
