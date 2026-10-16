# Browser results

## Mock contract suite

`qa/browser/test_console.py`: **6/6 PASS** on Chromium 141 / Playwright 1.56. These results only prove React interaction contracts and are not used as E2E acceptance.

## Live synthetic backend suite

`qa/browser/test_console_live.py`: **6/6 PASS** after one setup-only retry and one assertion-only retry. The final results used the real Go API, SQL Server fixture, PostgreSQL, ClickHouse and dbt worker.

- UI-01 Source Explorer: real `Erp.PartTran` metadata and `SysRowID` displayed.
- UI-02 Visual Preview: real `scratch.live_ui_raw` previewed; ClickHouse source remained at 2 rows.
- UI-03 Save/Reload: one `my_first_transform` metadata row verified in PostgreSQL.
- UI-04 Lineage: the real `live_ui_a → live_ui_b → live_ui_c` graph displayed; three model records verified.
- UI-05 Auto Build: UI reached `succeeded`; PostgreSQL flow status was `succeeded`; `dfv_live_ui_c` contained 2 rows.
- UI-06 Publish/Rollback: publish produced 2 rows / amount 50; rollback restored 1 row / amount 10; PostgreSQL state was `rolled_back`.

Each final case has a full-page PNG and Playwright trace ZIP under `artifacts/playwright-live/`. The initial setup error was caused by reusing a pre-existing table with a different schema; the fixture was isolated as `live_ui_raw`. UI-05 initially asserted the wrong PostgreSQL table name although UI and ClickHouse had succeeded; the assertion was corrected and rerun PASS.
