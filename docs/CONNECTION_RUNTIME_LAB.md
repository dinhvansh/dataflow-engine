# Runtime Connection Manager — disposable QA only

Status: **EXPERIMENTAL / feature OFF by default**. This is not production credential management, Vault/IdP/TLS approval, or an Epicor ERP connection. Do not enable this outside an explicitly isolated synthetic QA stack.

## Operator setup

Apply PostgreSQL migration `031_connection_runtime_profiles.sql` **only** to disposable QA PostgreSQL, after the existing managed stream migrations. Back up relevant QA evidence. Do not reset existing Docker volumes or touch unrelated environments without owner approval.

Configure the following variables through the **private local QA environment / Docker secrets**, not in Git:

```text
DATAFLOW_ENABLE_RUNTIME_CONNECTIONS_LAB=true
DATAFLOW_INTEGRATION_PG_ISOLATED=true
DATAFLOW_INTEGRATION_MSSQL_ISOLATED=true
DATAFLOW_INTEGRATION_CH_ISOLATED=true
DATAFLOW_ALLOW_TEST_LOAD=true
DATAFLOW_TEST_TARGET_DATABASE=scratch
DATAFLOW_CONNECTION_ADMIN_TOKEN=<different secret from DATAFLOW_API_TOKEN>
DATAFLOW_QA_CONNECTION_KEY_B64=<32 random bytes, standard base64>
DATAFLOW_QA_SQLSERVER_ALLOWED_ENDPOINT=<approved QA host:port>
DATAFLOW_QA_CLICKHOUSE_ALLOWED_ENDPOINT=<approved QA host:port>
# Only in a verified isolated fixture with self-signed SQL certificate:
DATAFLOW_QA_SQL_TRUST_CERT=true
# Only when ClickHouse QA uses plain HTTP on its private container network:
DATAFLOW_QA_CLICKHOUSE_ALLOW_HTTP=true
```

Generate a new 32-byte key in a trusted local shell, e.g. `openssl rand -base64 32`, and keep it outside Git. Do not rotate or lose this key until a deliberate credential re-encryption process exists; otherwise saved credentials become unreadable. Treat QA PostgreSQL snapshots as sensitive even though passwords are encrypted.

The exact approved host:port entries must come from **trusted deployment configuration**, not a browser choice. Network isolation remains essential: this first lab admission gate uses exact operator allowlists, **not** a production-grade DNS-rebinding-resistant pinned egress dialer. Do not approve production or arbitrary internet endpoints.

## Operator workflow

1. Provide the shared Lab API token once in the header (PR #127) and a separate connection admin token in the Live Lab Connection Manager.
2. `Load connections`; choose **Add connection** for SQL Server QA source / ClickHouse scratch destination. The backend rejects any host/port outside its allowlist.
3. `Save encrypted draft`. The server encrypts the password with AES-256-GCM and unique nonce/AAD; GET only returns metadata. No password echo. This stores only lab credentials, **not** production company secrets.
4. `Test real connection` for each saved profile. SQL Server must report the exact `DataFlowFixtureQA` database and effective SELECT-only Erp.Part permissions for `qa_erp_reader`. ClickHouse must successfully execute a bounded SELECT 1 on the approved scratch server. A failed retest downgrades the profile to DRAFT and invalidates its bindings.
5. Create an existing scratch `sync_streams` entry through the supported managed-stream flow. Select that stream and both **VERIFIED** profiles, then choose `Bind to pipeline`.
6. In Pipelines, run Core 8B on this stream. The managed worker loads only the persisted stream-scoped binding; it refuses missing/stale profile revisions, test age >30 minutes, changed server endpoint, SQL permissions or disabled runtime mode. The dbt/publish target remains the **same backend-provisioned scratch ClickHouse service**, never a new arbitrary warehouse.
7. Inspect run UUID, source/target rows, checkpoint, DQ, publish and rollback evidence; see `docs/OWNER_DEMO_CORE8B.md`.

## Current limitations and release blockers

- Add/Save/Test/Bind applies **only to isolated QA** and operator-approved hosts; SQL Server database/name/user are fixed to the disposable fixture. This is intentionally not a general production connector.
- No credentials editing, revocation UI, production Vault-backed secret rotation or independent tenant login yet.
- ClickHouse execution must target the same existing scratch service that dbt uses.
- Migration and encrypted store require secure backup/key custody policies.
- Host admission is exact allowlist but not yet the final pinned-IP egress policy; keep this on a trusted isolated Docker network.
- Real four-service **profile Add→Save→Test→Bind→Core8B execution acceptance** must PASS on disposable QA before calling the feature complete.
- Production security NO-GO and CORE-RUN-7 7M benchmark remain separate.

## Test & failure policy

```bash
go test -race ./cmd/dataflow ./internal/...
go vet ./cmd/...
go build ./cmd/...
npm run build --prefix web
```

Capture real browser/browser-accessible lab operations and verify no secrets in stdout, traces, Git evidence, API GET or screenshots. Keep API/static test and actual SQL/ClickHouse driver E2E evidence distinct. Never reduce disk guards or grant ERP write permissions to make a test pass.
