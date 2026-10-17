# DataFlow Engine — Master Roadmap, Status & Acceptance Gates

**Snapshot: 2026-10-18.** Checked against public `main@2c5b82d1c3304e1b9dad22903e6d2a1100797b18` after PR #65 merged, before this docs update. This is a planning/status report, not proof of production deployment. Live GitHub always wins.

Canonical handoff: [PROJECT_HANDOFF_LATEST.md](PROJECT_HANDOFF_LATEST.md). Original phased plan: [implementation/roadmap.md](../implementation/roadmap.md). Historical release evidence: [release-status-v13.md](../implementation/release-status-v13.md) and other `implementation/release-status-v*.md` files.

## 1. Product direction

**Vision:** DataFlow is a developer-oriented, visual ETL/ELT and orchestration workbench: discover sources → ingest → build ClickHouse raw/ODS → transform visually/SQL → dbt/model dependencies → data quality → controlled publish → monitor, investigate and recover.

Stack: Go API/workers; SQL Server source **SELECT-only**; ClickHouse for disposable TEST data/analytics; PostgreSQL metadata/queues/cursors; isolated dbt Core worker; React + TypeScript + Tailwind/shadcn-style primitives + React Flow/xyflow. Do not rewrite the frontend to Ant Design.

**Important separation of evidence:**
- **Implemented in repository** = functionality exists in code. This is NOT equivalent to approved deployment.
- **CI/Synthetic PASS** = automated tests pass on isolated/mocked inputs, not a real Epicor performance or security certificate.
- **Live TEST acceptance** = separate measured read-only SQL Server + PostgreSQL + ClickHouse verification, with explicit permission, data reconciliation and failure injection.
- **Production GO** = additional access security, operational readiness, monitoring, rollback, formal sign-off. **Current product remains Developer Preview / production NO-GO.**

## 2. Inventory: What has been built

| Area / original roadmap | Implementation status | Remaining gap |
| --- | --- | --- |
| Repo foundations / CI | **Implemented, CI green**: GitHub workflows, Go/React, PostgreSQL/ClickHouse/dbt synthetic tests; PR #41 Actions dedupe | Release automation, security scans/quality gates, audited deployment workflow |
| SQL Server discovery / full ingestion | **Developer Preview**: schema/PK/rowversion discovery, bounded readers/writers, TEST full-load paths | Actual Epicor TEST type coverage, heavy-table benchmark, approved source access |
| Managed Sync / incremental cursor | **Developer Preview**: PostgreSQL queue, durable leases/fencing/retries, checkpoint and ClickHouse verification | Real source transaction/snapshot edge cases, at-least-once behavior verification, operational HA |
| Source-key reconciliation | **Detection only**: key snapshots and missing/changed evidence | Reliable physical-delete source evidence, typed target application, ordering/replay guards; **never call it auto-delete** |
| SQL Jobs & DAG runtime | **Developer Preview**: saved SELECT jobs, DAG dependencies, PostgreSQL queue, run snapshots/states, retry/fencing | Mixed-step end-to-end Sync → Transform → DQ → Publish DAG, schedules/alerts, production permissions |
| Visual Transform Studio v0.8 | **Implemented**: Applied Steps, SQL compile/preview, save | Multi-table JOINs, curated operators/expressions, column-level lineage, enterprise DQ |
| dbt scratch Views v0.9 | **Implemented**: isolated dbt worker + scratch View build | Production model deployment security and multi-model release semantics |
| Data Lineage v1.0 | **Implemented**: saved model graph, impact, cycle guard | Full column-level lineage and more complex dependencies |
| Safe Table v1.1 | **TEST preview**: stage, DQ, explicit publish, UUID-aware atomic EXCHANGE, rollback | Real production-safe lifecycle, backup policy, cross-model atomic publish |
| Auto Build Dependencies v1.2 | **Implemented for View chains**: A → B → C, per-step DQ, restart-resumable parent state | Auto publish of Tables/Incremental, multi-model atomic release, richer business DQ |
| Safe Incremental CDC v1.3 | **TEST preview**: versioned insert/update/delete deltas, copy-on-write, publish/cursor/rollback | Truly efficient partial incremental/partition/key approach, source delete CDC, late-arrival rules, 7m+ row benchmark |
| Visual Pipeline Builder V2.1–V2.4 | **Implemented in React**: React Flow edit/drag/edges + JSON sync; validate; save/reopen; TEST run after confirmation; read-only run inspector; PRs #46–#49 | One full heterogeneous ETL pipeline canvas; production permissions; live cross-service QA |
| Connection Manager V3.1 | **Implemented**: authenticated redacted inventory of 3 environment slots; PR #51 | Inventory not reachability, not real multi-instance configuration |
| Connection Manager V3.2A | **Merged PR #52**: PostgreSQL named UUID **inactive lab-only DRAFTS**, separate admin token + opt-in, audit | NO endpoint/credentials, NO Test Connection, NO binding, NO execution |
| Connection Manager V3.2B-1 | **Merged #55**: offline scoped-policy/Vault/Audit contracts, redacted references and synthetic tests | Pure server-only foundation, not an endpoint |
| Connection Manager V3.2B-2 | **Merged #57**: migration 018 workspace/actor permissions, disconnected scoped profile Draft metadata, CAS reference staging, append-only audit with isolated PG QA | No live API or IdP; no vault/PG atomic completion |
| Connection Manager V3.2B-3 | **Merged #58**: optional server-only HashiCorp Vault KV-v2 HTTPS adapter, versioned read, CAS create/rotate, TLS synthetic tests | No actual Vault deployment, policies/secret provisioning, API or execution |
| Connection Manager V3.2B-4 | **Merged #59**: offline RS256/OIDC pinned issuer, audience/expiry/key check, verified pseudonymous actor and authenticated catalog facade | No real SSO/JWKS provisioning or workspace grant administration |
| Connection Manager V3.2B-5A | **Merged #61**: separate off-by-default **TLS-direct-only read-only** JWT-scoped workspace Draft list endpoint, startup config/schema gates, redacted DTO, revocation and synthetic PostgreSQL/TLS/JWT QA | Not useable on current plaintext Go listener; no true IdP enrollment or trusted HTTPS ingress, MANAGE API, Vault↔PG transaction, active connections or runtime binding. V3.2B still IN PROGRESS |
| Connection Manager V3.2B-5B | **Merged #63**: server-only workspace VIEW/USE/TEST grant/revoke, locked MANAGE authorization, idempotent transitions, append-only migration 019 grant audit; isolated PostgreSQL and synthetic JWT QA | No trusted IdP directory or live grant UI/API, MANAGE bootstrap out of scope |
| Connection Manager V3.2B-5C | **Merged #64**: durable migration 020 PREPARED/APPLIED/NEEDS_REVIEW journal, version-pinned fake vault proof, CAS metadata + immutable evidence, failure-injection/retry/revocation test | No actual external Vault CAS orchestration or operator reconciliation/compensation, credentials remain inactive |
| Connection Manager V3.2B-5D | **Merged #65**: isolated opt-in direct TLS 1.3 listener, private key/PEM validation, expiration and startup gate for secure read-only JWT route | TLS/cert provisioning and renewal not deployed, no production ingress/IdP/credential operations |
| Operations / UI | **Developer Preview**: run lists/status/errors and Signalroom Enterprise Light/Dark workspace | RBAC, alerts/SLOs, retention, long-run operations, role-tailored workflows |
| Airbyte replacement / performance | **Not accepted**: controlled comparison was proposed | Verified benchmark, migration playbook, parity checks, rollback and go/no-go |
| Epicor real acceptance | **Pending formal acceptance** despite synthetic QA evidence | Source SELECT-only proof, snapshot correctness, 7m-row/large-table test, reconcile, recovery |
| External reporting / AI reports | **Not this milestone**: ClickHouse can feed reports; report designer / NL-to-report not shipped here | Dedicated product decision, permission model and UX after core reliability |

**Do not treat the unchecked headings in the historical implementation roadmap as an up-to-date project completion matrix.** Its Phase 0–5 list is an initial plan; release-status notes and the actual source/CI are newer evidence.

## 3. Execution order (recommended, not silently approved for production)

### P0 — Connection Manager to real multi-instance lab operation

**V3.2B · Secure configuration, secrets and access control — IN PROGRESS (#55/#57/#58/#59/#61/#63/#64/#65 code merged; production NO-GO)**
- **Implemented V3.2B-1:** pure server-side workspace/actor scope checks (VIEW/USE/MANAGE/TEST) on a copied synthetic policy, redacted immutable secret-reference type, synthetic vault/audit contracts and fake-provider tests. PR #55 exact head CI run 39048231248 PASS. See [connections-v32b-security-core.md](connections-v32b-security-core.md). This does **not** secure the current V3.2A HTTP endpoints yet.
- **Implemented (synthetic only):** V3.2B-2 migration 018 workspace grants + revocation, separate inactive profile drafts, versioned reference CAS, application-transaction audit and append-only database audit trigger, integration on isolated PostgreSQL. PR #57 exact-head CI 39049689214 PASS. **Remaining:** real IdP/actor and workspace grant provisioning, fail-closed API route, dynamic permissions, production identity operations and secure migration rollout.
- **Implemented (synthetic only):** optional self-hosted HashiCorp KV-v2 HTTPS adapter with versioned read/CAS writes and fake TLS tests (#58 exact-head CI 39050285981 PASS); pinned offline RS256 OIDC verifier and verified actor facade (#59 exact-head CI 39050767874 PASS). **Remaining:** actual provider/IdP selection and credentials/keys/policies, certified TLS/egress, Vault↔PG dual-write transaction reconciliation, dynamic trusted API integration. Never put secrets in Git/browser/PG plaintext or logs.
- **Implemented V3.2B-5A**: off-by-default, direct-TLS only read-only OIDC scope-checked Draft-list handler, startup rejects missing public RSA key/isolated schema; synthetic unauthorized cross-workspace/revocation tests. #61 exact-head CI 39077211849 PASS. See [V3.2B-5A security](connections-v32b5a-http-view.md). **Remaining:** reviewed real TLS entrypoint (existing Go listener is HTTP), trusted IdP/JWKS lifecycle, safe workspace grant administration, protected MANAGE route, and durable Vault↔PG rotation journal/reconciliation.
- **Implemented 5B/5C/5D (synthetic only):** audited workspace grant lifecycle, detached crash-window rotation journal and a default-off direct TLS 1.3 listener. PR #63 CI 39079696879 PASS, #64 CI 39080149491 PASS, #65 CI 39080507766 PASS. See [grant audit](connections-v32b5b-grants.md), [rotation journal](connections-v32b5c-rotation-journal.md), [TLS](connections-v32b5d-direct-tls.md). **Still required:** real IdP/JWKS key lifecycle, trusted subject/workspace enrollment and MANAGE bootstrap, real Vault token/policy/egress, operator journal reconciliation and identity-safe MANAGE endpoints. Do not imply a real multi-connection service is operational.
- Define endpoint allowlist/network egress policy before accepting host names. Keep feature gated and QA-only, do not activate arbitrary sources.
- Unit/integration QA: unauthorized cross-workspace read/write denied, secret values never returned, rotation & audit, disabled feature fail-closed.
- **Exit:** metadata + vault + permission layer proven on synthetic credentials; **no network connection yet**.

**V3.2C · Bounded Test Connection**
- Add per-connection server-side TEST action with authorization, timeout, TLS/cert policy, DNS and IP-range controls, SSRF/redirect defense, rate limits, audit.
- SQL Server role only SELECT/schema reads. ClickHouse target only isolated scratch capabilities; no real ERP writes.
- UI displays bounded status, latency and sanitized reason; never credentials or raw driver errors.
- **Exit:** synthetic SQL Server + ClickHouse connect/fail/time-out/unreachable cases pass; unauthorized and internal-network probes blocked. A configured slot is not automatically “Connected”.

**V3.3 · Immutable binding into job and pipeline**
- Bind saved Sync Jobs, SQL Jobs and Pipeline nodes to stable connection IDs + revisions. Keep backward compatibility/migration from environment-level single connection.
- Freeze binding snapshot at run time. Version/effective-state checks, authority at execution, safe migration/rollback, audit.
- **Exit:** two independent synthetic sources/targets demonstrate correct routing; invalid/revoked connection stops execution; zero cross-connection leakage.

**V3.4 · Connection-specific QA and operations**
- Realistic synthetic parallelism, unavailable database, credential rotation, network failure and resume, idempotency.
- Only after consent and security review: approved **Epicor TEST SELECT-only** read validation with no source DML/DDL.
- **Exit:** reproducible QA report, permission matrix, read-only source evidence, safe release checklist.

### P1 — Unified ETL/ELT workflow (the earlier core product plan)

**V4.0 · End-to-end heterogeneous pipeline orchestration**
- Evolve from SQL-Job-only DAG to a **mixed node type canvas**: Source/Sync → ClickHouse raw/ODS → Transform/dbt → DQ → Publish → optional notification.
- Existing safe backend runtimes and PostgreSQL leasing must be reused; do not fake a pipeline by merely drawing nodes.
- Dependencies A→B→C, frozen run snapshots and observable lifecycle, retry/cancel/recovery; accurate blocked/skipped downstream semantics.
- **Exit:** one click executes a synthetic approved full flow from source through published target, with injected failure and zero unintended downstream run.

**V4.1 · Data Quality & release contracts**
- Explicit schema drift checks, nullable/unique/row-count/freshness and cross-table reconciliations (configurable threshold/owner), pre/post materialization checks.
- Gate Publish when any mandatory DQ fails, capture evidence, preserve old version, expose safe rollback.
- Business-level contracts and lineage impact. Do not claim quality from count-only checks.
- **Exit:** corrupted input, duplicate keys, missing upstream, changed schema all fail closed; release/rollback tested.

**V4.2 · Production-efficient incremental**
- Remove copy-on-write whole-table limitation where proven safe; design partition/key semantics, update/delete ordering and late arrivals.
- Separate source delete evidence (Epicor rowversion **does not prove physical deletes**) from applying safe tombstones to target.
- Restart replay, cursor monotonicity, fencing, idempotency, backfill and bounded resource use.
- **Exit:** 7m+ row synthetic/approved TEST benchmark with correctness+latency+RAM metrics and crash fault matrix; no production switch without acceptance.

### P2 — Operational maturity, reporting and productization
- Schedules / external triggers / webhook idempotency and time zones, alerting, incidents, SLA/SLO, history/retention, dashboard resource/performance traces.
- Expanded Transform Studio multi-source JOINs, column lineage, Preview cost guard, model/version branching and rollback.
- RBAC beyond Connection Manager: enterprise user identity/SSO, per-workspace resource scopes, developer/operator/reviewer separation.
- HA, backup/restore, deployment configuration & secret rotation, security scanning, disaster-recovery drills, staged releases.
- Measured Airbyte shadow-benchmark and cutover plan (record count, checksums, runtime, CPU/RAM, lag, rollback). **Do not decommission Airbyte until independently validated.**
- Optional report builder / AI assistance only after security/reliability and an explicit product roadmap decision.

## 4. Demo / funding readiness tracks

| Demo track | Can honestly demonstrate now | Still needed for stronger story |
| --- | --- | --- |
| Developer UI | Signalroom Light/Dark, Transform, DAG Builder, Inspector, Connection Drafts | Unified heterogeneous canvas and operator workflow |
| Controlled correctness | dbt View/TEST Table/TEST incremental, DQ, publish + rollback, bounded synthetic QA | Independent full synthetic E2E across the complete flow and documented acceptance |
| Multiple clients/connections | Multiple **draft names** and fixed env slots | Vault, scoped RBAC, real Test Connection, versioned binding |
| Scalability | CI integration with isolated DBs; prior selective local synthetic QA evidence | Instrumented 7m+ dataset benchmark, failure/recovery and Airbyte comparison |
| Enterprise readiness | Read-only ERP rule, opt-in scratch guards, admin-gated Draft | Security review, user RBAC/SSO, monitoring/on-call, backups, production approval |

**Investor-demo wording:** “Working developer preview with a verified synthetic CI foundation and an ongoing secure multi-connection roadmap.” Do **not** advertise multi-instance live connectivity, fully automated source deletes, production-grade incremental performance, or Epicor production certification as completed.

## 5. Verification and release discipline

1. Before every edit: get GitHub HEAD, open PRs (#25 screenshot gallery was open in this snapshot), workflow conclusions, differences/conflicts; warn if head moved. Never override local uncommitted user work.
2. Work on isolated feature branches; focused PRs Ready for review, exact SHA checked. CI: Go race/unit/vet, PostgreSQL/ClickHouse/dbt integration, frontend production build, Playwright mock UI; add security/performance tests per risk. Fix failures before merge; after merge verify new main SHA.
3. For local E2E, only identity-verified disposable containers; no global Docker prune, no unknown-source write tests. ERP is SELECT-only even in TEST unless explicitly and independently approved (do not assume source DML).
4. Separate feature completion (implemented), synthetic tests (PASS), local real-service tests (independently measured) and production sign-off. Keep evidence and blockers explicit.
5. Keep canonical `docs/PROJECT_HANDOFF_LATEST.md` and this roadmap updated **after every major feature merge**; don't allow summaries to overwrite actual code or CI evidence.

## 6. Immediate next session

1. Confirm current `main`, PRs #63–#65 merged, latest GitHub main CI and PR #25 screenshot-gallery still open; report divergence before edits.
2. V3.2B-1..4 plus 5A–5D are merged as synthetic/lab-only code, **not deployed to users**. Real IdP/JWKS, Vault token/policy, HTTPS cert lifecycle, trusted MANAGE grant bootstrap, protected API and operator-assisted rotation recovery remain unresolved.
3. Proceed to **V3.2B-5E** only with explicitly verified IdP/Vault trust policies and an audit-safe operator reconciliation path. Do not activate credential writes or arbitrary Test Connection; V3.2C SSRF/network allowlist and source SELECT-only verification still block actual data-source dialing.
4. Update this file and handoff once milestones actually merge.

**Canonical restart instruction:** “Đọc `docs/PROJECT_HANDOFF_LATEST.md` và `docs/MASTER_ROADMAP_AND_STATUS.md` trong https://github.com/dinhvansh/dataflow-engine; kiểm tra GitHub main/PR/Actions mới nhất; tiếp tục V3.2B-5E thực tế IdP/Vault/TLS trusted provisioning và an toàn operator recovery; không bật credentials/Test Connection, không chạm ERP ghi dữ liệu.”
