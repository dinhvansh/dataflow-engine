# v1.4 — Safe incremental delta performance QA

This runs **executable** 1k/10k/50k synthetic change benchmarks against
100,000-row and 1,000,000-row disposable baselines, using physical
copy-on-write staging. There are 3 measured trials at each size/baseline,
18 samples total.

Each trial checks fixture-defined ground truth: baseline row count,
\`uniqExact(ID)\`, and exact \`sum(Revenue)\`. If a comparison is wrong
the test **fails**; it does not emit a passing performance verdict.

## Execution

From the repository root, after starting the **QA-owned** Docker stack:
\`\`\`bash
bash deploy/local-qa/run-delta-benchmark.sh
\`\`\`

Requires the local QA ClickHouse on \`127.0.0.1:18123\`, Go 1.23+ and
\`.env.qa.local\`. Script verifies container ownership labels and SQL Server
QA-only read permissions before running. Direct Go test execution requires
explicit \`DATAFLOW_BENCHMARK_QA=true\` and
\`DATAFLOW_INTEGRATION_CH_ISOLATED=true\`, and rejects non-loopback endpoints.

Raw JSONL and logs are saved under ignored \`qa-results/delta-<timestamp>/\`.
Each sample includes changed rows, trial, baseline/output rows, events,
stage and partial-stage recovery milliseconds, physical publish/rollback
milliseconds, effective changed-row/s and reconciliation. The run also
captures CPU/RAM samples and ClickHouse system I/O counters.
The summary reports median/min/max among 3 samples, **not** statistically
reliable p95.

## What these measurements prove and do not prove

* This times ClickHouse BuildDeltaStage, rebuilding an intentionally partial
  stage, physical publish and rollback. Each trial verifies target UUID and
  count/unique/checksum before and after publication, plus replay safety.
  It is not source-to-destination end-to-end ingest, PostgreSQL orchestration,
  actual worker SIGKILL, complete crash-to-terminal recovery, or Airbyte comparison.
* Staged 100k baseline rows are physically recopied each time. 
  \`changed_rows_per_second\` therefore must not be interpreted as intrinsic
  fast-in-place CDC throughput.
* A real process-crash recovery sequence with retry-to-terminal timing,
  7m PartTran, and production capacity decision remain separate QA gates.
* No Epicor database is contacted, altered, or used as a test fixture.
  Destructive cleanup only affects uniquely generated \`scratch.dft_*\`,
  \`scratch.dfd_*\`, and \`scratch.dft_stage_*\` tables.

The 2026-10-27 owner-local run passed 18/18 ClickHouse delta samples on the
intended QA machine; see [sanitized evidence](../../docs/evidence/issue153-delta-20261027/README.md).
PERF-06 remains open because a simulated partial stage is not a process crash.
