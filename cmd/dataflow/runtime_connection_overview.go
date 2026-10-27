package main

// Admin-only read model for the isolated QA connection journey. A connection
// here is the persisted source/target profile pair shared by its bound streams.
import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type runtimeOverviewRun struct {
	Status      string     `json:"status"`
	RowsWritten int64      `json:"rows_written"`
	RequestedAt time.Time  `json:"requested_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}
type runtimeOverviewStream struct {
	ID              string              `json:"id"`
	SourceSchema    string              `json:"source_schema"`
	SourceTable     string              `json:"source_table"`
	TargetDatabase  string              `json:"target_database"`
	TargetTable     string              `json:"target_table"`
	SyncMode        string              `json:"sync_mode"`
	RevisionCurrent bool                `json:"revision_current"`
	BindingCurrent  bool                `json:"binding_current"`
	ScheduleEnabled bool                `json:"schedule_enabled"`
	ScheduleSeconds int                 `json:"schedule_seconds"`
	LatestRun       *runtimeOverviewRun `json:"latest_run,omitempty"`
}
type runtimeOverviewConnection struct {
	ID              string                  `json:"id"`
	Name            string                  `json:"name"`
	SourceID        string                  `json:"source_connection_id"`
	SourceName      string                  `json:"source_name"`
	SourceStatus    string                  `json:"source_status"`
	SourceCheckedAt *time.Time              `json:"source_checked_at,omitempty"`
	TargetID        string                  `json:"target_connection_id"`
	TargetName      string                  `json:"target_name"`
	TargetStatus    string                  `json:"target_status"`
	TargetCheckedAt *time.Time              `json:"target_checked_at,omitempty"`
	Streams         []runtimeOverviewStream `json:"streams"`
}

func (s *server) runtimeOverviewAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	if !runtimeLabEnabled() || !runtimeConnectionAuthorized(s, r) {
		http.Error(w, "not permitted", http.StatusForbidden)
		return
	}
	if s.pg == nil {
		http.Error(w, "metadata unavailable", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	rows, err := s.pg.Query(ctx, `SELECT
 g.id::text,g.name,
 b.source_profile_id::text,sp.name,sp.status,sp.checked_at,
 b.target_profile_id::text,tp.name,tp.status,tp.checked_at,
 s.id::text,s.source_schema,s.source_table,s.target_database,s.target_table,COALESCE(s.sync_mode,''),
	COALESCE(b.source_revision=sp.revision AND b.target_revision=tp.revision,false),
 COALESCE((b.source_revision=sp.revision AND b.target_revision=tp.revision AND sp.status='verified' AND tp.status='verified'
  AND sp.checked_at BETWEEN now()-interval '30 minutes' AND now()+interval '1 minute'
  AND tp.checked_at BETWEEN now()-interval '30 minutes' AND now()+interval '1 minute'),false),
 COALESCE(sc.enabled,false),COALESCE(sc.interval_seconds,0),
 COALESCE(last_run.status,''),COALESCE(last_run.rows_written,0),last_run.requested_at,last_run.finished_at
 FROM connection_runtime_bindings b
 JOIN qa_connections g ON g.id=b.connection_id AND g.workspace='lab'
 JOIN connection_runtime_profiles sp ON sp.id=b.source_profile_id AND sp.workspace='lab'
 JOIN connection_runtime_profiles tp ON tp.id=b.target_profile_id AND tp.workspace='lab'
 JOIN sync_streams s ON s.id=b.stream_id
 LEFT JOIN managed_schedules sc ON sc.stream_id=s.id
 LEFT JOIN LATERAL (SELECT status,rows_written,requested_at,finished_at FROM managed_runs
  WHERE stream_id=s.id ORDER BY requested_at DESC,id DESC LIMIT 1) last_run ON true
 ORDER BY g.name,s.created_at DESC,s.id DESC LIMIT 501`)
	if err != nil {
		log.Printf("QA connection overview query failed: %v", err)
		http.Error(w, "connection overview unavailable", http.StatusServiceUnavailable)
		return
	}
	defer rows.Close()
	out := make([]runtimeOverviewConnection, 0)
	pairIndex := make(map[string]int)
	count := 0
	for rows.Next() {
		count++
		if count > 500 {
			http.Error(w, "connection overview exceeds QA limit", http.StatusRequestEntityTooLarge)
			return
		}
		var c runtimeOverviewConnection
		var stream runtimeOverviewStream
		var runStatus string
		var rowsWritten int64
		var requestedAt, finishedAt sql.NullTime
		if err = rows.Scan(&c.ID, &c.Name, &c.SourceID, &c.SourceName, &c.SourceStatus, &c.SourceCheckedAt,
			&c.TargetID, &c.TargetName, &c.TargetStatus, &c.TargetCheckedAt,
			&stream.ID, &stream.SourceSchema, &stream.SourceTable, &stream.TargetDatabase, &stream.TargetTable, &stream.SyncMode,
			&stream.RevisionCurrent, &stream.BindingCurrent, &stream.ScheduleEnabled, &stream.ScheduleSeconds,
			&runStatus, &rowsWritten, &requestedAt, &finishedAt); err != nil {
			log.Printf("QA connection overview scan failed: %v", err)
			http.Error(w, "connection overview unavailable", http.StatusServiceUnavailable)
			return
		}
		if runStatus != "" && requestedAt.Valid {
			stream.LatestRun = &runtimeOverviewRun{Status: runStatus, RowsWritten: rowsWritten, RequestedAt: requestedAt.Time}
			if finishedAt.Valid {
				stream.LatestRun.FinishedAt = &finishedAt.Time
			}
		}
		key := c.ID
		index, ok := pairIndex[key]
		if !ok {
			index = len(out)
			pairIndex[key] = index
			c.Streams = make([]runtimeOverviewStream, 0)
			out = append(out, c)
		}
		out[index].Streams = append(out[index].Streams, stream)
	}
	if rows.Err() != nil {
		log.Printf("QA connection overview rows failed: %v", rows.Err())
		http.Error(w, "connection overview unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"connections": out})
}
