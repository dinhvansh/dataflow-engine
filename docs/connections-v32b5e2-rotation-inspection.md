# V3.2B-5E-2 — Safe operator inspection of rotation journal (synthetic QA)

**Status:** server-only, scoped diagnostic. No HTTP endpoint, operator UI, external Vault rollout, real credential lifecycle, or production reconciliation. The original V3.2B-5C journal's `prepared/applied/needs_review` state machine remains unchanged.

## Delivered

- `RotationJournal.Inspect(ctx, actor, workspace, journalID, provider)` accepts only a trusted-server `Vault` adapter and a verified actor key. Access requires a **fresh workspace MANAGE grant** held under PostgreSQL row lock for the entire bounded inspection; arbitrary browser headers or cross-workspace IDs cannot authorize access.
- The inspection uses a three-second deadline and an explicit, version-pinned `SecretRef`; it reads no arbitrary URLs and never performs database driver dialing. Credential buffers, including partial data returned with an error, are zeroed. Missing/unreachable Vault target versions produce **ErrUnavailable**, not a fabricated `not found` verdict.
- It compares immutable journal expectations against current profile revision and secret-version metadata to report *prepared-matching* or *applied-matching* evidence, without exposing the underlying handle, ID, actor, secret, or version. `RotationInspection` serializes only `[REDACTED]`. `NEEDS_REVIEW` always remains a manual block, even if Vault is readable.
- Successful inspections append `rotation_inspected` events to immutable `connection_security_audit`, under the same locked MANAGE transaction. Migration `022_connection_rotation_inspection_audit.sql` extends the allowed actions; apply only to independently verified disposable control PostgreSQL until approved for live operations.
- No profile/journal state transition, automatic conflict resolution, actor bootstrap, vault rotation, activation or ERP changes. The original reconciliation code cannot be bypassed by this method.

## Synthetic QA

Disposable PostgreSQL integration tests prove unknown Vault version fails closed, consistent PREPARED/APPLIED metadata, zeroed bytes, denied cross-workspace and revoked-admin access *before Vault read*, ambiguous NEEDS_REVIEW stays quarantined, three successful audited inspections, and migration replays. GitHub-hosted CI is necessary before merge. No claim of a deployed operator control plane.

## Remaining 5E blockers

A verified real IdP/target-subject enrollment and approved MANAGE bootstrap, Vault token/policy/egress and credential lifecycle, human-approved journal resolution/compensation with dual-write/retry safety, and production TLS/cert key rotation are still missing. This feature is **diagnostic evidence only**. Releasing `needs_review` without independent human audit and version provenance would be unsafe.

See [rotation journal](connections-v32b5c-rotation-journal.md), [OIDC pinned JWKS](connections-v32b5e-pinned-jwks.md) and the [master roadmap](MASTER_ROADMAP_AND_STATUS.md).
