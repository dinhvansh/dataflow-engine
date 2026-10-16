-- v0.2 schema: stream leases and fenced checkpoint.
CREATE TABLE IF NOT EXISTS sync_streams (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 source_schema text NOT NULL,
 source_table text NOT NULL,
 target_database text NOT NULL,
 target_table text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(source_schema,source_table,target_database,target_table)
);
CREATE TABLE IF NOT EXISTS sync_checkpoints (
 stream_id uuid PRIMARY KEY REFERENCES sync_streams(id),
 lower_cursor bytea NOT NULL DEFAULT decode('0000000000000000','hex')
  CHECK(octet_length(lower_cursor)=8),
 generation bigint NOT NULL DEFAULT 0,
 fence_token bigint NOT NULL DEFAULT 0,
 lease_owner uuid,
 lease_expires_at timestamptz,
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK ((lease_owner IS NULL AND lease_expires_at IS NULL)
     OR (lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS sync_checkpoints_lease_idx ON sync_checkpoints(lease_expires_at) WHERE lease_owner IS NOT NULL;
