# Core real-data QA slice — SQL Server → ClickHouse → DQ → publish

**Purpose:** produce the first reproducible **cross-service** core acceptance evidence for an investor demo. This is substantially stronger than independent SQL Server and ClickHouse smoke tests but still a **PARTIAL** pipeline, not complete runtime/production approval.

## Tested with actual services, not mock implementations

The dedicated **SQL Server Disposable Read-Only QA** workflow runs on one GitHub-hosted runner with Microsoft SQL Server 2022, real Go Microsoft SQL Server driver, disposable PostgreSQL for auth tests, and ClickHouse 25.3 scratch. The SQL Server fixture is initialized independently and only the `qa_erp_reader` account is allowed for the data-read step.

`TestRealSQLServerToClickHousePublishIntegration` is *opt in* and requires both isolated flags and configured endpoints. It fails closed unless:
- Source database exactly `DataFlowFixtureQA`, login exactly `qa_erp_reader` and the `Erp.PartTran` **effective permission** shows SELECT yes, INSERT/UPDATE/DELETE/ALTER all no.
- Destination is a **loopback-only HTTP** ClickHouse endpoint with `scratch` in `Atomic` database mode. No production endpoints, real ERP secrets or remote client hostnames are accepted. HTTP is allowed in this **disposable local runner only**.
- Source fixture has **10,000 unique TranNum rows**. The test queries the source using SELECT-only SQL to record its count, checksum-like `SUM(TranNum)`, distinct key count and a Vietnamese-Unicode string count.
- The existing bounded `engine.Run` streams SQL Server `Reader{Snapshot:true,BatchSize:250}` into a **new scratch-only** ClickHouse stage through the production writer implementation. Up to three worker writes and three queue slots exercise backpressure; no rowset is synthesized inside the Go test.
- Actual ClickHouse stage DQ verifies nonempty and unique `TranNum` values; cross-system source/destination counts, sum, distinct key and Unicode observations must match.
- The prevalidated staged table is published to a dedicated scratch target by the existing UUID-checked, replay-idempotent `safetable.Publish`. The target is read back and rechecked. A purposely duplicate **synthetic ClickHouse-only** bad stage must fail DQ and leave the published target intact. All newly created scratch tables are cleaned up by this test.

Evidence command (the real network services are preprovisioned by GitHub Actions): `go test ./internal/engine -run '^TestRealSQLServerToClickHousePublishIntegration$' -count=1 -timeout=4m -v`. Inspect the exact-head **SQL Server Disposable Read-Only QA** workflow for the successful run log; a skipped test or an unstarted action does **not** count as PASS.

## What is demonstrably still NOT done

- This QA check orchestrates a single **full snapshot** manually through Go integration-test code, not a persistent, user-triggered pipeline API/Canvas workflow or a durable run record. It does not prove true active connection binding, Vault credential retrieval, TLS-pinned driver network, workspace-scoped live runtime, queue crash recovery or source incremental checkpoints.
- No chained dbt Auto Build A→B→C, real production DQ enforcement, full rollback orchestration, joined stage lineage or 7m+ incremental benchmark in this same cross-system scenario yet. Independent isolated tests exist for some pieces, **not equivalent to one end-to-end result**.
- Cross-system sums and counts are correctness probes, **not** a cryptographic record-level reconciliation. Source-side physical deletes, backdated writes and concurrent version changes need separate acceptance.
- The fixture HTTP ClickHouse and SQL Server's disposable `TrustServerCertificate` setup are prohibited as production TLS patterns. No real ERP contact occurred and ERP remains strictly **SELECT-only**.
- User's fundraising condition is **NOT YET MET** just by merging this step. A repeatable UI/API-started, durable, monitored complete pipeline with failures/retries/incremental/quality/publish and benchmark proof remains a hard gate.

## Next implementation: funding-demo execution path

Implement production-code **durable pipeline run** in a disposable stack, using isolated connection bindings, real source snapshot/incremental + checkpoint, ClickHouse staging, dbt dependency execution, DQ publication and restart/retry. Make the Go API/Canvas Run button trigger the same persisted state machine. Collect actual run IDs, row counts, elapsed times, source/destination reconciliations and failure-injection evidence. Keep operator approval/Vault/IdP/TLS release certification separate; never mark Production GO based on QA fixture success.
