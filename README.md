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

## v0.3 Managed Incremental Preview

The repo now includes **opt-in asynchronous rowversion incremental runs** with PostgreSQL fencing/checkpoint, ClickHouse raw event logical dedup and row-count verification, plus an **opt-in interval scheduler** and corresponding UI/CLI/MCP surfaces. These features are **not integration-tested on Epicor** and are disabled by default.

- [Status and release gates](implementation/release-status-v03.md)
- [Managed stream setup and test plan](implementation/managed-incremental-v03.md)
- [ClickHouse raw-event schema](deploy/clickhouse/001_raw_events.sql)
- [One-command API smoke test](scripts/smoke-managed.sh)

## v0.4 — Source-key Snapshot Reconciliation (detection-only)

A verified, read-only SQL Server PK snapshot can now be compared against the last published complete snapshot to detect added, missing and rowversion-changed keys. Results are shown in React UI, CLI and MCP. This **does not apply DELETE tombstones** and is guarded by an explicit development flag.

**Actual CI passed:** Go race tests, vet, Go command build, React build and ClickHouse 25.3 integration tests for a +1/-1/~1 snapshot difference.

- [v0.4 Release Status](implementation/release-status-v04.md)
- [Snapshot Reconciliation Test Guide](implementation/reconciliation-v04.md)
- [Passing GitHub CI](https://github.com/dinhvansh/dataflow-engine/actions/runs/38864021334)

Real Epicor SQL Server + PostgreSQL + ClickHouse end-to-end reconciliation has not yet been exercised; see the release status for gates.

## v0.5 — Verified SQL Job + Executable DAG

DataFlow now offers an experimental SELECT-only ClickHouse SQL Job registry, an asynchronous executable DAG engine with ALL dependencies, retries and per-node status, plus UI, CLI and MCP controls. Job SQL/plan snapshots are immutable per run. Runs are **test-only** and require explicit env flags plus a separately permissioned ClickHouse read-only user.

- [v0.5 release status](implementation/release-status-v05.md)
- [SQL Job and DAG setup](implementation/sql-dag-v05.md)
- [Passing real PostgreSQL + ClickHouse CI](https://github.com/dinhvansh/dataflow-engine/actions/runs/38866264338)

**Not yet present:** SQL transformation DML jobs, restart-safe queue, production auth/RBAC and validated live Epicor synchronization at scale.
