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

## v0.6 — Stability pass: leased SELECT-DAG recovery

SQL DAGs now use an opt-in PostgreSQL-backed worker queue instead of API-owned goroutines. Queue entries survive Go restarts; workers use heartbeat, fencing tokens and a finite replay cap. No automatic DML is enabled.

- [Release status v0.6](implementation/release-status-v06.md)
- [Migration and recovery test guide](implementation/sql-dag-durable-queue-v06.md)
- [Passing Go + frontend + PostgreSQL/ClickHouse CI](https://github.com/dinhvansh/dataflow-engine/actions/runs/38868112698)

**Upgrade note:** existing PostgreSQL databases must apply migration `008_dag_leases.sql` before enabling SQL Job test execution. This is still a developer preview, not a production ETL release.


## v0.7 — Restart-safe Managed Sync (dev preview)

Managed Sync now uses a PostgreSQL durable run queue, one-active-run-per-stream constraint, heartbeat and fenced worker recovery. A fresh attempt UUID isolates ClickHouse row-count verification after a partial prior attempt. The checkpoint and successful run status commit atomically to PostgreSQL. Scheduler and API only enqueue runs.

**Verified GitHub CI:** [run 38874212220](https://github.com/dinhvansh/dataflow-engine/actions/runs/38874212220) passed Go race tests/vet/build, frontend build and PostgreSQL 16 + ClickHouse 25.3 integration/failure tests.

- [v0.7 release status](implementation/release-status-v07.md)
- [v0.7 migration and test guide](implementation/managed-durable-v07.md)
- [Merged PR #5](https://github.com/dinhvansh/dataflow-engine/pull/5)

**Upgrade warning:** apply `migrations/postgres/009_managed_queue.sql` after resolving legacy active runs. Default execution flags remain OFF. No real Epicor source integration, DELETE capture, or production readiness claim.


## v0.8 — Power Query-style Visual Transform Studio (preview only)

DataFlow now has a working Transform Studio in the React console. Users can click or drag transformation steps, reorder Applied Steps, preview output after any step, switch between Visual / SQL mode, inspect ClickHouse source columns, and save/reload models in PostgreSQL.

The Go SQL compiler generates restricted SELECT queries. Preview runs with a dedicated restricted ClickHouse read-only user. **No dbt Core worker, model materialization or Mart DML is enabled.**

- [v0.8 release status](implementation/release-status-v08.md)
- [v0.8 user guide and safety](implementation/transform-studio-v08.md)
- [Passing Go + React + PostgreSQL/ClickHouse CI](https://github.com/dinhvansh/dataflow-engine/actions/runs/38887366877)

**Upgrade note:** apply `migrations/postgres/010_transform_models.sql` in existing TEST PostgreSQL. Extraction/loading still require real Epicor TEST E2E acceptance before production.


## v0.9 — Optional dbt Core worker, scratch View materialization

Transform Studio now has **Build View (dbt)** after a visual model is saved. Go API enqueues an immutable transformation in PostgreSQL and a separately started, isolated worker uses pinned dbt Core/dbt-clickhouse to create a View in disposable `scratch.dfv_*`. Execution status, logs, attempt counts and dbt artifacts appear in DataFlow UI. Queue uses lease/fencing and finite restart replay.

- [v0.9 release status](implementation/release-status-v09.md)
- [dbt worker install and safety guide](implementation/dbt-worker-v09.md)
- [Passing real dbt + PostgreSQL + ClickHouse + Docker CI](https://github.com/dinhvansh/dataflow-engine/actions/runs/38897813476)

**Test-only:** use migration `011_dbt_runs.sql` and a separate scratch ClickHouse account. No Table/Incremental publishing, production RBAC, or completed Epicor end-to-end acceptance yet.


## v1.0 — Visual Data Lineage and safe dependency gating

Click **Data Lineage** to explore a live metadata-derived source → Visual model → downstream graph with transitive impact highlighting. Transform Studio can select another model's generated scratch View as the input source.

Model saves now reject dependency cycles atomically in PostgreSQL, while dbt View Build requires successful, up-to-date upstream View runs. Dependency build ordering is visible but still **manual**; this release does not introduce automatic dbt `ref()` orchestration.

- [v1.0 release status](implementation/release-status-v10.md)
- [Lineage usage guide](implementation/lineage-v10.md)
- [Passing Go + React + PostgreSQL/ClickHouse/dbt CI](https://github.com/dinhvansh/dataflow-engine/actions/runs/38900354620)

**Developer preview only:** no production model materializations, managed E2E Epicor acceptance, or browser drag/drop automation certification.


## v1.1 — Safe Table Materialization (TEST preview)

Transform Studio can now **Build Staging Table → inspect Data Quality → Approve & Publish → Rollback**. A separate pinned dbt worker materializes a physical staging table inside ClickHouse `scratch`. ClickHouse Atomic `EXCHANGE TABLES`, PostgreSQL durable UUID preparation and fenced worker locks protect the published table from broken or repeated publish steps. Previous physical table is retained for manual rollback.

- [v1.1 release status and CI proof](implementation/release-status-v11.md)
- [Installation, DQ and safe publish guide](implementation/safe-table-v11.md)
- [Passing dbt + ClickHouse + PostgreSQL tests](https://github.com/dinhvansh/dataflow-engine/actions/runs/38905418989)

**Warning:** apply migration `012_safe_table.sql` and opt in only on isolated TEST `scratch`. This is **full Table rebuild**, not Incremental and not production-ready.


## v1.2 — Auto Build dependencies, one-click View-only flow

Select final model C in **Data Lineage** and click **Run Model + Dependencies**. DataFlow snapshots the upstream A → B → C Visual model chain to PostgreSQL, queues each model into the existing durable dbt View worker, and only advances after a successful dbt View build **and read-only output row-count DQ**. The UI shows every step's status and verified row count. Parent state survives API restarts, and ancestor edits mid-flow cause a safe stop.

- [v1.2 release/CI status](implementation/release-status-v12.md)
- [Auto Build guide and limitations](implementation/auto-build-v12.md)
- [Passing real dbt + ClickHouse + PostgreSQL CI](https://github.com/dinhvansh/dataflow-engine/actions/runs/38911890898)

**Test-only:** migration `013_auto_build_flows.sql`; set `DATAFLOW_ENABLE_AUTO_BUILD_TEST=true` only in isolated scratch TEST. Individual Views are built one at a time; there is no global atomic multi-model publish, nor Table/Incremental auto-build.


## v1.3 — Safe Incremental CDC (copy-on-write TEST preview)

DataFlow now supports a standalone **Incremental CDC** UI: register a scratch target, its append-only delta stream and key, build a staging table from versioned insert/update/delete events, inspect DQ, then explicitly Publish or Rollback **with the cursor**. The existing UUID-fenced ClickHouse Atomic publish/rollback logic protects the published table and PostgreSQL checkpoint from incomplete retries.

- [v1.3 release status and passing CI](implementation/release-status-v13.md)
- [CDC event contract and test operations](implementation/safe-incremental-v13.md)
- [Go/Web/PostgreSQL/ClickHouse tests](https://github.com/dinhvansh/dataflow-engine/actions/runs/38916767766)

**This is logical incremental + full physical staging copy, not yet a fast high-volume Incremental engine.** No Epicor CDC/deletes pipeline is implemented, no 7m-row performance guarantee, and no production enablement. Apply migration 014 and `DATAFLOW_ENABLE_INCREMENTAL_TEST=true` only to an isolated scratch TEST instance.

## Local acceptance by Codex (v1.3)

Start at **[CODEX_LOCAL_QA.md](CODEX_LOCAL_QA.md)** to have Codex running on your own computer self-provision an isolated QA Docker stack, generate Epicor-like synthetic data and ground truth, execute the **[68-case test catalog](qa/local-acceptance/TEST_CASE_CATALOG.md)**, run local E2E and benchmarks, and produce a sanitized report for ChatGPT review. The complete [QA mission](qa/local-acceptance/CODEX_LOCAL_QA_MISSION.md) and [result template](qa/local-acceptance/REPORT_TEMPLATE.md) are tracked in Git.

**This is a test plan, not a completed local QA run.** Never mutate real Epicor, shared PostgreSQL/ClickHouse or unrelated Docker resources.
