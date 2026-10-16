# Build verification protocol

Purpose: trigger GitHub Actions validation of current Go server, CLI, MCP, and React frontend sources on an isolated pull request. No release or merge is implied.

Checks must pass:
- `go mod tidy`
- `go test -race ./...`
- `go vet ./...`
- `go build ./cmd/...`
- `cd web && npm install && npm run build`

Integration with private ERP SQL Server, ClickHouse and PostgreSQL is not possible on public GitHub-hosted runners without secure, isolated access. Never put database passwords in repository variables or PR logs.

Failures: paste exact workflow log lines into the PR, fix on the test branch, rerun. Merge only after CI passes.
