-- A QA connection is a named stream group over one immutable verified profile pair.
-- Existing bound streams become one-stream connections; no stream or checkpoint changes.
CREATE TABLE IF NOT EXISTS qa_connections (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 workspace text NOT NULL DEFAULT 'lab' CHECK(workspace='lab'),
 name text NOT NULL UNIQUE CHECK(length(name) BETWEEN 1 AND 80),
 source_profile_id uuid NOT NULL REFERENCES connection_runtime_profiles(id),
 target_profile_id uuid NOT NULL REFERENCES connection_runtime_profiles(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(id,source_profile_id,target_profile_id)
);
ALTER TABLE connection_runtime_bindings ADD COLUMN IF NOT EXISTS connection_id uuid;
INSERT INTO qa_connections(id,name,source_profile_id,target_profile_id)
SELECT b.stream_id, left(s.source_schema||'.'||s.source_table,40)||' · '||b.stream_id::text,
 b.source_profile_id,b.target_profile_id
FROM connection_runtime_bindings b JOIN sync_streams s ON s.id=b.stream_id
WHERE b.connection_id IS NULL
ON CONFLICT(id) DO NOTHING;
UPDATE connection_runtime_bindings SET connection_id=stream_id WHERE connection_id IS NULL;
ALTER TABLE connection_runtime_bindings ALTER COLUMN connection_id SET NOT NULL;
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='connection_runtime_group_pair_fk') THEN
  ALTER TABLE connection_runtime_bindings
   ADD CONSTRAINT connection_runtime_group_pair_fk
   FOREIGN KEY(connection_id,source_profile_id,target_profile_id)
   REFERENCES qa_connections(id,source_profile_id,target_profile_id);
 END IF;
END $$;
CREATE INDEX IF NOT EXISTS connection_runtime_group_stream_idx
 ON connection_runtime_bindings(connection_id,stream_id);
