# Incremental benchmark results

Command: `bash deploy/local-qa/run-delta-benchmark.sh`  
Baseline: 100,000 rows; 3 independent samples per delta; reconciliation mismatch: **0 for all 9 samples**.

| Delta | Trials (ms) | Min | Median | Max | Median changed rows/s |
|---:|---|---:|---:|---:|---:|
| 1,000 | 46.356, 47.730, 43.213 | 43.213 | 46.356 | 47.730 | 21,572 |
| 10,000 | 50.396, 45.523, 44.905 | 44.905 | 45.523 | 50.396 | 219,668 |
| 50,000 | 59.340, 50.088, 53.077 | 50.088 | 53.077 | 59.340 | 942,021 |

This measures `BuildDeltaStage`, including copy-on-write of the entire 100k target and reconciliation. It is not intrinsic in-place CDC throughput and is not a production capacity result.

- CPU peak: N/A — no reliable per-process sampler was attached to the sub-second runs.
- RAM peak: N/A — same limitation.
- ClickHouse I/O bytes: N/A — query-level byte counters were not captured by the supplied script.
- Copy-on-write cost: every sample physically rebuilt and validated 100,000 output rows even when only 1,000 changed.

Raw evidence: `artifacts/delta_samples.jsonl` and `artifacts/delta-go-test.log`.
