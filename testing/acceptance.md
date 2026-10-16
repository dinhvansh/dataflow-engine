# Acceptance tests

- [ ] Native binary(8) SysRevID cursor without conversion View.
- [ ] SQL Server UUID/composite pagination behaves exactly as SQL ORDER BY.
- [ ] Full PartTran load validates row counts, keys and sample values.
- [ ] Incremental latest-state materialization reflects inserts and updates.
- [ ] Complete snapshot reconciliation recognizes physical deletes.
- [ ] Long SQL transactions and out-of-order commits tested.
- [ ] Simulate updates during pagination.
- [ ] Crash after destination acknowledgment and before PostgreSQL checkpoint.
- [ ] Contiguous acknowledgment frontier prevents skipping failed batches.
- [ ] Source credentials strictly read-only.
- [ ] Configurable batch size, workers, queue and global caps.
- [ ] DAG cycles rejected and dependencies enforced.
- [ ] SQL job target credentials separated from Epicor.
- [ ] Immutable job versions and run audit are persisted.

Use unit, integration, E2E and fault tests; correctness takes priority over throughput.
