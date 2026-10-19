# DataFlow console — one-time local sign-in (isolated QA)

**Status:** Operator/Admin session UX for isolated, single-instance QA. This replaces routine API/Admin token entry from the browser when enabled. It is **not** Enterprise SSO, production RBAC, Vault/KMS, or multi-node deployment authorization.

## One-time owner setup (QA machine, not GitHub)

1. Use only the owned `dataflow-localqa` stack and verify the label-based isolation first. Do not run against Epicor/ERP production or remove Docker volumes.
2. Start the QA stack following `deploy/local-qa/README.md`; `.env.qa.local` must exist with chmod 600.
3. Run `bash deploy/local-qa/setup-login.sh` **interactively** (never from CI). Set distinct passwords for `operator` and `admin`. The script checks isolation, derives bcrypt hashes, and saves only the hashes in the ignored private environment. It also enables the already guarded QA connection features; there are **no feature toggles inside the UI**.
4. Restart approved isolated API and Web containers with existing `deploy/local-qa/run-core-operator.sh` or the non-destructive scoped docker compose `up -d --build` commands in the QA runbook. Avoid reinitializing databases or cleaning volumes.
5. Open the existing local DataFlow UI URL. Sign in as `admin` to Add/Save/Test/Bind connections, or `operator` to run existing pipelines.

The server keeps a 4-hour opaque session in process memory and exposes an HttpOnly SameSite=Strict cookie. Sign out revokes it; restarting the API also invalidates all active sessions. The API credential and Connection Admin token remain server-side; operator HTTP requests cannot supply or override an admin authorization header. The SQL Server/ClickHouse password is stored using the separately configured lab AES-GCM key; the browser never receives it back.

## Restrictions

- The protected UI route is `/api/v1/auth/session`. Both passwords must be configured as bcrypt hashes in the QA environment before local sign-in activates. No fixed default password.
- All management and execution endpoints are still guarded server-side, not only hidden from the sidebar. Operators cannot create/test/bind connections.
- ERP remains SELECT-only and connection profiles remain pinned to `DataFlowFixtureQA` and `scratch` in this lab. No new permission to mutate production data.
- Legacy API tokens remain available only when local auth is not configured for existing development tools and automated QA; they are **not** a production migration strategy.
- Use trusted loopback/isolated Docker networking for this trial. **Do not expose the QA app to an untrusted network**, and do not claim production GO. Production follow-up must support approved OIDC/SSO and durable, scoped RBAC, secure TLS, Vault/KMS and audit.

## Validation

The PR tests backend login/logout, operator/admin separation, header forgery, same-origin request checks and mocked browser login. The standard SQL read-only and Core E2E CI must remain green. This **does not substitute** for the owner's live browser sign-in and Core8B demonstration using the approved QA instance.
