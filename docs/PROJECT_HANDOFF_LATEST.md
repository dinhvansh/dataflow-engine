# DataFlow Engine — Latest Chat Handoff

**Snapshot:** 2026-10-18 · verified GitHub `main@bf6ff2d38823e0bdce29f0d96b980ee3b3b3deca` after V3.2B-5A PR #61 merge, before this docs update. **GitHub live state always wins**; read HEAD, open PRs and Actions at the start of every session.

**Required companion (FULL prior plans, delivered features, missing gates, acceptance criteria):**
**[MASTER_ROADMAP_AND_STATUS.md](MASTER_ROADMAP_AND_STATUS.md)**. Do not read only this short handoff and mistakenly assume all development plans are complete. Historical [implementation/roadmap.md](../implementation/roadmap.md) predates delivered milestones and has old checkboxes.

## Current project status

- Public repository: https://github.com/dinhvansh/dataflow-engine
- **Developer Preview only — NOT PRODUCTION READY.** Live Epicor SQL Server **SELECT-only**, no source INSERT/UPDATE/DELETE/DDL. QA/DDL/seed only in isolated, identity-verified disposable environments. No production ClickHouse changes.
- **Stack:** Go, SQL Server source, ClickHouse scratch, PostgreSQL metadata/queues, isolated dbt Core, React/TypeScript/Vite/Tailwind/shadcn, React Flow/xyflow.
- **UI design locked:** Signalroom Enterprise; Light white/off-white with #238047 green; Dark graphite #161D1B and volt #C0F273. Small radii, crisp thin lines, semantic success/warning/error. Do not switch wholesale to Ant Design.
- The user wants **actual implementation, GitHub PRs, tests, bug fixes and merges** rather than instructions or unnecessary Codex prompts. Vietnamese casual direct style, but honest QA claims.

## Merged work — code available (NOT a production certificate)

| Release area | Delivered | PR / evidence |
| --- | --- | --- |
| Older V0.4–V1.3 capabilities | Source discovery, bounded ingestion and Managed Sync; snapshot-key reconciliation (detection-only), SQL SELECT Job DAG, Transform Studio, dbt scratch View, lineage, Safe Table TEST publish/rollback, Auto Build A→B→C View dependency chain, copy-on-write Safe Incremental CDC TEST | Historical `implementation/release-status-v*.md`, especially [v1.3](../implementation/release-status-v13.md); see master roadmap for limitations |
| CI stability | GitHub Actions dedupe and green hosted runners since Public transition | PR #41 |
| Visual Pipeline V2.1 | xyflow canvas, draggable SQL Jobs, dependency edges, JSON round trip | PR #46 |
| Visual Pipeline V2.2 | DAG validate, guarded Save/reopen, overwrite confirmation | PR #47 |
| Visual Pipeline V2.3 | TEST Run confirmation, observed node statuses/errors/attempts via backend | PR #48 |
| Visual Pipeline V2.4 | Run **validated, saved, enabled** DAG right on canvas with server re-check and confirmation; not a heterogeneous ETL runtime | PR #49; 24 mocked browser tests on that feature |
| Connection Manager V3.1 | Redacted authorized **inventory** of fixed SQL Server, ClickHouse and PostgreSQL environment slots | PR #51; draft duplicate #50 closed |
| Connection Manager V3.2A | **Inactive lab-only** named connection Draft registry (UUID, PostgreSQL migration 017, admin token/feature flag, audit, UI). **NO endpoint, credential, Test Connection, runtime binding** | PR #52 merged at `71e97aca...`; PR head Go/Postgres/ClickHouse/dbt/Web CI and UI01–UI26 mocked browser PASS |
| Status consolidation after V3.2A | Full master roadmap/handoff and historical delivered-vs-pending inventory | PRs #53, #54; #54 docs-only main `e31c650`, CI PASS |
| Connection Manager V3.2B-1 (partial) | **Server-only** synthetic scope/Vault/Audit abstractions; independent VIEW/USE/MANAGE/TEST, secret redaction, fake providers | PR #55, GitHub CI run 39048231248 PASS |
| Connection Manager V3.2B-2 (partial) | Disconnected PostgreSQL migration 018: separate workspace grants, non-executable profile Drafts, opaque versioned reference with CAS, append-only audit; **no API wiring**. CI includes isolated PostgreSQL integration and revocation/cross-workspace denial | PR #57 merged (`4e097ce`), exact-head CI 39049689214 PASS; main CI 39049936996 PASS |
| Connection Manager V3.2B-3 (partial) | **Optional HashiCorp KV-v2 HTTPS adapter**, trusted server-only fixed provider config, CAS create/rotate, versioned read, blocked redirects, synthetic TLS QA; **no live Vault deployment/API** | PR #58 merged (`15510df`), exact-head CI 39050285981 PASS |
| Connection Manager V3.2B-4 (partial) | Offline OIDC/JWT **pinned RS256 verifier**, exact issuer/audience, expiration and signing-key checks, issuer-scoped pseudonymous actor and authenticated catalog facade | PR #59 merged (`aa7b545`), exact-head CI 39050767874 PASS |
| Connection Manager V3.2B-5A (partial) | **Opt-in, TLS-direct-only, read-only HTTP API** `GET /api/v1/secure/connections/definitions?workspace=...`, JWT/pinned-key bootstrap, workspace VIEW grant, revoked-access enforcement, metadata redaction, synthetic PostgreSQL/TLS/JWT integration. Defaults 404; existing plaintext Go listener cannot use it (403), until reviewed TLS deployment. **NO create/update, Vault/secret write, Test Connection or runtime binding.** | PR #61 merged (`bf6ff2d`), exact-head CI 39077211849 PASS. Main merge CI 39077422791 initiated; verify final before certification. See [V3.2B-5A details](connections-v32b5a-http-view.md). |

**Evidence:** PR heads #57 39049689214, #58 39050285981, #59 39050767874 and #61 39077211849 all GitHub CI PASS (Go race/vet, CI DB/dbt, Web build). Main #61 CI run 39077422791 initiated (re-check final status). Browser mocked UI 26-case PASS was on PR #52, not on a real credential UI. **No real Vault/IdP enrollment, live multi-instance or Epicor certification.** Re-check current workflows after further commits. At snapshot, **PR #25** sanitized screenshot gallery is still open, separate from active runtime work. Avoid merging it without inspection.

## Important plans that are NOT finished

1. **Connection Manager V3.2B (IN PROGRESS):** Foundation #55/#57/#58/#59 + V3.2B-5A read-only HTTP #61 merged. **NEXT V3.2B-5B:** approved TLS termination/IdP key lifecycle, managed workspace grant provisioning and revoked-grant QA; then audited **Vault↔PostgreSQL rotation outbox/reconciliation** (5C) with failure injection, before any credential writes or active bindings. New secured read API is **off by default** and direct-TLS-only; existing Go plaintext listener will return 403 when enabled, not a production route. Current legacy lab admin header is **not production RBAC**. No secret values in PostgreSQL plaintext/browser/Git.
2. **V3.2C:** guarded Test Connection with SSRF/egress allowlist, DNS/TLS controls, timeout, rate limits and audit; SQL Server read-only, ClickHouse scratch-only.
3. **V3.3:** immutable versioned connection ID binding into Sync Jobs/SQL Jobs/Pipelines; backward compatibility and safe rollback from environment connections. Multi-instance live routing **not implemented yet**.
4. **V3.4:** synthetic multi-instance E2E, restart/failure QA, permission and secret rotation tests; separate explicitly authorized read-only Epicor TEST acceptance.
5. **Unified product roadmap (P1, later):** true Source → Sync → ClickHouse → Transform/dbt → DQ → Publish **mixed-step orchestration**; dedicated DQ contracts/schema drift; atomic/safe multi-model publish; physically efficient incremental/late event/delete handling; tested 7m+ row throughput and Airbyte parity.
6. **Operations/product maturity (P2):** scheduling, webhooks, alerts, auditing, RBAC/SSO, incident/run retention, HA/backups/recovery, more Transform JOINs/lineage, deployment hardening, instrumented benchmarks. Optional report builder/AI only after explicit roadmap approval.

**Important false-completion traps:** Existing Visual Pipeline runs **saved SQL DAGs**, not the entire Source→Sync→Transform→DQ→Publish process. Incremental CDC is correct-by-construction scratch copy-on-write **not 7m-row fast incremental**. Reconciliation observes missing keys but **does not** apply source physical deletes. Existing Connection Draft names **cannot** make connections. Auto Build A→B→C is View-based, not multi-model atomic Table/Incremental publish. Do not claim production certification from synthetic CI.

## How to work in the next chat

1. Fetch live GitHub `main`, open PRs, branches, running/completed Actions and compare relevant changes; report divergence before edits.
2. Read **both this file and [MASTER_ROADMAP_AND_STATUS.md](MASTER_ROADMAP_AND_STATUS.md)**, then relevant source files and historical release evidence. Do not use old snapshot SHA as a hardcoded branch base.
3. Continue **V3.2B-5B/5C** in small security-first PRs: review TLS ingress/proxy trust, actual IdP signing-key rotation, workspace grant lifecycle, and implement a **durable, recoverable** Vault↔PostgreSQL rotation journal. V3.2B-5A safe read-only lab handler is merged but defaults OFF and rejects plaintext HTTP. No browser secret input, Test Connection, SQL Server egress, or ERP writes until security review.
4. For each PR: code + targeted tests, browser/Go/DB/dbt CI success on exact head, review scope, merge, recheck `main` CI; update docs after real merges.
5. Keep source ERP absolutely SELECT-only; all writes only isolated verified QA fixture. No secret exfiltration, destructive local cleanup or uncontrolled network probing.
6. Be honest about **Implemented / Synthetic PASS / Live QA BLOCKED or Pending / Production NO-GO**.

## Copy into a new chat

> Tiếp tục dự án **DataFlow Engine** từ GitHub https://github.com/dinhvansh/dataflow-engine. **BẮT BUỘC đọc đầy đủ** `docs/PROJECT_HANDOFF_LATEST.md` và `docs/MASTER_ROADMAP_AND_STATUS.md` ở branch `main`; đối chiếu `implementation/roadmap.md`, release notes và code thực tế. Kiểm tra GitHub main HEAD, PR đang mở, CI/Actions, conflict trước khi làm. Đọc phần **đã xong/chưa xong** của toàn bộ roadmap, không chỉ Connection Manager. Tiếp tục **V3.2B-5B/5C (TLS/IdP/workspace provisioning + Vault ↔ PG rotation journal/recovery)**; PR #61 V3.2B-5A read-only synthetic OIDC API đã merge, vẫn chưa bật kết nối. Sau đó V3.2C Test Connection, V3.3 binding, V3.4 E2E, rồi pipeline end-to-end/DQ/incremental optimization theo master roadmap. Mày trực tiếp làm code, PR, test, sửa và merge khi PASS; đừng chỉ đưa prompt. Bảo vệ ERP tuyệt đối read-only; mọi test ghi chỉ trên synthetic QA; không tự nhận sản phẩm production-ready. Giữ UI Signalroom Enterprise Light trắng xanh #238047 / Dark graphite volt #C0F273. Sau mỗi merge cập nhật hai file handoff/roadmap.

**No secrets in this handoff.**
