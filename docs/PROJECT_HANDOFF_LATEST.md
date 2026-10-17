# DataFlow Engine — Latest Chat Handoff

**Snapshot:** 2026-10-17 · verified GitHub `main@aa7b5452f64cdcf60a020449f57cae0b459926aa` after V3.2B-4 PR #59 merge, before this docs update. **GitHub live state always wins**; read HEAD, open PRs and Actions at the start of every session.

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
| Connection Manager V3.2B-4 (partial) | Offline OIDC/JWT **pinned RS256 verifier**, exact issuer/audience, expiration and signing-key checks, issuer-scoped pseudonymous actor, authenticated Catalog facade. **Not wired to HTTP/real IdP** | PR #59 merged (`aa7b545`), exact-head CI 39050767874 PASS |

**Evidence:** PR heads #57 39049689214, #58 39050285981, #59 39050767874 all GitHub CI PASS (Go race/vet, CI DB/dbt, Web build). Main #57 CI 39049936996 PASS. Main push for #59 (`aa7b545`) was started as run 39050989875 (recheck final result). Browser mocked UI 26-case PASS was on PR #52, not on any new live credential UI. **No real Vault/IdP, real multi-instance or Epicor certification.** Re-check current workflows after further commits. At snapshot, **PR #25** sanitized screenshot gallery is still open, separate from active runtime work. Avoid merging it without inspection.

## Important plans that are NOT finished

1. **Connection Manager V3.2B (IN PROGRESS):** V3.2B-1..4 core pieces merged (#55/#57/#58/#59). **NEXT V3.2B-5:** choose/approve real IdP deployment configuration, Vault KV-v2 policy/token provisioning, fail-closed API integration, actor/workspace grant lifecycle and rotation outbox/reconciliation for Vault↔PostgreSQL. No current secure Catalog endpoint, connection activation or live secret setup. Existing lab admin header is **not production RBAC**. No credentials in PostgreSQL plaintext/browser/Git. See [V3.2B-2](connections-v32b2-scoped-metadata.md), [KV-v2](connections-v32b3-vault-kvv2.md), [OIDC](connections-v32b4-oidc.md).
2. **V3.2C:** guarded Test Connection with SSRF/egress allowlist, DNS/TLS controls, timeout, rate limits and audit; SQL Server read-only, ClickHouse scratch-only.
3. **V3.3:** immutable versioned connection ID binding into Sync Jobs/SQL Jobs/Pipelines; backward compatibility and safe rollback from environment connections. Multi-instance live routing **not implemented yet**.
4. **V3.4:** synthetic multi-instance E2E, restart/failure QA, permission and secret rotation tests; separate explicitly authorized read-only Epicor TEST acceptance.
5. **Unified product roadmap (P1, later):** true Source → Sync → ClickHouse → Transform/dbt → DQ → Publish **mixed-step orchestration**; dedicated DQ contracts/schema drift; atomic/safe multi-model publish; physically efficient incremental/late event/delete handling; tested 7m+ row throughput and Airbyte parity.
6. **Operations/product maturity (P2):** scheduling, webhooks, alerts, auditing, RBAC/SSO, incident/run retention, HA/backups/recovery, more Transform JOINs/lineage, deployment hardening, instrumented benchmarks. Optional report builder/AI only after explicit roadmap approval.

**Important false-completion traps:** Existing Visual Pipeline runs **saved SQL DAGs**, not the entire Source→Sync→Transform→DQ→Publish process. Incremental CDC is correct-by-construction scratch copy-on-write **not 7m-row fast incremental**. Reconciliation observes missing keys but **does not** apply source physical deletes. Existing Connection Draft names **cannot** make connections. Auto Build A→B→C is View-based, not multi-model atomic Table/Incremental publish. Do not claim production certification from synthetic CI.

## How to work in the next chat

1. Fetch live GitHub `main`, open PRs, branches, running/completed Actions and compare relevant changes; report divergence before edits.
2. Read **both this file and [MASTER_ROADMAP_AND_STATUS.md](MASTER_ROADMAP_AND_STATUS.md)**, then relevant source files and historical release evidence. Do not use old snapshot SHA as a hardcoded branch base.
3. Continue **V3.2B-5** in small PRs: securely provision actual IdP verification keys, Vault endpoint/token/policies and actor/workspace grants; add fail-closed HTTP handler and durable rotation reconciliation only when trusted deployment boundaries are established. Existing KV-v2 provider is optional and not deployed. Never enable arbitrary Test Connection or new live ERP destinations before allowlist and source SELECT-only review.
4. For each PR: code + targeted tests, browser/Go/DB/dbt CI success on exact head, review scope, merge, recheck `main` CI; update docs after real merges.
5. Keep source ERP absolutely SELECT-only; all writes only isolated verified QA fixture. No secret exfiltration, destructive local cleanup or uncontrolled network probing.
6. Be honest about **Implemented / Synthetic PASS / Live QA BLOCKED or Pending / Production NO-GO**.

## Copy into a new chat

> Tiếp tục dự án **DataFlow Engine** từ GitHub https://github.com/dinhvansh/dataflow-engine. **BẮT BUỘC đọc đầy đủ** `docs/PROJECT_HANDOFF_LATEST.md` và `docs/MASTER_ROADMAP_AND_STATUS.md` ở branch `main`; đối chiếu `implementation/roadmap.md`, release notes và code thực tế. Kiểm tra GitHub main HEAD, PR đang mở, CI/Actions, conflict trước khi làm. Đọc phần **đã xong/chưa xong** của toàn bộ roadmap, không chỉ Connection Manager. Tiếp tục **V3.2B-5 API có OIDC/RBAC thật + Vault ↔ PG rotation outbox** (V3.2B-1..4 #55/#57/#58/#59 đã merge, vẫn chưa bật kết nối), sau đó V3.2C Test Connection, V3.3 binding, V3.4 E2E, rồi pipeline end-to-end/DQ/incremental optimization theo master roadmap. Mày trực tiếp làm code, PR, test, sửa và merge khi PASS; đừng chỉ đưa prompt. Bảo vệ ERP tuyệt đối read-only; mọi test ghi chỉ trên synthetic QA; không tự nhận sản phẩm production-ready. Giữ UI Signalroom Enterprise Light trắng xanh #238047 / Dark graphite volt #C0F273. Sau mỗi merge cập nhật hai file handoff/roadmap.

**No secrets in this handoff.**
