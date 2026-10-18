# V3.2B-5E-4A — Vault workspace-scoped adapter (isolated foundation)

**Status:** synthetic local HTTPS contract + Go unit tests. Not real Vault ACL provisioning, production credential management, connection activation, or a Test Connection API.

The existing Vault KV-v2 transport (#58) accepted workspace-scoped `SecretRef` metadata, but a service token could potentially be shared across workspaces. A Vault path convention alone is **not** a trusted authorization boundary. This milestone introduces `NewWorkspaceKVv2Vault(endpoint,mount,token,workspace,handlePrefix,client)` with an exact operator-approved workspace and subtree, such as `alpha` / `ref/area_a/`. Token/endpoint are server-side configuration only.

- Every `Read`, `Create`, and CAS `Rotate` fails before network access unless the ref belongs to the exact configured workspace and begins with its approved `ref/<subtree>/` prefix followed by an item. A bare `ref/`, malformed workspace, a neighboring prefix, trailing slash or wrong tenant is denied.
- The underlying provider still requires HTTPS and validated TLS, blocks HTTP redirects and follows fixed server configuration. The workspace wrapper adds no HTTP route, browser-supplied Vault token, connection credential readout or runtime binding.
- A local TLS fixture simulates separate server tokens/policies for two workspaces, verifies authorized version-pinned reads and CAS rotation, rejects cross-workspace read/write/rotation **with zero HTTP requests**, checks stale-version rejection and redacts debugging output.
- Independent **real Vault ACLs remain mandatory**. A different token with a *server-side* minimal policy per workspace and subtree must be provisioned and security-reviewed before external use. A sample conceptual ACL for workspace `alpha`, subtree `ref/area_a/` is `path "kv/data/workspaces/alpha/area_a/*" { capabilities = ["read", "create", "update"] }`; Vault KV-v2 metadata and delete/version permissions must be separately reviewed and should default to denied. This example is **not applied by DataFlow** and is **not** a complete production policy.
- The wrapper does not authorize users; the trusted service caller must enforce current workspace MANAGE for credential mutation and USE/TEST for credential access. It is disconnected from existing HTTP handlers and live runtime.
- The next release slice is signed, evidence-backed operator handling of `NEEDS_REVIEW`, without silent journal clearing. Production Vault token custody, IP egress, cert lifecycle, IdP subject enrollment and recovery procedures remain approval gates.

ERP source remains SELECT-only. PostgreSQL data mutations for CI are limited to disposable metadata fixtures; no production Vault request is made.
