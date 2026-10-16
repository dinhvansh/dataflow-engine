# DataFlow Engine — Engineering Documentation

> Status: **Proposed / Design baseline** · Version: **0.1.0** · Date: 2026-10-16
> Goal: replace required SQL Server/Epicor → ClickHouse Airbyte workloads with a Go streaming engine.


## Implementation status (2026-10-16)

**Code has started; this is NOT an Airbyte replacement yet.** The Go bootstrap adds:
- `cmd/dataflow/main.go`: authenticated experimental HTTP full-load endpoint and run logging.
- `internal/engine/`: bounded-channel worker pool with tests, cancellation and backpressure.
- `internal/connectors/sqlserver/`: read-only streaming `SELECT` reader.
- `internal/connectors/clickhouse/`: JSONEachRow HTTP batch writer.
- `migrations/postgres/001_bootstrap.sql` and `docker-compose.yml`: PostgreSQL run state bootstrap.

Read [Bootstrap usage and known limitations](implementation/bootstrap-run.md) before attempting to run. No integration benchmark has been executed here. This starter is **FULL READ + APPEND**, not crash-safe full refresh or incremental. Do not run it against production ClickHouse tables. The next engineering gate is reproducible `go mod tidy && go test ./...` plus a disposable SQL Server/ClickHouse/PostgreSQL integration test.

---

## Documentation map

- [System context](architecture/01-system-context.md)
- [Ingestion runtime](architecture/02-ingestion-runtime.md)
- [Cursor correctness](architecture/03-cursor-consistency.md) **read before coding**
- [Change detection without CDC](architecture/04-change-detection.md)
- [ClickHouse destination semantics](architecture/05-destination-semantics.md)
- [Pipeline runtime](architecture/06-pipeline-runtime.md)
- [Module catalog](modules/module-catalog.md)
- [PostgreSQL DDL](contracts/postgres-schema.md)
- [API v1](contracts/api-v1.md)
- [Go connector contract](contracts/connector-interface.md)
- [Sync-job JSON Schema](contracts/job-config.schema.json)
- [Security & deployment](operations/security-deployment.md)
- [Monitoring & runbook](operations/monitoring-runbook.md)
- [Acceptance tests](testing/acceptance.md)
- [Benchmark protocol](testing/benchmark.md)
- [ADR-0001](decisions/adr-0001-baseline.md)
- [Implementation roadmap](implementation/roadmap.md)

## Scope
Go backend, PostgreSQL from day one, SQL Server read-only connector, ClickHouse destination, native `SysRevID` rowversion, composite cursor, full/incremental sync, safe restart, bounded queue/worker pool, reconciliation for physical DELETE, SQL and quality jobs, scheduler and DAG pipeline.

## Source constraints
- Epicor: strictly SELECT-only, no source DDL/triggers/CDC installation.
- Test: SQL Server `SRVERPTEST / ERPDBTEST`, schema `Erp`; ClickHouse HTTP 8124, native 9001 (environment-specific, not constants).
- `Erp.PartTran` (~6.95m historical test rows) is the initial benchmark target; recheck current counts and schema.
- [DECIDED] = working architecture, [PROPOSED] = design suggestion, [VERIFY] = needs measured validation, [RISK] = correctness boundary.
- No code or benchmark results yet. Never claim exactly-once end-to-end or complete event history from rowversion polling.

## New development slice — 2026-10-16

- Read-only SQL Server source browser: list tables and inspect columns, PKs and rowversion.
- Native 8-byte rowversion window reader with keyset paging inside SQL Server SNAPSHOT transaction. This is a **manual scratch preview**, not automatic incremental synchronization.
- Cursor hex parser + concurrent contiguous-ACK frontier, with unit tests.
- PostgreSQL stream lease/fence/checkpoint package and migration 002; **not wired into run success yet**.
- Optional scratch-only full load into ClickHouse stage table, then `EXCHANGE TABLES` in an Atomic database. SQL Server source uses SNAPSHOT mode when atomic_replace=true.
- UI Sync Jobs switches between full and rowversion trials, and Connections displays source metadata. CLI/MCP expose read-only source inspection.

**Remaining blockers:** end-to-end Go/frontend build and 3-database integration are not verified; automatic checkpoint, reliable current-state dedup, schema migration, background jobs, scheduler/DAG, full operational UI are not complete. Do not decommission Airbyte.

Follow [safe test guide](implementation/safe-test-v02.md).
