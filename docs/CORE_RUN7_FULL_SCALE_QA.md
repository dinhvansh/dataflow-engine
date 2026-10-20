# CORE-RUN-7 full-scale follow-up

This owner-only suite complements, and does not redefine, the ingestion/replay
acceptance in CORE_RUN7_BENCHMARK_ACCEPTANCE.md. A skipped test is NOT PASS.

## Workload and assertions

- Existing `run-core-7m.sh`: seed exactly 7,000,000 synthetic SQL Server rows,
  verify SELECT-only access, managed ingestion and zero-write incremental replay.
- Real API pipeline: ingest that source again into a fresh raw table; execute
  row-preserving dbt A -> B -> C views, DQ and physical MergeTree publication.
  Final materialized output must contain 7,000,000 distinct keys (not one
  aggregate row). Compare source count/TranNum sum with raw, and raw/output
  key-plus-payload hash aggregate, count and uniqueness.
- Repeat the pipeline: zero additional raw writes, checkpoint generation 2,
  unchanged reconciliation, and a different physical published UUID.
- Rollback twice: restore the exact previous UUID, preserve all 7m rows and
  leave the source checkpoint unchanged.
- Negative DQ: retain 7m rows but choose a deliberately duplicated stream key.
  Require failure, no new publication, and unchanged prior published UUID/data.
- Active-SQL fault: a separate safe-table dbt job reads the actual 7m raw rows.
  Observe its SQL in ClickHouse before SIGKILL of the owned dbt worker. Expire
  only the killed fixture job's lease; rebuild, reconcile all rows, reject stale
  ownership update, publish, rollback and verify checkpoint preservation.

The fault job uses a small per-row sleep solely to make active SQL observable.
Its runtime is not a throughput benchmark. It is a real safe-table worker test,
not a claim that every core coordinator failure window or network topology was
exercised at 7m. View dependencies are not three separately materialized tables.
Source physical deletes, nonzero CDC deltas and production certification remain
outside this follow-up's claims.

## Owner-local execution

Use an already initialized, verified fresh disposable QA stack. Resetting owned
QA volumes still requires explicit owner approval; the runner does not reset.
On Windows set TEMP/TMP/GOCACHE/GOMODCACHE to E:, use Git Bash, supply the trusted
public CA path, and set the three explicit opt-ins:

```
DATAFLOW_LIVE_QA_OPT_IN=true
DATAFLOW_QA_ALLOW_7M=YES_I_ACCEPT_DISPOSABLE_QA_SEEDING
DATAFLOW_CORE_FULL_7M=YES_OWNED_DISPOSABLE_7M
bash deploy/local-qa/run-core-full-7m.sh
```

The existing separately authorized low-C-disk exception requires its existing
two flags; the normal ingestion disk guard remains unchanged. The full runner
monitors E: >=16 GiB and stops owned QA containers when C: approaches 2 GiB.
Docker Desktop storage must be independently confirmed on E: before invocation.

Evidence is local and sanitized under `qa-results/core-run7-<UTC>/`: per-stage
logs, matrix, environment/Git SHA, resource samples, pipeline and fault results.
The existing runner also writes its ingestion evidence in its own timestamped
directory. Keep both. Report FAIL/BLOCKED/NOT_RUN honestly; only all executed
assertions plus reviewed evidence permit a scoped owner-QA PASS.
