# DataFlow v1.3 — Safe Incremental (copy-on-write CDC TEST preview)

## Scope and why it is conservative

This release **does not append directly to a published report table** and it is **not yet a high-throughput incremental partition optimizer**. It performs changes by key/cursor against a physical **copy-on-write staging table**, then reuses v1.1's ClickHouse Atomic `EXCHANGE TABLES` and UUID-based idempotent replay so failures do not leave a partly refreshed report. The tradeoff is a full physical copy of unchanged target rows.

Only on disposable `scratch` ClickHouse with Atomic database; this feature is OFF by default.

## Data contract

A physical baseline table must already exist, e.g. `scratch.dft_sales_demo`:

```sql
CREATE TABLE scratch.dft_sales_demo
(InvoiceNum String, Revenue UInt64)
ENGINE=MergeTree ORDER BY InvoiceNum;
```

A delta table in `scratch` must contain the **exact same payload columns in the same order and type**, then the extra columns `_df_version UInt64` and `_df_deleted UInt8`:

```sql
CREATE TABLE scratch.dfd_sales_demo
(InvoiceNum String, Revenue UInt64, _df_version UInt64, _df_deleted UInt8)
ENGINE=MergeTree ORDER BY (InvoiceNum,_df_version);
```

Each CDC event needs a **globally unique increasing UInt64 version**; `_df_deleted=1` means delete, `0` means insert/update. The key must be non-null, stable, and unique in the published target. Target/delta schemas must match, and the delta stream must be **append-only**. Late arrivals with versions at/below an already published checkpoint are unsupported (must be prevented upstream). There is no built-in SQL Server CDC or Epicor delete tracking in this release.

The initial cursor is configured **only on first registration** of a target; subsequent POSTs never reset it. **Choosing the wrong initial cursor can skip data**, so register using verified CDC history, usually zero on a clean disposable test stream.

## Exact processing

1. Go API enqueues a durable `incremental_runs` row (migration 014); dedicated dbt/ClickHouse worker claims it under PostgreSQL lease/fencing.
2. Worker checks the published target's **immutable physical UUID**, its uniqueness and schema agreement with delta.
3. Worker freezes `cursor_lower` and `cursor_upper=max(_df_version)` for this run and rejects duplicate versions, invalid tombstone flags or >50k changes in TEST.
4. Worker builds `scratch.dft_stage_<run_uuid>`: copies old rows with unaffected keys; applies the last event per key from the cursor window, excluding deleted keys.
5. Worker checks staged output key uniqueness and <=1m rows, and verifies the captured delta window did not change during construction. An empty result is valid when all keys are deleted.
6. **Only then** status becomes `ready`. Publish requires explicit UI confirmation `PUBLISH`; until then the published table and cursor are unchanged.
7. During Publish the worker durably prepares old/new physical UUIDs, checks a **fenced PG transaction lock** and calls ClickHouse Atomic `EXCHANGE TABLES`. PostgreSQL records the new target UUID and the upper watermark **in the same transaction that finalizes the run**. If the process crashes between external DDL and PostgreSQL commit, retry checks UUID and recognizes a completed swap instead of reversing it.
8. Prior physical table is preserved under this run's stage name for rollback. `ROLLBACK` requires the last published version, matching physical UUID and no other pending run. It swaps the prior table back and restores `cursor_lower` in PostgreSQL.
9. `no_changes` terminates with no cursor movement and no table DDL.

## UI

Open **Incremental CDC**. Enter target name `dft_sales_demo`, delta name `dfd_sales_demo`, key `InvoiceNum` and verified initial cursor. Click **Build Delta Staging**, inspect event count/new row count and errors, then **Approve & Publish + Cursor**. If a valid earlier version exists, use **Rollback Data + Cursor**.

## Deployment checklist

- Back up isolated PostgreSQL TEST. Apply `migrations/postgres/014_safe_incremental.sql` after migrations 010–013.
- Set `DATAFLOW_ENABLE_INCREMENTAL_TEST=true` on BOTH the isolated Go API and worker; continue requiring `DATAFLOW_ALLOW_TEST_LOAD=true`, `DATAFLOW_ENABLE_DBT_TEST=true`, `DATAFLOW_ENABLE_SAFE_TABLE_TEST=true` and `DATAFLOW_TEST_TARGET_DATABASE=scratch`.
- Worker must have only scratch DDL/SELECT permissions and PostgreSQL job metadata access. Do not give it production warehouse or ERP write privileges.
- Keep the scratch target read-only for other users, including while staging. Physical target mutations outside controlled published versions are rejected when UUID changes; in-place mutations are not fully guarded.
- Do not enable in production. No automatic Epicor SQL Server CDC capture, no physical partition-level incremental speedup, no 7m-row benchmark, no automated rollback-safe global multi-model release, no prod RBAC, no workflow approvals/audit/retention.

## CI gates

- Go race/vet/build and React build.
- Real ClickHouse: multiple updates per key, delete/tombstone, insert, idempotent stage replay, atomic target publish, publish replay no-op, and physical rollback.
- PostgreSQL+ClickHouse: two cursor windows applied in order, cursor moves only after publication, rollback restores prior data **and** checkpoint, and delta can be replayed without double-counting.
- API: authorization, safe identifiers, immutable source/key registration, one active target, no publish before DQ, no publication with a stale checkpoint, and no rollback without a previous physical table.
- Existing View/table/dbt/Auto Build test regressions continue to pass.

This is **logical incremental with copy-on-write publication**, not a promise of lower I/O. Production-speed options to evaluate after Epicor TEST verification: partition replacement, CDC batch consolidation, and reconciliation with delete handling.
