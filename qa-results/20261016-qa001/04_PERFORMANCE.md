# Performance

End-to-end local API full refresh, atomic staging/publish, two workers. Three trials each; all mismatch 0.

| Workload | Raw ms | Median | Range | Median rows/s |
|---|---|---:|---:|---:|
| 10k | 89, 123, 114 | 114 ms | 89–123 | 87,719 |
| 100k | 796, 695, 670 | 695 ms | 670–796 | 143,885 |
| 1m | 6509, 6759, 6605 | 6605 ms | 6509–6759 | 151,400 |
| no-change | 12 ms | 12 ms | one trial | n/a |

CPU/IO peak telemetry was not available at sufficient sampling resolution and is N/A. Delta 1k/10k/50k and three-trial crash recovery are BLOCKED/not run. A 15-event correctness delta took 29 ms. No Airbyte comparison exists. Raw data: `perf.csv`.

