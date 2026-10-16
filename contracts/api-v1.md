# REST API v1 (proposed)

HTTPS + JSON. Base path: /api/v1. Authorization, pagination and request-id are mandatory; POST run returns 202 + run_id.

| Method | Route | Purpose |
|---|---|---|
| POST | /connections | Save connector reference |
| POST | /connections/{id}/test | Test connection |
| GET | /sources/{id}/schemas | Discover |
| GET | /sources/{id}/tables/{schema}/{table} | Column metadata |
| POST/GET | /jobs | Define/list jobs |
| POST | /jobs/{id}/validate | Validate config |
| POST | /jobs/{id}/runs | Start asynchronous run |
| GET | /runs/{id} | Run progress |
| POST | /runs/{id}/cancel | Cancel |
| POST | /runs/{id}/retry | Retry safely |
| GET | /runs/{id}/batches | Batch status |
| GET | /streams/{id}/checkpoint | Current cursor |
| POST | /streams/{id}/reconcile | Reconcile |
| POST | /pipelines/{id}/validate | DAG checks |
| POST | /schedules | Register cron |
| GET | /metrics | Protected metrics |

## Sample create job
```json
{"name":"epicor-parttran","type":"sync",
 "source_connection_id":"<uuid>","destination_connection_id":"<uuid>",
 "source":{"schema":"Erp","table":"PartTran"},
 "destination":{"database":"raw","table":"parttran"},
 "mode":"incremental_rowversion",
 "cursor":{"fields":[{"name":"SysRevID","type":"rowversion"},{"name":"SysRowID","type":"uniqueidentifier","role":"tie_breaker"}]},
 "primary_key":["SysRowID"],
 "performance":{"batch_size":10000,"transform_workers":4,"write_workers":2,"queue_capacity_batches":10,"max_inflight_bytes":134217728}}
```

Error shape: `{"error":{"code":"INVALID_CURSOR_TYPE","message":"...","request_id":"..."}}`. Signed webhooks use unique event IDs, delivery logs and retries.
