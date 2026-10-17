# DataFlow Connection Manager — V3.1 security contract

**Status:** implemented read-only environment inventory. This is deliberately **not** a
multi-instance connection registry and does not accept arbitrary network addresses.

## V3.1: server-owned inventory

`GET /api/v1/connections` requires the same API bearer token as Source Explorer.
It returns a fixed, non-secret inventory of configured *slots*: SQL Server source,
ClickHouse scratch destination, and PostgreSQL internal control plane.

Each slot contains only a stable, non-sensitive ID, display name, kind, driver,
intended usage, a `configured | not_configured` flag, management mode, and
feature-capability booleans (currently both false).

`configured` **does not mean reachable or permission-tested**. A GET never opens
a SQL connection, dials an endpoint, writes to PostgreSQL, or validates credentials.
No hostnames, ports, DSNs, usernames, passwords, user-supplied URLs, or raw error
messages are returned. Responses use `Cache-Control: no-store`.

This is an observation endpoint. It never changes the currently active fixed
SQL Server/ClickHouse/PG connections. The existing safety boundaries remain:
ERP source READ ONLY; TEST writes only to disposable ClickHouse scratch targets.

## Gate before independent instances (V3.2+)

- Design a `connection_definitions` registry with tenant/workspace ownership,
  unique IDs, soft deletion, audited revisions, and clear source/target roles.
- Store credentials only in a server-side secret manager with per-connection
  references and rotation; never in plaintext PostgreSQL, browser storage,
  logs, API GET payloads, or Git.
- Add RBAC (view/use/manage/test), API-token scoping and per-workspace isolation.
  The current single shared bearer token is **not** an adequate multi-tenant RBAC model.
- Restrict egress / SSRF via destination allowlists, DNS re-resolution protection
  and network policy before implementing Test Connection or editable endpoints.
- Require a read-only SQL Server role and enforce no DML/DDL at the connector
  boundary; keep ClickHouse write access isolated to disposable test targets.
- Implement separate `POST /connections/:id/test` with a bounded timeout,
  non-secret status codes and audit trail. No backend user-controlled connection
  endpoint should be accepted before this gate.
- Bind jobs/pipelines to immutable connection IDs/versions; migration of existing
  environment bindings must be explicit, backwards-compatible and recoverable.
- Add end-to-end QA with **synthetic databases only**, rollback, and high-risk
  security regression tests before enabling save/edit/test.

V3.1's green GitHub CI and mocked browser QA are **not** acceptance of
production or real ERP credentials.
