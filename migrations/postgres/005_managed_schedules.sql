-- Managed stream interval schedules (experimental single-process runner).
CREATE TABLE IF NOT EXISTS managed_schedules (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 stream_id uuid NOT NULL UNIQUE REFERENCES sync_streams(id),
 interval_seconds integer NOT NULL CHECK (interval_seconds BETWEEN 60 AND 86400),
 enabled boolean NOT NULL DEFAULT false,
 next_run_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS managed_schedules_due_idx
 ON managed_schedules(next_run_at) WHERE enabled = true;
