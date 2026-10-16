-- Durable dispatch for SELECT-only DAG runs. Apply after migration 007.
-- Worker fencing prevents an expired owner from overwriting a newer result.
ALTER TABLE dag_runs ADD COLUMN IF NOT EXISTS lease_owner uuid;
ALTER TABLE dag_runs ADD COLUMN IF NOT EXISTS lease_expires_at timestamptz;
ALTER TABLE dag_runs ADD COLUMN IF NOT EXISTS fence_token bigint NOT NULL DEFAULT 0;
ALTER TABLE dag_runs ADD COLUMN IF NOT EXISTS dispatch_attempts integer NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS dag_runs_dispatch_idx ON dag_runs(status,created_at)
 WHERE status IN ('queued','running');
