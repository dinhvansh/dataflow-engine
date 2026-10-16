# v0.3 engineering status — Managed Incremental preview

Updated 2026-10-16. Source of truth: GitHub main. This document intentionally distinguishes **implemented source** from **validated operational behavior**.

## Implemented and pushed

| Capability | Code | Validation state |
|---|---|---|
| SQL Server native rowversion & composite keyset | `internal/connectors/sqlserver/incremental.go` | Unit query generation only; needs real SQL Server integration |
| SNAPSHOT source isolation | reader + registration gate | Needs DBA-provided SNAPSHOT ON test database |
| PostgreSQL fenced checkpoints | `internal/state/postgres/checkpoint.go` | Migration exists; requires concurrent failure test |
| Worker/queue/backpressure | `internal/engine/` | Existing unit tests |
| Raw event envelopes | `internal/connectors/clickhouse/events.go` | HTTP test only |
| Logical latest-state view | `deploy/clickhouse/001_raw_events.sql` | Requires ClickHouse validation |
| Durable count check per run | `internal/connectors/clickhouse/verify.go` | HTTP test only |
| Managed stream registry & async run | `cmd/dataflow/managed.go` | Integration NOT run |
| Interval schedules (opt-in) | `cmd/dataflow/schedules.go` | Integration NOT run; not HA worker queue |
| React management UI | `web/src/components/{ManagedStreams,SchedulePanel}.tsx` | Frontend Vite/TypeScript build passed in GitHub Actions PR #1; browser smoke test pending |
| CLI + MCP | `cmd/dataflowctl`, `cmd/dataflow-mcp` | Go command build passed in GitHub Actions PR #1; client integration pending |
| CI pipeline | `.github/workflows/ci.yml` | PR #1 CI passed for Go race tests, vet, command build and React build on 2026-10-16 |

## Required environment preparation

1. Apply PostgreSQL migrations 001–005 in order. In a fresh Compose volume these are initialized automatically; for existing volumes run migrations explicitly after backup.
2. Create ClickHouse `scratch.dataflow_raw_events` and `scratch.dataflow_latest` using `deploy/clickhouse/001_raw_events.sql`, ensuring `scratch` is disposable/test-only.
3. SQL Server/Epicor connection must be SELECT-only with source database SNAPSHOT isolation already ON.
4. Start backend with `DATAFLOW_ALLOW_TEST_LOAD=true`, `DATAFLOW_ENABLE_MANAGED_TEST=true`, and set `DATAFLOW_TEST_TARGET_DATABASE=scratch`. Scheduler is off unless `DATAFLOW_ENABLE_MANAGED_SCHEDULER=true`.
5. Run `go mod tidy && go test -race ./... && go vet ./... && go build ./cmd/...` and `cd web && npm install && npm run build`.
6. Run `scripts/smoke-managed.sh` with `DATAFLOW_API_TOKEN`; inspect source/target and PostgreSQL checkpoint before/after.
7. Test no-change second run, controlled INSERT/UPDATE in a **test source**, CH failure, restart, and logical dedup.
8. Only after correctness tests, attempt PartTran (~7m rows) while monitoring ERP IO/version store.

## Explicit limitations

- Not a production-ready Airbyte replacement.
- Physical DELETE / snapshot reconciliation missing.
- SQL Jobs cannot yet execute; DAG only validates definitions.
- Interval schedules are in-process; queued runs are not a fully durable distributed queue.
- No stable production auth/RBAC, no end-to-end exactly-once, no historical event capture from rowversion.
- The first managed run may load a large table; start with a small representative source.
- Database integration and browser E2E tests were not performed. Go test -race, go vet, go build ./cmd/... and React npm run build passed in GitHub Actions PR #1; no production-data test performed.

## Failure invariant

**Never move a checkpoint manually to the largest observed SysRevID.** A managed run only commits its next lower-inclusive boundary after (1) SQL Server SNAPSHOT reader completes, (2) all ClickHouse writes return success, (3) destination run row count matches, and (4) PostgreSQL lease + generation CAS succeeds. Replays may produce physical raw duplicates; query `dataflow_latest` for logical latest state.

## Verified build evidence (2026-10-16)

- [PR #1 build verification](https://github.com/dinhvansh/dataflow-engine/pull/1): merged after both CI jobs passed.
- [Passing GitHub Actions run](https://github.com/dinhvansh/dataflow-engine/actions/runs/38862048696): Go `go mod tidy`, `go test -race ./...`, `go vet ./...`, `go build ./cmd/...`; Web `npm install` and `npm run build`.
- Fix: `web/src/lib/api.ts` now awaits `Promise<Response>` before checking `Response.ok`. This resolved five TypeScript errors.
- Remaining: no connected SQL Server/PostgreSQL/ClickHouse integration, browser E2E or data correctness benchmark.
