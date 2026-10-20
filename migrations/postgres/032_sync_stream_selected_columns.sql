-- NULL preserves legacy behavior: read every column.
-- Non-null projections always contain SysRevID and SysRowID, validated by Go.
ALTER TABLE sync_streams
  ADD COLUMN IF NOT EXISTS selected_columns text[] DEFAULT NULL;

DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint
                WHERE conrelid='sync_streams'::regclass
                  AND conname='stream_selected_columns_bounds') THEN
  ALTER TABLE sync_streams
    ADD CONSTRAINT stream_selected_columns_bounds
    CHECK (selected_columns IS NULL OR
           (cardinality(selected_columns) BETWEEN 2 AND 256
            AND array_position(selected_columns,NULL) IS NULL));
 END IF;
END $$;
