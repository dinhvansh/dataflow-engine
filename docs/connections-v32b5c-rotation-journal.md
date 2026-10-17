# V3.2B-5C — Crash-safe rotation reconciliation journal (synthetic only)

**Scope: disconnected PostgreSQL journal and server-only recovery. No live credential upload, external Vault provisioning, HTTP write route, SQL Server connection or runtime binding. Developer Preview / production NO-GO.**

## Why
A Vault KV-v2 rotation can succeed while the API crashes before PostgreSQL records the new reference. Without a durable intent record, another rotation may overwrite or lose the new secret version.

## Protocol
1. Under a fresh, row-locked **MANAGE** grant, `RotationJournal.Prepare` locks the inactive Draft connection profile, checks its exact workspace and expected metadata revision, and inserts a durable **PREPARED** journal row with source/target secret versions, opaque handle and identity **before** the external Vault mutation. No credential bytes/DSN/host/network endpoint are persisted. An immutable prepared event is appended in the same transaction.
2. A separate, trusted coordinator MAY perform Vault CAS rotation using the existing, operator-provisioned Vault adapter (not implemented/wired to HTTP by this PR). If it crashes after writing KV-v2 version N+1, the journal remains **PREPARED**.
3. `RotationJournal.Apply` first rechecks MANAGE, then verifies that the exact target version exists via `Vault.Read`, clears returned secret bytes, and re-checks MANAGE with a locked transaction. It applies secret version/revision CAS, existing append-only connection audit, and journal **APPLIED** transition evidence **atomically** in PostgreSQL. Replaying Apply after success is idempotent.
4. If the provider version is absent/unavailable, no pointer/version changes and the journal remains PREPARED for later retry. If the connection profile revision/handle/version changed, the journal enters **NEEDS_REVIEW** without overwriting metadata; a partial unique index prevents new rotations until an authorized operator reconciles the discrepancy. This PR deliberately does **not** implement automatic reset/manual override.
5. Distinct workspace, missing grant, revoked administrator or unavailable vault deny the operation. One unfinished rotation per connection is permitted.

## Security/operational limitations
- Vault and PostgreSQL are **NOT** one atomic transaction. This protocol provides durable evidence and a narrow retry path; it cannot automatically roll back external Vault writes or prove external availability/security. Manual resolution of `needs_review` is required.
- Current `Vault.Read` may return raw bytes to backend memory; the journal immediately zeroes its copy. Never log or HTTP-serialize the full profile, secret handle, rotation ticket or provider responses.
- **No real Vault invocation** is performed by CI. The service is not exposed to UI, and existing V3.2B-5A GET remains off-by-default, direct-TLS only.
- Real deployment still needs approved IdP/JWKS key rotation, HTTPS ingress, Vault policy/token/egress, secret issuance workflow, trusted MANAGE bootstrap, recovery operator controls and security review.

## Isolated acceptance tests
CI installs migration 020 twice in disposable PostgreSQL and uses a fabricated `Vault` implementation; validates replay-safe PREPARED/APPLIED events, missing vault version, version CAS, revocation, cross-workspace, ticket JSON redaction, inaccessible immutable audit, and NEEDS_REVIEW under injected metadata conflict. The ERP source remains SELECT-only and is not contacted.
