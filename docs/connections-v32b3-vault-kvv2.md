# V3.2B-3 — Optional HashiCorp Vault KV-v2 adapter, OFFLINE integration only

**Developer Preview; explicit self-hosted candidate, not a production deployment decision.**

This PR supplies a real server-side provider implementation of the V3.2B `Vault` interface. It is **not wired to HTTP, UI, PostgreSQL, SQL Server, ClickHouse or the DataFlow job runtime**.

## Contract and network boundaries
- `NewKVv2Vault(endpoint, mount, token, trustedHTTPClient)` is a trusted server-only constructor. **HTTPS only**, fixed operator-supplied Vault base address and mount; no user-controlled endpoint, redirects disabled, request timeouts bounded.
- Rejects embedded URL credentials/paths/query, invalid mounts, short/header-injectable tokens and explicit `InsecureSkipVerify` on standard transport. A production deployment must add egress/DNS policy and a trusted CA/certificate chain, and must not use a custom insecure transport.
- HashiCorp Vault **KV version 2** semantics: `POST /v1/{mount}/data/workspaces/{workspace}/{opaqueHandleSuffix}` with CAS=0 on Create or CAS=priorVersion on Rotate; `GET` with pinned secret version. Version mismatch and HTTP errors are sanitized.
- Credential bytes are base64-encoded for the Vault KV JSON API payload (transport is TLS). Base64 is **not** itself encryption; Vault handles encryption at rest with correctly configured policies/keys. Secret values are never returned to a browser or stored in PostgreSQL by this code.
- `String` and `GoString` redact token/provider details. GET returns raw bytes only to the backend `WithSecret` callback, which clears them after use.
- A HashiCorp token must be provisioned with least-privilege per-workspace policies and supplied to the backend securely (not in git, logs, or client storage); this PR does NOT create/publish a real Vault token.
- All tests use ephemeral `httptest.NewTLSServer` with fabricated secret bytes, covering CAS, versioned GET, no redirect, no plaintext URL, timeout, path isolation and token redaction.

## No new operational privileges
- These methods do **NOT** prove caller identity or `MANAGE` permission by themselves. Only a future authenticated authorization facade may expose provider operations after durable audit/reconciliation.
- V3.2B-2 `SecureCatalog.StageReference` is not automatically connected. Failure between Vault CAS rotation and PostgreSQL reference update requires an outbox/reconciler and idempotency protection, still pending.
- No connection activation, database test query, arbitrary host input, ERP write, production ClickHouse write or runtime connection binding.
- Next: trusted identity provider + actual scoped API handler; provider deployment policy / authn; durable rotation-state workflow; V3.2C bounded Test Connection only after approved allowlist.
