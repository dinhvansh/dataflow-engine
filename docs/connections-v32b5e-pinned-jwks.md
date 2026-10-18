# V3.2B-5E-1 — Operator-pinned offline JWKS rollover (synthetic, opt-in)

**Status: implementation / synthetic CI only. NOT a live IdP integration, not SSO enrollment, not production-approved.** ERP source remains SELECT-only; no DB connection probes, Vault writes, MANAGE endpoint or profile activation are enabled.

## Scope

An enabled, direct-TLS isolated-lab **read-only** secure connection view can now verify RS256 JWTs against an immutable *operator-provisioned* local JWKS file containing multiple overlapping public keys. This enables an explicit old-key/new-key rotation without dynamic network retrieval. Existing single PEM public-key configuration remains supported.

- The only additional trust input is `DATAFLOW_SECURE_OIDC_PINNED_JWKS_FILE=/trusted/operator/oidc-pins.json`, alongside the existing fixed HTTPS `DATAFLOW_SECURE_OIDC_ISSUER` and `DATAFLOW_SECURE_OIDC_AUDIENCE`.
- In JWKS mode **unset both** `DATAFLOW_SECURE_OIDC_KID` and `DATAFLOW_SECURE_OIDC_RS256_PUBLIC_KEY_FILE`. Setting either with a JWKS path is a startup error; accidental union of old and new configurations is forbidden.
- The file must be a real regular file (not a symlink), with no group/other write permission and <=128 KiB. It is read and validated at startup; there is **no** hot reload, remote JWKS fetch, `jku`/`x5u` token-header resolution, browser-supplied key, endpoint or redirect.
- Strictly curated JWKS shape: `{"keys":[{"kty":"RSA","kid":"approved-key-1","alg":"RS256","use":"sig","n":"<base64url RSA modulus>","e":"AQAB"}]}` (the angle-bracket value is a placeholder, not a real key). Only these six fields are accepted for each key. Key IDs must be unique; 1–32 keys, RSA modulus 2048–4096 bits, public exponent 65537. Private-key parameters or unrecognized fields make startup fail. Generate public modulus values from **trusted, independently verified** IdP metadata, never a link from an inbound token.
- JWT issuer, audience, lifetime and pseudonymous actor computation still use the existing pinned `RS256Verifier`, while database VIEW/USE/TEST/MANAGE grant checks still run independently per workspace. No implicit elevation.

## Offline operator rotation procedure (not an automatic trust service)

1. An authorized operator validates issuer, JWKS provenance and the new RSA public-key fingerprint **out of band**, records the approval and places an owner-controlled file containing both old and new approved key entries in the isolated service host.
2. Stage the replacement file atomically, restart the opt-in direct-TLS service instance and verify controlled JWTs signed by both keys with correct issuer/audience. Do not add unknown signers to the overlap.
3. After the trusted IdP has stopped issuing tokens from the old key **and** all previously issued tokens have expired (max accepted token lifetime: two hours), provision a new file containing only the approved new key, restart, and verify that old JWTs are denied.
4. Document run IDs, approval and key fingerprints outside Git; keep the prior approved config available for incident recovery only under operator control. Never embed private keys, real tokens or secret endpoints in tests/docs.

**Not implemented:** automated JWKS polling/refresh, trustworthy IdP discovery/provisioning, signing-key emergency revocation across already-running instances, trusted target-user directory and MANAGE bootstrap, production TLS/cert renewal, Vault deployment, audited Vault journal compensation, active driver Test Connection or production acceptance. JWT verification alone does not grant workspace access.

## CI acceptance

Pure Go unit tests prove valid overlap, stable actor ID and rejection after key removal; malformed/weak/unknown/private/untrusted key inputs and unsafe files are rejected. Isolated PostgreSQL + synthetic TLS handler integration checks live VIEW grant with both keys, post-restart removal, conflicting trust configuration, and grant revocation. This does **not** certify real IdP key authenticity or production permissions.

Read alongside [V3.2B-4](connections-v32b4-oidc.md), [V3.2B-5A](connections-v32b5a-http-view.md) and [V3.2B-5D](connections-v32b5d-direct-tls.md).
