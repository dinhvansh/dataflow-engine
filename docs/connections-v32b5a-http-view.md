# DataFlow V3.2B-5A — guarded connection metadata HTTP VIEW (lab only)

**Security status: synthetic/lab implementation; V3.2B remains IN PROGRESS and production NO-GO.**

## What this PR adds

A separate `GET /api/v1/secure/connections/definitions?workspace=<id>` route, mounted without replacing V3.2A. It lists **inactive synthetic Draft metadata only**. It does not create/edit credentials, Test Connection, attach runtime jobs, execute SQL, configure hosts/DSNs, or contact Epicor/Vault.

Request security:
- New route is **404 by default**. It requires `DATAFLOW_ENABLE_SECURE_CONNECTION_VIEW_TEST=true` and `DATAFLOW_SECURE_CONNECTIONS_ISOLATED_LAB=true`; startup rejects missing PostgreSQL migration 018, invalid OIDC issuer/audience/kid or missing/invalid pinned RS256 PEM public-key file.
- JWT signature/claims verified by previously merged pinned `RS256Verifier`. `Authorization: Bearer ...` **is not** accepted from existing shared API bearer token. No `X-Actor` or `X-Workspace` trust: actor comes solely from verified JWT issuer+subject, workspace comes from a validated query parameter and DB authorization.
- Only `VIEW` permits listing in the requested workspace; `MANAGE`, `USE`, `TEST` are not implicit. `SecureCatalog.ListDrafts` locks grants in transaction, filters workspace and returns at most 100 rows. Revocation denies subsequent requests.
- Sanitized errors, response `no-store`, `Vary: Authorization`, no secret handles/versions/passwords/network addresses in public JSON.
- **Direct TLS is mandatory** (`r.TLS != nil`); spoofed `X-Forwarded-Proto` does not grant transport authority. The current DataFlow Go listener is plaintext HTTP, therefore this route will return 403 even when enabled on that listener. Deployment must add an explicitly reviewed TLS listener (or a verified reverse-proxy trust boundary) before it can serve authenticated browser traffic. Never solve this by trusting public forwarded headers.
- Only a trusted operator may provision the **public** RSA key file, issuer, audience and kid; rotating signing keys / real IdP and HTTPS ingress is not delivered.

## Synthetic QA
- Disabled/missing config deny startup or 404; invalid signature fails before contacting PG.
- Real isolated PostgreSQL migration 018, synthetic pinned RSA JWT and profile fixture; good VIEW → 200, cross-workspace → 403, wrong signature → 403, missing bearer → 401, plaintext transport → 403, POST → 405, malformed workspace/duplicate query → 400; revoke grant → 403.
- No real IdP, Vault or Epicor network/credential tests. CI covers Go race/vet/build and disposable PostgreSQL/ClickHouse/dbt plus web build.

## Next gates
V3.2B-5B: key-management plan (key rotation, trusted JWKS provisioning, IdP identity life cycle), verified HTTPS ingress, trusted workspace grant issuance/revocation, then reviewed **MANAGE** API. V3.2B-5C: durable Vault ↔ PostgreSQL rotate protocol with reconciliation and failure injection. V3.2C only after egress/SSRF/DNS/TLS controls and source role verification. Do not call this stage production-ready.
