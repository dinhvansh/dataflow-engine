# ADR-0001 — Baseline decisions

Status: proposed · 2026-10-16.

- Go as the engine; Rust only if profiling identifies costly parsing/transform.
- PostgreSQL as state from day one, ClickHouse as destination.
- Strictly SELECT-only Epicor SQL Server integration.
- Native SysRevID rowversion and composite cursor with typed SysRowID.
- Streaming, bounded queue and worker pool configurable by job.
- Rowversion polling plus snapshot reconciliation without CDC.
- SQL/quality jobs, DAG pipelines, scheduler and optional n8n.
- Replace existing relevant Airbyte workloads only after tested correctness.

Open: frontend technology, ClickHouse current-state view strategy, source snapshot consistency, secrets, RBAC and scaling. No claim of full event history or exactly-once delivery.
