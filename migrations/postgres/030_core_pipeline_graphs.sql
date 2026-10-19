-- CORE-RUN-8B: disposable scratch-only versioned heterogeneous Canvas definitions.
-- Each launched Core Run retains an immutable graph snapshot and exact version.
CREATE TABLE IF NOT EXISTS core_pipeline_graphs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 name text NOT NULL UNIQUE CHECK(length(name) BETWEEN 1 AND 100),
 version integer NOT NULL DEFAULT 1 CHECK(version>0),
 graph jsonb NOT NULL,
 graph_digest text NOT NULL CHECK(length(graph_digest)=64),
 plan_digest text NOT NULL CHECK(length(plan_digest)=64),
 stream_id uuid NOT NULL REFERENCES sync_streams(id),
 target_model_id uuid NOT NULL REFERENCES transform_models(id),
 unique_column text NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
ALTER TABLE core_pipeline_runs ADD COLUMN IF NOT EXISTS graph_id uuid REFERENCES core_pipeline_graphs(id);
ALTER TABLE core_pipeline_runs ADD COLUMN IF NOT EXISTS graph_version integer;
ALTER TABLE core_pipeline_runs ADD COLUMN IF NOT EXISTS graph_digest text;
ALTER TABLE core_pipeline_runs ADD COLUMN IF NOT EXISTS graph_snapshot jsonb;
