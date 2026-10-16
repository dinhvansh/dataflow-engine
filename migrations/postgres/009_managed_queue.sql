-- v0.7 durable Managed Sync queue. Apply after 004 (and 008 for SQL DAG).
-- Operator: resolve any existing concurrent queued/running rows before unique index.
ALTER TABLE managed_runs ADD COLUMN IF NOT EXISTS dispatch_attempts integer NOT NULL DEFAULT 0;
ALTER TABLE managed_runs ADD COLUMN IF NOT EXISTS lease_owner uuid;
ALTER TABLE managed_runs ADD COLUMN IF NOT EXISTS lease_expires_at timestamptz;
ALTER TABLE managed_runs ADD COLUMN IF NOT EXISTS fence_token bigint NOT NULL DEFAULT 0;
-- Prevent two active logical runs for one stream, across API and scheduler.
CREATE UNIQUE INDEX IF NOT EXISTS managed_runs_one_active_stream
  ON managed_runs(stream_id) WHERE status IN ('queued','running');
CREATE INDEX IF NOT EXISTS managed_runs_dispatch_idx
  ON managed_runs(status,requested_at) WHERE status IN ('queued','running');
