# Issue #153 local delta and partial-stage recovery QA

On 2026-10-27, the owner's existing labeled `dataflow-localqa` services passed the isolation, SELECT-only SQL source, ownership and disk preflight. `run-delta-benchmark.sh` then completed **18/18** ClickHouse `scratch` samples: three trials each for 1k, 10k and 50k changed rows against 100k and 1m row baselines. It created and removed only its random benchmark tables. No volume reset, ERP endpoint or 7m rerun was involved.

The enhanced test measures a clean copy-on-write stage, replaces that private stage with a single partial row to simulate an interrupted write, and measures rebuilding the same stage. Before publication the target UUID must remain unchanged. It reconciles row count, unique keys and checksum after recovery and physical publication; publish replay must be idempotent. It then rolls back the physical table, checks the original UUID and checksum, and replays rollback safely. Every trial passed these checks.

| Baseline rows | Changed rows | Median stage ms | Median recovery ms |
|---:|---:|---:|---:|
| 100,000 | 1,000 | 188.980 | 154.538 |
| 100,000 | 10,000 | 143.249 | 157.685 |
| 100,000 | 50,000 | 188.002 | 162.280 |
| 1,000,000 | 1,000 | 687.633 | 772.876 |
| 1,000,000 | 10,000 | 333.698 | 299.616 |
| 1,000,000 | 50,000 | 282.957 | 406.388 |

See [summary](summary.json), [100k samples](delta_samples_100000.jsonl), [1m samples](delta_samples_1000000.jsonl), and [resource peaks](resource_summary.json). Original test logs and 1-second resource samples remain ignored under `qa-results/delta-20261027T022703Z/` on this QA machine. The ClickHouse system I/O counters are included as system-wide context, not attributed to one trial.

This is disposable ClickHouse stage recovery with a simulated partial write. It is not a real worker SIGKILL at each delta size, an end-to-end SQL Server latency test, a production capacity/SLO approval, or certification of source-consistent late events and physical deletes. Issue #153 and production **NO-GO** remain unchanged.
