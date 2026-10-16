-- v1.4 developer preview: detect historical-prefix CDC mutation / late event.
-- Nullable state intentionally blocks automatic adoption of nonzero legacy
-- cursors: an operator must first establish trustworthy historical evidence.
ALTER TABLE incremental_targets
 ADD COLUMN IF NOT EXISTS history_cursor numeric(20,0),
 ADD COLUMN IF NOT EXISTS history_count bigint,
 ADD COLUMN IF NOT EXISTS history_digest text;
ALTER TABLE incremental_runs
 ADD COLUMN IF NOT EXISTS history_lower_count bigint,
 ADD COLUMN IF NOT EXISTS history_lower_digest text,
 ADD COLUMN IF NOT EXISTS history_upper_count bigint,
 ADD COLUMN IF NOT EXISTS history_upper_digest text;
