# V3.2B-5E-4B — Two-operator evidence for quarantined rotations

**Status: code + synthetic integration only; NOT a live operator recovery / compensation service.** This slice adds auditable **acknowledgment**, not a state transition out of `NEEDS_REVIEW`.

## Behavior

A rotation journal from V3.2B-5C has a durable `needs_review` state if Vault has a target version but PostgreSQL metadata revision CAS cannot be applied. The earlier `Inspect` tool reports bounded facts, but it does not prove that a legitimate operator reviewed them.

`ManageBootstrap.AcknowledgeRotationReview` provides a separate, off-by-default, **server-only** review path within identity-verified disposable PostgreSQL. It reuses a trusted RS256 OIDC verifier and two **independently pinned** Ed25519 operator public keys. Two operators must approve the exact **rotation UUID, workspace, issuer-scoped MANAGE actor, intended revision and target version, action `acknowledge_needs_review`, request ID, and 15-minute validity window** in a purpose-separated signing envelope. Signature replay from MANAGE bootstrap cannot authorize rotation review.

- Current MANAGE authorization is row-locked in the same PostgreSQL transaction as journal/profile inspection. Revoked actors and requests for a different workspace are denied before Vault IO.
- The exact target Vault version is read under a three-second deadline; credentials are cleared, including partial provider buffers on error. Missing version or Vault outage is **unavailable**, never interpreted as safe to discard.
- PostgreSQL profile evidence is categorized as `prior`, `target`, or `diverged` using frozen version/revision expectations. This is an observation, **not proof that the external Vault's latest version or real identity/ACL is correct**.
- Migration 024 adds immutable, append-only signed-review evidence and one `rotation_reviewed` audit action. Request UUIDs are idempotent; different payload under an accepted UUID is rejected. The journal stays **`NEEDS_REVIEW`**, blocking all new rotations on that profile.
- The service does not clear the journal, change the profile, rotate Vault credentials, deploy policies, make a Test Connection, or activate drafts. No HTTP endpoint or UI is added.

## QA and security boundary

Tests reject forged/duplicate operator signatures, modified workspace/rotation/revision/version/issuer, stale/expired or cross-purpose approvals, revoked MANAGE and missing Vault versions. Disposable PostgreSQL integration inserts a deliberate metadata CAS conflict, requires version-pinned evidence, verifies exactly one immutable review, correct audit, no duplicate replay, and confirms `NEEDS_REVIEW` continues to block new rotations. Existing SQL Server 2022 CI is still SELECT-only.

**Still required to actually recover:** reviewed human reconciliation/compensation runbook with a trusted external Vault policy/token authority, proof of latest KV-v2 metadata and intent, safe CAS repair/rollback design and explicit two-person sign-off, idempotency/failure injection, revocation and security approval. Do not release the quarantine from a signed acknowledgment alone.

This milestone is part of [V3.2B-5E-4A workspace Vault scope](connections-v32b5e4a-vault-scope.md) and follows the [5E-2 inspection](connections-v32b5e2-rotation-inspection.md) and [5E-3 two-operator bootstrap](connections-v32b5e3-manage-bootstrap.md).
