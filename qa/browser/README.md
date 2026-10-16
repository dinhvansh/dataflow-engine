# Browser UI QA — 6 mocked-contract tests

These 6 Playwright Chromium tests exercise actual React interactions:
source explorer, Applied Steps preview, saved model reload, lineage selection,
Auto Build action, and staged incremental Publish/Rollback confirmations.

**Scope:** The HTTP API is mocked in `test_console.py`. These are browser UI
contract/smoke tests, **NOT** live end-to-end tests and **NOT** proof that
SQL Server, PostgreSQL, ClickHouse or dbt performed any operation. The
existing local QA matrix UI-01..UI-06 stays BLOCKED until a separate
disposable full-stack browser run uses real API endpoints and compares
backend row/state evidence.

## Local run (no real ERP or credentials)

```bash
cd web && npm ci && npm run dev -- --host 127.0.0.1 --port 5173
# In another terminal from repo root:
python -m pip install pytest==8.4.2 playwright==1.56.0
python -m playwright install chromium
python -m pytest qa/browser/test_console.py -v
```

The tests use the fake bearer token `synthetic-browser-token-not-a-secret`;
all API routes are intercepted and never reach Epicor. The UI root health
request is also mocked. Browser traces are written under the ignored
`qa/browser/artifacts/` directory.

## Remaining live acceptance

On the Windows QA stack created in `deploy/local-qa`, use a separate
Playwright suite with actual read-only synthetic SQL Server,
PostgreSQL, ClickHouse and dbt worker, checking the database after every
UI operation and collecting screenshots/video/trace. Ensure staging and
publishing point only to `scratch`, never production; require explicit
manual authorization before any real Epicor TEST read. Mark any missing
browser/database evidence BLOCKED, not PASS.

`test_console_live.py` is skipped unless `DATAFLOW_LIVE_QA_OPT_IN=true`. It
accepts loopback HTTP endpoints only, verifies all three database containers
carry the local-QA owner/role labels, and confirms the source database is
`DataFlowFixtureQA`. Configure the API itself and `DATAFLOW_LIVE_API_TOKEN`
with the literal `synthetic-live-browser-token-not-a-secret`; a real bearer
credential is refused because Playwright traces can contain request headers.
Every run uses unique scratch names, deletes only those owned resources, and
scans each trace payload for protected environment values.
