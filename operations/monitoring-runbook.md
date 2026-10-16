# Monitoring and operations runbook

Record rows/bytes read and written, elapsed seconds, throughput, queue depth, in-flight memory, retries, checkpoint lag, run state and source/target validation differences.

Each log event includes run_id, stream_id, batch_seq, schema_hash and error code. Never log passwords or raw business records.

| Symptom | Recommended response |
|---|---|
| Empty incremental | Check typed cursor and upper watermark |
| Slow load | Profile SQL scan, transform and destination |
| Duplicate logical rows | Check idempotent ODS query strategy |
| Data discrepancies | Pause and perform verified reconciliation |
| Stale lease | Fence outdated worker |
| Changed schema | Pause job and review migration |

Alert when runs fail repeatedly, no success occurs on schedule or reconciliation finds material mismatches.
