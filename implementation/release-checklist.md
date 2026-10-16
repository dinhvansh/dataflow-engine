# First integrated testing release — checklist

## Current build status
This repository is a pre-production prototype. Go and React sources exist, but no live Epicor/ClickHouse integration has been verified. MCP is local stdio. **Do not retire Airbyte or run against a production ClickHouse reporting table.**

## Local commands
```bash
go mod tidy
go test ./...
go vet ./...
cd web && npm install && npm run build
```

## Required user-side setup
1. Use isolated SQL Server read-only credentials and an intentionally disposable ClickHouse target.
2. Start PostgreSQL and apply migration 001.
3. Supply DATAFLOW_POSTGRES_DSN, DATAFLOW_SQLSERVER_DSN, DATAFLOW_CH_URL and DATAFLOW_API_TOKEN.
4. Explicitly set DATAFLOW_ALLOW_TEST_LOAD=true for testing only.
5. Run Go server; start web UI. Inspect API status before and after the run.
6. Compare row count and representative values; clear the disposable target between repeated tests.

## Release blockers
- Full refresh needs staging, verification, safe generation switch and idempotent retries.
- Incremental requires verified rowversion boundary/isolation and durable checkpoint.
- Current UI Connections/Pipelines are not operational; roles, persistent job registry and scheduler pending.
- Typed SQL Server → ClickHouse mapping needs real-world integration, especially decimal/date/UUID/binary.
- Async job ownership, long-running health and graceful shutdown pending.
- Evaluate SQL Server/ClickHouse resource usage on actual test server.
