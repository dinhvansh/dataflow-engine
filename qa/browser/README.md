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
