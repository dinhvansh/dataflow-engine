# Commands and versions

Major executed groups:

- Git status/fetch/SHA/divergence and branch creation.
- Docker Compose QA database startup, migrations, fixture seed, isolation checks.
- `go test -race ./...`, `go vet ./...`, `go build -buildvcs=false ./cmd/...` in Go 1.23.12.
- `npm install`, then reproducible `npm ci`, `npm run build`.
- Real ClickHouse/dbt integrations and PostgreSQL/ClickHouse worker/API integrations.
- Real SQL Server snapshot/full/rowversion integration.
- API full loads 10k/100k/1m, rowversion delta/no-change, CH and PG outages, auth/wrong DB checks.

Exact stdout/stderr and failure/retry chronology are in `artifacts/`. Environment versions are in `artifacts/environment.txt`; patch is `artifacts/code.diff`.

