# v0.2 safe test guide — developer preview

**Not a production release.** SQL Server/Epicor stays SELECT-only. ClickHouse writes are forcibly restricted to `DATAFLOW_TEST_TARGET_DATABASE` (defaults to `scratch`) and additionally require `DATAFLOW_ALLOW_TEST_LOAD=true`.

## Prerequisites

- Go 1.23+, Node 22, PostgreSQL 16, SQL Server login with SELECT privileges, ClickHouse HTTP endpoint.
- SQL Server database must have `ALLOW_SNAPSHOT_ISOLATION ON` for rowversion windows and atomic full refresh. Check using read-only SQL:
  ```sql
  SELECT name, snapshot_isolation_state_desc
  FROM sys.databases WHERE name=DB_NAME();
  ```
  If OFF, do not modify ERP settings; request DBA review or use non-atomic scratch append trial without strict source snapshot guarantees.
- ClickHouse scratch database must use the Atomic engine and support CREATE TABLE, EXCHANGE TABLES and DROP TABLE privileges when atomic_replace is used. Create and validate target table schema manually; field types must match serialized source values.
- PostgreSQL migration 001 and 002: on **new empty volume**, the Compose init directory runs both. On **existing volume**, PostgreSQL init scripts do not rerun; apply migration 002 manually after backup:
  `psql "$DATAFLOW_POSTGRES_DSN" -v ON_ERROR_STOP=1 -f migrations/postgres/002_sync_checkpoint.sql`.

## Start

```bash
docker compose up -d postgres
go mod tidy
go test ./...
go vet ./...
go run ./cmd/dataflow
# second shell:
cd web && npm install && npm run dev
```

Go service: http://127.0.0.1:8080, UI: http://localhost:5173. Supply env vars from `.env.example` in the process environment. Do not commit secrets.

## Safe API examples
Read-only metadata:
```bash
curl -H "Authorization: Bearer $DATAFLOW_API_TOKEN" 'http://localhost:8080/api/v1/source/tables?schema=Erp'
curl -H "Authorization: Bearer $DATAFLOW_API_TOKEN" 'http://localhost:8080/api/v1/source/columns?schema=Erp&table=PartTran'
curl -H "Authorization: Bearer $DATAFLOW_API_TOKEN" 'http://localhost:8080/api/v1/source/rowversion-upper'
```

Scratch full refresh with atomic exchange:
```bash
curl -X POST http://localhost:8080/api/v1/bootstrap/runs \
 -H "Authorization: Bearer $DATAFLOW_API_TOKEN" -H 'Content-Type: application/json' \
 -d '{"mode":"full","atomic_replace":true,"source_schema":"Erp","source_table":"Part","target_database":"scratch","target_table":"part_test","batch_size":1000,"workers":2,"queue_capacity":4}'
```

Incremental test using native binary cursor (no persisted state):
```bash
curl -X POST http://localhost:8080/api/v1/bootstrap/runs \
 -H "Authorization: Bearer $DATAFLOW_API_TOKEN" -H 'Content-Type: application/json' \
 -d '{"mode":"rowversion","lower_cursor":"0x0000000000000000","source_schema":"Erp","source_table":"PartTran","target_database":"scratch","target_table":"parttran_changes","batch_size":1000,"workers":2,"queue_capacity":4}'
```
Successful response includes `upper_exclusive`. This is **not** a saved checkpoint. It is meaningful only for a completely successful and verified window. Re-running may duplicate rows in raw append table.

## High-priority test sequence

1. Test metadata/column types, especially `SysRevID` and `SysRowID`.
2. Compare source and scratch ClickHouse record counts after atomic full refresh.
3. Repeat full refresh: no duplicate logical target rows, original table remains if stage load fails before publish.
4. Validate SQL Server snapshot availability and source query performance/impact.
5. Test rowversion window with controlled INSERT and UPDATE in a **test** database, never modifying the ERP database through DataFlow.
6. Kill process mid-load: note orphan stage table; target must remain prior version if exchange not attempted.
7. Intentionally revoke destination permissions and verify errors do not turn into success.
8. Run `go test -race ./internal/cursor`; run Go+web CI.
9. Do **not** test automatic cursor checkpoint or delete detection: not connected yet.

## Cautions
A network failure during `EXCHANGE TABLES` may leave publish outcome ambiguous; inspect actual ClickHouse table status before retry. Old target is dropped only after successful confirmation, and cleanup failures are logged. Long SQL Server SNAPSHOT transactions can increase version store and IO use. Current API is synchronous: keep the requesting client connected. This is still development-only.
