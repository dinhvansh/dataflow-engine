# QA Local — Connection → Sync Setup → Bind → Run

Status: **disposable local QA, not production**. No real ERP test or writes are required. Main acceptance uses synthetic SQL Server `DataFlowFixtureQA`, PostgreSQL `dataflow_qa`, and ClickHouse `scratch`.

## Before you start

- Pull the latest **merged** `main` after CI passes. Do not use an unmerged PR as an accepted release.
- Use Windows PowerShell with WSL/bash, Docker Desktop, Go and Node available. Docker's WSL VHDX should be on E: if C: is small; scripts do not relocate it automatically.
- Never point `DATAFLOW_SQLSERVER_DSN` or approved runtime endpoints at company ERP.
- Never reset unknown Docker resources. Only project `dataflow-localqa` with verified ownership labels can be touched.

## Start the isolated QA databases

From repository root, in a trusted terminal:

```powershell
bash deploy/local-qa/create-env.sh
bash deploy/local-qa/up.sh
```

This creates private ignored `.env.qa.local` with generated secrets and builds labeled fixture containers (subject to disk preflight). PostgreSQL init scripts run **only on a new database volume**. For an existing preserved QA volume, safely apply migration `032_sync_stream_selected_columns.sql` if not yet present; do not drop/reset the database just to apply a schema update.

For the live QA Connection Manager, change the existing line `DATAFLOW_ENABLE_RUNTIME_CONNECTIONS_LAB=false` to `true` in ignored `.env.qa.local`; it must retain an independent admin token and AES-GCM key. Do not commit these values. For browser login, `bash deploy/local-qa/setup-login.sh` sets a local admin/operator password (terminal and permission constraints apply).

See `docs/CONNECTION_RUNTIME_LAB.md` for exact approved SQL/ClickHouse endpoints, TLS/cert and credential rules. The Docker API service pins the only approved internal destinations to `mssql:1433` and `clickhouse:8123`.

Start the QA UI using `DATAFLOW_LIVE_QA_OPT_IN=true bash deploy/local-qa/start-sync-setup.sh` in a trusted bash shell. This guarded command verifies the labeled QA project and disk headroom, applies additive PostgreSQL migrations 031/032 without resetting volumes, then builds and starts the QA API/Web/dbt worker. Never point it at unrelated containers. It expects the generated private QA env and a readable trusted CA bundle.

## Browser workflow

1. Open the loopback QA web URL `http://127.0.0.1:15173` and sign in as local admin, or use the isolated lab token flow.
2. Open **Connections** → **Live Lab Connection Manager**. Add SQL Server using approved `mssql:1433` / `DataFlowFixtureQA` / `qa_erp_reader`, and ClickHouse using approved `clickhouse:8123` / `scratch`. Passwords are those generated in the **ignored local QA env**, not Epicor. Choose **Save encrypted draft** and **Test real connection** for both.
3. In **Sync Setup · saved connections**, select the two verified profiles. Click **Discover**, select schema `Erp`, choose a table with a unique `SysRowID` and native `SysRevID`, then click **Columns**.
4. Uncheck optional data columns as needed; the key/cursor pair cannot be unchecked. Select batch/workers/queue and **Save stream + Bind**. This is an atomic database operation. For an existing stream, the UI must refuse reconfiguring its source or projection; choose another isolated table or a fresh approved fixture instead of resetting its checkpoint.
5. Click **Run selected stream**, follow its run UUID and inspect the corresponding rows in ClickHouse `scratch.dataflow_raw_events`. A replay should not advance the physical row count when there are no source changes. For downstream Core dbt/DQ/publish use the saved stream in Pipelines.

## Expected QA gates

- Wrong password: Test fails and profile is not usable. Changed profile revisions revoke old bindings.
- Wrong source key/cursor, duplicate/unknown columns, target other than `scratch.dataflow_raw_events`, unapproved endpoint: server rejects without any stream/checkpoint/binding write.
- Valid new stream: selected columns persisted; source remains SELECT-only; worker reads the verified profile; checkpoint updates only after sink verification.
- Repeat save on the same table/destination: HTTP 409 rather than silently rebind or reset a prior cursor.

## Explicit limitations

This wizard supports **one table per save** and **Incremental RowVersion** to the existing scratch raw-events table. Full refresh remains supported by the separate *legacy isolated bootstrap*, but NOT by this verified-profile managed wizard. Multi-stream selection, arbitrary destination table/mapping, CDC hard deletes and production IdP/Vault/TLS are **not** certified. Do not present these as working features until corresponding backend/runtime QA exists.

No re-benchmark of 7 million rows is required to evaluate this connection/configuration slice. Local acceptance should use small fixture tables and respect disk/CPU/RAM preflight.
