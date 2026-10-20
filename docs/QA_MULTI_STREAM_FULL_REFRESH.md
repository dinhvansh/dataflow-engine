# QA multi-stream Sync Setup — Full Refresh + Incremental

## Scope

The isolated owner QA Connection Manager now has a **Batch Sync Setup** wizard in addition to the earlier single-table incremental wizard. The full workflow uses recently verified, encrypted SQL Server and ClickHouse profiles; the source remains SELECT-only, and all writes stay inside the disposable ClickHouse `scratch` database. This is a QA acceptance slice, **not production certification**.

## Flow

1. Add/Test two approved QA connections (`DataFlowFixtureQA` SQL Server reader and `scratch` ClickHouse writer) in Connections.
2. In **Batch Sync Setup**, select source + target; Discover the schema, choose a table and inspect columns.
3. Select data columns; this raw-events pipeline currently requires catalog-proven `SysRowID` (uniqueidentifier with unique index) and `SysRevID` (native rowversion) even when using Full Refresh. Unsupported source tables are explicitly rejected.
4. Choose **Incremental RowVersion** or **Full Refresh (atomic replacement)** per table; enter a safe destination **alias**, resource limits and **Add to batch**. Repeat for at most 10 distinct source tables.
5. Press **Save & Bind**. The API validates all stream definitions and both profile revisions, requires SQL Server SNAPSHOT isolation and per-table SELECT-only grants, and inserts every stream, checkpoint and binding in a **single PostgreSQL transaction**. A bad table/alias rolls everything back.
6. Click **Run** per saved stream. The durable queue and worker execute with the saved verified profile. A unique ClickHouse physical table of form `scratch.dfqa_<alias>_<12-hex-id>` is provisioned lazily by the isolated worker. The worker never adopts a user-named production table.

## Runtime semantics

- **Incremental:** selected columns, SQL Server RowVersion paging, durable fenced checkpoint, append-only raw event writer; replay can physically duplicate interrupted events, and readers must honor logical dedup semantics.
- **Full Refresh:** SQL Server SNAPSHOT transaction, catalog-validated projection, insert into a private ClickHouse stage, verify inserted row count, then `EXCHANGE TABLES` against the stream's **dedicated** scratch table. A second run replaces that table's contents, not appends. Full Refresh does **not** turn into Incremental automatically and does not assign a RowVersion cutoff.
- Both modes use the existing raw event schema `(_stream_id,_pk,_sysrevid,_payload,_run_id,_batch_seq,_ingested_at)`. Destination mapping here means safe table **naming/isolation**, **not** automatic typed SQL→ClickHouse column materialization/mapping.
- Failures before publish leave the prior full-refresh target unchanged; incomplete staging artifacts can require isolated QA cleanup. A crash after an ambiguous ClickHouse EXCHANGE must be reconciled before treating its database state as final; bounded queue replay is a test behavior, **not** a production HA guarantee.
- No execution against real Epicor/ERP, no source schema changes, no arbitrary ClickHouse database, no DML against SQL Server.

## Local QA

Pull merged `main`. Use `docs/QA_CONNECTION_SYNC_QUICKSTART.md` and the guarded `deploy/local-qa/start-sync-setup.sh` launcher (which now applies **migration 033** for preserved QA PostgreSQL volumes). **Do not reset Docker volumes** to install an additive migration. For exact owner acceptance run `DATAFLOW_LIVE_QA_OPT_IN=true bash deploy/local-qa/run-runtime-connections-real.sh`: real QA SQL Server/PG/ClickHouse tests exercise a two-table atomic setup, snapshot reload of 200 rows twice, independent Incremental sync and physical row counts. CI runs the equivalent isolated disposable fixture via `.github/workflows/sqlserver-readonly-qa.yml`.

## Future production gates

Connection Manager production IdP/Vault/TLS and workspace permissions (#135), general schema/type mapping into typed ClickHouse tables, multi-schema/destination policy, CDC hard deletes, alias rotation and zero-downtime full/incremental cutover (#137) remain open. Do not present these QA test behaviors as production-ready.
