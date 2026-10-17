# DataFlow PR #40 — Retest after fixes

## Verdict

**FAIL — FIX REQUIRED**

The build passes and the SQL textarea now has a real internal vertical viewport, but the required automated result remains **17/18 PASS**. UI-18 fails because the gutter does not follow the textarea. Manual measurement reproduces the product defect at all three required viewports: textarea `scrollTop=240`, gutter `scrollTop=0`.

This is mocked browser UI QA, not live full-stack E2E acceptance.

## Tested revision and scope

- Target branch: `feat/sql-editor-source-explorer-ux-20261017`
- Exact tested SHA: `827494a52b244df129b815a20c6d6e285542b42f`
- Previous tested SHA: `e1b6e8187655cb4dec0d3667f4daf2beef4bd438`
- Base `origin/main`: `d13199fc52a3516d4d4a635217a1edd811e6efc0`
- Isolated detached worktree: `/workspace/dataflow-pr40-retest-827494a`
- Remote branch and local checkout SHA matched before testing.

Changes since the previous failed revision are limited to:

```text
M qa/browser/test_console.py
M web/src/index.css
```

The full PR diff against main remains limited to the intended four UI/test files:

```text
M qa/browser/test_console.py
M web/src/components/SQLJobStudio.tsx
M web/src/components/SourceExplorer.tsx
M web/src/index.css
```

No backend, API contract, database, migration, execution-engine, or deployment file changed.

## Commands and exit codes

| Command | Exit | Actual result |
|---|---:|---|
| `git fetch origin --prune` | 0 | Exact target SHA verified. |
| `cd web && npm ci` | 0 | PASS; 165 packages installed from lockfile. |
| `cd web && npm run build` | 0 | PASS; 1,765 modules transformed. Advisory only: 517.83 kB JS chunk exceeds Vite's 500 kB warning threshold. |
| `python -m pip install pytest==8.4.2 playwright==1.56.0` | 0 | PASS; pinned versions installed in `/tmp/dataflow-pr40-retest-venv`. |
| `python -m playwright install chromium` | 0 | PASS. |
| `cd web && npm run dev -- --host 127.0.0.1 --port 5173` | N/A (server stopped with Ctrl-C) | Started successfully; `ss` confirmed listener `127.0.0.1:5173`; HTTP smoke exited 0. |
| `DATAFLOW_BROWSER_BASE_URL=http://127.0.0.1:5173 python -m pytest qa/browser/test_console.py -v` | 1 | **17 passed, 1 failed in 20.50 s.** UI-18 failed. |
| `python qa-results/pr40-retest/manual_qa.py` (complete evidence run) | 0 | Completed all three viewports; recorded the gutter defect instead of masking it. |

## UI-18 failure

Failure at `qa/browser/test_console.py:525`:

```text
expect(page.locator(".df-studio-line-numbers"))
  .to_have_js_property("scrollTop", scrolled["top"])
Expected: 120
Actual:   0
```

The test sets the textarea property directly without dispatching a `scroll` event, so React's `onScroll` handler is not invoked by that test action. However, this is not the only problem: the manual run explicitly dispatches the scroll event and still finds the gutter cannot scroll because its own `clientHeight` equals its full 60-line `scrollHeight` (1472 px).

Root cause: the new CSS constrains `.df-studio-sql-area` to 340 px, but `.df-studio-line-numbers` remains unconstrained. Its 60 children make the gutter 1472 px high; setting its `scrollTop` has no effect. The gutter needs the same bounded viewport height (or a bounded parent with the gutter stretched to that viewport). The automated test should also generate a real scroll event.

## Manual 60-line measurements

The query contained 60 lines; the first line was 400 characters to exercise horizontal overflow.

| Viewport | Textarea content/client height | textarea `scrollTop` | Gutter content/client height | gutter `scrollTop` | Horizontal content/client | `scrollLeft` | Result |
|---|---:|---:|---:|---:|---:|---:|---|
| 1440×900 | 1472 / 340 | 240 | 1472 / 1472 | 0 | 2915 / 598 | 80 | FAIL gutter sync |
| 1366×768 | 1472 / 340 | 240 | 1472 / 1472 | 0 | 2915 / 551 | 80 | FAIL gutter sync |
| 390×844 | 1472 / 340 | 240 | 1472 / 1472 | 0 | 2915 / 266 | 80 | FAIL gutter sync |

Functional results:

- 60 line numbers and `60 lines` count: PASS.
- SQL textarea internal vertical overflow: PASS.
- `scrollTop > 0`: PASS at all viewports.
- Gutter synchronous scrolling: **FAIL** at all viewports.
- Horizontal scrolling for long SQL: PASS at all viewports.
- No document-level horizontal overflow: PASS at all viewports.
- Light and Dark readability: PASS at all viewports.
- Source Explorer schema/table filter: PASS.
- Column filter and metadata table: PASS.
- Editing table/schema clears stale columns: PASS.
- Browser console errors: 0.
- Uncaught page errors: 0.
- API calls: six expected GETs only (`/source/tables` and `/source/columns`, once each per viewport); no unexpected calls or mutation methods.

## Screenshots and evidence

- [1440×900 Light/Dark](screenshots/contact-sheet-1440x900.png)
- [1366×768 Light/Dark](screenshots/contact-sheet-1366x768.png)
- [390×844 Light/Dark](screenshots/contact-sheet-390x844.png)
- Local raw pytest log: `/workspace/dataflow-pr40-retest-827494a/qa-results/pr40-retest/logs/06-pytest-browser.log`
- Local manual measurements: `/workspace/dataflow-pr40-retest-827494a/qa-results/pr40-retest/logs/07-manual-results.json`
- Local traces: `/workspace/dataflow-pr40-retest-827494a/qa-results/pr40-retest/traces/`

Only this sanitized report and three contact sheets are intended for GitHub. Raw logs, traces, scripts, tokens, and caches remain local.

## Safety

- Playwright intercepted all `/healthz` and `/api/v1/**` traffic locally.
- Only a documented synthetic token was entered.
- No real ERP, SQL Server, PostgreSQL, ClickHouse, or dbt endpoint was accessed.
- No database mutation, DDL, integration stack, source modification, merge, or push to the PR branch/main occurred.

## Recommendation

**FIX REQUIRED.** Bound the line-number gutter to the same 340 px editor viewport and make UI-18 trigger an actual scroll event. Then rerun the full suite and require 18/18 PASS plus equal non-zero textarea/gutter `scrollTop` across all three viewports before merge.
