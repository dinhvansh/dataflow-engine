<div align="center">

![DataFlow Engine banner](docs/assets/dataflow-hero.svg)

# DataFlow Engine

**A visual, developer-focused data integration workbench**  
**Ingest → Transform → Orchestrate → Validate → Publish**

*Go-powered data movement · ClickHouse analytics · dbt transformations · PostgreSQL-backed reliability*

[![CI](https://github.com/dinhvansh/dataflow-engine/actions/workflows/ci.yml/badge.svg)](https://github.com/dinhvansh/dataflow-engine/actions/workflows/ci.yml)
![Stage](https://img.shields.io/badge/stage-developer%20preview-0e7490)
![Release](https://img.shields.io/badge/milestone-v1.3-164e63)
![Source](https://img.shields.io/badge/Epicor%20%2F%20SQL%20Server-SELECT--only-2563eb)

[**Explore the architecture**](#architecture) ·
[**See features**](#what-is-implemented) ·
[**Run local QA**](#run-the-local-qa-suite) ·
[**Get started**](#get-started) ·
[**Roadmap**](#roadmap)

</div>

> [!IMPORTANT]
> **Developer preview · Not production-ready.** Automated GitHub CI has verified Go, React, PostgreSQL, ClickHouse and dbt integration paths. **Real Epicor SQL Server end-to-end acceptance, Windows/local QA and large-table benchmarks are still pending.** Most write/materialization features are opt-in and restricted to disposable `scratch` environments.

---

## Why DataFlow?

Business data pipelines are often spread between an ingestion tool, SQL scripts, transformation jobs and separate reporting tools. DataFlow brings the **development workflow and operational state** into one place while keeping its execution responsibilities explicit.

| Discover & ingest | Transform visually | Run with dependencies |
|:---|:---|:---|
| Inspect SQL Server metadata and experiment with bounded, read-only extraction into ClickHouse. | Build a sequence of visual transformations, preview each step and inspect the generated SQL. | See which model depends on which source; select the final model and run an upstream-first View chain. |

| Publish carefully | Recover from failures | Investigate problems |
|:---|:---|:---|
| Stage a physical ClickHouse Table, verify it, then explicitly approve publication and rollback. | PostgreSQL-backed queues, leases, fencing and run snapshots reduce restart and retry hazards. | Follow run history, model-level statuses, error messages, lineage and affected descendants. |

**In Vietnamese:** DataFlow hướng tới trải nghiệm *đồng bộ dữ liệu → kéo-thả xử lý như Power Query → dùng dbt build → theo dõi pipeline → kiểm tra trước khi publish*, trên một giao diện thống nhất. Mục tiêu là dễ kiểm soát và dễ kiểm chứng, **không** phải tuyên bố đã thay thế hoàn toàn Airbyte/dbt/n8n.

## Architecture

```mermaid
flowchart LR
    subgraph SOURCES["DATA SOURCES"]
        ERP[("SQL Server / Epicor<br/>SELECT only")]
    end
    subgraph ENGINE["DATAFLOW ENGINE · Go"]
        DISC["Source Explorer"]
        ING["Bounded ingestion<br/>workers + backpressure"]
        API["REST API / Coordinator"]
    end
    subgraph DATA["DATA & EXECUTION"]
        CH[("ClickHouse<br/>scratch / raw")]
        PG[("PostgreSQL<br/>jobs · models · cursors")]
        DBT["Isolated dbt Core<br/>View / Table worker"]
    end
    subgraph EXPERIENCE["WORKSPACE"]
        UI["React console<br/>Transform Studio · Lineage"]
        REPORT["Downstream queries<br/>and reporting tools"]
    end
    ERP -->|Read-only SELECT| DISC
    ERP -->|Read-only batches| ING
    ING -->|TEST target only| CH
    DISC --> API
    UI <--> API
    API <-->|Checkpoints / leases| PG
    API -->|Queue dbt runs| PG
    PG <-->|Claim and commit status| DBT
    DBT -->|Scratch View / staged Table| CH
    API -->|SELECT preview / DQ| CH
    CH --> REPORT
    classDef source fill:#172b4d,stroke:#60a5fa,color:#eaf6ff
    classDef engine fill:#13304a,stroke:#38bdf8,color:#eaf6ff
    classDef data fill:#143a37,stroke:#2dd4bf,color:#eafffa
    classDef ui fill:#273049,stroke:#a5b4fc,color:#f1f5ff
    class ERP source
    class DISC,ING,API engine
    class CH,PG,DBT data
    class UI,REPORT ui
```

**Separation of responsibilities:** Go handles API, ingestion and durable scheduling; PostgreSQL persists state; ClickHouse stores/query data; an isolated **dbt Core** process materializes saved visual SELECT models; React presents operations and diagnostics.

The SQL Server connection remains **strictly read-only**. DataFlow must never perform DML/DDL on Epicor.

## From a raw source to a built model

```mermaid
flowchart TD
    S["1. Inspect a source"] --> I["2. Read & load into scratch"]
    I --> A["3. Visual Model A · Clean"]
    A --> B["4. Model B · Enrich"]
    B --> C["5. Model C · Aggregate"]
    C --> RUN["Select Model C → Run dependencies"]
    RUN --> CHECKA{"A: dbt + DQ pass?"}
    CHECKA -->|Yes| CHECKB{"B: dbt + DQ pass?"}
    CHECKB -->|Yes| CHECKC{"C: dbt + DQ pass?"}
    CHECKC -->|Yes| OK["Flow succeeded · View result"]
    CHECKA -->|No| STOP["Stop; show failure and logs"]
    CHECKB -->|No| STOP
    CHECKC -->|No| STOP
    classDef complete fill:#065f46,stroke:#34d399,color:#f0fdfa
    classDef stop fill:#7f1d1d,stroke:#f87171,color:#fff1f2
    class OK complete
    class STOP stop
```

**What this flow means today:** v1.2 runs **single-source Visual models materialized as ClickHouse Views** in dependency order, one at a time. It validates nonempty output and a TEST row cap **after** each View build. It does **not** atomically hide the Views from other readers before DQ, roll back the whole chain, or automatically publish Tables.

## What is implemented?

| Module | Capabilities available in code | Boundary |
|:---|:---|:---|
| **Source Explorer** | SQL Server table/column/PK/rowversion discovery | Real Epicor TEST acceptance pending |
| **Ingestion / Managed Sync** | Full / rowversion experiments, bounded Go workers, durable queue, source-to-scratch writing | TEST-only; physical DELETE capture is not complete |
| **Reconciliation** | Compare verified key snapshots for added / removed / changed keys | Detection-only, not automatic DELETE repair |
| **SQL Jobs & DAG** | SELECT-only jobs, dependencies, persisted runs, retry/fencing | No unrestricted write-SQL execution |
| **Transform Studio** | Applied Steps, reorder/drag, preview, visual-to-SQL compilation, saved models | Visual models have limited operators and single-source scope |
| **Data Lineage** | Source-to-model dependency map, cycle checks, downstream impact analysis | Metadata-based, not full SQL/column lineage |
| **Auto Build** | Select an output model; coordinate upstream-first dbt View builds + DQ | View-only; no global atomic release |
| **Safe Table** | Build staging Table → DQ → manual publish → atomic exchange → rollback | Disposable scratch, max 1M rows per TEST output |
| **Incremental CDC** | Versioned insert/update/delete events; staged copy-on-write publish + cursor rollback | Needs prepared append-only delta; full physical copy; max 50k events per TEST batch |
| **Operations UI** | Run history, job/model status, errors, linked dbt runs | Not a production-grade alerting/RBAC console |

### The console today

The current React workspace includes **Overview**, **Sync Jobs**, **Connections**, **Pipelines**, **SQL Jobs**, **Transform Studio**, **Data Lineage**, **Incremental CDC** and **Run History**.

> [!NOTE]
> These are actual code-level modules, not claimed production screenshots. Captured and annotated UI screenshots will be added **after local browser E2E acceptance**, so the project page does not present mockups as tested software.

## Safe materialization

A report-facing Table should not be emptied halfway through a refresh. DataFlow's **v1.1 Table** and **v1.3 Incremental** previews therefore use private staging plus explicit approval:

```mermaid
flowchart LR
    SQL["Compiled SELECT<br/>or validated CDC window"] --> STAGE[("Private staging Table")]
    STAGE --> DQ{"Quality gate"}
    DQ -->|Fail| KEEP["Keep existing target"]
    DQ -->|Pass| REVIEW["Manual approval"]
    REVIEW -->|Publish| SWAP["ClickHouse Atomic<br/>EXCHANGE TABLES"]
    SWAP --> TARGET[("Published scratch target")]
    SWAP --> BACKUP[("Previous version<br/>kept for rollback")]
    BACKUP -.->|Approved rollback| TARGET
    classDef secure fill:#0f3d37,stroke:#2dd4bf,color:#edfffa
    classDef reject fill:#522329,stroke:#fb7185,color:#fff1f2
    class STAGE,SWAP,TARGET,BACKUP secure
    class KEEP reject
```

**Table:** full rebuild of a staged physical Table, with an optional unique-key check, confirmation and UUID-protected rollback.

**Incremental:** logically apply the most recent version of each changed key, including tombstones, **but copy unchanged rows into a new physical stage** before swapping. That prioritizes correctness over I/O efficiency. It does not automatically produce CDC events from Epicor or detect SQL Server physical deletions.

### Technology stack

| Component | Technology | Purpose |
|---|---|---|
| Core services | **Go 1.23+** | Streaming reader, bounded workers, APIs, coordinator |
| Source | **Microsoft SQL Server** | Read-only ERP / synthetic fixture |
| Analytical store | **ClickHouse** (25.3 in CI) | Raw data, preview, View and Table materialization |
| State | **PostgreSQL 16** | Model registry, runs, checkpoints, leases |
| Transform worker | **dbt Core 1.11.0 + dbt-clickhouse 1.10.3** | Isolated SQL model execution |
| Frontend | **React + TypeScript + Vite** | Enterprise-style data operations console |
| Development environment | **Docker Compose** | PostgreSQL and opt-in dbt worker; isolated QA stack is a separate mission |

## Get started

### Recommended: run the local QA suite first

If you're evaluating DataFlow for the first time, start with the full, reproducible local acceptance mission. It instructs a local coding agent to **create an isolated synthetic SQL Server + PostgreSQL + ClickHouse stack**, generate ground-truth fixtures and test the full application rather than touching an existing ERP.

**[→ Open the Codex Local QA mission](CODEX_LOCAL_QA.md)** · [68 acceptance cases](qa/local-acceptance/TEST_CASE_CATALOG.md) · [Report template](qa/local-acceptance/REPORT_TEMPLATE.md)

### Manual developer startup

**Prerequisites:** Go 1.23+, Node.js 22, Docker with Compose, a reachable **TEST SQL Server**, a disposable ClickHouse `scratch` database, and configured PostgreSQL. The root Compose file starts **PostgreSQL only**; it does **not** provision SQL Server, ClickHouse, Go API or Web UI.

```bash
git clone https://github.com/dinhvansh/dataflow-engine.git
cd dataflow-engine
cp .env.example .env
# Populate .env with ONLY isolated TEST endpoints and credentials.
docker compose --env-file .env up -d postgres
```

For **Windows PowerShell**, use `Copy-Item .env.example .env` instead of `cp`.

**Before launching the API**, export the environment settings from your local `.env` into the Go process, including `DATAFLOW_POSTGRES_DSN`, `DATAFLOW_SQLSERVER_DSN`, `DATAFLOW_CH_URL` and `DATAFLOW_API_TOKEN`. Startup requires both PostgreSQL and SQL Server to be reachable. **Do not commit `.env` or real passwords.**

```bash
# Terminal 1 — from repository root, after env vars are loaded
go run ./cmd/dataflow

# Terminal 2
cd web
npm ci
npm run dev
```

- Web UI: `http://localhost:5173`
- API health: `http://localhost:8080/healthz`
- PostgreSQL Compose port: `127.0.0.1:5433`

**Existing PostgreSQL volumes:** init SQL scripts do **not** rerun on a populated volume. Back up TEST state and apply missing migrations in order before turning on the corresponding feature flags.

**dbt worker:** run it separately **only after** providing limited scratch-only ClickHouse credentials, PostgreSQL migration 011+ and appropriate opt-in flags. See the [dbt worker guide](implementation/dbt-worker-v09.md) and [safe Table guide](implementation/safe-table-v11.md); the Compose overlay is `docker-compose.dbt.yml`.

### Default-deny test flags

The following features start **off** and must be explicitly enabled only in a disposable environment:

```dotenv
DATAFLOW_ALLOW_TEST_LOAD=false
DATAFLOW_ENABLE_MANAGED_TEST=false
DATAFLOW_ENABLE_SQL_JOBS_TEST=false
DATAFLOW_ENABLE_DBT_TEST=false
DATAFLOW_ENABLE_SAFE_TABLE_TEST=false
DATAFLOW_ENABLE_AUTO_BUILD_TEST=false
DATAFLOW_ENABLE_INCREMENTAL_TEST=false
DATAFLOW_TEST_TARGET_DATABASE=scratch
```

Read [.env.example](.env.example) and the individual release instructions before enabling any flag. Use **different read-only and scratch writer identities**. Never point any write-enabled path at ERP or production ClickHouse.

## Quality and verification

| Validation layer | Status | Evidence |
|---|---|---|
| Go unit / race / vet / build | **Passed on GitHub CI** | [CI workflow](.github/workflows/ci.yml) |
| React TypeScript production build | **Passed on GitHub CI** | [CI workflow](.github/workflows/ci.yml) |
| PostgreSQL / ClickHouse / dbt integration | **Passed on disposable CI fixtures** | [v1.3 release evidence](implementation/release-status-v13.md) |
| Synthetic SQL Server full local E2E | **Pending local Codex QA** | [Local QA mission](CODEX_LOCAL_QA.md) |
| Real Epicor `SRVERPTEST / ERPDBTEST` E2E | **Not yet accepted** | [Acceptance checklist](testing/acceptance.md) |
| 7M-row throughput vs Airbyte | **Not yet benchmarked** | [Benchmark protocol](testing/benchmark.md) |
| Production availability / security | **Not certified** | [Security and deployment](operations/security-deployment.md) |

A PASS from GitHub CI is **not** a PASS from the user's Windows computer. Local Codex QA must report actual test outcomes, logs, row-level mismatches, performance measurements and unresolved bugs; **BLOCKED** is not equivalent to **PASS**.

## Project milestones

| Release | Delivered capability |
|---|---|
| v0.7 | Restart-safe Managed Sync |
| v0.8 | Visual Transform Studio |
| v0.9 | Isolated dbt Core View worker |
| v1.0 | Data Lineage + impact analysis |
| v1.1 | Safe staged Table publication & rollback |
| v1.2 | One-click Auto Build View dependencies |
| **v1.3** | **Versioned CDC-style Incremental staging & cursor rollback** |

**Next acceptance gate:** local synthetic E2E + bug fixing → source-consistent Epicor TEST E2E → correctness and performance benchmark. Future development candidates include business-grade Data Quality contracts, observability, efficient partition-level Incremental, RBAC and release-level publishing.

See the complete [implementation roadmap](implementation/roadmap.md) and [v1.3 release notes](implementation/release-status-v13.md).

## Documentation

| Area | Resources |
|---|---|
| Architecture | [System context](architecture/01-system-context.md) · [Ingestion runtime](architecture/02-ingestion-runtime.md) · [Cursor consistency](architecture/03-cursor-consistency.md) |
| Developer contracts | [API](contracts/api-v1.md) · [Go connectors](contracts/connector-interface.md) · [PostgreSQL schema](contracts/postgres-schema.md) |
| Transform / dbt | [Transform Studio](implementation/transform-studio-v08.md) · [dbt worker](implementation/dbt-worker-v09.md) · [Lineage](implementation/lineage-v10.md) |
| Releases | [Safe Table](implementation/safe-table-v11.md) · [Auto Build](implementation/auto-build-v12.md) · [Safe Incremental](implementation/safe-incremental-v13.md) |
| QA / Operations | [Codex local acceptance](CODEX_LOCAL_QA.md) · [Benchmark rules](testing/benchmark.md) · [Security](operations/security-deployment.md) |
| Historical notes | [Original engineering README (v0.1–v1.3)](implementation/README-engineering-archive.md) |

## Safety and project status

- Epicor is a **SELECT-only source**, including `SRVERPTEST` / `ERPDBTEST`; no schema changes, triggers or direct writeback are permitted.
- Destructive fault tests belong exclusively to **QA-owned disposable containers, databases and volumes**.
- Complete source-to-target *exactly-once*, native physical DELETE CDC, multi-model atomic publish, production RBAC and enterprise-scale performance remain **unverified or out of scope**.
- No `LICENSE` file is currently present. Do not assume the repository is licensed for unrestricted redistribution until a license is chosen.

---

<div align="center">

**Build pipelines that are observable, testable and safe to change.**

*Developed in milestones. Verified by evidence. Not by marketing claims.*

[Top ↑](#dataflow-engine) · [Local QA](CODEX_LOCAL_QA.md) · [Roadmap](implementation/roadmap.md)

</div>
