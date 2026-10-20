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

This creates private ignored `.env.qa.local` with generated secrets and builds labeled fixture containers (subject to disk preflight). PostgreSQL init scripts run **only on a new database volume**. For an existing preserved QA volume, the guarded `start-sync-setup.sh` applies additive migrations 031, 032 and **033**; do not drop/reset the database just to apply a schema update.

For the live QA Connection Manager, change the existing line `DATAFLOW_ENABLE_RUNTIME_CONNECTIONS_LAB=false` to `true` in ignored `.env.qa.local`; it must retain an independent admin token and AES-GCM key. Do not commit these values. For browser login, `bash deploy/local-qa/setup-login.sh` sets a local admin/operator password (terminal and permission constraints apply).

See `docs/CONNECTION_RUNTIME_LAB.md` for exact approved SQL/ClickHouse endpoints, TLS/cert and credential rules. The Docker API service pins the only approved internal destinations to `mssql:1433` and `clickhouse:8123`.

Start the QA UI using `DATAFLOW_LIVE_QA_OPT_IN=true bash deploy/local-qa/start-sync-setup.sh` in a trusted bash shell. This guarded command verifies the labeled QA project and disk headroom, applies additive PostgreSQL migrations 031/032 without resetting volumes, then builds and starts the QA API/Web/dbt worker. Never point it at unrelated containers. It expects the generated private QA env and a readable trusted CA bundle.

## Batch wizard (Full Refresh + Incremental, 1–10 streams)

After two saved and **verified** QA profiles are available, use **Airbyte-style Batch Sync Setup · QA** in Connections. Choose the source/destination, Discover metadata, select a table/columns, choose **Full Refresh** (atomic replacement) or **Incremental RowVersion**, and supply a destination table alias. Click **Add configured table to batch**, repeat up to 10 distinct tables, then **Save & Bind** once. Each saved item has an independent **Run** and status control. A generated private ClickHouse target has the form `scratch.dfqa_<alias>_<random suffix>`; the database is always scratch, not an arbitrary production target. See [QA_MULTI_STREAM_FULL_REFRESH.md](QA_MULTI_STREAM_FULL_REFRESH.md) for exact runtime guarantees and limitations. Full runs leave their incremental cursor unchanged; do not switch modes on a saved stream.

## Legacy single-table incremental browser workflow

1. Open the loopback QA web URL `http://127.0.0.1:15173` and sign in as local admin, or use the isolated lab token flow.
2. Open **Connections** → **Live Lab Connection Manager**. Add SQL Server using approved `mssql:1433` / `DataFlowFixtureQA` / `qa_erp_reader`, and ClickHouse using approved `clickhouse:8123` / `scratch`. Passwords are those generated in the **ignored local QA env**, not Epicor. Choose **Save encrypted draft** and **Test real connection** for both.
3. In **Sync Setup · saved connections**, select the two verified profiles. Click **Discover**, select schema `Erp`, choose a table with a unique `SysRowID` and native `SysRevID`, then click **Columns**.
4. Uncheck optional data columns as needed; the key/cursor pair cannot be unchecked. Select batch/workers/queue and **Save stream + Bind**. This is an atomic database operation. For an existing stream, the UI must refuse reconfiguring its source or projection; choose another isolated table or a fresh approved fixture instead of resetting its checkpoint.
5. Click **Run selected stream**, follow its run UUID and inspect the corresponding rows in ClickHouse `scratch.dataflow_raw_events`. A replay should not advance the physical row count when there are no source changes. For downstream Core dbt/DQ/publish use the saved stream in Pipelines.

## Expected QA gates

- Wrong password: Test fails and profile is not usable. Changed profile revisions revoke old bindings.
- Wrong source key/cursor, duplicate/unknown columns, unapproved endpoint or non-scratch target: server rejects without any new stream/checkpoint/binding write. The legacy single-table endpoint remains limited to `scratch.dataflow_raw_events`; the batch endpoint generates distinct approved scratch tables.
- Valid new stream: selected columns persisted; source remains SELECT-only; worker reads the verified profile; checkpoint updates only after sink verification.
- Repeat save on the same table/destination: HTTP 409 rather than silently rebind or reset a prior cursor.

## Explicit limitations

The **legacy single-table wizard** still supports one Incremental Stream at a time into `scratch.dataflow_raw_events`. The **new batch wizard** supports Full Refresh and Incremental RowVersion for up to 10 tables with generated isolated `scratch` raw-event destinations. This is **NOT typed SQL→ClickHouse column mapping, arbitrary databases, full-source table support without SysRowID/SysRevID, CDC physical deletes, or production IdP/Vault/TLS certification**. New destinations do not automatically feed the Core dbt model path that expects the shared raw-events table. Owner-local acceptance is still required.

No re-benchmark of 7 million rows is required to evaluate this connection/configuration slice. Local acceptance should use small fixture tables and respect disk/CPU/RAM preflight.
