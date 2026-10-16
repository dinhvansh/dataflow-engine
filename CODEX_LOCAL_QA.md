# Codex Local QA — DataFlow Engine v1.3

**Entry point for Codex running on the user's Windows PC.** This file is a task handoff, **not a report that testing already succeeded**.

Start by reading ALL of these tracked documents end to end:

1. [Start prompt](qa/local-acceptance/START_PROMPT_FOR_CODEX.md)
2. [Full local test mission](qa/local-acceptance/CODEX_LOCAL_QA_MISSION.md)
3. [68 test cases and expected results](qa/local-acceptance/TEST_CASE_CATALOG.md)
4. [Report template](qa/local-acceptance/REPORT_TEMPLATE.md)
5. [README and handoff](qa/local-acceptance/README_FIRST.md)

## Execution order

**Inspect Git state first.** Do not overwrite uncommitted changes, change `main`, merge or push without the user's authorization. The historic reference SHA in the documents is informational: revalidate `origin/main` and report divergence.

Then execute the mission locally:

- Build an **isolated QA-only Docker environment**: synthetic SQL Server Epicor-like fixture, PostgreSQL, ClickHouse scratch Atomic, restricted dbt worker, DataFlow API, Web UI. Use QA labels, volumes, localhost ports and freshly generated uncommitted secrets.
- Generate deterministic synthetic fixtures plus **ground-truth expectations**, not only random data; include insert, update, delete, CDC cursor, corrupted data, retries and rollback scenarios.
- Execute the 68 cases with actual results and evidence. Tests that cannot run remain **BLOCKED/NOT RUN**, not PASS. Distinguish GitHub CI from local execution.
- Diagnose and fix bugs **only on a local test branch** with reproduction, regression tests, and retest. Record unresolved P0/P1 release blockers.
- Benchmark measured throughput/memory/latency on 10k, 100k and larger batches only when resources allow. **Do not** attempt 7m rows without a resource gate and user's consent.
- Return `qa-results/<RUN_ID>/HANDOFF_TO_CHATGPT.md` and sanitized `DATAFLOW_LOCAL_QA_RESULTS.zip` with matrix, mismatches, logs, commands, patches, raw performance data and verdict.

## Absolute safety

Actual Epicor `SRVERPTEST / ERPDBTEST` is **read-only**, and may only be accessed with explicit permission. **Never** seed or mutate that ERP. All DML/DDL fault injection must target the **new synthetic SQL Server container**; ClickHouse and PostgreSQL destructive tests must use **QA-owned isolated databases**. Do not stop other user's containers, weaken DLP, expose secrets, or run global Docker prune.

Once complete, give the user exact artifact paths so they can submit the handoff back into ChatGPT for review.
