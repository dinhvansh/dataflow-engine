# Run the Phase 0–1 Bootstrap

**Safety:** This is an experimental FULL READ and APPEND writer, not finished full-refresh replacement. Target table must already exist and be disposable: rerunning appends duplicates. No checkpoint, no incremental, no auto schema creation, no durable resume and no production migration. Do not point to live reporting tables.

## Requirements
Go 1.23+, PostgreSQL 16, SQL Server read-only login and ClickHouse target. Download Go dependencies with network access once.

## Start
1. Copy .env.example values into your secure runtime environment; do not commit credentials.
2. `docker compose up -d postgres`. Migration runs only on first initialization of its volume.
3. `go mod tidy && go test ./...`
4. Set `DATAFLOW_ALLOW_TEST_LOAD=true` only for deliberate trial destination.
5. `go run ./cmd/dataflow`
6. Create a ClickHouse table compatible with chosen SQL Server columns (manual step).
7. Trigger:
```bash
curl -X POST http://localhost:8080/api/v1/bootstrap/runs \
 -H "Authorization: Bearer YOUR_API_TOKEN" \
 -H "Content-Type: application/json" \
 -d '{"source_schema":"Erp","source_table":"Part","target_database":"scratch","target_table":"part_test","batch_size":1000,"workers":2,"queue_capacity":4}'
```

The bootstrap API currently executes synchronously and returns after completion. Disconnecting the client can cancel the job. Production API requires async run queue, lease, durable stage-and-publish and scoped authorization.

## Known limitations
- SQL Server SELECT uses one streaming cursor and no paging; very large source scans may be expensive.
- Binary values are hex encoded, numeric/date/UUID mapping depends on the destination table.
- SQL Server date and decimal types require end-to-end testing.
- No source-side modification; no index creation.
- ClickHouse HTTP writer requires compatible target columns and accepts possible duplicate writes.
- Service only suitable for isolated internal development networks; static bearer token is not RBAC.
- No JWT auth, TLS termination, schema evolution, reconciler, scheduler, DAG or UI yet.
