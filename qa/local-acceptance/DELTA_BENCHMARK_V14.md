# v1.4 — Safe incremental delta performance QA

This adds **executable** 1k/10k/50k synthetic change benchmarks against a
100,000-row disposable baseline, using physical copy-on-write staging.
There are 3 separate measured trials at each size, 9 samples total.

Each trial checks source-defined ground truth: \`count=100000\`,
\`uniqExact(ID)=100000\`, and exact \`sum(Revenue)\`. If a comparison is wrong
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
stage elapsed milliseconds, effective changed-row/s and reconciliation.
The summary reports median/min/max among 3 samples, **not** statistically
reliable p95.

## What these measurements prove and do not prove

* This times **only** ClickHouse BuildDeltaStage (including safety checks,
  physical copying and row validation). It is not source-to-destination
  end-to-end ingest, PostgreSQL orchestration, approve/publish/rollback,
  complete crash-to-terminal recovery, or Airbyte comparison.
* Staged 100k baseline rows are physically recopied each time. 
  \`changed_rows_per_second\` therefore must not be interpreted as intrinsic
  fast-in-place CDC throughput.
* CPU/RAM/IO sampling, 3-run fault recovery, 7m PartTran, and production
  capacity decision remain separate QA gates.
* No Epicor database is contacted, altered, or used as a test fixture.
  Destructive cleanup only affects uniquely generated \`scratch.dft_*\`,
  \`scratch.dfd_*\`, and \`scratch.dft_stage_*\` tables.

**Do not mark PERF-05 fully PASS until executed on the intended local QA
hardware, and PERF-06 remains BLOCKED.**
