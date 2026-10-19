# Owner Demo — CORE-RUN-8B from the real Canvas UI

**Priority (owner decision, 2026-10-19):** demonstrate the existing executable strict-linear Canvas path before running the deferred CORE-RUN-7 7-million-row benchmark. Do **not** trigger `run-core-7m.sh`, reduce its disk guard, or invent throughput results.

## Goal and boundaries

Show a repeatable, **operator-controlled, non-mock browser** demonstration of persisted SOURCE → SYNC → dbt model A → model B → model C → QUALITY → PUBLISH, using real Go API, PostgreSQL state, disposable SQL Server fixture, ClickHouse and dbt. This is an isolated **synthetic** product demo, **not production certification**, not an ERP performance test.

Never write or run DDL against ERP; the demonstration must use ONLY `DataFlowFixtureQA` synthetic SQL Server and owned ClickHouse/PostgreSQL test services. Use no company secrets. Keep other running Docker projects untouched. **Do not reset existing QA volumes without separate, explicit owner consent and verification of ownership and scope.**

## Evidence already available

- Local one-shot Core8B result: commit `95dafb5`, `qa-results/local-core8b-20261019T053354Z/HANDOFF_TO_CHATGPT.md` records 17 PASS/0 FAIL/0 BLOCKED. The test used 10,000 synthetic rows; it is *not* a user-operated demo.
- Existing entrypoints/documentation: `deploy/local-qa/run-core-operator.sh`, `qa/browser/test_core_live.py`, `qa/local-acceptance/v8b/README.md`, `docs/CORE_FIRST_ENGINE_GATE.md`.
- The scripted E2E is an independent fallback regression, **not evidence the owner clicked and observed the UI**.

## Preparation (no destructive commands)

1. On a working branch, fetch and review latest `main`, current PRs and Actions; do not discard any local changes. Confirm the Core8B Canvas implementation and PR #123's interactive editor test status.
2. Check RAM, Docker Desktop/WSL integration, free disk (at least the current local-QA preflight limit), bind ports, and the labels of **all** `dataflow-localqa` containers, networks, and volumes. No automatic cleanup or disk-guard bypass.
3. Start/verify the already approved **disposable** QA stack by following `deploy/local-qa/README.md`, `create-env.sh`, `up.sh`, and `verify-isolation.sh` as applicable. Stop and request explicit approval before any labeled QA volume reset. If resources or ownership cannot be established: `BLOCKED`, report exactly why.
4. Use the validated isolated 10,000-row synthetic dataset; no 7m loading. Confirm QA-only server/DB configuration and SELECT-only read path.

## Browser-only operator acceptance script

**All intended operator actions must actually be performed via the UI, not silently substituted with HTTP or Playwright clicks** (Playwright may independently verify observed results).

| Step | Owner action in React UI | Required verifiable outcome |
|---|---|---|
| DEMO-01 | Open Core operator and reload the page | Existing graph list loads from PostgreSQL, not mocked data; auth checks preserved |
| DEMO-02 | Load/create a strictly linear saved SOURCE → SYNC → A → B → C → QUALITY → PUBLISH graph in the editor | Typed edges, correct QA source stream, saved Visual models, DQ key and publish target |
| DEMO-03 | Use Validate and Save in UI | Server accepts graph, stores graph UUID/version/digest; refresh loads same graph |
| DEMO-04 | Press the UI Run control and confirm execution | Real Core Run UUID and persisted status; no direct API-only run substituted |
| DEMO-05 | Observe SYNC and dbt model A→B→C statuses via UI | Real persisted steps in order; verify SQL Server source count vs CH raw, PG dbt steps, model outputs; log run IDs and row counts |
| DEMO-06 | Inspect QUALITY → PUBLISH and results | DQ PASS before physical publish; accessible physical target UUID and representative SELECT result in ClickHouse, no fake UI success |
| DEMO-07 | Reload browser and inspect same run | Same run UUID, statuses, immutable graph snapshot/version/digest survive reload |
| DEMO-08 | Run the same graph again with no source changes | Actual incremental rows written = 0; no duplicate keys/rows, checkpoint advances according to documented semantics |
| DEMO-09 | Perform an authorized safe rollback through existing supported operator flow | Correct exact target/run UUID rolled back, previous healthy published version preserved and auditable |
| DEMO-10 | Try an invalid graph or stale revision in a safe QA-only branch | Explicit UI/server refusal and no new managed run; do not modify previously accepted demo evidence |

Do **not** claim the UI implements a function based solely on an API test. If any listed owner click is missing or broken, mark that step `FAIL` with screenshot and exact problem. Fix minimal blocking UI/backend wiring in a separate PR with regression tests, keep backend safety/ERP contract intact. Do not add cosmetic changes or parallel/IF-ELSE.

## Deliverables and acceptance

- `qa-results/owner-demo-<UTC>/OWNER_DEMO_REPORT.md`: operator identity (sanitized), branch+SHA, environment/resource envelope, START/END and each DEMO-01..10 marked PASS/FAIL/BLOCKED/NOT_RUN with a human-executed-vs-automated distinction.
- Screenshots/video of user clicking Validate/Save/Run, running step states, persisted results after reload, second incremental, DQ gate, publish and rollback; redact tokens, company IPs and credentials.
- Correlated **real** run/graph IDs, version/digest, before/after checkpoint, source/raw/target counts, dbt steps, quality result, ClickHouse physical UUID, log excerpts, errors and rollback proof.
- A short, repeatable local runbook describing exact environment launch commands and URL discovered from the *real* local configuration (never invent ports or addresses).
- Compare operator outputs with already published local Core8B E2E evidence, without conflating them. Explain any disagreement.
- **Owner Demo PASS** only if all mandatory operator steps actually pass with correlated evidence. Local QA PASS remains independent; this demo does not certify 7m, parallel routing, IdP/Vault/TLS or production.

## Safety / release gates

ERP: **SELECT-only, no source writes**. Scratch SQL Server DML solely in approved synthetic QA fixtures. No unauthorized QA-volume deletion. CI exact-head PASS before merging any code PR. Update `docs/PROJECT_HANDOFF_LATEST.md` and roadmap when the real owner demo is complete, preserving **CORE-RUN-7 = DEFERRED / NOT RUN** until adequate owner-approved hardware is available.
