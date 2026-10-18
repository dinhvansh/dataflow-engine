# V3.2B-5E-4C — Signed confirmed-applied recovery (isolated TEST)

**Status: partial recovery primitive proven only on disposable PostgreSQL and synthetic Vault fixtures. Not live recovery, credential compensation, Vault locking or production readiness.**

## Scope: safe subset only

`ManageBootstrap.FinalizeAlreadyApplied` handles exactly one case: the rotation journal is `NEEDS_REVIEW` but the inactive draft profile already points at the signed target secret version **with exactly the originally expected revision + 1**. Neither the profile nor Vault is altered by recovery. This is reconciliation of an *already-applied* state, **not rollback**, credential re-creation, or automatic repair of divergent metadata.

This off-by-default server-only method requires:
- Explicit trusted isolated-lab and isolated-PostgreSQL flags, validated pinned RS256 JWT matching exact issuer/workspace/actor and a **current MANAGE grant locked in the same transaction**.
- A *prior* immutable independently two-operator signed review for the same rotation, workspace and actor **classified `target`**, using one review UUID. A previous `diverged` review is intentionally insufficient even if later metadata changes.
- A **new**, different, two-operator Ed25519 signed recovery envelope with purpose `confirm_already_applied`, binding the review UUID, rotation UUID, target version, expected revision, workspace, pseudonymous verified actor and exact issuer. Two independent signers, max 15-minute life; no privilege or signature reuse from the review action.
- Under row locks: draft profile's version, revision and handle must exactly match the frozen journal target. No compensation for mismatches.
- From the approved workspace-scoped Vault provider: KV-v2 `current_version` exactly equals target, metadata confirms current version not soft-deleted/destroyed, pinned target secret is readable and nonempty. Latest version checked a second time to detect common races. Secret bytes are always zeroed.
- In one PostgreSQL transaction only after these checks: insert immutable signed recovery evidence (migration 025), change journal `needs_review -> applied`, insert immutable `reconciled` transition event and `rotation_reconciled` security audit. Exact identical replay is a no-op if the signed grant and metadata are still consistent; a different approval cannot re-finalize. Revoked MANAGE blocks even a replay.

## What must still remain quarantined

- Profile currently at old secret version, a later version, a different handle, an unexpected revision, or conflicting concurrent metadata. **Never auto-apply, delete, revert or overwrite** in this case.
- A Vault target that is not the latest or is unavailable/deleted/destroyed, a missing review, a review classified `diverged`, forged/expired signature, changed workspace/version/revision, or revoked MANAGE. All deny without touching profile/Vault/journal.
- Any real Vault whose token/ACL, issuer provenance, network egress, cert lifecycle, out-of-band rotation coordination and latest-version concurrency controls have not been explicitly approved. PostgreSQL row locks **cannot** lock external Vault writes, and two latest-version reads are not an atomic distributed CAS. This implementation deliberately **cannot be trusted in production** without an external write-freeze/coordination protocol and security review.

## Verification

Go tests cover wrong issuer/actor/workspace/rotation/review ID/version/revision/purpose, stale/duplicate signers and redaction; disposable PG integration injects CAS conflict, requires a fresh target-classified review after synthetic metadata repair, tests stale/new Vault version, outage, version changing between probes, credential zeroing, replay, immutable audit/event record and revoked access. Local generated TLS fixture verifies KV-v2 current-version endpoint and rejection of missing/deleted/destroyed/redirect or foreign-workspace responses.

No HTTP endpoint, UI action, real Vault provision, source write, Epicor login, runtime binding or active connection is introduced.

**Follow-up:** V3.2B-5E-4D operator-approved divergent recovery plan and external source-of-truth / Vault write-freeze policy, then V3.2C-2B driver-authenticated TEST with audited workspace TEST scope and approved network egress. Product remains Developer Preview / production NO-GO.
