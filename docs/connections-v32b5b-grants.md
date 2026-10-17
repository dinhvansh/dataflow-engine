# V3.2B-5B — Disconnected Workspace Grant Lifecycle (Synthetic CI)

**Scope:** trusted server-only RBAC grant management. **No HTTP write path, no real IdP provisioning or Vault activation.** This is not a production admin console.

## Delivered
- Migration `019_connection_workspace_grant_audit.sql` adds independent, append-only grant/revoke audit records (actor, target issuer-scoped opaque subject hash, exact workspace, scope, action, timestamp); an UPDATE/DELETE/TRUNCATE trigger blocks ordinary mutation. Privileged database owners still have bypass abilities and require operational control.
- `GrantAdmin.SetScope(ctx, issuerActor, workspace, targetActor, scope, enable)` applies **exact one** VIEW/USE/TEST grant or revoke with idempotent transitions. Repeating a grant/revocation does not duplicate events. Row-locked MANAGE authorization and state change plus audit occur in one PostgreSQL transaction; audit failure aborts the grant update.
- **MANAGE cannot be delegated by this API.** It must be provisioned via an independently audited trusted-operator bootstrap after IdP/workspace ownership verification. An actor cannot grant into a workspace where that actor lacks MANAGE.
- Target actor is a 64-character hex SHA-256 issuer-scoped key, not a browser-supplied email, username or header. The `AuthenticatedGrantAdmin` additionally requires a valid admin JWT and a `VerifiedIdentity` target from the **same issuer**. Its target proof must originate from an approved trusted directory/IdP enrollment process, not be self-selected.
- Existing V3.2B-5A HTTP GET stays off-by-default, read-only, TLS-direct-only. No grant API or new UI is exposed, and V3.2A shared-token routes do not gain authority.

## Synthetic QA
- New isolated PostgreSQL integration exercises migration replay, VIEW grant + no-implicit-MANAGE, repeated grant/no audit duplication, revoke/re-grant, instant access denial after revocation, cross-workspace isolation, denied MANAGE delegation, spoofed target/actor, revoked administrator and append-only grant audit. Unit RSA JWT tests reject foreign IdP subjects.
- No new network destination, SQL Server mutation, ClickHouse production change, secret or endpoint exposure.

## Still blocking V3.2B exit
- Verified real IdP/JWKS trust chain and target-user enrollment (no trusted target directory exists in this PR), robust TLS ingress and operator auditable MANAGE ownership bootstrap.
- Approved Vault deployment + token/policy management, paired journal/outbox for atomic Vault↔PostgreSQL rotation compensation/recovery, workspace-scoped MANAGE endpoints, full failure/abuse testing.
- V3.2C guarded Test Connection and V3.3 runtime binding remain separate future milestones. ERP remains SELECT-only.
