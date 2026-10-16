-- v1.3 isolated scratch copy-on-write incremental materialization.
-- The target cursor is moved ONLY after physical ClickHouse publication succeeds.
CREATE TABLE IF NOT EXISTS incremental_targets(
 target_name text PRIMARY KEY CHECK(target_name ~ '^dft_[a-z][a-z0-9_]*$'),
 delta_table text NOT NULL CHECK(delta_table ~ '^[a-z][a-z0-9_]*$'),
 key_column text NOT NULL CHECK(key_column ~ '^[A-Za-z_][A-Za-z0-9_]*$'),
 cursor_value numeric(20,0) NOT NULL DEFAULT 0 CHECK(cursor_value>=0),
 target_uuid text,
 latest_run_id uuid,
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE IF NOT EXISTS incremental_runs(
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 target_name text NOT NULL REFERENCES incremental_targets(target_name),
 delta_table text NOT NULL,
 key_column text NOT NULL,
 stage_name text NOT NULL UNIQUE CHECK(stage_name ~ '^dft_stage_[a-f0-9]{32}$'),
 cursor_lower numeric(20,0) NOT NULL CHECK(cursor_lower>=0),
 cursor_upper numeric(20,0),
 target_uuid text,
 stage_uuid text,
 old_target_uuid text,
 publish_prepared boolean NOT NULL DEFAULT false,
 delta_rows bigint,
 rows_verified bigint,
 state text NOT NULL DEFAULT 'queued'
 CHECK(state IN ('queued','building','ready','publishing','published','rolling_back','rolled_back','failed','no_changes')),
 attempt_count integer NOT NULL DEFAULT 0,
 lease_owner uuid,
 lease_until timestamptz,
 fence_token bigint NOT NULL DEFAULT 0,
 error_message text,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 finished_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS incremental_one_active_target ON incremental_runs(target_name)
 WHERE state IN ('queued','building','ready','publishing','rolling_back');
CREATE INDEX IF NOT EXISTS incremental_work_queue ON incremental_runs(state,created_at)
 WHERE state IN ('queued','building','publishing','rolling_back');
