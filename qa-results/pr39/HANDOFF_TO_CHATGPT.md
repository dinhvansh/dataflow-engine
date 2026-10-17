# PR #39 — Local UI QA handoff

## Verdict

**FAIL — FIX REQUIRED**

The React production build and the visual checks passed, but the required automated result was not met: **16/17 mocked browser tests passed; UI-17 failed**. This is a test defect rather than evidence that the Lineage screen fails to mount: UI-17 expects `Dependency Map` without supplying an API token, while `DataLineage.tsx` renders that text only after the graph request completes. UI-04 proves the same panel works when the shared helper supplies a synthetic token.

This result is **mocked frontend QA, not live full-stack E2E acceptance**.

## Git safety preflight

- Tested detached HEAD: `01fde93715d947db67e4629fe3647dc5f8c502b5`
- Requested branch tip verified: `origin/style/frontend-theme-consistency-audit-20261017` points to the tested SHA.
- Requested base verified: `a3a8b3fb5d4ccdacb4301778b476877d6838015b` and is an ancestor of the target.
- Isolated worktree: `/workspace/dataflow-pr39-qa-01fde937`
- Original checkout was not changed: `/workspace/dataflow-engine` remains on local `main` at `ac3a794869eb3a22b2c8fba2055343cdfe42c103`, four commits behind fetched `origin/main` at the requested base.
- Detached worktree was clean before testing. Final tracked source remains unchanged. The only visible untracked source-tree item is test-generated `qa/browser/__pycache__/`; QA output is ignored by repository rules under `qa-results/pr39/`.
- No merge, rebase, commit, push, or modification of `main` was performed.

## Commands and outcomes

| Command | Exit | Result |
|---|---:|---|
| `git fetch origin` | 0 | Target and base objects fetched and verified. |
| `git worktree add --detach /workspace/dataflow-pr39-qa-01fde937 01fde93715d947db67e4629fe3647dc5f8c502b5` | 0 | Isolated exact-SHA checkout created. |
| `cd web && npm ci` | 0 | PASS. |
| `cd web && npm run build` | 0 | PASS; 1,765 modules transformed. Advisory only: JS chunk 515.08 kB exceeds Vite's 500 kB warning threshold. |
| `/tmp/dataflow-pr39-venv/bin/pip install pytest==8.4.2 playwright==1.56.0` | 0 | PASS; versions match `qa/browser/README.md`. |
| `/tmp/dataflow-pr39-venv/bin/python -m playwright install chromium` | 0 | PASS. |
| `cd web && npm run dev -- --host 127.0.0.1 --port 5173` | running during QA | Vite listened only on `127.0.0.1:5173` (confirmed with `ss`). |
| `DATAFLOW_BROWSER_BASE_URL=http://127.0.0.1:5173 /tmp/dataflow-pr39-venv/bin/python -m pytest qa/browser/test_console.py -v` | 1 | **16 PASS, 1 FAIL**, 18.92 s. |
| `/tmp/dataflow-pr39-venv/bin/python qa-results/pr39/visual_qa.py` | 0 | 60 screen/theme/viewport checks plus confirmation-dialog test completed. |

Logs are in `/workspace/dataflow-pr39-qa-01fde937/qa-results/pr39/logs/`.

## Automated browser failure

**UI-17 — `test_ui17_all_ten_navigation_screens_mount_in_light_mode`**

- Failure: `qa/browser/test_console.py:478`
- Expected by test: visible text `Dependency Map` after clicking **Data Lineage**.
- Actual: the Lineage workspace and its static hero mount, but `Dependency Map` is conditional on `graph && layout`. UI-17 never enters a token, so no graph is loaded and the conditional panel is absent.
- Corroboration: UI-15 checks the static `.df-lineage-workspace` and passes without a token. UI-04 enters the synthetic token, loads mocked lineage data, finds `Dependency Map`, and passes.
- Reproduction: run the pytest command above at the tested SHA.
- Proposed test-only correction: either use `open_tab(page, "Data Lineage")` before asserting `Dependency Map`, or make UI-17 assert the unconditional `Data Lineage & Impact Analysis` / `.df-lineage-workspace` marker. Do not alter product behavior merely to satisfy this assertion.

No source fix was made, per the instruction to diagnose and report only.

## Visual QA results

The actual Vite-served React application was inspected at 1440×900, 1366×768, and 390×844. Each of the 10 navigation screens was opened in both Light and Dark modes: **60 screen states checked**.

| Check | Actual result | Status |
|---|---|---|
| Light palette | Computed `--console-bg: #fafbfa`, card `#ffffff`, accent `#238047`; clean near-white page and white cards. | PASS |
| Dark palette | Computed background `#161d1b`, accent `#c0f273`; graphite surfaces and readable light text. | PASS |
| Legacy blue/mint/default styling | No legacy fixed palette utilities found by UI-16; no old-blue primary accent observed in screenshots. | PASS |
| Semantic statuses | Separate theme tokens are present: success/error/warning dark `#78dbad/#f29890/#edca83`, light `#16683b/#b42332/#9b5816`. | PASS |
| 10 primary screens | Overview, Connections, Sync Jobs, Transform Studio, Data Lineage, SQL Jobs, Operations Center, Pipelines, Incremental CDC, Run Inspector all mounted in both themes. | PASS |
| SQL/Transform/DAG/Lineage/Incremental usability | Editors/workspaces mounted and controls were visible; loaded Lineage panel was also captured after synthetic token state was set. | PASS (mocked APIs) |
| Desktop layout | No document-level horizontal overflow or clipped visible controls at 1440×900 or 1366×768. | PASS |
| Mobile smoke | No document-level horizontal overflow at 390×844. Navigation is intentionally horizontally scrollable; off-viewport nav items become visible through horizontal scrolling and were all activated. | PASS |
| Confirmation Cancel | Incremental publish dialog opened; Cancel closed it; `/incremental/action` call count stayed `0 → 0`. | PASS |
| Browser errors | 0 uncaught page errors; 0 console errors/warnings captured during visual run. | PASS |

The visual API data was synthetic and intercepted locally. No real SQL Server, Epicor, PostgreSQL, ClickHouse, or backend was contacted.

## Screenshots and traces

- All screenshots: `/workspace/dataflow-pr39-qa-01fde937/qa-results/pr39/screenshots/`
- Screenshot manifest: `/workspace/dataflow-pr39-qa-01fde937/qa-results/pr39/logs/09-screenshot-manifest.csv`
- Representative contact sheets:
  - [Desktop dark](screenshots/contact-sheet-desktop-dark.png)
  - [Desktop light](screenshots/contact-sheet-desktop-light.png)
  - [Mobile smoke](screenshots/contact-sheet-mobile.png)
- Confirmation dialog: [Incremental publish Cancel](screenshots/1440x900-dark-confirm-publish.png)
- Visual traces: `/workspace/dataflow-pr39-qa-01fde937/qa-results/pr39/traces/visual-1440x900.zip`, `visual-1366x768.zip`, `visual-390x844.zip`
- Per-test Playwright traces: `/workspace/dataflow-pr39-qa-01fde937/qa-results/pr39/traces/mock-tests/`
- Trace manifest: `/workspace/dataflow-pr39-qa-01fde937/qa-results/pr39/logs/10-trace-manifest.csv`
- Machine-readable visual findings: `/workspace/dataflow-pr39-qa-01fde937/qa-results/pr39/logs/07-visual-results.json`

Artifacts contain synthetic values only. The entered token is explicitly synthetic and not a credential.

## Scope and safety review

Diff against the requested base contains only:

- `qa/browser/test_console.py`
- `web/UI_AUDIT_2026-10-17.md`
- `web/index.html`, `web/public/dataflow-mark.svg`
- React/CSS files under `web/src/`

No Go backend, API contract, database migration, execution engine, deployment, or infrastructure file changed. No real credentials were used. No integration stack was started. No DML, DDL, destructive infrastructure action, or connection to Epicor occurred.

## Recommendation

**FIX REQUIRED before merge.** Correct UI-17 so its setup matches its conditional expectation (preferred: enter the synthetic token/load the graph), then rerun all 17 tests and require 17/17 PASS. The observed product UI and scoped visual acceptance checks otherwise passed at the exact target SHA.
