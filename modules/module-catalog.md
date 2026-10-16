# Module catalog

| Module | Purpose |
|---|---|
| Connection Manager | Connection pools, access and credential references |
| Schema Discovery | Detect typed columns, PK and indexes |
| Job Registry | Version job definitions |
| Sync Planner | Mode, checkpoint, safe bounds |
| Source Reader | Typed keyset pages, query cancellation |
| Bounded Queue | Count and byte backpressure |
| Worker Pool | Parallel transform and validation |
| Destination Writer | Batch stage, write and verification |
| Checkpoint Manager | Durable contiguous progress and fencing |
| Reconciliation | Compare complete PK snapshots |
| Scheduler | Cron, time zones and due-runs |
| Job Orchestrator | Lifecycle, lease, retry and cancellation |
| DAG Executor | Dependencies and join semantics |
| SQL Job Runner | Privilege-scoped transformations |
| Data Quality | Assertions and reconciliation checks |
| Auth/API | RBAC, requests and audit |
| Observability | Metrics, traces, error logs |

## Proposed layout
```text
cmd/dataflow/main.go
internal/api/
internal/auth/
internal/config/
internal/metadata/postgres/
internal/connectors/sqlserver/
internal/connectors/clickhouse/
internal/engine/{planner,reader,queue,worker,writer,checkpoint}/
internal/reconcile/
internal/jobs/{scheduler,executor,dag,sql,quality}/
internal/observability/
migrations/postgres/
web/
tests/{integration,fault,benchmark}/
```

These are Go package boundaries, not separate microservices. One executable initially.
