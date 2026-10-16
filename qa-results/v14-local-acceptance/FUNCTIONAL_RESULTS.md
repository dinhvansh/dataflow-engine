# Functional results

- SQL Server 2022 fixture: `DataFlowFixtureQA`, 10,009-row `Erp.PartTran` baseline; `qa_erp_reader` SELECT succeeded and mutation was denied.
- Full SQL Server → ClickHouse atomic sync: 10,009 rows written in 11 batches; destination count and distinct `SysRowID` were both 10,009; elapsed 99.7 ms on this local run.
- SQL Server snapshot/type reader integration passed against the real fixture.
- PostgreSQL migration 015 applied twice successfully. All seven history witness columns are present.
- Incremental integration passed insert/update/delete tombstone replay, two cursor commits, publish, preserved backup, rollback, and exact output reconciliation.
- Late arrival/history mutation was rejected with `LATE_EVENT_OR_HISTORY_MUTATION`. A subsequent run was also rejected until integrity was restored. Disabling History Guard after use was rejected by the v1.4 regression.
- Visual compile/preview executed against real ClickHouse and remained read-only.
- dbt 1.11.0 + dbt-clickhouse 1.10.3 materialized real ClickHouse Views.
- Safe Table staging did not alter the published target; publish and rollback preserved expected values and UUID fencing.
- Auto Build executed A → B → C, enforced DQ, blocked drift/failure propagation and produced the expected downstream output.
- Lineage API returned the saved model dependency chain and impact metadata.

Known limitation: physical deletion from a SQL Server source is invisible to rowversion polling unless a tombstone/change-log/CDC mechanism exists. This remains BLOCKED for production acceptance.
