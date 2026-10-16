# Recovery and fault injection

All destructive actions targeted containers labeled `com.dataflow.owner=dataflow-localqa` and database `scratch` only.

| Scenario | Trials | Terminal result | Recovery time ms | Integrity result |
|---|---:|---|---|---|
| Stop ClickHouse during 1M-row atomic sync | 3 | failed, failed, failed | 4683, 4634, 4869 (median 4683) | Published target stayed at exactly 1 row each time; no partial publish |
| Stop PostgreSQL during 1M-row workflow | 3 | failed, failed, failed | 9913, 5053, 5057 (median 5057) | Published target stayed at exactly 1 row each time |
| Restart dbt worker immediately after queue | 3 | succeeded ×3 | 3331, 3369, 3362 (median 3362) | Each View had exactly 8 rows; attempts=1; no duplicate output |

Managed queue integration additionally passed lease expiry, fencing, bounded retry, count validation and atomic checkpoint commit. Incremental integration proved checkpoint rollback and no duplicate event replay.

During the final rerun, the 32 GB execution filesystem reached 100% after repeated 1M copy-on-write trials. ClickHouse correctly returned `NOT_ENOUGH_SPACE`; no result was misreported as a product failure. Only QA-generated scratch/staging tables and disposable caches were removed, freeing 3.2 GB. All affected packages then reran PASS. This is an operational capacity warning for future runners.
