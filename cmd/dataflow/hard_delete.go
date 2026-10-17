package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/dinhvansh/dataflow-engine/internal/connectors/clickhouse"
)

const hardDeleteConfirmation = "EMIT_TOMBSTONES"

func hardDeleteEnabled() bool {
	return reconciliationEnabled() && envEnabled("DATAFLOW_ENABLE_HARD_DELETE_TEST")
}

func (s *server) hardDeleteAPI(w http.ResponseWriter, r *http.Request) {
	if !s.validBearer(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !hardDeleteEnabled() {
		http.Error(w, "hard delete disabled", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var input struct {
		RunID   string `json:"run_id"`
		Confirm string `json:"confirm"`
	}
	if dec.Decode(&input) != nil || !validUUID.MatchString(input.RunID) || input.Confirm != hardDeleteConfirmation {
		http.Error(w, "valid run_id and exact confirmation required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	count, err := s.emitHardDeleteTombstones(ctx, input.RunID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"run_id": input.RunID, "emitted_tombstones": count, "status": "succeeded"})
}

func (s *server) emitHardDeleteTombstones(ctx context.Context, runID string) (int64, error) {
	var streamID, database string
	err := s.pg.QueryRow(ctx, `SELECT r.stream_id::text,ss.target_database
		FROM reconcile_runs r JOIN sync_streams ss ON ss.id=r.stream_id
		WHERE r.id=$1::uuid AND r.status='succeeded'`, runID).Scan(&streamID, &database)
	if err != nil {
		return 0, errors.New("successful reconciliation run required")
	}
	if database != scratchDatabase() {
		return 0, errors.New("hard delete requires disposable scratch database")
	}
	writer := &clickhouse.Writer{Endpoint: s.chURL, Database: database, Username: s.chUser, Password: s.chPass}
	chMax, err := writer.HardDeleteMaxVersion(ctx, streamID)
	if err != nil {
		return 0, fmt.Errorf("read tombstone version: %w", err)
	}
	if chMax > math.MaxInt64 {
		return 0, errors.New("tombstone version range exhausted")
	}
	tx, err := s.pg.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, streamID); err != nil {
		return 0, err
	}
	var last int64
	err = tx.QueryRow(ctx, `INSERT INTO reconcile_delete_sequences(stream_id,last_version)
		VALUES($1::uuid,$2) ON CONFLICT(stream_id) DO UPDATE
		SET last_version=GREATEST(reconcile_delete_sequences.last_version,EXCLUDED.last_version)
		RETURNING last_version`, streamID, int64(chMax)).Scan(&last)
	if err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, `SELECT source_key::text,first_missing_snapshot_id::text,
		confirmed_snapshot_id::text,status,COALESCE(tombstone_version,0)
		FROM reconcile_delete_candidates WHERE stream_id=$1::uuid
		AND confirmed_snapshot_id=$2::uuid AND status IN ('confirmed','reserved','emitted')
		ORDER BY source_key FOR UPDATE`, streamID, runID)
	if err != nil {
		return 0, err
	}
	type evidence struct {
		key, witness, confirm, status string
		version                       int64
	}
	var all []evidence
	for rows.Next() {
		var e evidence
		if err = rows.Scan(&e.key, &e.witness, &e.confirm, &e.status, &e.version); err != nil {
			rows.Close()
			return 0, err
		}
		all = append(all, e)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	for i := range all {
		if all[i].status != "confirmed" {
			continue
		}
		if last == math.MaxInt64 {
			return 0, errors.New("tombstone version range exhausted")
		}
		last++
		all[i].version = last
		all[i].status = "reserved"
		if _, err = tx.Exec(ctx, `UPDATE reconcile_delete_candidates SET status='reserved',tombstone_version=$3
			WHERE stream_id=$1::uuid AND source_key=$2::uuid AND status='confirmed'`, streamID, all[i].key, last); err != nil {
			return 0, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE reconcile_delete_sequences SET last_version=$2 WHERE stream_id=$1::uuid`, streamID, last); err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}

	items := make([]clickhouse.HardDeleteTombstone, 0, len(all))
	for _, e := range all {
		if e.status == "reserved" {
			items = append(items, clickhouse.HardDeleteTombstone{StreamID: streamID, Key: e.key, WitnessSnapshotID: e.witness, ConfirmSnapshotID: e.confirm, Version: uint64(e.version)})
		}
	}
	if err = writer.WriteHardDeleteTombstones(ctx, items); err != nil {
		return 0, fmt.Errorf("write tombstones: %w", err)
	}
	tx, err = s.pg.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, streamID); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `UPDATE reconcile_delete_candidates SET status='emitted',emitted_at=now()
		WHERE stream_id=$1::uuid AND confirmed_snapshot_id=$2::uuid AND status='reserved'`, streamID, runID); err != nil {
		return 0, err
	}
	var emitted int64
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM reconcile_delete_candidates
		WHERE stream_id=$1::uuid AND confirmed_snapshot_id=$2::uuid AND status='emitted'`, streamID, runID).Scan(&emitted); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `UPDATE reconcile_runs SET emitted_tombstones=$2 WHERE id=$1::uuid`, runID, emitted); err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return emitted, nil
}
