# Connection Manager V3.2B-1 — server-only security core (synthetic)

**Status:** implementation slice only; NOT V3.2B exit, NOT production RBAC, NOT a real credential vault.

## Implemented in this PR

- `internal/connections/security.go`: explicit independent `VIEW`, `USE`, `MANAGE`, `TEST` permissions. Policy is deny-by-default, actor/workspace exact-match, immutable copied grants (synthetic use), no wildcard or inherited admin scope.
- `SecretRef`: opaque handle + workspace + revision, server-only; `String`, `GoString`, JSON redacted. `SafeConnection` serializes only minimal public state.
- Provider-neutral `Vault` interface for credential read and rotation. No default disk, environment, database or external vault provider has been chosen. No secret material stored in PostgreSQL, Git, UI, or HTTP handlers by this PR.
- `Service.WithSecret`: independently authorized `USE`/`TEST`, mandatory pre-access audit, a bounded server-side callback, cleared byte buffer after return. `Service.RotateSecret`: `MANAGE` only, pre-rotation intent audit, workspace/handle and monotonically increasing secret-reference version checks. These service operations are not wired into the API or pipeline runtime.
- Synthetic tests assert no cross-workspace privilege, no implicit role escalation, zero vault calls on denial/audit failure, redacted serialization, byte zeroization, version/tenant rotation guards.

## Security boundary and intentionally blocked behavior

1. **No trusted actor identity yet.** Existing `DATAFLOW_API_TOKEN` and `X-DataFlow-Connection-Admin` are shared lab credentials, not enterprise principals. Do not trust `X-User-ID` or workspace IDs supplied by browsers. Real actor/workspace identity requires a verified reverse-proxy/JWT/SSO contract and a revocation strategy.
2. **No vault provider selected.** A production adapter must implement encryption at rest, workspace isolation, access policies, rotation transaction/reconciliation, stable secret identifiers, revocation, version concurrency and operational audit. No credential-registration form or endpoint is enabled.
3. **Audit in this slice is a trusted interface, not a durable audit store.** Intent is recorded before calling a vault; the `RotateSecret` result does **not** represent durable completion, metadata persistence or a transactional outbox. Crash/partial-rotation reconciliation must be resolved before API activation.
4. **No endpoints or Test Connection.** No hostname, DSN, secret values, credentials, arbitrary egress, database connections or new execution/binding paths accepted. V3.2A registry remains lab-only, inactive and uses its existing guarded bearer/admin header.
5. **Not a security certificate.** Go unit/race tests are synthetic; CI does not assert live Epicor, SQL Server privileges or production deployment.

## Remaining V3.2B work before V3.2C

- Decide a server-side vault provider and private/internal vs internet-exposed deployment mode; define workspace administration and actor identity authority with threat model.
- Persist workspace-bound connection metadata, opaque secret reference, immutable actor/action audit and versioned rotation in migrations; avoid secret material and hostnames in metadata. Require fail-closed migration/schema/config gates.
- Wire real authenticated policy to V3.2A read/write operations, including cross-workspace list filtering and revoked permission tests. Maintain legacy lab compatibility only inside explicitly isolated nonproduction deployments.
- Implement secure credential setup via server-only trusted administration after approval, controlled egress policy, durable audit / partial-failure recovery.
- Before V3.2C: approved connector endpoint allowlist and SSL/DNS/SSRF/rate-limit test plan.

## Test coverage

`go test -race ./internal/connections` is the target package check; repository-wide `go test -race ./...` runs in hosted CI. Tests use fake provider, fake audit and fabricated secret bytes only, with **no network or ERP access**.
