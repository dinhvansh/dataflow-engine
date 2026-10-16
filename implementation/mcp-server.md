# DataFlow MCP Server v0.1 (stdio)

## Goal
Expose controlled AI access using Model Context Protocol via a separate Go executable. The MCP adapter is a client of DataFlow REST API, not a database connector and does not bypass application authorization.

## Run
```bash
go run ./cmd/dataflow-mcp
```

Environment:
- `DATAFLOW_API_URL` optional (default `http://127.0.0.1:8080`)
- `DATAFLOW_API_TOKEN` needed to read protected run status
- `DATAFLOW_MCP_ALLOW_WRITES=false` default; only set true for isolated development instances. The Go backend ALSO needs `DATAFLOW_ALLOW_TEST_LOAD=true`.

### Example desktop MCP client configuration
```json
{"mcpServers":{"dataflow":{"command":"/absolute/path/to/dataflow-mcp","env":{"DATAFLOW_API_URL":"http://127.0.0.1:8080","DATAFLOW_API_TOKEN":"YOUR_LOCAL_TOKEN"}}}}
```
Compile with `go build -o dataflow-mcp ./cmd/dataflow-mcp`, then set command to its absolute path. Do not store API secrets in committed config.

## Tools currently available
- `dataflow_health`: calls GET /healthz.
- `dataflow_get_run`: calls GET /api/v1/bootstrap/status?id={run_id}.
- `dataflow_run_test_load`: **hidden unless explicitly enabled**; calls experimental full-load append endpoint; requires the MCP client to seek explicit human approval before each operation.

## Safety constraints
Current backend's API bearer token is coarse-grained, not production RBAC. Do not expose MCP over a public interface or register with untrusted agents. Start with local stdio; OAuth/remote Streamable HTTP, per-tool scopes, audited approvals and async jobs are future features. A successful MCP response is not proof a load was correct; verify with reconciliation before use.

## Contract gaps
MCP tools for list jobs, schema discovery, pipeline controls and metrics must wait for genuine backend APIs; no stub tools that claim operational success.
