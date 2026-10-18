# V3.2B-5E-3 — signed two-operator MANAGE bootstrap (isolated CI foundation)

**Status: server-only, disconnected synthetic authorization and disposable PostgreSQL CI; not production identity enrollment.** This feature adds no HTTP route, UI, runtime binding, named connection activation, credential handling, external IdP integration or network egress. Epicor remains SELECT-only, with **zero** source DML/DDL.

## Why this exists

Prior VIEW/USE/TEST delegation (#63) deliberately could not grant MANAGE. A legitimate initial workspace administrator requires an independently authorized operator control path, not self-service OAuth claims, arbitrary browser headers or a shared admin token.

## Trusted prerequisites (not provided by this milestone)

Two separate organizational operators must independently validate the intended OIDC issuer/tenant, target-subject ownership, exact workspace ownership, approval purpose and revocation history **outside the application**. Their distinct Ed25519 public keys must be pinned in reviewed server configuration; their **private keys never enter DataFlow, Git or PostgreSQL**. A signed JWT only proves that a principal is authenticated by the configured issuer; it **does not** attest entitlement or workspace ownership.

## Server-only isolated prototype

- `NewIsolatedManageBootstrap` requires all three opt-in conditions: explicit `isolated=true`, `DATAFLOW_SECURE_CONNECTIONS_ISOLATED_LAB=true`, and `DATAFLOW_INTEGRATION_PG_ISOLATED=true`. If these are absent, the feature fails closed. No main API route invokes this constructor.
- It copies **2–16 operator-approved pinned public keys** and rejects duplicated key material under different IDs. Exactly **two separate signers** must sign the **same** domain-separated canonical payload with Ed25519.
- Signed request binds: request UUID, exact workspace, the target's 64-character issuer-scoped SHA-256 actor ID, exact HTTPS issuer, fixed `bootstrap_manage` action, and issue/expiry timestamps with a maximum 15-minute lifetime. No signer/workspace/actor is trusted from a caller header.
- Target identity must be derived from a valid already-pinned RS256 JWT and must match the signed target actor and issuer. Expired/forged/wrong-issuer/wrong-workspace/wrong-purpose/tampered/missing/duplicate approvals are rejected before DB changes.
- Migration 023 adds an **append-only accepted-approval ledger**, recording only workspace, pseudonymous target actor, canonical request digest, two signer IDs and time — **no JWT, private signing key, credential, Vault path, connection string or token**.
- In one PostgreSQL transaction, a workspace row lock serializes bootstrap decisions, inserts the immutable approval evidence and grants exactly the MANAGE scope. Repeated identical approved requests become idempotent no-ops while the grant is still active.
- **Revocation wins:** an old accepted signed request can never restore a later-revoked MANAGE grant, even if its signatures and JWT remain valid. A fresh, separately signed request ID is required to reinstate access. The existing manager API still cannot delegate MANAGE.
- No connections are activated and no Vault secret or SQL Server driver is touched.

## Validation and threat cases

Go unit tests cover tampered workspace/actor/issuer/action/UUID, stale/future/expired approvals, duplicate/untrusted signers, invalid signature, foreign RS256 JWT, non-isolated operation and redaction. Disposable PostgreSQL integration covers migration replay, initial grant, exact replay, cross-workspace deny, VIEW-only downstream delegation, immutable audit, revocation that cannot be undone by replay, and separately signed reinstatement.

**Release/production blockers:** independently verified real IdP and tenant policies, trusted operator key custody and approval provenance, directory-backed actor enrollment, dual-control operations workflow, production database permission separation and external security review. Do **not** call this a real SSO/identity registry or deploy migration 023 against a production metadata database without change approval.

The follow-on milestone is V3.2B-5E-4: approved Vault policies and operator-controlled reconciliation, then V3.2C authenticated bounded Test Connection. Existing isolated SQL Server 2022 SELECT-only smoke QA (#76) does not by itself enable Test Connection.
