# CORE-RUN-7 — real 7,000,000-row performance and full-scale acceptance

## Owner-local QA result — PASS (2026-10-20)

Tested SHA `dc40d9cc4a0b7953f1a7410bf09c71023d06d088` on branch `test/core-run7-full-scale-20261020` completed both the original managed-ingestion gate and the full-scale dbt/DQ/fault/publish follow-up on the owner's disposable Windows QA host.

| Gate | Status | Measured evidence |
| --- | --- | --- |
| Preflight / ownership / source isolation | PASS | only labeled `dataflow-localqa`; synthetic `DataFlowFixtureQA`; `qa_erp_reader` verified SELECT-only |
| Initial managed ingestion | PASS | 7,000,000 rows, 700 batches, 126.269 s, 55,437.07 rows/s |
| Ingestion replay/idempotency | PASS | 0 writes, 0.165 s, generation 2, original 7m unchanged |
| dbt A→B→C / DQ / physical publish | PASS | 7,000,000 rows and unique keys, fingerprint reconciled, 188.766 s, 37,082.95 rows/s |
| Full pipeline replay | PASS | 0 writes, generation 2, 7,000,000 verified, 69.265 s |
| Rollback/idempotency/checkpoint | PASS | repeated physical rollback restored prior UUID and all rows; 2.516 s |
| Negative DQ / no publish | PASS | all 7m evaluated; failure preserved prior UUID; 39.390 s |
| Active SQL crash recovery / fencing | PASS | actual SQL observed, dbt worker SIGKILL, reclaim, 2 attempts, stale owner update 0 rows, rebuild/publish/rollback; 114.062 s |

Local sanitized evidence:

- `qa-results/core-run7-20261020T051740Z/` — report, test matrix, environment, pipeline/fault results, resource samples, logs and SHA256 manifest.
- `qa-results/core-seven-million-20261020T051748Z/` — ingestion benchmark result, seed/test logs and Docker samples.

PR [#134](https://github.com/dinhvansh/dataflow-engine/pull/134) had all 10 hosted checks PASS at the execution SHA. Hosted checks are regression evidence, not a substitute for the local 7m evidence.

## Resource and disk conditions

The owner-authorized Windows exception permits a **2 GiB C: floor** only when the Docker virtual disk, workspace, Go caches, temporary files and evidence are on E:, labeled disposable services are verified, and a C: watchdog stops only those QA containers if the floor is reached. The normal 16 GiB E:/Docker guard remains active. This is a hardware-specific, non-certifying exception and does not change repository defaults or production sizing.

For the accepted full-scale run:

- Docker VHDX: `E:\docker\DockerDesktopWSL\disk\docker_data.vhdx`
- C: minimum sampled free: 8.727 GiB
- E: minimum sampled free: 2310.082 GiB
- 180 samples; peak ClickHouse 586.60% CPU / 4.554 GiB RAM; peak SQL Server 99.38% / 1.572 GiB

## Executable opt-in on an owned disposable QA machine

Normal preconditions remain: at least 16 GiB free on workspace and Docker storage, sufficient RAM/CPU, Go/Python3/Docker, and isolated volumes verified by preflight. `SRVERPTEST`, `ERPDBTEST` and company production servers are prohibited. The SQL reader may only SELECT from the disposable fixture source.

Original ingestion/replay gate:

```bash
bash deploy/local-qa/up.sh
DATAFLOW_QA_ALLOW_7M=YES_I_ACCEPT_DISPOSABLE_QA_SEEDING bash deploy/local-qa/run-core-7m.sh
```

Full-scale follow-up on an already initialized and verified disposable stack:

```bash
DATAFLOW_LIVE_QA_OPT_IN=true \
DATAFLOW_QA_ALLOW_7M=YES_I_ACCEPT_DISPOSABLE_QA_SEEDING \
DATAFLOW_CORE_FULL_7M=YES_OWNED_DISPOSABLE_7M \
bash deploy/local-qa/run-core-full-7m.sh
```

The guarded scripts refuse missing opt-ins, non-owned resources, insufficient disk, wrong source database/login and incorrect source counts. Resetting owned volumes remains a separate explicitly authorized action. Never bypass guards to obtain PASS.

## Claim boundary

**CORE-RUN-7 is PASS only for this scoped owner-local synthetic QA acceptance.** It proves one host's 7m full sync, zero-write replay, row-preserving transform/DQ/publish/rollback and the documented active-SQL recovery/fencing case. It does not prove real Epicor data behavior, source physical deletes, nonzero CDC deltas, all crash/network topologies, parallel/conditional workflows, production capacity, security/IdP/Vault/TLS deployment, HA/DR, funding readiness or Production GO. Those remain separate acceptance decisions.
