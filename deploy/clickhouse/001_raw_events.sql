-- Run ONLY in a disposable ClickHouse database reserved for DataFlow tests.
-- _sysrevid is fixed-width canonical hex, lexicographically ordered like binary(8).
CREATE DATABASE IF NOT EXISTS scratch ENGINE = Atomic;
CREATE TABLE IF NOT EXISTS scratch.dataflow_raw_events
(
  _stream_id String,
  _pk String,
  _sysrevid String,
  _payload String,
  _run_id String,
  _batch_seq UInt64,
  _ingested_at DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = MergeTree
ORDER BY (_stream_id, _pk, _sysrevid);

-- Latest surviving state for each source row.
-- This is LOGICAL dedup: physical retries can produce duplicate event rows.
CREATE VIEW IF NOT EXISTS scratch.dataflow_latest AS
SELECT _stream_id, _pk,
       argMax(_payload, _sysrevid) AS _payload,
       max(_sysrevid) AS _sysrevid
FROM scratch.dataflow_raw_events
GROUP BY _stream_id, _pk;

-- No physical DELETE detection here: a later snapshot-diff module will
-- introduce ordered tombstones. Never pretend this is CDC event history.
