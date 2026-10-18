# CORE-RUN-6 — real fault injection & recovery (disposable QA only)

**NOT ACCEPTED until exact-head REAL fault workflow PASS.** This code drives real
containers; a source-only review, static CI, or mocked tests do not prove recovery.

## Reproduce

Use fresh isolated QA volumes and the explicit owner-verified opt-ins:

\`bash deploy/local-qa/up.sh\`
\`DATAFLOW_LIVE_QA_OPT_IN=true DATAFLOW_CORE_FAULT_OPT_IN=true bash deploy/local-qa/run-core-faults.sh\`

The QA runner validates the owned Docker network, exact container labels, and
a 10,000-row SQL Server \`DataFlowFixtureQA\` source with SELECT-only
\`qa_erp_reader\`, refusing existing ClickHouse raw and persisted stream metadata.
**Never** use \`SRVERPTEST\` / \`ERPDBTEST\` or a production source.

It pauses the isolated dbt worker to hold a real in-flight core job at
\`waiting_flow\` after committing real SQL Server ingestion. It SIGKILLs the
actual API container, confirms PostgreSQL state survives, restarts it and
verifies that same Run completes dbt A→B→C, DQ and physical ClickHouse publish.

It then **disconnects ClickHouse from the QA network before enqueueing** a
separate durable run; demands a failed terminal job, unchanged cursor,
unduplicated source raw data and untouched published target UUID. The
container is reconnected in \`finally\`. A deliberate new operator Run must
succeed, transfer zero incremental rows, create a new published physical
version and allow UUID-safe rollback without rewinding cursor.

Evidence: \`qa-results/core-run6/fault-result.json\`, uploaded as the
14-day GitHub Actions artifact \`core-run6-real-fault-evidence\` on PASS.

## Explicitly OUT of scope

A failure that becomes terminal **does not automatically resume**; the
recovery test creates a **new** run. SIGKILL here affects the API during
\`waiting_flow\`, not a dbt child in the middle of ClickHouse DDL or an
active SQL Server reader lease. Real dbt mid-execution lease expiry,
concurrent publish race, fault-tolerant 7m benchmark, owner-local demo, and
production IdP/Vault/TLS remain separate acceptance gates. Production NO-GO.
