# COPY TO CODEX LOCAL — DATAFLOW ENGINE v1.3 LOCAL ACCEPTANCE

**You are the implementation and QA agent running on my Windows PC. Execute the task; do not merely write a test plan.**

Repository: `https://github.com/dinhvansh/dataflow-engine`.

Your complete mandatory specification is **`qa/local-acceptance/CODEX_LOCAL_QA_MISSION.md`** and **`qa/local-acceptance/TEST_CASE_CATALOG.md`**, both tracked in this repository. Also read `qa/local-acceptance/REPORT_TEMPLATE.md` and `qa/local-acceptance/README_FIRST.md` end to end, then inspect the actual code, README, migrations, CI workflow, implementation docs, environment variables and current Git state **before running any write, test or container command**. The historical repo baseline reference when this QA mission was drafted is `b7cfc40d3ca88f0c21ace3eb28adc361f705ce74`, but verify current `origin/main` and report divergence/conflicts before proceeding. Do not overwrite any local uncommitted changes.

## Your work is hands-on, not advisory

1. Check Windows/WSL2/Docker Desktop resources, versions, ports, current containers and Git. Do not stop/remove unrelated existing services.
2. Create a new branch `test/local-qa-v14-<date>` and **self-provision an isolated, reproducible test stack** under `deploy/local-qa/`: SQL Server 2022 fixture representing Epicor `Erp` schema, ClickHouse 25.3 `scratch` Atomic, PostgreSQL 16, separate dbt worker, Go API and React UI. Choose free localhost ports and project-specific volumes and labels. Generate secure secrets in ignored `.env.qa.local`, no production secrets.
3. Self-generate deterministic synthetic datasets from SQL Server fixture: Part, PartTran, InvcHead/InvcDtl with `uniqueidentifier`, native `rowversion`, decimal, datetime, nullable and Vietnamese Unicode; baseline 50–200, 10k, 100k and optional 1m / ~7m if machine resources allow. Generate insert/update/delete/duplicate/out-of-order/late/rollback/no-change scenarios, fixture manifest and **ground truth** for each scenario.
4. Build repeatable PowerShell scripts for setup, seed, smoke, integration, chaos/recovery, performance, logs and reporting. Make tests fail closed if any connection points to ERP TEST/production, non-QA PostgreSQL/ClickHouse or unknown Docker containers. Existing Go integration tests may drop tables: run them ONLY against your new disposable QA DBs.
5. Run the actual Go unit/race/vet/build, React build, PostgreSQL/ClickHouse/dbt integration suite and **browser interaction tests** when browser automation is available. Then execute real local E2E: SQL Server fixture → Extract → ClickHouse Load → Visual Transform/SQL → dbt View → Lineage → Auto Build A→B→C → Safe Table staging/DQ/publish/rollback → Incremental insert/update/delete + correct cursor publish/rollback. Collect exact source/target reconciliations (counts, PKs, decimals, aggregates and stable row samples).
6. Run fault injection against **ONLY your QA-owned services**: worker/API restart, ClickHouse outage, wrong credentials, retries, stale lease, SQL changed mid-flow, DQ fail, publish interrupted. Verify no phantom success, skipped cursor, broken target or lost backup.
7. Benchmark 10k/100k/1m and delta scenarios if resource safe; at least 3 trials per workload, log throughput/latency/CPU/RAM and mismatch. Test ~7m only after a resource gate and user consent. Airbyte comparison only if an isolated comparable test baseline exists. Never invent performance numbers.
8. When a bug appears, record severity P0–P3 and evidence, reproduce with minimal test, add regression test, patch **only local QA branch**, retest affected + full suite, document commits. P0/P1 remain release blockers. Do not push/merge unless I authorize it.
9. Create `qa-results/<RUN_ID>/` with ALL specified reports/evidence and **`HANDOFF_TO_CHATGPT.md`**. Zip sanitized report/artifacts as `DATAFLOW_LOCAL_QA_RESULTS.zip`, and give exact paths/summary so I can upload to ChatGPT for independent review.

### Non-negotiable safety

**Never run INSERT/UPDATE/DELETE/DDL on actual `SRVERPTEST` / `ERPDBTEST`.** Actual Epicor TEST, if available, is read-only and opt-in after synthetic pass; do not wait for Epicor credentials to proceed with synthetic tests. Never change corporate DLP, firewall, gateway or ERP isolation settings. No `docker system prune` or destructive commands outside QA allowlist. No real passwords or actual company data in reports/commits.

### Required final answer format

`Verdict | Git SHA/branch | Environment/services | PASS/FAIL/BLOCKED counts | Top bugs | Data mismatch count | Cursor/rollback result | Benchmark median and raw measurements | Test gaps | QA result folder/ZIP | Exact next actions`.

Provide a short status update at meaningful milestones; run the tasks in this session where tools are available. If local execution is blocked, leave reproducible scripts, mark tests BLOCKED and report precisely what was not executed. **Do not label planned, mocked, or skipped tests as PASS.**