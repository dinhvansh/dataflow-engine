# Fair benchmark: Airbyte vs DataFlow

Compare equal source tables and column projections, hardware limits, ClickHouse schemas, target correctness guarantees and source load conditions.

Workloads: cold full load, incremental inserts, updates, no-change startup, fault recovery and batch/worker size sweeps.

Measurements: startup latency, completion time, rows/sec, MB/sec, source CPU/IO, destination CPU/IO, loader RAM, network bytes, mismatch count and recovery success.

Repeat trials at least three times. Report medians, ranges and database impact. Do not claim speedups without measured evidence.
