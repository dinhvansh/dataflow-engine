-- Managed incremental run history and restart diagnostics.
-- Migration only. On existing PostgreSQL volume apply using psql.
CREATE TABLE IF NOT EXISTS managed_runs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 stream_id uuid NOT NULL REFERENCES sync_streams(id),
 status text NOT NULL CHECK (status IN ('queued','running','succeeded','failed')),
 lower_cursor bytea CHECK (lower_cursor IS NULL OR octet_length(lower_cursor)=8),
 upper_cursor bytea CHECK (upper_cursor IS NULL OR octet_length(upper_cursor)=8),
 rows_written bigint NOT NULL DEFAULT 0,
 batches_written bigint NOT NULL DEFAULT 0,
 error_message text,
 requested_at timestamptz NOT NULL DEFAULT now(),
 started_at timestamptz,
 finished_at timestamptz
);
CREATE INDEX IF NOT EXISTS managed_runs_stream_idx ON managed_runs(stream_id,requested_at DESC);
CREATE INDEX IF NOT EXISTS managed_runs_status_idx ON managed_runs(status,requested_at);
