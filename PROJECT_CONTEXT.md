# DataFlow Engine — Project Context / Chat Handoff

> **Canonical current status:** [Latest handoff](docs/PROJECT_HANDOFF_LATEST.md) + [Full master roadmap, completion matrix and acceptance gates](docs/MASTER_ROADMAP_AND_STATUS.md). This long original context is historical; always check current GitHub main/PRs/Actions before using old SHAs or runner observations.


> **Purpose:** Persistent, public-safe source of truth for resuming the DataFlow project in a new ChatGPT or Codex conversation without carrying a huge chat transcript.
>
> **Snapshot:** 2026-10-17, checked against GitHub `main` at `ac3a794869eb3a22b2c8fba2055343cdfe42c103`. This SHA, PR statuses, CI and QA statuses are a **point-in-time snapshot, not permanent facts**. At every new session, inspect the live repository before drawing conclusions.
>
> **User language:** Mostly Vietnamese, casual and direct. The user expects actual implementation, commits, reviews and measured tests, not merely advice or prompts. Do not pretend that QA screenshots are live app screenshots.

## 0. Read me first — the 60-second briefing

- **Repository:** https://github.com/dinhvansh/dataflow-engine
- **Vision:** Developer-oriented ETL/ELT + orchestration workbench: `source discovery → ingestion → raw ClickHouse → visual/SQL transform → dbt → DAG/quality → safe publish → inspect`.
- **Backend:** Go 1.23+, SQL Server read-only source, PostgreSQL 16 metadata/queues/checkpoints, ClickHouse 25.3 analytics/scratch, isolated dbt Core 1.11.0 / dbt-clickhouse 1.10.3.
- **Web:** React 18, TypeScript, Vite, Tailwind, shadcn-style primitives, `@xyflow/react`/React Flow. Do **not** replace with Ant Design.
- **UI identity:** DataFlow / Signalroom Enterprise. Light = white `#FFFFFF` panels, neutral off-white `#FAFBFA` page, brand/logo green `#238047`. Dark = graphite `#161D1B` background + volt `#C0F273` accent. Error/warning/success semantic colors remain distinct.
- **Critical safety:** The real Epicor/ERP SQL Server is strictly **SELECT-only** and is not the QA fixture. **Never execute any source DML/DDL or seed test data in the actual ERP.** Any destructive tests belong only in an identity-verified isolated Docker QA environment and disposable targets. Never modify corporate security/DLP.
- **Git practice:** Before every coding/QA task inspect `main`, branch, open PRs, divergence, conflicts and working tree. Preserve uncommitted user work. Prefer small reviewable PRs with clear base/HEAD SHA. Do not merge/rebase blindly or make unauthorized production changes.
- **Evidence standard:** CI green, synthetic browser QA, live synthetic local E2E, real ERP read-only tests and production certification are different claims. Never report unexecuted, blocked or runner-unassigned tests as PASS.
- **Next step:** Verify any new Codex Local QA handoff (the user has reported local tests completed), then address open UI/CI PRs and choose the next feature milestone. Proposed next release is a real multi-instance Connection Manager, **not yet an approved implementation decision**.

## 1. Boundaries and collaboration rules

1. At the start of each task: check GitHub HEAD, open PRs, current changes, branch topology, mergeability, CI and known blockers. Announce divergence/conflicts **before touching code**.
2. Implement in source when instructed; create a branch and PR, add targeted regression tests, report commit SHA, test results and issues. Do not answer “ask someone to review” when the task is to review/fix.
3. Do not edit API behavior, Go execution logic, ClickHouse, PostgreSQL schema or ERP connections **merely to fix UI**. Functional milestones require a separate explicit scope and QA.
4. Test on isolated QA infrastructure. Don't stop/delete unrelated containers, volumes or services. Do not use global Docker prune. Bind QA services to loopback and redact credentials.
5. For physical source deletes, rowversion alone is insufficient. Key-snapshot evidence is distinct from safely applying typed tombstones to target data. Fail closed when late/out-of-order events may be lost.
6. Frontend mock data may be used for **clearly labeled** mock screenshots. Don't say a synthetic HTML render is a screenshot of the real React app, or claim live availability/performance without measured evidence.
7. Keep color/status semantics. No bright mint page-wide tint or blue primary brand accent. Small radii (3–5px), thin lines, technical mono annotations, compact table and inspector, subtle grid only on graph.
8. User prefers **Codex Local as CI runner** while GitHub Actions dispatch may be unavailable. Capture exact commands/logs/exit codes, output checksums and artifact paths.
9. Keep this document up to date **at feature/merge/QA milestones** (or in a follow-up docs PR). Update status via live source, not memory. Avoid recording secrets, private customer details, or corporate data here.

## 2. Current architecture and implemented capabilities

| Area | Implemented in code (not necessarily production certified) | Important constraints |
| --- | --- | --- |
| Source | SQL Server schema/table/PK/rowversion discovery | SQL Server/ERP strictly read-only |
| Ingestion | Bounded Go extraction/full and native rowversion trials, queue/worker/checkpoint | Only safe test target allowed; physical DELETE needs separate handling |
| Managed Sync | PostgreSQL-persisted runs, leases, retries, fencing and cursor promotion | Verify source snapshot/ordering and restart correctness |
| ClickHouse | Scratch writer, SELECT preview and data reconciliation | Strong namespace/user separation; never reuse operational ClickHouse for destructive tests |
| Transform Studio | Visual filter/select/rename/calculate/group/sort/limit; SQL mode; generated SQL; step preview; saved models | Existing operators and single-source scope are limited; no unrestricted SQL |
| SQL Jobs / DAG | SELECT-only SQL Job definitions, DAG executor, durable run/history/retries; visible node states | DAG snapshots/observed run states must be truthful |
| Data Lineage | Metadata dependency graph, downstream impact, cycle prevention | Does not imply full SQL/column-level lineage |
| dbt | Isolated worker building scratch Views | Auto Build dependencies are View-based, with per-step DQ |
| Auto Build | One selected output Model C triggers A→B→C in order; stops downstream on upstream failure | Does not guarantee atomic multi-model publication |
| Safe Table | Stage → quality checks → explicit Publish → ClickHouse Atomic EXCHANGE → UUID-guarded rollback | Scratch-only; size/approval guardrails |
| Incremental CDC | Versioned INSERT/UPDATE/DELETE delta window; stage → explicit publish + cursor commit → rollback | Copy-on-write physical staging; late events/source ordering must be verified |
| Snapshot reconciliation | Added/missing/changed source-key detection | Source physical deletes and tombstone *evidence* require careful distinction |
| Hard-delete evidence | Additional v1.4 key-snapshot and guard code merged after earlier UI work | **Do not claim automatic deletion application is production-safe** without new evidence |
| Operations Center | Run history, SQL DAG node details, React Flow graph, status/error/attempt inspector | Historic topology may use current DAG if no versioned graph snapshot; disclose limitation |
| Connector Hub | Responsive source/destination catalog, route preview, SQL Server Explorer | Mostly UI scaffolding. **No real multi-instance connection registry yet** |

## 3. UI decisions (explicit user approvals)

- **Brand:** keep existing green logo and coherent brand-green `#238047` on active tabs/navigation, focus, buttons and small signals in Light Mode; **white pages and panels**, no mint tint. Dark retains `#C0F273` highlights over graphite.
- **Enterprise information hierarchy:** filters/search → indicators → main table/workspace → details/inspector. Avoid oversized generic KPI cards, gradients and inconsistent spacing.
- **React Flow:** true dependency edges for DAGs, viewport controls and minimap; read-only outcome display, not an editable workflow unless separately implemented.
- **Shadcn-style components:** preserve React/TypeScript and application APIs. Don't switch to Ant Design because of previous UI discussions.
- **Core screens:** Overview, Connections / Source Explorer, Sync Jobs, Transform Studio, SQL Jobs, Data Lineage, Operations Center, Pipelines, Incremental CDC, Run Inspector, Scheduler and Reconciliation.
- **Reference document:** [web/DESIGN_IDENTITY.md](web/DESIGN_IDENTITY.md).
- **Testing:** visual preview screenshots are for design feedback; automated Playwright mock routes do not validate live database behavior.

## 4. Git and PR status — snapshot only (2026-10-17)

At snapshot, `main` HEAD = `ac3a794869eb3a22b2c8fba2055343cdfe42c103`.

### Integrated into main

- UI PR **#28–#38** merged sequentially, plus **#42** fractional Light CSS selector fix (integrated into #38 before merging). They introduced Connector Hub, dual theme, Overview, Operations Center, React Flow DAG, Signalroom identity, SQL/Transform Studio, shared design-system styles.
- Earlier backend v0.x–v1.3 milestones (dbt, Lineage, Safe Table, Auto Build, Incremental) already in main.
- v1.4-related safety/recovery/hard-delete PR **#21, #22, #26, #23**, and disk-guard follow-up **#43**, have also been merged by snapshot. Always inspect fresh code before claiming the exact semantics/coverage.
- Current HEAD may advance while chat is active; re-fetch `main` before making a PR.

### Still open at snapshot

| PR | Purpose | Disposition |
| --- | --- | --- |
| [#39](https://github.com/dinhvansh/dataflow-engine/pull/39) | Full frontend theme/consistency audit | Base main, **merge conflict (dirty)** at snapshot; requires careful rebase/cherry-pick and tests |
| [#40](https://github.com/dinhvansh/dataflow-engine/pull/40) | SQL Editor line gutter and Source Explorer searches | Stacked on #39; evaluate after #39 resolution |
| [#41](https://github.com/dinhvansh/dataflow-engine/pull/41) | Deduplicate Actions triggers/runner usage | Independent workflow change; assess runner/account limits |
| [#25](https://github.com/dinhvansh/dataflow-engine/pull/25) | Sanitized UI screenshot gallery | Evidence/doc-only, manual review |

**Do not assume stale PR numbers/status remain correct.** Use live GitHub PR/branch/check APIs and report changed dependencies before merge.

### CI status nuance

After merge, some GitHub Actions runs returned FAIL with **no assigned runner and zero steps**. This is an infrastructure dispatch/quota/billing candidate, **not proof that code tests failed**, but also **not proof of PASS**. The user can run CI on their local Codex Windows/Docker test stack. Require a SHA-pinned local report and checksums, especially after recent merges.

## 5. QA and release safety

- QA mission & 68-case catalog: [CODEX_LOCAL_QA.md](CODEX_LOCAL_QA.md), [qa/local-acceptance/START_PROMPT_FOR_CODEX.md](qa/local-acceptance/START_PROMPT_FOR_CODEX.md), [qa/local-acceptance/TEST_CASE_CATALOG.md](qa/local-acceptance/TEST_CASE_CATALOG.md).
- User reported Codex Local completed testing, but **a new assistant must verify the actual handoff or artifacts before stating overall PASS/production readiness**. Preserve old results, compare exact SHA and fixture.
- Synthetic QA environment recommended: project-specific Docker SQL Server fixture + PostgreSQL + ClickHouse Atomic `scratch` + dbt worker + Go API + Vite/React. Source seed admin separate from app read-only login. Never enable write paths against actual ERP.
- **Quick checks** (from current workflows):
  - `cd web && npm ci && npm run build`.
  - Start Vite on `127.0.0.1:5173`; `python -m pytest qa/browser/test_console.py -v`, Python 3.12 / Playwright 1.56.0 / pytest 8.4.2; synthetic mock routes only.
  - `go test -race ./...`, `go vet ./...`, `go build ./cmd/...`.
  - Integration tests from [.github/workflows/ci.yml](.github/workflows/ci.yml), **against identity-verified disposable PostgreSQL and ClickHouse ONLY**; dbt pinned and SQL Server synthetic E2E separately.
- Ensure SQL Server source grants only SELECT and known schema exploration. Allow DML/DDL only in the isolated test fixture, NEVER in the actual ERP.
- Handoff should include `HANDOFF_TO_CHATGPT.md`, test matrix, reconciliation, bug log, commands, performance and sanitized ZIP. Tests are PASS/FAIL/BLOCKED/NOT RUN — never convert “not executed” to PASS.
- Source `SysRevID` rowversion may not reflect physical deletes; beware late arrivals, stale cursors, ambiguous publish after crash and recovery. Require zero-mismatch evidence for scoped tests.
- Treat production deployment as **NO-GO until release gates (security/RBAC, acceptance, Epicor source read-only validation, operational safeguards) are explicitly signed off**, regardless of synthetic PASS.

## 5A. Backend safety QA gate — independently reviewed 2026-10-17

**Decision:** **CONDITIONAL GO** to continue new feature development on isolated DEV/QA branches and synthetic databases; **NO-GO** for Epicor production deployment, operational DB changes, or live hard-delete application.

Evidence hierarchy:

1. [PR #20](https://github.com/dinhvansh/dataflow-engine/pull/20) contains the earlier synthetic full-stack QA report **52 PASS / 0 FAIL / 1 BLOCKED / 3 N/A** (base commit `61cfd7a7...`), including live disposable-DB Playwright 6/6, SQL Server 10,009-row sync with zero PK/count mismatch, late-event rejection, dbt/Auto Build, publish/rollback and recovery. **PR #20 is closed unmerged**, because raw traces/screenshots are not suitable for public `main`. Treat these archived artifacts as earlier evidence, not post-merge test results.
2. [PR #43](https://github.com/dinhvansh/dataflow-engine/pull/43) reports a **more recent local full acceptance of 67 PASS / 0 FAIL / 0 BLOCKED / 1 optional NOT RUN** and a disk-capacity guard regression; it is merged. The **underlying 67-case logs and exact tested SHA were not independently available during this review**, so that assertion is a PR-reported result, not an independently verified PASS of the current `main`.
3. On `main`: [PR #21](https://github.com/dinhvansh/dataflow-engine/pull/21) includes explicit synthetic opt-in, QA label/database/loopback guards and source read-only checks; [#22](https://github.com/dinhvansh/dataflow-engine/pull/22) + [#26](https://github.com/dinhvansh/dataflow-engine/pull/26) add two-complete-snapshot missing-key evidence, typed 8-byte rowversion, stale-approval fencing, and **`applied_to_target=false`**, not a production CDC/tombstone adapter. [#23](https://github.com/dinhvansh/dataflow-engine/pull/23) adds recovery-to-success and resource measurements. Hard-delete still needs a typed target adapter and ordering contract before any live application.
4. GitHub Actions currently have **runner-unassigned / zero-step failures** on some commits; this is **not a code test failure but not a PASS either**. Ask Codex Local for sanitized `HANDOFF_TO_CHATGPT.md` and CI result ZIP, with exact post-merge `main` SHA, before issuing a definitive release certificate.
5. [Issue #15](https://github.com/dinhvansh/dataflow-engine/issues/15) still has unchecked v1.4 acceptance gates at review time; may be stale, but do not silently close or claim that production risks are resolved without checking the new evidence.

**Permitted next work:** Design and implement optional v1.5 features on separate branches with own migration/permission/security tests, read-only ERP guarantees, synthetic QA and explicit publish checks. **Do not deploy to production or enable new write-capable services against the user's real ERP.** Existing QA reports cover a different commit and selected scenarios, not all future behavior.

## 6. Next milestones — proposals, not yet authorized

The most recent product direction discussed after UI completion:

| Candidate | Scope | Why |
| --- | --- | --- |
| **v1.5 Connection Manager** (suggested first) | Multi-instance connectors, credential isolation, schema discovery, test connection, selectable source/destination bindings | Connector Hub currently mostly static |
| **v1.6 Visual Pipeline Builder** | True xyflow editable Source→Extract→Transform→DQ→Destination graph with existing DAG runtime contracts | End-to-end workbench |
| **v1.7 Data Quality Contracts** | Not-null, uniqueness, range/freshness, reconciliation, publish gate and history | Prevent bad data release |
| **v1.8 Efficient Incremental V2** | Optimize copy-on-write, late/out-of-order handling, safe deletes, performance benchmark | Current large-table limitations |
| **v2.0 Enterprise Operations** | RBAC, audit, approvals, secrets, scheduler/webhooks, Teams alerts, environments/releases | Multi-user operations |

**Decision not yet made:** Which feature to implement next. Ask one brief priority question only if the user has not chosen. Finish current QA/conflicts first if they block safe development. Do not silently implement backend simply because UI scaffolding exists.

## 7. New chat bootstrap — copy just this

> Tiếp tục dự án DataFlow Engine. Đọc toàn bộ `https://github.com/dinhvansh/dataflow-engine/blob/main/PROJECT_CONTEXT.md` làm context chính, sau đó **kiểm tra live GitHub** (main SHA, open PR, conflicts, CI, QA) vì snapshot có thể đã cũ. Không yêu cầu tôi giải thích lại kiến trúc, UI decisions, trạng thái merge hoặc ERP safety. Tóm tắt ngắn những thay đổi mới so với handoff rồi tiếp tục nhiệm vụ tôi giao. Nếu định sửa code thì kiểm tra Git trước, tạo PR reviewable, test đúng phạm vi và không động đến ERP thật.

For new assistants: This file is **project context**, not an authorization to access real ERP, run destructive experiments, merge future PRs or deploy production. Direct user request and current verified evidence always take precedence over stale handoff text.

## 8. Update protocol for the handoff

At each significant milestone, update this file in a small docs-only commit/PR:
1. Set `Snapshot` date + freshly verified `main` SHA.
2. Reconcile what actually merged, open/closed PRs, dependency branches/conflicts.
3. Include exact tested SHA, test command, real pass/fail/blocked counts, evidence links.
4. Record newly approved UI/architecture decisions and the next task; mark proposed work separately.
5. Do not turn reports into fictional PASS, share credentials/internal infrastructure, or copy raw logs into this public repo.
