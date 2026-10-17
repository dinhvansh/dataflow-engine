# DataFlow Engine — Latest Chat Handoff

**Snapshot:** 2026-10-17 · verified GitHub `main@e56ddf38624ace8e7caa4e2817d2ceace83dc912` before this docs update. **GitHub live state always wins**; read HEAD, open PRs and Actions at the start of every session.

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
| Status docs after V3.2A | Earlier handoff refreshed; current HEAD became docs-only merge commit | PR #53, `main@e56ddf38624ace8e7caa4e2817d2ceace83dc912`, GitHub CI PASS |

**Evidence:** main CI run 39040829060 PASS for `e56ddf38624ace8e7caa4e2817d2ceace83dc912`; browser UI 26-case mocked QA PASS on PR #52 tested head / merge commit, not a live ERP test. Re-check current workflows after further commits. At snapshot, **PR #25** sanitized screenshot gallery is still open, separate from active runtime work. Avoid merging it without inspection.

## Important plans that are NOT finished

1. **Connection Manager V3.2B (NEXT):** server-side vault/secret references, rotation/redaction, permission scopes (VIEW/USE/MANAGE/TEST), workspace/actor audit, separate secure configuration. Existing lab admin header is **not production RBAC**. No hostname/credentials stored in PostgreSQL plaintext/browser/Git.
2. **V3.2C:** guarded Test Connection with SSRF/egress allowlist, DNS/TLS controls, timeout, rate limits and audit; SQL Server read-only, ClickHouse scratch-only.
3. **V3.3:** immutable versioned connection ID binding into Sync Jobs/SQL Jobs/Pipelines; backward compatibility and safe rollback from environment connections. Multi-instance live routing **not implemented yet**.
4. **V3.4:** synthetic multi-instance E2E, restart/failure QA, permission and secret rotation tests; separate explicitly authorized read-only Epicor TEST acceptance.
5. **Unified product roadmap (P1, later):** true Source → Sync → ClickHouse → Transform/dbt → DQ → Publish **mixed-step orchestration**; dedicated DQ contracts/schema drift; atomic/safe multi-model publish; physically efficient incremental/late event/delete handling; tested 7m+ row throughput and Airbyte parity.
6. **Operations/product maturity (P2):** scheduling, webhooks, alerts, auditing, RBAC/SSO, incident/run retention, HA/backups/recovery, more Transform JOINs/lineage, deployment hardening, instrumented benchmarks. Optional report builder/AI only after explicit roadmap approval.

**Important false-completion traps:** Existing Visual Pipeline runs **saved SQL DAGs**, not the entire Source→Sync→Transform→DQ→Publish process. Incremental CDC is correct-by-construction scratch copy-on-write **not 7m-row fast incremental**. Reconciliation observes missing keys but **does not** apply source physical deletes. Existing Connection Draft names **cannot** make connections. Auto Build A→B→C is View-based, not multi-model atomic Table/Incremental publish. Do not claim production certification from synthetic CI.

## How to work in the next chat

1. Fetch live GitHub `main`, open PRs, branches, running/completed Actions and compare relevant changes; report divergence before edits.
2. Read **both this file and [MASTER_ROADMAP_AND_STATUS.md](MASTER_ROADMAP_AND_STATUS.md)**, then relevant source files and historical release evidence. Do not use old snapshot SHA as a hardcoded branch base.
3. Start V3.2B in **small reviewable security-first PRs**. Clarify vault provider, deployment/egress assumptions and user/workspace identity if necessary; do not silently choose a production storage or external connection endpoint. Prefer an abstraction + synthetic tests until assumptions are approved.
4. For each PR: code + targeted tests, browser/Go/DB/dbt CI success on exact head, review scope, merge, recheck `main` CI; update docs after real merges.
5. Keep source ERP absolutely SELECT-only; all writes only isolated verified QA fixture. No secret exfiltration, destructive local cleanup or uncontrolled network probing.
6. Be honest about **Implemented / Synthetic PASS / Live QA BLOCKED or Pending / Production NO-GO**.

## Copy into a new chat

> Tiếp tục dự án **DataFlow Engine** từ GitHub https://github.com/dinhvansh/dataflow-engine. **BẮT BUỘC đọc đầy đủ** `docs/PROJECT_HANDOFF_LATEST.md` và `docs/MASTER_ROADMAP_AND_STATUS.md` ở branch `main`; đối chiếu `implementation/roadmap.md`, release notes và code thực tế. Kiểm tra GitHub main HEAD, PR đang mở, CI/Actions, conflict trước khi làm. Đọc phần **đã xong/chưa xong** của toàn bộ roadmap, không chỉ Connection Manager. Tiếp tục **V3.2B bảo mật vault/RBAC** bằng PR nhỏ theo CI, sau đó V3.2C Test Connection, V3.3 binding, V3.4 E2E, rồi pipeline end-to-end/DQ/incremental optimization theo master roadmap. Mày trực tiếp làm code, PR, test, sửa và merge khi PASS; đừng chỉ đưa prompt. Bảo vệ ERP tuyệt đối read-only; mọi test ghi chỉ trên synthetic QA; không tự nhận sản phẩm production-ready. Giữ UI Signalroom Enterprise Light trắng xanh #238047 / Dark graphite volt #C0F273. Sau mỗi merge cập nhật hai file handoff/roadmap.

**No secrets in this handoff.**
