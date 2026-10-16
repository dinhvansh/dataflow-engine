# Bug register

## BUG-V14-QA-01 — fixed

Severity: P2 test repeatability. dbt integration tests hard-coded `127.0.0.1:8123`, but the isolated stack deliberately exposes ClickHouse at loopback port 18123. Tests could not run against the documented QA stack.

Fix: accept `DATAFLOW_INTEGRATION_CH_HOST` and `DATAFLOW_INTEGRATION_CH_PORT`, retaining the original defaults. Regression passed for dbt View, Auto Build, Safe Table and incremental worker tests.

## BUG-V14-QA-02 — fixed

Severity: P2 test repeatability. `TestDBTQueueIntegration` used the fixed model name `dbt_ci_model`, so a second run violated `transform_models_name_key`.

Fix: generate a unique model/target name per run. Repeated and sequential regressions passed.

## LIMIT-V14-01 — open

Severity: production blocker for hard-delete fidelity. Native SQL Server rowversion polling cannot observe a row after it has been physically deleted. Require source tombstones, a permitted change-log, CDC/change tracking, or periodic key reconciliation before production GO.

## ENV-V14-01 — mitigated

The local 32 GB filesystem exhausted free space after repeated 1M-row copy-on-write and build caches. ClickHouse failed closed with `NOT_ENOUGH_SPACE`. QA-only cleanup restored 3.2 GB and all affected regressions passed. Add disk preflight/cleanup thresholds to the harness.
