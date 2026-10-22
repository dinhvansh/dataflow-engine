-- Typed sink metadata is immutable after stream binding. No migration changes
-- the destination of any existing stream or changes its checkpoint.
ALTER TABLE sync_streams ADD COLUMN IF NOT EXISTS destination_layout text NOT NULL DEFAULT 'raw_event';
ALTER TABLE sync_streams ADD COLUMN IF NOT EXISTS destination_mapping jsonb NOT NULL DEFAULT '[]'::jsonb;
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='sync_streams'::regclass
  AND conname='sync_destination_layout_guard') THEN
  ALTER TABLE sync_streams ADD CONSTRAINT sync_destination_layout_guard
   CHECK (destination_layout IN ('raw_event','typed'));
 END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='sync_streams'::regclass
  AND conname='sync_typed_mapping_guard') THEN
  ALTER TABLE sync_streams ADD CONSTRAINT sync_typed_mapping_guard
   CHECK ((destination_layout='raw_event' AND destination_mapping='[]'::jsonb)
      OR (destination_layout='typed' AND jsonb_typeof(destination_mapping)='array'
          AND jsonb_array_length(destination_mapping) BETWEEN 1 AND 256));
 END IF;
END $$;
-- Full-only snapshots without a RowVersion require only a unique key.
ALTER TABLE sync_streams DROP CONSTRAINT IF EXISTS stream_selected_columns_bounds;
ALTER TABLE sync_streams ADD CONSTRAINT stream_selected_columns_bounds
 CHECK (selected_columns IS NULL OR
  (cardinality(selected_columns) BETWEEN 1 AND 256
  AND array_position(selected_columns,NULL) IS NULL));
-- Same physical target cannot be assigned to a second stream.
-- Legacy shared scratch raw-event targets existed before this feature;
-- protect typed targets explicitly at the service layer and index new typed rows.
CREATE UNIQUE INDEX IF NOT EXISTS sync_typed_target_unique
 ON sync_streams (target_database,target_table) WHERE destination_layout='typed';
