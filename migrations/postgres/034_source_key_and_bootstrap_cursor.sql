-- Additive opt-in mapping: do not rewrite established checkpoint identities.
-- The mode transition is only allowed by the managed worker after a verified
-- ClickHouse stage publication; full snapshots alone never set a cursor.
ALTER TABLE sync_streams
 ADD COLUMN IF NOT EXISTS source_key_column text NOT NULL DEFAULT 'SysRowID';
ALTER TABLE sync_streams
 ADD COLUMN IF NOT EXISTS source_cursor_column text NOT NULL DEFAULT 'SysRevID';
ALTER TABLE sync_streams DROP CONSTRAINT IF EXISTS sync_stream_mode_guard;
ALTER TABLE sync_streams ADD CONSTRAINT sync_stream_mode_guard
 CHECK (sync_mode IN ('incremental_rowversion','full_refresh','full_then_incremental'));
ALTER TABLE sync_streams ADD CONSTRAINT sync_source_key_column_guard
 CHECK (length(source_key_column) BETWEEN 1 AND 128);
ALTER TABLE sync_streams ADD CONSTRAINT sync_source_cursor_column_guard
 CHECK (length(source_cursor_column) <= 128);
-- Existing streams preserve exact previous default key/cursor and sync mode.
