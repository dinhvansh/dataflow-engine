-- Run in the disposable/test ClickHouse database only.
-- Snapshot PKs remain isolated by stream and snapshot/run ID.
CREATE TABLE IF NOT EXISTS scratch.dataflow_key_snapshots (
 _stream_id String,
 _snapshot_id String,
 _pk String,
 _sysrevid String,
 _ingested_at DateTime64(3, 'UTC') DEFAULT now64(3)
) ENGINE = MergeTree
ORDER BY (_stream_id, _snapshot_id, _pk);

-- Snapshot generations are published only through PostgreSQL
-- reconcile_baselines AFTER source SNAPSHOT read and ClickHouse count/unique
-- verification succeed. Aborted/partial snapshots remain unpublished and
-- should be cleaned up by a separately validated retention process.
