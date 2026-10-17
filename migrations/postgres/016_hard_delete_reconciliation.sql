-- v1.4 hard-delete evidence and explicit tombstone approval.
-- Source ERP remains read-only; this is DataFlow control-plane state only.
CREATE TABLE IF NOT EXISTS reconcile_delete_candidates (
 stream_id uuid NOT NULL REFERENCES sync_streams(id),
 source_key uuid NOT NULL,
 first_missing_snapshot_id uuid NOT NULL REFERENCES reconcile_runs(id),
 confirmed_snapshot_id uuid REFERENCES reconcile_runs(id),
 tombstone_version bigint,
 status text NOT NULL DEFAULT 'candidate'
  CHECK(status IN ('candidate','confirmed','reserved','emitted')),
 emitted_at timestamptz,
 PRIMARY KEY(stream_id,source_key),
 CHECK((status='candidate' AND confirmed_snapshot_id IS NULL AND tombstone_version IS NULL AND emitted_at IS NULL)
    OR (status='confirmed' AND confirmed_snapshot_id IS NOT NULL AND tombstone_version IS NULL AND emitted_at IS NULL)
    OR (status='reserved' AND confirmed_snapshot_id IS NOT NULL AND tombstone_version IS NOT NULL AND emitted_at IS NULL)
    OR (status='emitted' AND confirmed_snapshot_id IS NOT NULL AND tombstone_version IS NOT NULL AND emitted_at IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS reconcile_delete_confirmed_idx
 ON reconcile_delete_candidates(stream_id,confirmed_snapshot_id,status);

CREATE TABLE IF NOT EXISTS reconcile_delete_sequences (
 stream_id uuid PRIMARY KEY REFERENCES sync_streams(id),
 last_version bigint NOT NULL DEFAULT 0 CHECK(last_version>=0)
);

ALTER TABLE reconcile_runs
 ADD COLUMN IF NOT EXISTS candidate_deleted_keys bigint,
 ADD COLUMN IF NOT EXISTS confirmed_deleted_keys bigint,
 ADD COLUMN IF NOT EXISTS emitted_tombstones bigint NOT NULL DEFAULT 0;
