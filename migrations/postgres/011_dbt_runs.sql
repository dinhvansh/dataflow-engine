-- dbt v0.9: opt-in scratch VIEW-only runs. PostgreSQL is control plane only.
CREATE TABLE IF NOT EXISTS dbt_runs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 model_id uuid NOT NULL REFERENCES transform_models(id),
 model_name text NOT NULL,
 materialization text NOT NULL DEFAULT 'view' CHECK(materialization='view'),
 target_database text NOT NULL CHECK(target_database='scratch'),
 target_relation text NOT NULL CHECK(target_relation ~ '^dfv_[a-z][a-z0-9_]*$'),
 source_definition jsonb NOT NULL,
 compiled_sql text NOT NULL,
 status text NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','running','succeeded','failed')),
 created_at timestamptz NOT NULL DEFAULT now(),
 started_at timestamptz,
 finished_at timestamptz,
 attempt_count integer NOT NULL DEFAULT 0,
 lease_owner uuid,
 lease_until timestamptz,
 fence_token bigint NOT NULL DEFAULT 0,
 log_tail text,
 run_results jsonb,
 error_message text
);
CREATE UNIQUE INDEX IF NOT EXISTS dbt_one_active_model ON dbt_runs(model_id)
 WHERE status IN ('queued','running');
CREATE INDEX IF NOT EXISTS dbt_runs_queue ON dbt_runs(status,created_at)
 WHERE status IN ('queued','running');
