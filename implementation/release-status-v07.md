# Release Status v0.7 — Managed Sync Recovery (PRE-MERGE)

Status at authoring: **PR #5 open; CI run not yet visible for the latest head SHA. DO NOT deploy or merge until tests pass.**

## Delivered on the feature branch
- Durable PostgreSQL queue dispatch for Managed Sync; API and interval scheduler only enqueue, worker polls and claims.
- Atomic PostgreSQL lease claim locks both `managed_runs` and `sync_checkpoints`; at most one active logical run per stream (partial unique index).
- New ClickHouse `_run_id` for each retry attempt prevents partial old attempts from inflating row-count verification.
- Transactionally commit checkpoint **and** run status only after successful SQL Server snapshot read and verified ClickHouse event count.
- Fenced heartbeat and completion reject stale workers. Known write/count failures leave checkpoint unchanged.
- Lease-expired runs replay from last committed cursor, capped at three claims; crash/shutdown no longer marks run failed prematurely.
- Expose dispatch attempt count and lease expiry from Managed Run GET.
- Isolated PostgreSQL+ClickHouse integration test added to CI for crash simulation, stale-fence CAS, duplicate-run rejection, destination count mismatch, atomic completion and maximum retry cutoff.

## Test matrix
| Test | State |
|---|---|
| Go race tests / vet / build | **PENDING CI** |
| React Vite build | **PENDING CI** |
| PostgreSQL 16 queue/fence/atomic checkpoint integration | **PENDING CI** |
| ClickHouse 25.3 attempt-level row count | **PENDING CI** |
| Epicor SQL Server source SNAPSHOT integration | **NOT AVAILABLE; separate test needed** |

## Required pre-deployment steps
1. Stop all old DataFlow processes (old goroutines must not compete with new leases).
2. Back up PostgreSQL.
3. Inspect `managed_runs` with status `queued` or `running`; resolve any legacy active duplicates/ambiguous checkpoint before migration.
4. Apply migration `009_managed_queue.sql` after 004 (also 008 if SQL DAG enabled).
5. Keep both test execution flags disabled by default. Enable only for isolated scratch testing.
6. Confirm `go test -race ./...`, `go vet ./...`, `go build ./cmd/...`, and `cd web && npm run build`.
7. Run the CI PostgreSQL+ClickHouse integration suite: `go test ./cmd/dataflow -run 'Test(ManagedQueueIntegration|SQLDAGIntegration)$' -count=1 -v` on disposable databases.
8. Before production evaluation: real test SQL Server → PostgreSQL → ClickHouse correctness tests, long-running snapshots and deliberate network failure injection.

## Known limitations
- Physical ClickHouse duplicates across attempts may exist; the latest-state view is the canonical logical result.
- No reliable physical DELETE CDC from SQL Server rowversion; reconciliation is detection-only.
- This change doesn't implement actual SQL Server end-to-end testing on Epicor data.
- This remains a developer preview with bearer-token-only API access and no production operations guarantees.

CI result must be updated with an actual passing workflow link before changing this status.
