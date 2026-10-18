# DataFlow Connection Manager — V3.2B / V3.2C release acceptance and operations handoff

**2026-10-18 status: Developer Preview. Production: NO-GO.** This document is the actionable, negative-case-driven closure matrix for the multi-PR V3.2B-5E and V3.2C-2 workstream. CI success on synthetic evidence proves precisely the test contracts, **not** that trusted identity, external Vault, SQL production network policy or signing-key custody exist.

## Completed isolated evidence

| Milestone | Delivered | Immutable evidence | NOT claimed |
| --- | --- | --- | --- |
| V3.2B-5E-1 (#74) | Strict operator-curated offline JWKS RS256 overlap/retirement | Exact-head hosted CI PASS | Actual IdP tenant enrollment / trusted JWKS lifecycle |
| V3.2B-5E-2 (#77) | MANAGE-locked, bounded, read-only rotation inspection | PG audit migration 022 | A credential repair |
| V3.2B-5E-3 (#80) | Short-lived two-signer disconnected initial MANAGE bootstrap | PG migration 023 | Real directory authorization or operator key custody |
| V3.2B-5E-4A (#82) | Server-only one-workspace/one-subtree Vault KV-v2 adapter | Local HTTPS + per-token fake ACL QA | Real Vault ACL / token policy |
| V3.2B-5E-4B (#83) | Signed dual-operator NEEDS_REVIEW observation | PG immutable review + audit migration 024 | Unquarantine |
| V3.2B-5E-4C (#85) | Confirmed-applied-only safe recovery, locked MANAGE, latest Vault proof | Migration 025 / no profile or Vault mutation | Divergent compensation, external distributed CAS |
| V3.2B-5E-4D (#87) | Signed immutable manual compensation **escalation plan** for divergence | Migration 026 + QA; **journal stays quarantined** | Automatic credential rollback/overwrite |
| V3.2C-1 (#67) / 2A (#71) | Operator CIDR/hostname pinned address admission, bounded synthetic TCP probe | Synthetic DNS/transport tests | Real SQL driver/TLS TEST from a bound connection |
| V3.2C-2B1 (#88) | Actual disposable SQL Server 2022 session with verified `TEST` grant and signed-issuer JWT; immutable requested/pass/fail audit | Migration 027 + disposable PG + SQLServer QA | Vault-bound real test, production TLS/network pin, exposed API/UI |

The disposable SQL Server test's source statements are **only read-only metadata queries** (`DB_NAME`, `ORIGINAL_LOGIN`, `IS_ROLEMEMBER`, `HAS_PERMS_BY_NAME`). The separate existing QA source tests execute SELECT-only. Neither tests against Epicor nor writes to ERP.

## Hard production release gates — ALL must pass

| Gate | Acceptance and negative tests | Current |
| --- | --- | --- |
| A. Identity authority | Independently approve issuer, tenant, audience and pinned JWKS origin; enroll verified actor/workspace ownership from organization-controlled directory; reject wrong tenant, forged kid, retired key, user-chosen target, expired token; document incident key revocation and renewal | **BLOCKED:** no verified live IdP enrollment |
| B. Dual control | Independently attest two named operators, separate private-key custody/HSM or approved secure equivalent, no shared private keys; document break-glass and expiry, revoke then confirm replay cannot restore MANAGE; independent audit reviewer | **BLOCKED:** CI-only fake operators |
| C. Vault policy | Approve dedicated least-privilege KV-v2 token/policy per workspace+subtree; review metadata read vs data read/create/update and deny all unwanted delete/destroy/list; prove unauthorized cross-workspace reads are rejected **by real Vault**; token rotation and egress ownership | **BLOCKED:** no production token/ACL proof |
| D. Network and TLS | Approve internal host:port/CIDR, DNS policy and **transport dialing the exact admitted pinned IP**; TLS certificate chain/SAN/SNI validated against original hostname, no `InsecureSkipVerify` or `TrustServerCertificate=true`; deny redirects, localhost, metadata IP and rebinding | **BLOCKED:** SQL driver QA loopback certificate trust is strictly disposable |
| E. Credential activation | Bind each inactive profile to independently proved Vault handle/version and operator-approved endpoint; no JSON/HTTP secret readout, no workspace privilege bypass, no disabled audit; reject stale revisions, revoked TEST/USE/MANAGE | **BLOCKED:** real connection binding absent |
| F. Divergent compensation | Obtain documented external Vault **write freeze / distributed coordination**, correct source-of-truth decision, immutable prior+target evidence and separately signed replay-protected approval; probe failure injection, concurrent rotation, soft-delete, version mismatch and rollback plan. Only release quarantine with independent proof. Never silently force journal state or overwrite Vault history | **BLOCKED:** #87 records escalation plan, not safe external repair |
| G. TEST control plane | Enforce JWT + TEST before SQL network access, rate/time/attempt budgets, sanitized response and audit for failures; real scoped service must use approved Vault credential and pinned/TLS transport and return stable redacted outcome. Test revoked permission and credential expiry under load | **BLOCKED:** #88 only QA pre-opened driver, no approved production endpoint |
| H. Release and operation | Security code review, least-privilege PG roles/migration approval, recovery drill, monitoring and alert on NEEDS_REVIEW/test failures, log redaction, retention and evidence review, pen test and explicit deployment/rollback owner sign-off | **BLOCKED:** no formal production review |

**Decision:** any failed or unverified gate remains **NO-GO** even when all GitHub Actions tests are green. Do not flip experimental flags on a real control DB, configure live Vault secrets, or add an HTTP TEST handler merely to bypass these gates.

## Controlled end-to-end acceptance in an approved isolated corporate lab

1. Deploy an organization-approved IdP and independently checked issuer/JWKS. Confirm subject enrollment maps to the intended workspace, with two independent operator signing keys managed outside Git.
2. Provision separate real Vault policies and narrowly scoped tokens, loopback or approved internal network ingress only, bounded certificate lifecycle, a Vault audit device and independently verified KV-v2 CAS revision behavior.
3. Pin approved source endpoint + CIDR in `connectionnet.Policy` and force the actual SQL Server driver to dial only a `Plan.Addresses()` IP while verifying TLS hostname. Run negative tests for DNS rebinding, multi-answer DNS, wrong certificate, credential rotation and unexpected redirects.
4. Grant TEST using a verified identity and no shared admin token. From the GUI/API (once implemented), test a staged inactive SQLServer source and observe exactly one requested/one pass-or-fail audit for each attempt. Verify source account SELECT-only via server-side role inspection; **never use DML as a permission probe**.
5. Induce a synthetic Vault→PG version CAS conflict, confirm NEEDS_REVIEW. Record two-signer observation and compensation plan. Show that malformed, expired, wrong-tenant, revoked, competing-writer, missing/deleted Vault version and divergent metadata all keep the journal quarantined.
6. Under documented external writer-freeze and separate operator approval, perform the prescribed *manual* reconciliation using the approved actual source of truth, then fresh `target` evidence and separately signed `confirm_already_applied` only if state exactly matches. Check audit, idempotency, rollback and no credential exposure.
7. Run E2E pipeline with active named bindings in TEST only after the above gates; verify source ERP SELECT-only with account permissions, no unrestricted egress, and safe rollback/activation control. Security/IT owner formally signs a GO or retains NO-GO.

## Reproducible CI (non-production evidence)

- General Go/race/vet + PostgreSQL/ClickHouse/dbt/frontend: [Actions CI](https://github.com/dinhvansh/dataflow-engine/actions/workflows/ci.yml).
- Disposable Microsoft SQL Server 2022, seeded QA reader account and disposable PostgreSQL workspace TEST/audit integration: [SQL Server QA](https://github.com/dinhvansh/dataflow-engine/actions/workflows/sqlserver-readonly-qa.yml).
- Approved source fixture is `DataFlowFixtureQA` + `qa_erp_reader`. Any attempt to substitute a real Epicor source or change its grants without external explicit approval is prohibited.
- Go tests running with missing isolated fixture environment skip integrations rather than infer production reachability. Review the exact PR head's hosted CI conclusions and main post-merge status.

## Next implementation slices (do not mark DONE from this plan)

1. **V3.2B-5E-4E:** externally approved Vault writer-freeze/lease fencing with real metadata CAS and operator source-of-truth runbook; robust undo/incident drill. Requires real infrastructure/security owner.
2. **V3.2C-2B2:** operator-configured live driver opening from the admitted IP snapshot with verified TLS ServerName, bound to scoped Vault secret handle/version; TEST role/audit and budget/rate controls.
3. **V3.2C-2B3:** opt-in, direct-TLS scoped TEST API + enterprise UI only after B2 is verified. Then V3.3 *active* binding and multi-instance E2E must be tested separately.

Update [PROJECT_HANDOFF_LATEST](PROJECT_HANDOFF_LATEST.md) and [MASTER_ROADMAP_AND_STATUS](MASTER_ROADMAP_AND_STATUS.md) after each merged milestone, not when a branch or synthetic smoke test merely exists.
