# V3.2B-4 — Offline trusted OIDC/JWT principal verifier

**Status:** security foundation, not SSO deployment, no public API wiring.

## Verifier
- `NewRS256Verifier` pins exact HTTPS issuer, audience and 2048+ bit RSA public keys supplied by trusted server configuration. Keys copied to prevent caller mutation. No remote JWKS fetching, token-provided `jku`/`x5u` network requests or implicit trust of client actor/workspace fields.
- `Verify`: RS256 PKCS#1 v1.5 signature verification on the exact JWT input; strict `alg`, `kid`, optional `typ`, reject unsupported `crit`, issuer, audience string or list, nonempty subject, exp/iat/nbf and maximum 2-hour token lifetime. Max 16 KiB token size; no clock grace for expiry.
- Pseudonymous actor ID = SHA-256(issuer + NUL + subject). This provides a stable issuer-scoped grant key; the DB still enforces workspace and action on each operation.
- `AuthenticatedCatalog` wrapper accepts a bearer JWT and only then delegates to `SecureCatalog` with the verified actor ID. This wrapper is **NOT an HTTP route**, so cannot yet be used as enterprise login.

## Required before production
- Select real OIDC IdP (Microsoft Entra ID or other), accepted audiences/tenants, out-of-band JWKS provisioning/key rotation, clock-sync, token revocation/expiry controls, least-privilege workspace membership provisioning and secure bootstrap. Do not accept custom `X-Actor`, `X-Workspace`, unsigned JWT or shared lab token as a production identity.
- Configure HTTPS ingress, CSRF/CORS/session/token-handling policy, controlled frontend storage, authorization error responses and abuse rate limits.
- Only after completing trusted IdP integration may real API endpoints use this verifier and profile catalog. Token verification alone does not authorize `VIEW/USE/MANAGE/TEST`; database grants remain independent.
- Existing lab-only V3.2A remains unchanged; no endpoints, secret values, DB connections or ERP writes added.

## QA
- Pure synthetic RSA signing fixtures verify valid JWTs, array audiences, wrong signer, wrong issuer/tenant, wrong audience, invalid roles, alg=none/HS256, key ID spoof, long/expired/future tokens, redaction and no database on invalid signature.
- CI Go race tests run these without network, secrets or real ERP access.
