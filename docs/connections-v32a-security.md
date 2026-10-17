# Connection Manager V3.2A — draft registry security and rollout

V3.1 was **read-only environment-slot inventory**; it did not support more than one real SQL Server or ClickHouse. V3.2A adds **inert metadata drafts only**, not live connections.

## Data model and default behavior
- Migration: migrations/postgres/017_connection_definitions.sql, idempotent. Apply **only to a verified isolated control-plane PostgreSQL fixture** before enabling the feature.
- Fields: generated UUID, validated display name (max 80), driver (`sqlserver` source or `clickhouse` destination), fixed lab workspace, immutable `draft` state and revision 1, UTC creation time. Audit table tracks only `created` records.
- No endpoint/network address, credentials, secret path, token, free-form JSON, admin/corporate metadata, execution status or active binding in the table.
- No UI/REST endpoint can activate or run these drafts. Existing Sync Jobs and SQL DAGs continue to use existing fixed backend/environment credentials, never draft IDs.
- No migration backfill from production or ERP source. This PR must not change any existing DSN or runtime.

## Request authorization
- GET /api/v1/connections/definitions requires existing API bearer token. When feature disabled, returns empty list + write_enabled=false.
- POST requires existing bearer **and distinct admin header** `X-DataFlow-Connection-Admin` matching the configured server-side `DATAFLOW_CONNECTION_ADMIN_TOKEN` (constant-time compare), plus opt-in `DATAFLOW_ENABLE_CONNECTION_REGISTRY=true`. Feature off -> 403.
- Admin token must be independent and randomly generated, and supplied only over HTTPS / localhost through trusted paths; held in React component memory, never committed or persisted in browser storage.
- The current shared bearer + separate admin header is **not full per-user RBAC**, and does not provide tenant isolation. Restrict deployments to isolated lab/internal trusted users; production multi-tenant registration remains blocked.
- Fail closed on missing migration when enabled; PostgreSQL must have applied migration 017. Response is no-store. Server errors omit detailed SQL exceptions, credentials and DSNs.
- Body has 2 KiB cap, rejects unknown fields and extra JSON, limits driver/role combinations; unique names produce HTTP 409. Transactionally inserts draft and audit.
- Lists are capped to latest 100 rows. Unlimited creation and revisions are not yet a production quota solution.

## QA and gates
- Unit: invalid names, forbidden connectors, wrong role, unauthorized GET/POST, disabled feature, absent PostgreSQL, response headers.
- Integration: only when `DATAFLOW_INTEGRATION_POSTGRES_DSN` and `DATAFLOW_INTEGRATION_PG_ISOLATED=true` point to synthetic fixture; migration applied twice; create/list/audit; duplicate -> 409, unknown "host" -> 400, missing admin -> 403, cleanup test-owned records.
- Browser mocked UI: admin token memory, no create until feature enabled + correct admin permission; draft persistence and no Run/Sync request.
- Github CI success does not certify real ERP, network connectivity or production secret handling.

## Before enabling real multi-instance connections
Server-side vault + rotation, per-workspace RBAC, audited scopes, SSRF/egress policy, DNS resolution revalidation and source SELECT-only authorization; then versioned binding and synthetic runtime tests. No real source write access.
