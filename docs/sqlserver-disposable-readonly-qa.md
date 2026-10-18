# SQL Server QA — real disposable source, read-only integration

**Status:** test lane only, not a production Test Connection API or an Epicor live acceptance.

## What this proves

GitHub Actions `SQL Server Disposable Read-Only QA` boots **Microsoft SQL Server 2022 Developer** as a fresh runner-local Docker container. It only exposes TCP port 11433 on 127.0.0.1, requires the CI-owned Docker label, and seeds the repo's existing `deploy/local-qa/sql/001_fixture.sql` and `002_seed.sql`. The isolated database is *only* `DataFlowFixtureQA`, with `Erp.Part`, `Erp.PartTran`, `Erp.InvcHead`, `Erp.InvcDtl`, rowversion columns and snapshot isolation.

The Go tests connect over the **real Microsoft SQL Server wire protocol** with `qa_erp_reader`:

- Assert **database identity** `DataFlowFixtureQA` and exact login `qa_erp_reader` before any table query.
- Query effective SELECT/INSERT/UPDATE/DELETE/ALTER permissions on `Erp.Part` (read-only verification, no attempted DML/DDL), and reject db_owner / db_datawriter membership.
- Execute real read-only `sys.tables` discovery, `sys.columns` discovery, rowversion cursor metadata/upper-bound and count 200 seeded `Erp.Part` rows.
- Run the pre-existing `TestSnapshotReaderSQLServerIntegration` for 200 snapshot and 200 incremental rows and `TestKeySnapshotBinaryRowVersionSQLServerIntegration` to prevent rowversion truncation.

**No SQL Server writes are executed by the application tests**. Only the independently checked ephemeral QA initializer performs fixture DDL and seed writes. The fixture credentials are disposable test constants in a GitHub-hosted runner and MUST NEVER be reused for any persistent service.

## How to read the evidence

Require `SQL Server Disposable Read-Only QA / SQL Server 2022 fixture + SELECT-only integration` to succeed on the **exact PR commit** before merge, in addition to existing main CI. Merge only after both pass. Check the new `main` workflow again after merge.

Standalone local use remains available using `deploy/local-qa/compose.qa.yml` and the repo's isolation scripts; tests need `DATAFLOW_INTEGRATION_MSSQL_ISOLATED=true` and a DSN to the **identity-verified** disposable fixture. Never point them at an Epicor or any non-disposable host. GitHub CI is the durable default when local Docker is unavailable.

## Not proven

No production IdP/Vault integration, no dynamic saved connection routing, no user-facing Test Connection endpoint, no driver connection to real Epicor, no 7m+ benchmarking, no hardening of SSL certificate trust for production (runner-local self-signed cert uses lab-only trust override), no production GO. Future V3.2C-2B secure driver probe must still combine OIDC-scoped TEST, trusted workspace authorization, operator-fixed egress/DNS/TLS, rate limiting, Vault secret access and audit **before any real target dialing**.
