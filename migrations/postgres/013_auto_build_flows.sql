-- DataFlow v1.2 Auto Build: TEST scratch View pipeline only.
-- Flow controller persists plan and step checkpoints, but delegates actual dbt
-- View materialization to the existing durable dbt_runs worker.
CREATE TABLE IF NOT EXISTS model_flow_runs(
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 target_model_id uuid NOT NULL REFERENCES transform_models(id),
 target_model_name text NOT NULL,
 status text NOT NULL DEFAULT 'queued'
  CHECK(status IN ('queued','running','succeeded','failed')),
 current_step integer NOT NULL DEFAULT 0 CHECK(current_step>=0),
 step_count integer NOT NULL CHECK(step_count BETWEEN 1 AND 30),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 started_at timestamptz,
 finished_at timestamptz,
 error_message text
);
-- One scratch flow at a time prevents overlapping chains from publishing
-- different versions of the same upstream View simultaneously.
CREATE UNIQUE INDEX IF NOT EXISTS model_flow_one_active
 ON model_flow_runs ((1)) WHERE status IN ('queued','running');
CREATE INDEX IF NOT EXISTS model_flow_history
 ON model_flow_runs(created_at DESC);
CREATE TABLE IF NOT EXISTS model_flow_steps(
 flow_id uuid NOT NULL REFERENCES model_flow_runs(id),
 step_index integer NOT NULL CHECK(step_index>=0),
 model_id uuid NOT NULL REFERENCES transform_models(id),
 model_name text NOT NULL,
 source_definition jsonb NOT NULL,
 compiled_sql text NOT NULL,
 dbt_run_id uuid REFERENCES dbt_runs(id),
 status text NOT NULL DEFAULT 'pending'
  CHECK(status IN ('pending','queued','running','succeeded','failed')),
 error_message text,
 rows_verified bigint,
 PRIMARY KEY(flow_id,step_index),
 UNIQUE(flow_id,model_id)
);
