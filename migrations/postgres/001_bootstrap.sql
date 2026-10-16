-- Bootstrap metadata only; production job registry schema remains in docs.
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE IF NOT EXISTS bootstrap_runs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 status text NOT NULL CHECK(status IN ('running','succeeded','failed')),
 source_table text NOT NULL,
 target_table text NOT NULL,
 rows_written bigint NOT NULL DEFAULT 0,
 batches_written bigint NOT NULL DEFAULT 0,
 error_message text,
 created_at timestamptz NOT NULL DEFAULT now(),
 finished_at timestamptz
);
CREATE INDEX IF NOT EXISTS bootstrap_runs_created_at_idx ON bootstrap_runs(created_at DESC);
