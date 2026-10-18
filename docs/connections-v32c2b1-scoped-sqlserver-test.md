# V3.2C-2B1 — Real SQL Server read-only driver TEST under isolated TEST RBAC

**Scope: disposable QA fixture only.** The real Microsoft SQL Server 2022 driver performs an actual login and queries session identity and effective source-table permissions. This is stronger than the previous TCP reachability probe but is **NOT** a deployable Connection Manager Test Connection endpoint or a production Epicor connection.

`ScopedSQLServerSessionTest.Test` accepts a **trusted server-injected, pre-opened** `*sql.DB` (never a client-supplied DSN, SQL, password, host or login) plus independently verified pinned RS256 JWT, workspace and inactive source profile ID. It is disabled unless **all** isolated lab, disposable PG and disposable SQL Server environment gates are present. The profile must be an inactive SQL Server source draft with a configured secret reference; **the passed SQL session is not yet bound to that reference**, and no real Vault credentials are activated.

The action:
1. Verifies JWT issuer/signature via existing pinned verifier, validates workspace/profile UUID and acquires a row lock on the current **TEST** grant in the metadata transaction. A revoked role or foreign workspace is denied before any SQL Server query.
2. Audits `test_requested` under the lock in disposable PostgreSQL (migration **027**).
3. Executes **only static, non-user-controlled, read-only metadata SQL** via real go-mssqldb driver: `DB_NAME()`, `ORIGINAL_LOGIN()`, SQL `IS_ROLEMEMBER()`, and `HAS_PERMS_BY_NAME()` against the disposable QA object `Erp.Part`. Requires exact `DataFlowFixtureQA` / `qa_erp_reader`, SELECT=1, INSERT/UPDATE/DELETE/ALTER=0, and not `db_owner`/`db_datawriter`. No DML/DDL is attempted at any point.
4. Audits `test_passed` or `test_failed` in the same transaction. Returns only `passed: true` or a sanitized error, never a raw SQL driver/network failure or credentials.
5. Tests verify issuer/TEST/workspace denial, post-revocation denial, actual QA login and effective SQL grants, pass/fail/request audit counts, immutable audit rows and unreachable session handling. GitHub's SQL Server QA workflow now starts **a separate disposable PostgreSQL container** to run this scoped end-to-end test; the original SELECT-only reader tests continue to run.

### Still blocked: V3.2C-2B production Test Connection

- Server operator must bind the SQL session to an approved, workspace-scoped Vault reference, pinned secret version and approved connection configuration. In this milestone the session is injected by the isolated fixture. No production credential is loaded.
- No proof yet that the driver dials exactly the `connectionnet.Plan` pinned approved IP with the expected hostname/TLS certificate, and no infrastructure-enforced egress restriction. The SQLServer QA fixture uses loopback and `TrustServerCertificate=true` **only in the disposable runner**; this must never be allowed in production.
- Real organizational OIDC subject enrollment, trusted operator key custody, TEST scope policy approvals, rate limits, DLP/Vault egress, audit retention, credential revocation and TLS lifecycle remain unapproved. No HTTP Test Connection route or UI action is enabled.
- A successful test of the **QA** SQL Server account proves nothing about live Epicor access, live network policy, or user-selected server credentials. Production remains **NO-GO**, ERP always SELECT-only.

See [V3.2C endpoint admission](connections-v32c1-egress-admission.md), [synthetic pinned probe](connections-v32c2a-pinned-probe.md), and [Vault compensation](connections-v32b5e4d-divergence-runbook.md).
