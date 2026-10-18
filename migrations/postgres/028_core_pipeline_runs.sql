-- CORE-RUN-3: isolated scratch-only unified operator Run state.
-- Required upstream migrations: 003, 009, 010, 011, 013.
-- One active mixed flow globally until workspace routing and per-stream
-- snapshot publication isolation are independently proven.
CREATE TABLE IF NOT EXISTS core_pipeline_runs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 stream_id uuid NOT NULL REFERENCES sync_streams(id),
 managed_run_id uuid NOT NULL UNIQUE REFERENCES managed_runs(id),
 target_model_id uuid NOT NULL REFERENCES transform_models(id),
 target_model_name text NOT NULL,
 plan_digest text NOT NULL CHECK (length(plan_digest)=64),
 unique_column text NOT NULL,
 flow_id uuid UNIQUE REFERENCES model_flow_runs(id),
 status text NOT NULL DEFAULT 'waiting_sync'
  CHECK (status IN ('waiting_sync','waiting_flow','publishing','succeeded','failed')),
 stage_name text NOT NULL,
 target_name text NOT NULL,
 stage_uuid text,
 old_uuid text,
 rows_verified bigint,
 error_message text,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 finished_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS core_pipeline_one_active
 ON core_pipeline_runs ((1))
 WHERE status IN ('waiting_sync','waiting_flow','publishing');
CREATE INDEX IF NOT EXISTS core_pipeline_recent
 ON core_pipeline_runs(created_at DESC);
