package clickhouse

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dinhvansh/dataflow-engine/internal/cursor"
	"github.com/dinhvansh/dataflow-engine/internal/engine"
)

const maxDeleteEvidenceKeys = 100000

type KeySnapshotWriter struct {
	Destination          *Writer
	StreamID, SnapshotID string
}

func (w *KeySnapshotWriter) Write(ctx context.Context, b engine.Batch) error {
	if w.Destination == nil || !canonicalRunID.MatchString(w.StreamID) || !canonicalRunID.MatchString(w.SnapshotID) {
		return errors.New("invalid snapshot destination or IDs")
	}
	next := engine.Batch{Sequence: b.Sequence, Rows: make([]map[string]any, 0, len(b.Rows))}
	for _, source := range b.Rows {
		key, ok := source["SysRowID"].(string)
		if !ok || !canonicalRunID.MatchString(key) {
			return errors.New("source SysRowID is not canonical UUID")
		}
		rv, ok := source["SysRevID"].(string)
		if !ok {
			return errors.New("source SysRevID is not a hex string")
		}
		version, err := cursor.ParseRowVersion(rv)
		if err != nil {
			return err
		}
		next.Rows = append(next.Rows, map[string]any{
			"_stream_id": w.StreamID, "_snapshot_id": w.SnapshotID,
			"_pk": strings.ToLower(key), "_sysrevid": version.String(),
		})
	}
	return w.Destination.Write(ctx, next)
}

type KeySnapshotStats struct {
	Rows         uint64 `json:"rows"`
	DistinctKeys uint64 `json:"distinct_keys"`
}
type KeySnapshotDiff struct {
	Added   uint64 `json:"added"`
	Deleted uint64 `json:"deleted"`
	Changed uint64 `json:"changed"`
}

func (w *Writer) keySnapshotContext(streamID, snapshotID string) (string, string, error) {
	if !identifier.MatchString(w.Database) || !identifier.MatchString(w.Table) {
		return "", "", errors.New("invalid target identifier")
	}
	if !canonicalRunID.MatchString(streamID) || !canonicalRunID.MatchString(snapshotID) {
		return "", "", errors.New("invalid IDs")
	}
	table := "`" + w.Database + "`.`" + w.Table + "`"
	filter := fmt.Sprintf("_stream_id='%s' AND _snapshot_id='%s'", streamID, snapshotID)
	return table, filter, nil
}
func (w *Writer) scalar(ctx context.Context, query string) (uint64, error) {
	endpoint, err := url.Parse(strings.TrimRight(w.Endpoint, "/"))
	if err != nil {
		return 0, err
	}
	q := endpoint.Query()
	q.Set("query", query+" FORMAT TabSeparated")
	endpoint.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return 0, err
	}
	if w.Username != "" {
		req.SetBasicAuth(w.Username, w.Password)
	}
	client := w.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	res, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 4096))
	if err != nil {
		return 0, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return 0, fmt.Errorf("ClickHouse query status %d: %s", res.StatusCode, string(data))
	}
	n, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("unexpected scalar: %w", err)
	}
	return n, nil
}
func (w *Writer) queryText(ctx context.Context, query string) (string, error) {
	endpoint, err := url.Parse(strings.TrimRight(w.Endpoint, "/"))
	if err != nil {
		return "", err
	}
	q := endpoint.Query()
	q.Set("query", query)
	endpoint.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return "", err
	}
	if w.Username != "" {
		req.SetBasicAuth(w.Username, w.Password)
	}
	client := w.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return "", err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("ClickHouse query status %d: %s", res.StatusCode, string(data))
	}
	return string(data), nil
}
func (w *Writer) snapshotKeys(ctx context.Context, query string) ([]string, error) {
	raw, err := w.queryText(ctx, query+" FORMAT TabSeparatedRaw")
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0)
	scan := bufio.NewScanner(strings.NewReader(raw))
	for scan.Scan() {
		key := strings.TrimSpace(scan.Text())
		if key == "" {
			continue
		}
		if !canonicalRunID.MatchString(key) {
			return nil, errors.New("snapshot returned invalid source key")
		}
		keys = append(keys, strings.ToLower(key))
		if len(keys) > maxDeleteEvidenceKeys {
			return nil, fmt.Errorf("delete evidence exceeds safety limit %d", maxDeleteEvidenceKeys)
		}
	}
	return keys, scan.Err()
}
func (w *Writer) SnapshotStats(ctx context.Context, streamID, snapshotID string) (KeySnapshotStats, error) {
	var result KeySnapshotStats
	table, filter, err := w.keySnapshotContext(streamID, snapshotID)
	if err != nil {
		return result, err
	}
	result.Rows, err = w.scalar(ctx, "SELECT count() FROM "+table+" WHERE "+filter)
	if err != nil {
		return result, err
	}
	result.DistinctKeys, err = w.scalar(ctx, "SELECT uniqExact(_pk) FROM "+table+" WHERE "+filter)
	return result, err
}

// SnapshotDiff compares two *fully verified* source-key snapshots.
// Purely diagnostic. No tombstones or deletion mutations are written.
func (w *Writer) SnapshotDiff(ctx context.Context, streamID, oldID, newID string) (KeySnapshotDiff, error) {
	var result KeySnapshotDiff
	table, oldFilter, err := w.keySnapshotContext(streamID, oldID)
	if err != nil {
		return result, err
	}
	_, newFilter, err := w.keySnapshotContext(streamID, newID)
	if err != nil {
		return result, err
	}
	oldQuery := "SELECT _pk,_sysrevid FROM " + table + " WHERE " + oldFilter
	newQuery := "SELECT _pk,_sysrevid FROM " + table + " WHERE " + newFilter
	result.Added, err = w.scalar(ctx, "SELECT count() FROM ("+newQuery+") n LEFT ANTI JOIN ("+oldQuery+") p USING (_pk)")
	if err != nil {
		return result, err
	}
	result.Deleted, err = w.scalar(ctx, "SELECT count() FROM ("+oldQuery+") p LEFT ANTI JOIN ("+newQuery+") n USING (_pk)")
	if err != nil {
		return result, err
	}
	result.Changed, err = w.scalar(ctx, "SELECT count() FROM ("+oldQuery+") p INNER JOIN ("+newQuery+") n USING (_pk) WHERE p._sysrevid != n._sysrevid")
	return result, err
}

// MissingSnapshotKeys returns candidates only after both snapshots have been
// fully verified by the caller. It never mutates the source or emits deletes.
func (w *Writer) MissingSnapshotKeys(ctx context.Context, streamID, oldID, newID string) ([]string, error) {
	table, oldFilter, err := w.keySnapshotContext(streamID, oldID)
	if err != nil {
		return nil, err
	}
	_, newFilter, err := w.keySnapshotContext(streamID, newID)
	if err != nil {
		return nil, err
	}
	oldQuery := "SELECT _pk FROM " + table + " WHERE " + oldFilter
	newQuery := "SELECT _pk FROM " + table + " WHERE " + newFilter
	return w.snapshotKeys(ctx, "SELECT p._pk FROM ("+oldQuery+") p LEFT ANTI JOIN ("+newQuery+") n USING (_pk) ORDER BY p._pk LIMIT 100001")
}

// ConfirmedMissingSnapshotKeys requires two consecutive complete snapshots to
// omit a key that existed in the older baseline.
func (w *Writer) ConfirmedMissingSnapshotKeys(ctx context.Context, streamID, oldID, missingID, confirmID string) ([]string, error) {
	table, oldFilter, err := w.keySnapshotContext(streamID, oldID)
	if err != nil {
		return nil, err
	}
	_, missingFilter, err := w.keySnapshotContext(streamID, missingID)
	if err != nil {
		return nil, err
	}
	_, confirmFilter, err := w.keySnapshotContext(streamID, confirmID)
	if err != nil {
		return nil, err
	}
	oldQuery := "SELECT _pk FROM " + table + " WHERE " + oldFilter
	missingQuery := "SELECT _pk FROM " + table + " WHERE " + missingFilter
	confirmQuery := "SELECT _pk FROM " + table + " WHERE " + confirmFilter
	query := "SELECT o._pk FROM (" + oldQuery + ") o LEFT ANTI JOIN (" + missingQuery + ") m USING (_pk) LEFT ANTI JOIN (" + confirmQuery + ") c USING (_pk) ORDER BY o._pk LIMIT 100001"
	return w.snapshotKeys(ctx, query)
}

type HardDeleteTombstone struct {
	StreamID, Key, WitnessSnapshotID, ConfirmSnapshotID string
	Version                                             uint64
}

func (w *Writer) EnsureHardDeleteTombstones(ctx context.Context) error {
	if w.Database != "scratch" {
		return errors.New("hard-delete tombstones require scratch database")
	}
	table := "`" + w.Database + "`.`dataflow_delete_tombstones`"
	return w.exec(ctx, "CREATE TABLE IF NOT EXISTS "+table+" (_stream_id String,_pk String,_df_version UInt64,_df_deleted UInt8,_witness_snapshot_id String,_confirm_snapshot_id String,_emitted_at DateTime64(3,'UTC') DEFAULT now64(3)) ENGINE=ReplacingMergeTree(_emitted_at) ORDER BY (_stream_id,_pk,_df_version)")
}
func (w *Writer) WriteHardDeleteTombstones(ctx context.Context, items []HardDeleteTombstone) error {
	if len(items) == 0 {
		return nil
	}
	if err := w.EnsureHardDeleteTombstones(ctx); err != nil {
		return err
	}
	dst := &Writer{Endpoint: w.Endpoint, Database: w.Database, Table: "dataflow_delete_tombstones", Username: w.Username, Password: w.Password, Client: w.Client}
	rows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if !canonicalRunID.MatchString(item.StreamID) || !canonicalRunID.MatchString(item.Key) || !canonicalRunID.MatchString(item.WitnessSnapshotID) || !canonicalRunID.MatchString(item.ConfirmSnapshotID) || item.Version == 0 {
			return errors.New("invalid hard-delete tombstone")
		}
		rows = append(rows, map[string]any{"_stream_id": item.StreamID, "_pk": strings.ToLower(item.Key), "_df_version": item.Version, "_df_deleted": uint8(1), "_witness_snapshot_id": item.WitnessSnapshotID, "_confirm_snapshot_id": item.ConfirmSnapshotID})
	}
	return dst.Write(ctx, engine.Batch{Rows: rows})
}
func (w *Writer) HardDeleteMaxVersion(ctx context.Context, streamID string) (uint64, error) {
	if !canonicalRunID.MatchString(streamID) {
		return 0, errors.New("invalid stream ID")
	}
	if err := w.EnsureHardDeleteTombstones(ctx); err != nil {
		return 0, err
	}
	return w.scalar(ctx, fmt.Sprintf("SELECT ifNull(max(_df_version),0) FROM `scratch`.`dataflow_delete_tombstones` FINAL WHERE _stream_id='%s'", streamID))
}

// Satisfy Go compile-time import usage: report structure intentionally does
// not serialize rows, only scalar statistics and aggregate differences.
var _ = json.Valid
