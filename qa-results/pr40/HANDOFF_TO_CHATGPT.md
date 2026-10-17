# DataFlow PR #40 — Local UI QA

## Verdict

**FAIL — FIX REQUIRED**

The production build passes and UI-01 through UI-17 pass, but the required suite result is **17/18 PASS** rather than 18/18. UI-18 fails on an ambiguous Playwright locator. Manual continuation with an exact locator confirms Source Explorer behavior works, but also finds a product layout defect: the SQL textarea expands to all 60 lines and therefore has no vertical scroll region or gutter-scroll synchronization.

This is mocked browser UI QA, not live full-stack E2E acceptance.

## Git preflight and scope

- Target PR: `#40`
- Target branch: `feat/sql-editor-source-explorer-ux-20261017`
- Exact tested SHA: `e1b6e8187655cb4dec0d3667f4daf2beef4bd438`
- Base: `d13199fc52a3516d4d4a635217a1edd811e6efc0`
- Base was verified as an ancestor of the target.
- Isolated detached worktree: `/workspace/dataflow-pr40-qa-e1b6e818`
- Original checkout and existing worktrees were preserved.

Diff against base contains exactly the intended four files:

```text
M qa/browser/test_console.py
M web/src/components/SQLJobStudio.tsx
M web/src/components/SourceExplorer.tsx
M web/src/index.css
```

No backend, API contract, migration, database, execution-engine, or deployment file changed.

## Commands and exit codes

| Command | Exit | Actual result |
|---|---:|---|
| `git fetch origin --prune` | 0 | Exact remote branch SHA and base verified. |
| `cd web && npm ci` | 0 | PASS; 165 packages installed from lockfile. |
| `cd web && npm run build` | 0 | PASS; 1,765 modules transformed. Advisory only: 517.83 kB JS chunk exceeds Vite's 500 kB warning threshold. |
| `python -m pip install pytest==8.4.2 playwright==1.56.0` | 0 | PASS; pinned versions installed in `/tmp/dataflow-pr40-venv`. |
| `python -m playwright install chromium` | 0 | PASS. |
| `cd web && npm run dev -- --host 127.0.0.1 --port 5173` | N/A (long-running server, stopped with Ctrl-C) | Started successfully; `ss` confirmed only `127.0.0.1:5173`; HTTP smoke exited 0. |
| `DATAFLOW_BROWSER_BASE_URL=http://127.0.0.1:5173 python -m pytest qa/browser/test_console.py -v` | 1 | **17 passed, 1 failed in 15.99 s.** |
| `python qa-results/pr40/manual_qa.py` (complete evidence run) | 0 | Completed desktop/mobile manual interaction checks with mocked APIs. |

## Automated failure: UI-18

Test: `test_ui18_sql_line_gutter_and_source_metadata_filters_are_local`

Failure at `qa/browser/test_console.py:503`:

```text
Locator.fill: strict mode violation: get_by_label("SQL Server table") resolved to 2 elements:
1) <input aria-label="SQL Server table" ...>
2) <section aria-label="Available SQL Server tables" ...>
```

Root cause: Playwright label matching is non-exact by default, so `SQL Server table` also matches the region named `Available SQL Server tables`. The test stops before its stale-metadata and SQL-gutter assertions. Manual continuation using `get_by_role("textbox", name="SQL Server table", exact=True)` works.

Proposed test fix: use an exact role/name selector for the textbox. Product code was not modified during QA.

## Manual functional results

The actual Vite-served React application was exercised at 1440×900 and 390×844 in Light and Dark modes using synthetic intercepted responses.

| Check | Actual | Status |
|---|---|---|
| Table search by schema/table | `ice.syst` isolated `Ice.SysTask`; `erp.part` isolated `Erp.PartTran`. | PASS |
| Column metadata table | Three metadata rows loaded; headers and PK/cursor metadata rendered. | PASS |
| Column filter | Filtering by SQL type `timestamp` showed `SysRevID` and hid `SysRowID`. | PASS |
| Stale metadata clear | Editing the exact table textbox to `NewTable` removed the inspected-column region; changing schema kept it cleared. | PASS |
| SQL line numbers/count | A 60-line query rendered 60 gutter entries and displayed `60 lines`. | PASS |
| SQL horizontal scrolling | 400-character line produced horizontal overflow; `scrollLeft` reached 80 px at both viewports. | PASS |
| SQL vertical scrolling/gutter sync | `scrollHeight = clientHeight = 1472 px`; `scrollTop` remained 0 at both viewports. The editor expands instead of forming a vertical scroll region. | **FAIL** |
| Desktop/mobile layout | No document-level horizontal overflow at 1440×900 or 390×844. | PASS |
| Light/Dark | Both themes rendered with readable controls and no observed clipping on tested panels. | PASS |
| Console/page errors | 0 console warnings/errors; 0 uncaught page errors. | PASS |
| API activity | Exactly four expected GETs: two `/source/tables` and two `/source/columns`. No unexpected calls and no mutation methods. | PASS |

Likely vertical-scroll root cause: `.df-studio-sql-area` has `min-h-[270px]` and `overflow-auto` but no fixed/max editor height; its flex layout allows the textarea to grow to the full content height, making vertical overflow impossible. Reproduce by entering a 60-line query and comparing `scrollHeight` with `clientHeight`.

## Screenshots and local evidence

- [Desktop Light/Dark contact sheet](screenshots/contact-sheet-desktop.png)
- [Mobile Light/Dark contact sheet](screenshots/contact-sheet-mobile.png)
- Local individual screenshots: `/workspace/dataflow-pr40-qa-e1b6e818/qa-results/pr40/screenshots/`
- Local pytest log: `/workspace/dataflow-pr40-qa-e1b6e818/qa-results/pr40/logs/06-pytest-browser.log`
- Local manual JSON: `/workspace/dataflow-pr40-qa-e1b6e818/qa-results/pr40/logs/07-manual-results.json`
- Local Playwright traces: `/workspace/dataflow-pr40-qa-e1b6e818/qa-results/pr40/traces/`

Only the sanitized report and two contact sheets are intended for the report branch. Raw logs, traces, scripts, tokens, and test caches remain local.

## Safety

- All `/healthz` and `/api/v1/**` traffic was intercepted locally by Playwright.
- Only the documented synthetic token was entered.
- No real ERP, SQL Server, PostgreSQL, ClickHouse, or dbt endpoint was accessed.
- No database mutation, DDL, integration stack, source modification, merge, or push to the PR branch/main occurred.

## Recommendation

**FIX REQUIRED.** Correct the UI-18 locator and constrain the SQL editor to a real vertically scrollable viewport with synchronized gutter scrolling. Then rerun the full suite and manual 60-line scrolling check; require 18/18 PASS and non-zero synchronized `scrollTop` before merge.
