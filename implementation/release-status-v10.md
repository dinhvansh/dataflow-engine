# DataFlow v1.0 — Data Lineage & Impact Analysis (Developer Preview)

**Merged into main:** [PR #8](https://github.com/dinhvansh/dataflow-engine/pull/8), merge commit `6cf4bd3fe7c911963da4cd749ced643e8a35f255`.

**Verified:** [GitHub Actions 38900354620](https://github.com/dinhvansh/dataflow-engine/actions/runs/38900354620) completed successfully in both Go and Web jobs.

## What was shipped

- New **Data Lineage** navigation page with visual source → model dependencies, connector arrows, selectable nodes, upstream/downstream inspector, and transitive **Impact Analysis**.
- Model output sources in Transform Studio: select a previously saved Visual model to use `scratch.dfv_<upstream_name>` as the source for another Visual transform.
- Deterministic lineage extraction from saved model metadata. Model dependency edges follow `scratch.dfv_<name>`, external/raw sources are separate nodes.
- Backend `GET /api/v1/transform/lineage` returns nodes, edges, topological model order, diagnostics and optional downstream impact. Requires existing authenticated test-mode SQL/Transform feature.
- **Cycle protection at Save:** serializes model catalog edits under a PostgreSQL transaction advisory lock and rejects cyclic new/updated models before commit, including concurrent cross-process saves.
- **Upstream View freshness gate:** a requested dbt View Build checks every ancestor is a saved Visual model and its latest dbt run succeeded using the current compiled SQL. Missing/not-yet-built/stale ancestors prevent queueing; it does **not** rebuild upstream silently.

## CI evidence

- Go race, vet, build — PASS.
- React TypeScript/Vite production build — PASS.
- PostgreSQL integration: A → B → C graph and transitive impact; cyclic A → C update rollback; block Build C before ancestors have succeeded; allow Build after successful ancestors; block again when A SQL changes — PASS.
- Existing managed queue, SQL DAG, Transform Studio, and PostgreSQL/ClickHouse integration tests — PASS.
- Real dbt Core → ClickHouse View materialization and Docker dbt worker image build — PASS.

## Quick demonstration

1. Build `staging` as a TEST View from `scratch.invoices` in Transform Studio.
2. Click **Models**, and create/save `revenue` using `scratch.dfv_staging` via the new source selector.
3. Build View for `revenue` after `staging` has successfully built. Another Visual model can depend on `revenue`.
4. Open **Data Lineage** and select `staging`. It highlights all affected descendants and shows direct parents/children.
5. Change `staging` SQL without rebuilding, then try to Build `revenue`. The API rejects it until the latest `staging` SQL has had a successful dbt View build.

## Boundaries

- This is **metadata-inferred lineage**, not complete SQL parsing/column-level lineage. Raw SQL mode is marked untracked. Each Visual model has one source today; complex joins/multi-input dependencies are not represented.
- **No automatic dbt `ref()` dependency builds.** Operators must build upstream manually; Graph informs them of the right order and impact.
- A successful historical View build is validated using its stored compiled SQL at queue time, but this is not a production cross-system transaction/atomic publishing guarantee. There is no automatic revalidation between queue and worker execution if upstream changes afterward.
- Limited to disposable **scratch View** materialization with existing test flags. No Table/Incremental, actual Mart publish/rollback or production RBAC.
- The graph client displays at most 100 nodes for responsiveness; backend validates up to 500 models.
- Production Epicor TEST full E/L/T acceptance, browser interaction E2E and sustained performance testing remain incomplete.
- No PostgreSQL schema migration is added in v1.0; relies on existing `transform_models` (010) and `dbt_runs` (011).

[Usage and safety guide](lineage-v10.md).
