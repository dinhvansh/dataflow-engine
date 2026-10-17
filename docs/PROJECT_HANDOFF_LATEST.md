# DataFlow Engine — Current handoff (2026-10-17)

> **Live GitHub state always wins.** This document is a snapshot. Before editing, read main HEAD, open PRs, branch divergence, Actions jobs and ongoing QA. Do not assume a pending PR has merged.

## Project and user expectations
- Repo: https://github.com/dinhvansh/dataflow-engine (Public).
- Go API, PostgreSQL control plane, SQL Server source (strict ERP SELECT-only), ClickHouse scratch target, isolated dbt worker. React/TypeScript/Vite/Tailwind/shadcn + React Flow/xyflow.
- User wants direct commits, PRs, bug fixes and merges, with independent CI verification rather than repeated Codex prompts. Vietnamese, informal collaborative style.
- Critical: **never INSERT/UPDATE/DELETE/DDL against real ERP**, never reset non-QA services, never leak DSNs/secrets or place them in browser storage. All real DB integration QA uses isolated synthetic infrastructure.
- Design: Signalroom Enterprise. Light white/neutral + brand #238047; Dark graphite + volt #C0F273. Status colors distinct. No Ant Design replacement.
- Preferred release method: one isolated branch/PR at a time; full GitHub-hosted Actions CI and Playwright mocked UI regression on exact head; after merge recheck main CI. Public repo Actions is now operational.
- Do not equate mocked browser PASS with live ERP acceptance or production readiness.

## Delivered milestones (verified at this snapshot)
- PR #41 CI workflow dedupe — merged. Public repo actions runner dispatch restored.
- PR #46 Visual Pipeline Builder V2.1 — React Flow edit canvas + JSON round trip.
- PR #47 V2.2 — Validate, Save/reopen DAG, explicit overwrite and TEST enablement.
- PR #48 V2.3 — Run and Inspector (backend status, attempts, errors).
- PR #49 V2.4 — Run directly on saved validated TEST canvas, server re-read and confirmation. All through existing backend, no ERP write.
- PR #51 Connection Manager V3.1 — authenticated GET-only redacted inventory of SQL Server source, ClickHouse scratch target and PostgreSQL control plane. #50 was a duplicate draft closed in favor of #51.
- At start of V3.2A, main: 334e02887c33431883e8aefc233a52e7cbf48487. Go + Web + browser CI PASSED on this SHA.
- Other non-feature work: PR #25 screenshot gallery remained open at snapshot; inspect current state before changing.

## Current milestone: V3.2A (feature branch; check PR status live)
- Branch: feat/connections-v32a-draft-registry-20261017
- Deliberate first slice of multi-instance Connection Manager: PostgreSQL registry of multiple **inactive lab-only named draft definitions**, limited to SQL Server sources or ClickHouse destinations.
- No host, IP, URL, port, DSN, credential, secret_ref, active connection, Test Connection, job binding or execution is accepted or stored.
- New migration 017 introduces connection_definitions and append-only created audit event.
- New authenticated GET/POST /api/v1/connections/definitions; POST requires separately configured DATAFLOW_CONNECTION_ADMIN_TOKEN plus DATAFLOW_ENABLE_CONNECTION_REGISTRY=true. Feature is OFF by default.
- UI draft form only; no creation of a real SQLServer/ClickHouse network connection. Synthetic Go/PG/browser tests must PASS on head before merge.
- **Do not call V3.2A the complete multi-instance connector feature.** It lays down safe metadata and authorization gates for later runtime binding.

## Proposed follow-up milestones
1. V3.2B: vault-backed credentials and admin RBAC/workspaces with least privilege. Avoid plaintext PostgreSQL or browser storage.
2. V3.2C: bounded Test Connection with SSRF protection (egress allowlist, DNS revalidation, timeouts, audit), SQL Server read-only and scratch ClickHouse test roles.
3. V3.3: versioned, immutable connection IDs bound to Sync Jobs/Pipelines with audit and explicit migration from env connections; careful rollback.
4. V3.4: synthetic integration QA, fault injection, local E2E and performance testing; separate real ERP SELECT-only approval.

## How to resume in a new chat

Send: **“Tiếp tục DataFlow Engine từ docs/PROJECT_HANDOFF_LATEST.md trên https://github.com/dinhvansh/dataflow-engine. Tự kiểm tra GitHub main, PR, Actions và triển tiếp theo roadmap; giữ ERP read-only.”**

This file contains no secrets. Refresh it following major merges to prevent stale snapshots.
