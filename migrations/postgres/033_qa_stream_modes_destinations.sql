-- QA-only mode metadata. Old managed streams remain RowVersion incremental.
-- Full refresh checkpoints never become incremental cursors implicitly.
ALTER TABLE sync_streams
 ADD COLUMN IF NOT EXISTS sync_mode text NOT NULL DEFAULT 'incremental_rowversion';
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint
                WHERE conrelid='sync_streams'::regclass
                  AND conname='sync_stream_mode_guard') THEN
  ALTER TABLE sync_streams ADD CONSTRAINT sync_stream_mode_guard
   CHECK (sync_mode IN ('incremental_rowversion','full_refresh'));
 END IF;
END $$;
-- Distinct table destinations cannot be silently shared across streams
-- (legacy dataflow_raw_events remains the single shared compatible sink).
CREATE UNIQUE INDEX IF NOT EXISTS sync_streams_qa_dedicated_target
 ON sync_streams(target_database,target_table)
 WHERE target_database='scratch' AND target_table LIKE 'dfqa\_%' ESCAPE '\';
