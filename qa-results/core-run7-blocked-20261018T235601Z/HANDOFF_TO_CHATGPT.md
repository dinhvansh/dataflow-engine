# CORE-RUN-7 — Cloud execution handoff

## Verdict

**BLOCKED — benchmark not started. CORE-RUN-7 remains OPEN.**

The repository's mandatory disk guard refused the 7,000,000-row workload before stack startup or fixture seeding. Workspace and Docker storage share the 32 GB root filesystem, which had only **2,388,766,720 bytes (2.22 GiB) free**, while CORE-RUN-7 requires **17,179,869,184 bytes (16 GiB)** free.

No smaller workload was substituted and no performance result was invented.

## Git and work order

- GitHub issue: [#109](https://github.com/dinhvansh/dataflow-engine/issues/109)
- Exact inspected `main`: `ee8401e0ca1b21c666d2dd365f11142f478fce3c`
- Latest handoff, `docs/CORE_RUN7_BENCHMARK_ACCEPTANCE.md`, `docs/CORE_FIRST_ENGINE_GATE.md`, runner and isolation/preflight scripts were read from that SHA.
- Open PRs at preflight: none.
- Latest `main` CI and disposable SQL Server read-only workflow: PASS.

## Resource facts

Captured at `2026-10-18T23:56:01Z`:

| Resource | Actual |
|---|---:|
| CPU | 3 vCPU, AMD EPYC 9V45 |
| RAM | 10,452,471,808 bytes total; 9,667,764,224 available |
| Swap | 0 bytes |
| Root/workspace filesystem | 33,770,192,896 bytes total |
| Free workspace/Docker storage | **2,388,766,720 bytes** |
| Filesystem use | 93% |
| Docker | 28.4.0, `vfs`, local socket healthy |
| Docker images | 3.177 GB |
| Docker local volumes | 1.758 GB; zero reclaimable reported |

The available RAM was not the blocking guard. Disk capacity alone is a mandatory blocker. With only 3 vCPU and no swap, performance representativeness would also require owner review even after capacity is expanded.

## Commands and actual results

```text
DATAFLOW_QA_MIN_FREE_BYTES=17179869184 \
  bash deploy/local-qa/check-disk-capacity.sh "$PWD"
exit 1
QA disk guard: workspace has 2388779008 bytes free; requires at least 17179869184
```

```text
bash deploy/local-qa/preflight.sh
exit 1
QA disk guard: workspace has 2388774912 bytes free; requires at least 4294967296
```

The slight byte difference is normal filesystem activity between samples. Both are far below the hard threshold.

## QA isolation observations

- Docker daemon is local and reachable at `/var/run/docker.sock`.
- Existing database containers are stopped and labeled `com.docker.compose.project=dataflow-localqa`, `com.dataflow.owner=dataflow-localqa`, `com.dataflow.role=local-qa`.
- Named SQL Server, PostgreSQL and ClickHouse volumes carry `dataflow-localqa` owner/role labels.
- Services were **not restarted** because disk preflight failed.
- `verify-isolation.sh` was not executed against live databases because the services remained stopped and the isolated worktree intentionally had no copied credential file.
- No Docker volumes, containers, images or user worktrees were deleted or reset to manufacture capacity.

## Benchmark results

| Metric | Result |
|---|---|
| 7,000,000-row seed | NOT RUN |
| SQL Server source rows | N/A |
| ClickHouse physical rows | N/A |
| Initial elapsed time | N/A |
| Throughput | N/A |
| Incremental run 2 | NOT RUN |
| Incremental rows written | N/A |
| Duplicate count | N/A |
| Checkpoint generation | N/A |
| Peak benchmark CPU/RAM | N/A — benchmark monitor never started |
| ClickHouse I/O | N/A |

This is **not FAIL** because the application workload never ran. It is **BLOCKED** by the environment acceptance gate.

## Safety

- No real ERP endpoint or credential was used.
- No SQL Server, PostgreSQL or ClickHouse write was performed.
- `DataFlowFixtureQA.Erp.PartTranScale7M` was not seeded.
- `deploy/local-qa/up.sh` and `run-core-7m.sh` were not invoked after the failing guard.
- No destructive cleanup occurred.

## Minimum owner-local rerun checklist

1. Use an owner-approved disposable machine where both workspace and Docker root each show at least 16 GiB free; preserve unrelated volumes.
2. Check out immutable SHA `ee8401e0ca1b21c666d2dd365f11142f478fce3c` or revalidate the newer `main` and runner diff.
3. Create the ignored local QA environment with synthetic credentials only; confirm no `SRVERPTEST` or `ERPDBTEST` reference.
4. Run `bash deploy/local-qa/up.sh`; require isolation/read-only checks to pass for `DataFlowFixtureQA` and `qa_erp_reader`.
5. Run exactly:

   ```bash
   DATAFLOW_QA_ALLOW_7M=YES_I_ACCEPT_DISPOSABLE_QA_SEEDING \
     bash deploy/local-qa/run-core-7m.sh
   ```

6. Preserve the complete generated `qa-results/core-seven-million-<UTC>/` directory before cleanup.
7. Transfer sanitized `benchmark-result.json`, `actual_benchmark.log`, `docker_stats.csv` and `seed_result.log`; verify 7,000,000 physical rows, replay 0 rows, no duplicates and checkpoint generation 2 before declaring PASS.

## Recommendation

Provision a larger disposable runner or expand the current workspace/Docker filesystem, then rerun from the start. Do not close issue #109 and do not mark CORE-RUN-7 PASS from CI-contract tests or this blocked preflight.
