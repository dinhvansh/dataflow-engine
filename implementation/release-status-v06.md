# DataFlow v0.6 — Stability release gate (Developer Preview)

Date: 2026-10-16. GitHub `main`; PR #4 merged after CI passed.

## What changed
- SELECT-only SQL DAG jobs no longer run from the API request goroutine. The POST endpoint commits an immutable queued run in PostgreSQL and returns HTTP 202.
- PostgreSQL-backed dispatcher claims queued work and expired workers using `FOR UPDATE SKIP LOCKED`.
- Each claim has a unique lease owner, monotone fencing token, lease expiry in PostgreSQL, heartbeat and capped dispatch attempts (3).
- Expired workers cannot commit their result over a newer worker. An interrupted run resumes after lease expiry, using its immutable SQL snapshot; completed SELECT steps may replay.
- SIGINT/SIGTERM stop new claims and cancel active execution. Missing migration 008 causes an explicit startup failure when SQL DAG execution is enabled.
- All SQL Jobs are still test-only, ClickHouse SELECT-only and require a separately grant-restricted ClickHouse account. They do not create marts or mutate ERP.

## Verified evidence
- [PR #4](https://github.com/dinhvansh/dataflow-engine/pull/4) merged to main.
- [Passing GitHub Actions 38868112698](https://github.com/dinhvansh/dataflow-engine/actions/runs/38868112698): `go test -race ./...`, `go vet ./...`, `go build ./cmd/...`, `npm run build`.
- Integration CI provisions PostgreSQL 16 and ClickHouse 25.3; verifies a DAG succeeds, frozen SQL remains stable despite registry edits, expiring/reclaiming leases increments fence, stale completion is rejected, double claim is prevented, and exhausted work is marked failed after 3 leases.

## Required operator action
Existing PostgreSQL instances **must apply `migrations/postgres/008_dag_leases.sql`** after 007. The dev runner refuses to start with SQL DAG flags enabled if this migration is absent. On a new Compose PostgreSQL volume, SQL scripts in the init directory are applied; existing volumes never auto-run new migration files.

## Current boundaries
- The replay guarantee covers **read-only SELECT SQL DAGs only**. It is not an exactly-once executor for INSERT/UPDATE/DELETE or materializations.
- SQL Server Epicor → ClickHouse full 3-database integration is still unverified.
- Managed Sync and reconciliation runs have their own execution/recovery semantics and are **not made durable by this change**.
- No production RBAC, credentials vault, high-availability failover SLA, database DML transformations, end-to-end GUI automation, benchmark, or cross-store exact-once proof.
- A previously running DAG might be re-executed after lease expiry: concurrency/fencing protects metadata result, not avoidance of duplicate SELECT workload.
- Avoid production data and leave test-mode flags disabled until the full deployment acceptance tests pass.

## Next stability-first gates
1. Automated PostgreSQL + SQL Server TEST + ClickHouse full ingestion with forced write failures, snapshot isolation and checkpoint verification.
2. Restart recovery and fenced worker execution for managed sync and reconciliation.
3. Idempotent staging/atomic publish for true SQL transformations, with DQ gates and rollback.
4. User/RBAC security hardening and operational monitoring/alerts.
