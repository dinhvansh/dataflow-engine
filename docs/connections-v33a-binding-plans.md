# V3.3-1 — immutable connection binding plan metadata (synthetic only)

This is an **inactive, non-executable** precursor to versioned multi-connection runtime routing.

## Delivered
- Migration 021 `connection_security_binding_plans` stores exact workspace, pipeline/node IDs, profile UUID, profile revision, source/destination role and issuer-scoped actor; **never** a credential, hostname, DSN or secret handle.
- Composite `FOREIGN KEY(profile_id, workspace_id)` rejects cross-tenant metadata even if a future application bug attempts a direct SQL insert; immutable UPDATE/DELETE/TRUNCATE trigger protects the historical binding row under normal application credentials.
- Server-only `BindingPlans.Create` requires a fresh row-locked workspace `USE` grant, a matching inactive Draft profile, expected revision and strict stored pipeline/node IDs. Duplicate binding/revision/foreign-workspace denied.
- `CheckPlannedCurrent` requires new `USE` permission each call and exact current connection revision/role/workspace; any revocation or profile revision drift rejects the plan. It returns **no execution authorization**, driver credentials, endpoint or active connection.
- Synthetic PostgreSQL QA: migration idempotence, foreign-key isolation, immutable binding, duplicated plan, stale revision after profile edit, privilege separation (MANAGE is not USE), grant revocation and unsafe node input.

## Not yet built
- A real ACTIVE state, approval workflow, privileged connector credentials, source/target multi-instance dial, immutable execution-time snapshot or binding in the existing SQL DAG/Managed Sync runtime
- Approved tenant identity provisioning/HTTPS/Vault policies and auditable secret recovery, production test/egress guards
- Explicit fallback/migration from legacy environment slots, multi-instance acceptance, metrics/rollback

Before any job uses a binding, the real runtime must **recheck** `USE` at dispatch under its own verified service/actor authorization, guarantee exact revision/state, keep source Epicor SELECT-only and destination scratch-only; an inactive Draft and a successful `CheckPlannedCurrent` MUST NOT be treated as an executable source.

**Current status:** synthetic immutable *plan* persistence only; V3.3 production runtime bindings and V3.4 E2E remain pending.
