-- v1.1 Safe Table on TEST scratch only; migration 012 after 011.
-- One active model across table builds; preserve old physical versions for rollback.
CREATE TABLE IF NOT EXISTS table_runs(
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 model_id uuid NOT NULL REFERENCES transform_models(id),
 model_name text NOT NULL,
 compiled_sql text NOT NULL,
 source_definition jsonb NOT NULL,
 stage_name text NOT NULL UNIQUE CHECK(stage_name ~ '^dft_stage_[a-f0-9]{32}$'),
 target_name text NOT NULL CHECK(target_name ~ '^dft_[a-z][a-z0-9_]*$'),
 unique_column text,
 state text NOT NULL DEFAULT 'queued'
  CHECK(state IN ('queued','building','ready','publishing','published','rolling_back','rolled_back','failed')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 finished_at timestamptz,
 stage_uuid text,
 old_target_uuid text,
 publish_prepared boolean NOT NULL DEFAULT false,
 rows_verified bigint,
 attempt_count integer NOT NULL DEFAULT 0,
 lease_owner uuid,
 lease_until timestamptz,
 fence_token bigint NOT NULL DEFAULT 0,
 error_message text,
 log_tail text,
 run_results jsonb
);
CREATE UNIQUE INDEX IF NOT EXISTS table_runs_one_active_model ON table_runs(model_id)
 WHERE state IN ('queued','building','ready','publishing','rolling_back');
CREATE INDEX IF NOT EXISTS table_runs_work_queue ON table_runs(state,created_at)
 WHERE state IN ('queued','building','publishing','rolling_back');
