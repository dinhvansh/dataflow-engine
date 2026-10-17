# Connection Manager V3.2B-2 — scoped PostgreSQL metadata (synthetic only)

**Status:** Security infrastructure increment; **NOT** a user-facing multi-connection manager. Real credentials and endpoint configuration remain blocked. Epicor is SELECT-only.

## Implemented
- Idempotent migration `018_connection_security_metadata.sql`, intended for **identity-verified disposable PostgreSQL QA**. Separate V3.2A lab drafts remain untouched. Security-workspace table, scoped four-permission grants with revoked marker, new disconnected profile Drafts, server-only opaque secret references with CAS-style revision/version and append-only audit.
- `internal/connections/catalog.go` restricted server-side `SecureCatalog` with fresh DB scope check for each action. `MANAGE` creates Draft/associates versioned vault pointer. `VIEW` lists only exact workspace, max 100; `USE` and `TEST` cannot list or mutate by default.
- Read/modify and audit written together in one PostgreSQL transaction. Authorization grant locked `FOR SHARE` while processing; revocation is not bypassed by a previously cached memory policy. Stale revisions and cross-workspace IDs fail closed.
- HTTP/JSON only returns public metadata + boolean `secret_configured`, never `secret_handle`, version, endpoint, password, URI or DSN.
- Dedicated synthetic PostgreSQL integration test, migration idempotency, role matrix, cross-workspace access denial, independent scopes, revoked VIEW, revision CAS, immutable audit UPDATE/DELETE/TRUNCATE. Full CI includes it.

## What this does NOT provide yet
- Actor identity: `actor` parameters must ONLY be created by a future trusted backend verifier, not accepted from HTTP headers/browser. There is no external API wire-up in this PR.
- Vault: reference is **staged metadata only**, not a guarantee a secret exists. No external provider configured or queried, and no credentials transmitted/stored. Only an approved server-only vault adapter may stage references later.
- This PR does not create a production-grant bootstrap or operational revoke/admin UI. Tests provision actors/workspaces with synthetic SQL in a **disposable** PG fixture only.
- PostgreSQL append-only trigger protects application writes under normal permissions; DB owner/superuser may bypass it. Database owner, logging, physical backup and privileged access must be governed by deployment controls. There is no WORM certification.
- CAS in PostgreSQL metadata is **not** atomic with external vault rotation. A recoverable dual-write protocol is required before exposing rotation. Draft profiles never become executable.
- No Test Connection, DNS/SSRF egress, endpoint fields, SQL Server writes, ClickHouse production changes, pipeline binding or mixed ETL runtime.

## Next security gates
1. Select OIDC / verified internal identity resolver, trust boundary, immutable actor subject and workspace provisioning/role issuance. Guard ALL connection routes with these principals; keep shared bearer/admin lab-only until replaced.
2. Choose first real Vault adapter and trust/egress policy. Add separate provider-backed write/rotation with failure reconciliation and an explicit durable audit state.
3. Configure secure metadata schema in deployment, deny cross-workspace access in synthetic app+DB tests, inspect browser responses for redaction.
4. Only then V3.2C Test Connection with bounded TCP/TLS egress, source SELECT-only role, ClickHouse scratch-only role.

## QA
- `go test -race ./...` exercises all pure Go tests.
- GitHub CI invokes `go test ./internal/connections -run TestConnectionCatalogIntegration -count=1 -v` only with a fresh isolated PostgreSQL container. The QA intentionally retains generated audit fixture rows in this disposable DB (no destructive cleanup).
- CI success is NOT a certificate of verified real Epicor access, security hardening or production deployment.
