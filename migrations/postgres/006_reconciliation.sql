-- Metadata for verified, full source-PK snapshot reconciliation.
-- This does NOT modify ERP or emit delete tombstones.
CREATE TABLE IF NOT EXISTS reconcile_runs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 stream_id uuid NOT NULL REFERENCES sync_streams(id),
 previous_snapshot_id uuid REFERENCES reconcile_runs(id),
 status text NOT NULL CHECK (status IN ('queued','running','succeeded','failed')),
 scanned_rows bigint NOT NULL DEFAULT 0,
 verified_rows bigint NOT NULL DEFAULT 0,
 added_keys bigint,
 deleted_keys bigint,
 changed_versions bigint,
 error_message text,
 created_at timestamptz NOT NULL DEFAULT now(),
 started_at timestamptz,
 finished_at timestamptz
);
CREATE INDEX IF NOT EXISTS reconcile_runs_stream_idx ON reconcile_runs(stream_id,created_at DESC);
CREATE TABLE IF NOT EXISTS reconcile_baselines (
 stream_id uuid PRIMARY KEY REFERENCES sync_streams(id),
 snapshot_id uuid NOT NULL REFERENCES reconcile_runs(id),
 generation bigint NOT NULL DEFAULT 1,
 updated_at timestamptz NOT NULL DEFAULT now()
);
