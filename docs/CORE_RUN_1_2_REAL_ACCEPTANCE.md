# DataFlow CORE-RUN-1 + CORE-RUN-2 — real execution acceptance

**Date:** 2026-10-18. **Developer Preview / isolated QA only.** Funding-demo gate **NOT YET MET**; production remains **NO-GO**. Source ERP is always **SELECT-only**; real Epicor never used in these runs.

## Merged code and exact-head green evidence

| Milestone | Code PR | Tested head | Hosted CI | Hosted SQL Server QA |
| --- | --- | --- | --- | --- |
| CORE-RUN-1 | [#93](https://github.com/dinhvansh/dataflow-engine/pull/93) | `dde8a1fd565e6a39ee1056e0b7c5662815140417` | [39106694398](https://github.com/dinhvansh/dataflow-engine/actions/runs/39106694398) **PASS** | [39106694427](https://github.com/dinhvansh/dataflow-engine/actions/runs/39106694427) **PASS** |
| CORE-RUN-2 | [#94](https://github.com/dinhvansh/dataflow-engine/pull/94) | `d33893b670c457b337b4f3a7eddc5df04f97b6d0` | [39107267980](https://github.com/dinhvansh/dataflow-engine/actions/runs/39107267980) **PASS** | [39107268009](https://github.com/dinhvansh/dataflow-engine/actions/runs/39107268009) **PASS** |

Both SQL Server hosted QA jobs actually show `=== RUN` and `--- PASS` (NOT SKIP) for the new real three-service tests. `main` merge commits after these code PRs: #93 `beeb71d7ebfad3c2a256e4ca50f053e238ad1930`, #94 `50d055091ca48e99606c407831afeb137327fb9c`. Always recheck latest main/head and its actions.

### CORE-RUN-1 — authenticated operator HTTP Run really executes

Existing product endpoints `POST /api/v1/managed/streams`, `POST /api/v1/managed/runs`, and `GET /api/v1/managed/runs?id=<uuid>` were already implemented; #93 connects them in a reproducible **real multi-service end-to-end QA test** rather than adding a second mocked engine.

1. Real HTTP request with Bearer creates a source stream, durable PostgreSQL metadata and checkpoint; second concurrent run is rejected with **409**.
2. Authenticated HTTP queues a persistent logical run and returns HTTP **202** + `run_id`. A **fresh worker object** (separate from HTTP handler object) claims the queued run from PG.
3. Real Microsoft SQL Server 2022 driver, explicitly verified `DataFlowFixtureQA/qa_erp_reader` **SELECT-only**, streams **200** `Erp.Part` rows by rowversion with bounded worker queue into real ClickHouse 25.3 scratch raw events.
4. Operator polls real HTTP GET and sees `succeeded`, **200** `rows_written`, one dispatch, persisted lower/upper cursor, no active lease. ClickHouse verifies **200** source keys. PG checkpoint generation advances **0 → 1**.
5. A SECOND operator HTTP POST creates a new run, which reads **zero new rows** from real SQL Server, inserts no duplicates, keeps the cursor stable and advances generation **1 → 2**.

Exact CI test command (requires three independently provisioned disposable services and all three isolated environment flags): `go test ./cmd/dataflow -run '^TestCoreRun1RealHTTPManagedPipelineIntegration$' -count=1 -timeout=4m -v`.

This is real HTTP/PG/SQL/CH **managed incremental execution**, not yet a unified Canvas DAG or a production network/credential-approved connection.

### CORE-RUN-2 — fenced recovery and bounded transport retry

The existing PG-managed lease/heartbeat/checkpoint architecture is reused and improved. #94 adds sanitized typed ClickHouse HTTP errors, transient classification, and atomic **requeue of the same logical run** only for eligible connection/transport failures; max **3 total claims**; deterministic schema/permission/data errors remain terminal. Each new attempt uses a unique `_run_id`, and a stale worker is rejected by PG fencing.

**Real integration evidence:** first claim extracts **one actual SQL Server row via SELECT**, writes one raw event to real ClickHouse and then the test **expires the two QA PostgreSQL leases** to model worker disappearance. A **new Go worker object** claims the same durable job with higher fence and new attempt ID. The stale worker cannot commit success. New worker streams **200 real SQL Server source rows**, commits PG checkpoint generation 1, and succeeds on the original logical run at attempt 2. ClickHouse preserves **201 physical raw event rows** (1 partial + 200 replay) but the source-key/version identity has **200** distinct records. This is **at-least-once raw storage plus version-aware dedup**, not a false physical exactly-once promise.

Then an **injected synthetic transient HTTP 503 classification** exercises the new requeue transition on real PostgreSQL. No checkpoint is advanced by that failed attempt. A fresh claim at attempt 2 executes a real SQL Server incremental no-op, commits generation 2 and zero new rows. Both obsolete leases remain unable to mutate the checkpoint.

Exact CI test command: `go test ./cmd/dataflow -run '^TestCoreRun2RealCrashRetryCheckpointIntegration$' -count=1 -timeout=4m -v`. The test uses **QA lease-expiry fault injection, not an actual OS process SIGKILL or physical network outage**; broader fault injection is still required.

### Safety and remaining acceptance

- Source SQL Server QA is verified as `DataFlowFixtureQA` with exact login `qa_erp_reader`, SELECT yes, INSERT/UPDATE/DELETE/ALTER no. All source queries are SELECT-only; destination writes and QA migrations target verified disposable PG/ClickHouse only. ClickHouse connection is loopback-only; TEST `TrustServerCertificate` must never be carried into production.
- Managed sync writes **append-only raw event history**. Application queries/consumers must deduplicate by `(stream_id, PK, SysRevID)` as appropriate and must not count raw physical rows as unique business records. Physical source deletions are **not** proven.
- A worker restart after lease expiry and bounded transient requeue have isolated QA evidence, **not** a complete end-user production recovery SLA, SIGKILL/host failure or outage test.
- Current API path is `/api/v1/managed/...` with a test-only Bearer and isolation feature flag. It is not yet a workspace-scoped, Vault-bound, TLS-pinned, universally permitted live runtime and does not prove enterprise security provisioning.
- No **single operator-triggered mixed-stage** SQL source → dbt A→B→C → actual DQ → atomic publish/rollback is proven by CORE-RUN-1/2. Separate real dbt and DQ tests exist and #91 demonstrated a manually orchestrated real QA publish; not the same product job.
- No approved 7m+ source incremental throughput/HA claim. **Not funding-demo ready** until a rerunnable operator-visible unified pipeline, results dashboard/Canvas minimal Run, failure/restart checks, and benchmark are evidenced. Production remains NO-GO even after a demo.

## Next build gate — CORE-RUN-3

Use this existing **real managed Run queue** as the source step of a persisted mixed-step DAG. Attach saved, immutable dbt dependencies **A→B→C**, real DQ checks and versioned ClickHouse publish/rollback to the same logical operator run. Enforce dependency failure blocking, lineaged run status, safe retry/recovery, 3-service/4-service E2E and idempotency. Add only the minimum Canvas Run/monitor wire-up after backend passes. Subsequent release gate: independent real process-kill and injected network outage, 7m+ measured performance, active secure binding/IdP/Vault/TLS plus operational approval.

**No new UI redesign or fundraising GO claim until real unified core demo works.**
