# V4.0-1 — Heterogeneous Pipeline Synthetic Runtime (NOT live orchestration)

DataFlow already has an operational SQL-Job DAG and a React Flow canvas. This milestone adds an **isolated pure-Go typed mixed-step planner/executor** that reuses the existing `internal/jobs/dag` dependency model. It does not replace the SQL-Job runner or add a user-facing mixed pipeline API.

## Synthetic stage chain
`SOURCE → SYNC → TRANSFORM → QUALITY → PUBLISH`; Transform can depend on multiple Sync/Transform ancestors (fork/join), but Quality must follow Transform, and Publish can only follow Quality. Every defined step must feed the single Publish terminal node. Invalid cycles, skipped stages, duplicate dependencies/nodes, missing steps, bare SQL, URLs/credentials as job reference, orphan nodes, or uncoordinated publishes fail validation.

## Runtime behavior
- `mixedpipeline.Execute` clones the whole graph and computes a deterministic SHA-256 snapshot; handler callbacks only receive copies.
- Existing DAG `Ready` and `Blocked` semantics control scheduling. All prerequisites must succeed. A failed/missing upstream handler blocks and skips downstream stages; failing QUALITY **never** executes PUBLISH.
- Handler errors are sanitized (no raw SQL/credentials/backend error message enters `Report`), and cancellation marks pending stages canceled.
- Stages are synthetic **in-process callbacks**, currently deterministic single-threaded execution. Concurrency, PostgreSQL queue/leasing/fencing, durable run states, retry/idempotency, UI/API integration and approved connector bindings **are not implemented** here.
- Source ERP is NEVER contacted. CI uses fabricated handlers and confirms healthy end-to-end, fork/join, failure and cancellation paths.

## Next phases and gates
- V3.2B still needs real IdP/Vault security provisioning and operator-approved workspace/credential lifecycle; V3.2C-1 network admission only validates offline allowlists. Do not activate real Test Connection before operator-approved egress + pinned TLS, read-only SQL Server roles and scratch-only ClickHouse credentials.
- V3.3 connection/revision binding and V3.4 realistic cross-connection E2E remain prerequisites for live mixed-step runtime.
- V4.0 later phases must bind frozen stage execution requests to existing durable SQL Job, Sync, dbt/transform, DQ and publish services, transactionally persist run/audit metadata and expose statuses in React Flow. Mocked handler success is **not** proof of real ETL.
- V4.1 richer data-quality contracts and V4.2 efficient 7m+ incremental benchmarks are still unfinished.

**Status:** implemented pure-Go synthetic orchestration proof / live mixed-pipeline feature **NOT IMPLEMENTED**, production NO-GO.
