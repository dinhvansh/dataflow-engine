# DataFlow PR #40 — Final scroll regression retest

## Verdict

**PASS — READY TO MERGE** for the requested mocked browser UI scope.

The production build passes, the complete suite is **18/18 PASS**, and manual testing confirms bounded vertical scrolling, synchronized gutter scrolling, equal textarea/gutter heights, horizontal scrolling, and synchronization after a real vertical resize at all three required viewports.

This remains mocked frontend QA, not live full-stack E2E acceptance.

## Tested revision and scope

- Target branch: `feat/sql-editor-source-explorer-ux-20261017`
- Exact tested SHA: `6807947f678329f45ed77a2b5c22912286943163`
- Previous failed SHA: `827494a52b244df129b815a20c6d6e285542b42f`
- Base `origin/main`: `d13199fc52a3516d4d4a635217a1edd811e6efc0`
- Isolated detached worktree: `/workspace/dataflow-pr40-final-6807947`
- Remote branch and local checkout SHA matched before testing.

Changes since the previous failed SHA are limited to `qa/browser/test_console.py` and `web/src/index.css`. The complete PR diff against main remains limited to four UI/test files:

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
| `python -m pip install pytest==8.4.2 playwright==1.56.0` | 0 | PASS; pinned versions installed in `/tmp/dataflow-pr40-final-venv`. |
| `python -m playwright install chromium` | 0 | PASS. |
| `cd web && npm run dev -- --host 127.0.0.1 --port 5173` | N/A (server stopped with Ctrl-C) | Started successfully; `ss` confirmed listener `127.0.0.1:5173`; HTTP smoke exited 0. |
| `DATAFLOW_BROWSER_BASE_URL=http://127.0.0.1:5173 python -m pytest qa/browser/test_console.py -v` | 0 | **18 passed in 16.19 s.** UI-18 PASS. |
| `python qa-results/pr40-final/manual_qa.py` | 0 | All manual assertions passed at all three viewports. |

## Automated regression result

```text
qa/browser/test_console.py::test_ui18_sql_line_gutter_and_source_metadata_filters_are_local PASSED [100%]
============================= 18 passed in 16.19s ==============================
```

UI-18 now dispatches a real scroll event and checks that both the textarea and gutter have bounded, equal-height scroll viewports before comparing `scrollTop`.

## Manual 60-line and resize measurements

The query contained 60 lines and a 400-character first line. The editor's CSS resize handle was dragged downward by 72 px, producing a measured 74 px content-height increase. Scrolling was repeated after resize.

| Viewport | Before heights: editor / textarea / gutter | Before top: textarea / gutter | Before horizontal | After heights: editor / textarea / gutter | After top: textarea / gutter | After horizontal | Status |
|---|---:|---:|---:|---:|---:|---:|---|
| 1440×900 | 340 / 340 / 340 | 220 / 220 | 80 | 414 / 414 / 414 | 360 / 360 | 100 | PASS |
| 1366×768 | 340 / 340 / 340 | 220 / 220 | 80 | 414 / 414 / 414 | 360 / 360 | 100 | PASS |
| 390×844 | 340 / 340 / 340 | 220 / 220 | 80 | 414 / 414 / 414 | 360 / 360 | 100 | PASS |

Additional measured values:

- Textarea and gutter content height: 1472 px at every viewport.
- Textarea horizontal content width: 2915 px.
- Textarea client widths: 598 px at 1440×900, 551 px at 1366×768, 266 px at 390×844.
- Bounded vertical overflow remained present after resize: 1472 px content > 414 px viewport.
- No document-level horizontal overflow at any viewport.

## Other manual checks

- 60 gutter entries and `60 lines` indicator: PASS.
- Light and Dark readability after resize: PASS at all three viewports.
- Source Explorer schema/table filtering: PASS.
- Column metadata filtering: PASS.
- Editing table/schema clears stale metadata: PASS.
- Browser console errors: 0.
- Uncaught page errors: 0.
- API calls: six expected GETs only (`/source/tables` and `/source/columns`, once each per viewport).
- Unexpected API calls: 0.
- Mutation methods: 0.

## Screenshots and evidence

- [1440×900 Light/Dark](screenshots/contact-sheet-1440x900.png)
- [1366×768 Light/Dark](screenshots/contact-sheet-1366x768.png)
- [390×844 Light/Dark](screenshots/contact-sheet-390x844.png)
- Local pytest log: `/workspace/dataflow-pr40-final-6807947/qa-results/pr40-final/logs/06-pytest-browser.log`
- Local machine-readable measurements: `/workspace/dataflow-pr40-final-6807947/qa-results/pr40-final/logs/07-manual-results.json`
- Local traces: `/workspace/dataflow-pr40-final-6807947/qa-results/pr40-final/traces/`

Only this sanitized report and three contact sheets are intended for GitHub. Raw logs, traces, scripts, tokens, and caches remain local.

## Safety

- Playwright intercepted all `/healthz` and `/api/v1/**` traffic locally.
- Only a documented synthetic token was entered.
- No real ERP, SQL Server, PostgreSQL, ClickHouse, or dbt endpoint was accessed.
- No database mutation, DDL, integration stack, source modification, merge, or push to the PR branch/main occurred.

## Recommendation

**READY TO MERGE** for PR #40 based on the requested scope: build PASS, 18/18 mocked browser tests PASS, and final manual scroll/resize regression PASS at all required desktop and mobile viewports.
