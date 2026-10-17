package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/dinhvansh/dataflow-engine/internal/connectors/clickhouse"
	"github.com/dinhvansh/dataflow-engine/internal/connectors/sqlserver"
	"github.com/dinhvansh/dataflow-engine/internal/engine"
	pgstate "github.com/dinhvansh/dataflow-engine/internal/state/postgres"
	"github.com/jackc/pgx/v5"
)

func (s *server) reconciliationAPI(w http.ResponseWriter, r *http.Request) {
	if !s.validBearer(r) {
		http.Error(w, "unauthorized", 401)
		return
	}
	switch r.Method {
	case http.MethodPost:
		if !managedEnabled() || !reconciliationEnabled() {
			http.Error(w, "reconciliation test disabled", 403)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		var input struct {
			StreamID string `json:"stream_id"`
		}
		if dec.Decode(&input) != nil || !validUUID.MatchString(input.StreamID) {
			http.Error(w, "valid stream_id required", 400)
			return
		}
		select {
		case s.managedSlots <- struct{}{}:
		default:
			http.Error(w, "runner capacity reached", 429)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		var runID string
		err := s.pg.QueryRow(ctx, `INSERT INTO reconcile_runs(stream_id,status) SELECT id,'queued' FROM sync_streams WHERE id=$1::uuid RETURNING id::text`, input.StreamID).Scan(&runID)
		cancel()
		if err != nil {
			<-s.managedSlots
			http.Error(w, "unknown stream or cannot queue", 404)
			return
		}
		go func() { defer func() { <-s.managedSlots }(); s.executeReconciliation(runID, input.StreamID) }()
		jsonResponse(w, 202, map[string]any{"run_id": runID, "status": "queued"})
	case http.MethodGet:
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		id := r.URL.Query().Get("id")
		if id != "" {
			if !validUUID.MatchString(id) {
				http.Error(w, "invalid run ID", 400)
				return
			}
			var streamID, status, previousID, errMessage string
			var scanned, verified int64
			var added, deleted, changed, candidates, confirmed *int64
			var emitted int64
			err := s.pg.QueryRow(ctx, `SELECT stream_id::text,status,COALESCE(previous_snapshot_id::text,''),scanned_rows,verified_rows,added_keys,deleted_keys,changed_versions,candidate_deleted_keys,confirmed_deleted_keys,emitted_tombstones,COALESCE(error_message,'')
      FROM reconcile_runs WHERE id=$1::uuid`, id).Scan(&streamID, &status, &previousID, &scanned, &verified, &added, &deleted, &changed, &candidates, &confirmed, &emitted, &errMessage)
			if err != nil {
				http.Error(w, "run not found", 404)
				return
			}
			jsonResponse(w, 200, map[string]any{"run_id": id, "stream_id": streamID, "status": status, "previous_snapshot_id": previousID, "scanned_rows": scanned, "verified_rows": verified, "added_keys": added, "deleted_keys": deleted, "changed_versions": changed, "candidate_deleted_keys": candidates, "confirmed_deleted_keys": confirmed, "emitted_tombstones": emitted, "error": errMessage, "detection_only": true})
			return
		}
		rows, err := s.pg.Query(ctx, `SELECT r.id::text,r.stream_id::text,r.status,COALESCE(r.previous_snapshot_id::text,''),r.scanned_rows,r.verified_rows,r.added_keys,r.deleted_keys,r.changed_versions,COALESCE(r.error_message,'')
   FROM reconcile_runs r ORDER BY r.created_at DESC LIMIT 50`)
		if err != nil {
			http.Error(w, "cannot list reconciliation runs", 500)
			return
		}
		defer rows.Close()
		results := make([]map[string]any, 0)
		for rows.Next() {
			var id, streamID, status, previous, msg string
			var scanned, verified int64
			var added, deleted, changed *int64
			if err = rows.Scan(&id, &streamID, &status, &previous, &scanned, &verified, &added, &deleted, &changed, &msg); err != nil {
				http.Error(w, "cannot scan reconciliation runs", 500)
				return
			}
			results = append(results, map[string]any{"run_id": id, "stream_id": streamID, "status": status, "previous_snapshot_id": previous, "scanned_rows": scanned, "verified_rows": verified, "added_keys": added, "deleted_keys": deleted, "changed_versions": changed, "error": msg})
		}
		if err = rows.Err(); err != nil {
			http.Error(w, "cannot read reconciliation runs", 500)
			return
		}
		jsonResponse(w, 200, map[string]any{"runs": results})
	default:
		http.Error(w, "method not allowed", 405)
	}
}
func reconciliationEnabled() bool {
	return managedEnabled() && envEnabled("DATAFLOW_ENABLE_RECONCILIATION_TEST")
}
func envEnabled(k string) bool { return os.Getenv(k) == "true" }

func (s *server) executeReconciliation(runID, streamID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Hour)
	defer cancel()
	var runErr error
	completed := false
	var stats engine.Stats
	var prevID string
	var verified uint64
	var diff clickhouse.KeySnapshotDiff
	var candidateKeys, confirmedKeys []string
	var witnessID string
	hasBaseline := false
	defer func() {
		if completed {
			return
		}
		m := "reconciliation interrupted"
		if runErr != nil {
			m = runErr.Error()
		}
		finalizeCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_, err := s.pg.Exec(finalizeCtx, `UPDATE reconcile_runs SET status='failed',scanned_rows=$2,verified_rows=$3,error_message=$4,finished_at=now() WHERE id=$1::uuid`, runID, stats.Rows, verified, m)
		if err != nil {
			log.Printf("reconcile finalization %s: %v", runID, err)
		}
	}()
	if _, err := s.pg.Exec(ctx, `UPDATE reconcile_runs SET status='running',started_at=now() WHERE id=$1::uuid`, runID); err != nil {
		runErr = err
		return
	}
	var cfg managedStream
	err := s.pg.QueryRow(ctx, `SELECT source_schema,source_table,target_database,target_table,batch_size,workers,queue_capacity
   FROM sync_streams WHERE id=$1::uuid`, streamID).Scan(&cfg.SourceSchema, &cfg.SourceTable, &cfg.TargetDatabase, &cfg.TargetTable, &cfg.BatchSize, &cfg.Workers, &cfg.QueueCapacity)
	if err != nil {
		runErr = err
		return
	}
	if cfg.TargetDatabase != scratchDatabase() || cfg.TargetTable != "dataflow_raw_events" {
		runErr = errors.New("reconciliation requires managed scratch stream")
		return
	}
	store := pgstate.Store{Pool: s.pg}
	lease, err := store.Claim(ctx, streamID, runID, 5*time.Minute)
	if err != nil {
		runErr = fmt.Errorf("reconciliation lease: %w", err)
		return
	}
	defer func() {
		c, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = store.Release(c, lease)
	}()
	// Renew lease during long SNAPSHOT transaction; cancellation prevents
	// publishing a partial source snapshot after losing ownership.
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		tick := time.NewTicker(30 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				rc, stop := context.WithTimeout(ctx, 10*time.Second)
				e := store.Renew(rc, lease, 5*time.Minute)
				stop()
				if e != nil {
					log.Printf("reconcile run %s lease lost: %v", runID, e)
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-heartbeatDone }()
	var baselineGeneration int64
	err = s.pg.QueryRow(ctx, `SELECT snapshot_id::text,generation FROM reconcile_baselines WHERE stream_id=$1::uuid`, streamID).Scan(&prevID, &baselineGeneration)
	if errors.Is(err, pgx.ErrNoRows) {
		prevID = ""
		baselineGeneration = 0
	} else if err != nil {
		runErr = err
		return
	}
	hasBaseline = prevID != ""
	reader := &sqlserver.KeySnapshotReader{DB: s.sql, Schema: cfg.SourceSchema, Table: cfg.SourceTable, BatchSize: cfg.BatchSize}
	writer := &clickhouse.Writer{Endpoint: s.chURL, Database: cfg.TargetDatabase, Table: "dataflow_key_snapshots", Username: s.chUser, Password: s.chPass}
	snapWriter := &clickhouse.KeySnapshotWriter{Destination: writer, StreamID: streamID, SnapshotID: runID}
	stats, err = engine.Run(ctx, engine.Config{Workers: cfg.Workers, QueueCapacity: cfg.QueueCapacity}, reader, snapWriter)
	if err != nil {
		runErr = fmt.Errorf("snapshot load: %w", err)
		return
	}
	// Fail closed on partial snapshots or destination retries that generated
	// duplicate keys. No baseline publication until both checks pass.
	dst, err := writer.SnapshotStats(ctx, streamID, runID)
	if err != nil {
		runErr = fmt.Errorf("snapshot count verification: %w", err)
		return
	}
	verified = dst.Rows
	if verified != stats.Rows || dst.DistinctKeys != stats.Rows {
		runErr = fmt.Errorf("snapshot incomplete/duplicate: read=%d stored=%d distinct=%d", stats.Rows, dst.Rows, dst.DistinctKeys)
		return
	}
	if hasBaseline {
		diff, err = writer.SnapshotDiff(ctx, streamID, prevID, runID)
		if err != nil {
			runErr = fmt.Errorf("snapshot difference: %w", err)
			return
		}
		candidateKeys, err = writer.MissingSnapshotKeys(ctx, streamID, prevID, runID)
		if err != nil {
			runErr = fmt.Errorf("delete candidate evidence: %w", err)
			return
		}
		var witnessScanned, witnessVerified, previousScanned, previousVerified int64
		err = s.pg.QueryRow(ctx, `SELECT COALESCE(p.previous_snapshot_id::text,''),
      COALESCE(w.scanned_rows,0),COALESCE(w.verified_rows,0),p.scanned_rows,p.verified_rows
     FROM reconcile_runs p LEFT JOIN reconcile_runs w ON w.id=p.previous_snapshot_id
     WHERE p.id=$1::uuid AND p.stream_id=$2::uuid AND p.status='succeeded'
       AND (w.id IS NULL OR (w.stream_id=p.stream_id AND w.status='succeeded'))`, prevID, streamID).
			Scan(&witnessID, &witnessScanned, &witnessVerified, &previousScanned, &previousVerified)
		if err != nil {
			runErr = fmt.Errorf("delete confirmation history: %w", err)
			return
		}
		if previousScanned != previousVerified {
			runErr = errors.New("previous snapshot is not complete")
			return
		}
		if witnessID != "" {
			if witnessScanned != witnessVerified {
				runErr = errors.New("witness snapshot is not complete")
				return
			}
			confirmedKeys, err = writer.ConfirmedMissingSnapshotKeys(ctx, streamID, witnessID, prevID, runID)
			if err != nil {
				runErr = fmt.Errorf("confirmed delete evidence: %w", err)
				return
			}
		}
	}
	if err = ctx.Err(); err != nil {
		runErr = err
		return
	}
	tx, err := s.pg.Begin(ctx)
	if err != nil {
		runErr = err
		return
	}
	defer tx.Rollback(ctx)
	// Serializing on the checkpoint row guarantees the writer still owns the
	// same stream fence at publication time.
	var owned bool
	err = tx.QueryRow(ctx, `SELECT true FROM sync_checkpoints WHERE stream_id=$1::uuid
    AND lease_owner=$2::uuid AND fence_token=$3 AND lease_expires_at>clock_timestamp()
    FOR UPDATE`, streamID, runID, lease.FenceToken).Scan(&owned)
	if err != nil {
		runErr = fmt.Errorf("fenced snapshot publication denied: %w", err)
		return
	}
	var newGeneration int64
	err = tx.QueryRow(ctx, `INSERT INTO reconcile_baselines(stream_id,snapshot_id,generation)
    VALUES($1::uuid,$2::uuid,1)
    ON CONFLICT(stream_id) DO UPDATE SET snapshot_id=EXCLUDED.snapshot_id,
     generation=reconcile_baselines.generation+1,updated_at=now()
    WHERE reconcile_baselines.generation=$3
    RETURNING generation`, streamID, runID, baselineGeneration).Scan(&newGeneration)
	if err != nil {
		runErr = fmt.Errorf("baseline changed during scan: %w", err)
		return
	}
	if hasBaseline {
		_, err = tx.Exec(ctx, `DELETE FROM reconcile_delete_candidates
    WHERE stream_id=$1::uuid AND status='candidate'
      AND NOT (source_key=ANY($2::uuid[]))`, streamID, candidateKeys)
		if err != nil {
			runErr = fmt.Errorf("invalidate delete candidates: %w", err)
			return
		}
		_, err = tx.Exec(ctx, `INSERT INTO reconcile_delete_candidates(stream_id,source_key,first_missing_snapshot_id)
    SELECT $1::uuid,k,$2::uuid FROM unnest($3::uuid[]) k
    ON CONFLICT(stream_id,source_key) DO NOTHING`, streamID, runID, candidateKeys)
		if err != nil {
			runErr = fmt.Errorf("record delete candidates: %w", err)
			return
		}
		if len(confirmedKeys) > 0 {
			tag, e := tx.Exec(ctx, `UPDATE reconcile_delete_candidates c
     SET status='confirmed',confirmed_snapshot_id=$3::uuid
     FROM unnest($4::uuid[]) k
     WHERE c.stream_id=$1::uuid AND c.first_missing_snapshot_id=$2::uuid
       AND c.source_key=k AND c.status='candidate'`, streamID, prevID, runID, confirmedKeys)
			if e != nil {
				runErr = fmt.Errorf("confirm delete candidates: %w", e)
				return
			}
			if tag.RowsAffected() != int64(len(confirmedKeys)) {
				runErr = errors.New("delete confirmation evidence mismatch")
				return
			}
		}
	}
	var added, deleted, changed any
	if hasBaseline {
		added, deleted, changed = diff.Added, diff.Deleted, diff.Changed
	}
	_, err = tx.Exec(ctx, `UPDATE reconcile_runs
  SET status='succeeded',previous_snapshot_id=NULLIF($2,'')::uuid,
   scanned_rows=$3,verified_rows=$4,added_keys=$5,deleted_keys=$6,
   changed_versions=$7,candidate_deleted_keys=$8,confirmed_deleted_keys=$9,
   finished_at=now() WHERE id=$1::uuid`,
		runID, prevID, stats.Rows, verified, added, deleted, changed, len(candidateKeys), len(confirmedKeys))
	if err != nil {
		runErr = err
		return
	}
	if err = tx.Commit(ctx); err != nil {
		runErr = err
		return
	}
	completed = true
}
