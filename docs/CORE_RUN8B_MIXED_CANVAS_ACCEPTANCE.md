# CORE-RUN-8B — persisted editable mixed-node Canvas, strict Core bindings

**State: QA pending, do not merge before exact-head CI and real browser E2E PASS.**

Scope: React Flow editor with SOURCE / SYNC / 1–30 saved dbt TRANSFORM / QUALITY / PUBLISH nodes, saved stream/model/quality bindings, draggable positions, editable directed dependencies, backend Validate and optimistic versioned Save. The existing SQL DAG builder remains unchanged. Only strict linear chains can execute in v8B; no UI or API may imply support for parallel branches, merge, arbitrary SQL, unsupported source or real production permissions.

PostgreSQL migration `030_core_pipeline_graphs.sql` creates versioned `core_pipeline_graphs` and nullable run binding/snapshot columns. `POST /api/v1/core/graphs` with `action: validate|save`:
- Verifies one unique SOURCE root, exactly one successor each, 1–30 TRANSFORM models, one QUALITY and PUBLISH, no cycles/disconnected/forks/multiple parents.
- Checks explicit stream points only to `scratch.dataflow_raw_events`, the exact saved Visual model chain compiled by Core, and final DQ key.
- Creates new graph with expected_version=0 or updates exact name+revision via CAS. Never silently overwrites another editor.
- Canonicalizes graph order from actual edges, includes bounded xyflow coordinates; computes SHA-256 snapshot digest.

Existing `POST /api/v1/core/runs` accepts optional `graph_id` and `graph_version`: validates the saved revision, stream/model/DQ, compiled model plan digest and graph digest, **then only** inserts real managed/Core Run in the same PG transaction. New `graph_id`, `graph_version`, `graph_digest`, `graph_snapshot` are immutable per run; GET recovers persisted graph identity/snapshot after reload. Any stale graph or changed saved model rejects launch with HTTP 409 before sync job insertion. No graph Run is ever executed from draft JSON.

E2E acceptance: saved graph creation, backend invalid/fork rejection, revision CAS stale write rejection; real selected Core graph → 10k synthetic SQL Server read-only → ClickHouse raw → PostgreSQL dbt A→B→C → DQ → publish → browser reload persisted Core state and graph snapshot; safe rollback and incremental replay checks. Owner-local demo/prod IdP/Vault/TLS separate NO-GO. CORE-RUN-7 7m hardware benchmark deferred due cloud disk guard; never lower disk safety.
