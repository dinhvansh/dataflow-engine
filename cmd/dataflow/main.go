package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	connectionsecurity "github.com/dinhvansh/dataflow-engine/internal/connections"
	"github.com/dinhvansh/dataflow-engine/internal/connectors/clickhouse"
	"github.com/dinhvansh/dataflow-engine/internal/connectors/sqlserver"
	"github.com/dinhvansh/dataflow-engine/internal/cursor"
	"github.com/dinhvansh/dataflow-engine/internal/engine"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RunRequest struct {
	SourceSchema   string `json:"source_schema"`
	SourceTable    string `json:"source_table"`
	TargetDatabase string `json:"target_database"`
	TargetTable    string `json:"target_table"`
	BatchSize      int    `json:"batch_size"`
	Workers        int    `json:"workers"`
	QueueCapacity  int    `json:"queue_capacity"`
	Mode           string `json:"mode"`
	LowerCursor    string `json:"lower_cursor"`
	AtomicReplace  bool   `json:"atomic_replace"`
}
type server struct {
	pg                           *pgxpool.Pool
	sql                          *sql.DB
	chURL, chUser, chPass, token string
	managedSlots                 chan struct{}
	sqlDAGSlots                  chan struct{}
}

func (s *server) run(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", 405)
		return
	}
	if !s.validBearer(r) {
		http.Error(w, "unauthorized", 401)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	var req RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if req.BatchSize < 100 || req.BatchSize > 100000 || req.Workers < 1 || req.Workers > 32 || req.QueueCapacity < 1 || req.QueueCapacity > 100 {
		http.Error(w, "invalid runtime settings", 400)
		return
	}
	if req.Mode != "" && req.Mode != "full" && req.Mode != "rowversion" {
		http.Error(w, "unsupported mode", 400)
		return
	}
	if req.Mode == "rowversion" {
		if _, err := cursor.ParseRowVersion(req.LowerCursor); err != nil {
			http.Error(w, "invalid lower_cursor: "+err.Error(), 400)
			return
		}
	}
	if req.Mode == "rowversion" && req.AtomicReplace {
		http.Error(w, "atomic_replace supported only for full mode", 400)
		return
	}
	// Only an explicitly designated scratch database may be mutated by the bootstrap.
	allowedDB := os.Getenv("DATAFLOW_TEST_TARGET_DATABASE")
	if allowedDB == "" {
		allowedDB = "scratch"
	}
	if req.TargetDatabase != allowedDB {
		http.Error(w, "bootstrap destination must use the configured disposable database", 403)
		return
	}
	// This bootstrap only supports intentionally isolated TEST targets.
	if os.Getenv("DATAFLOW_ALLOW_TEST_LOAD") != "true" {
		http.Error(w, "test load disabled", 403)
		return
	}
	ctx := r.Context()
	var id string
	err := s.pg.QueryRow(ctx, `INSERT INTO bootstrap_runs(status,source_table,target_table)
 VALUES ('running',$1,$2) RETURNING id`, req.SourceSchema+"."+req.SourceTable, req.TargetDatabase+"."+req.TargetTable).Scan(&id)
	if err != nil {
		http.Error(w, "cannot create run", 500)
		return
	}
	started := time.Now()
	var rd engine.Reader
	var incremental *sqlserver.RowVersionReader
	if req.Mode == "" || req.Mode == "full" {
		rd = &sqlserver.Reader{DB: s.sql, Schema: req.SourceSchema, Table: req.SourceTable, BatchSize: req.BatchSize, Snapshot: req.AtomicReplace}
	} else if req.Mode == "rowversion" {
		lower, parseErr := cursor.ParseRowVersion(req.LowerCursor)
		if parseErr != nil {
			http.Error(w, "invalid lower_cursor: "+parseErr.Error(), 400)
			return
		}
		incremental = &sqlserver.RowVersionReader{DB: s.sql, Schema: req.SourceSchema, Table: req.SourceTable, BatchSize: req.BatchSize, Lower: lower}
		rd = incremental
	} else {
		http.Error(w, "unsupported mode", 400)
		return
	}
	wr := &clickhouse.Writer{Endpoint: s.chURL, Database: req.TargetDatabase, Table: req.TargetTable, Username: s.chUser, Password: s.chPass}
	var stage *clickhouse.Staging
	var runErr error
	var stats engine.Stats
	targetWriter := wr
	if req.AtomicReplace {
		stage, runErr = targetWriter.PrepareStage(ctx, id)
		if runErr == nil {
			wr = stage.Writer
		}
	}
	if runErr == nil {
		stats, runErr = engine.Run(ctx, engine.Config{Workers: req.Workers, QueueCapacity: req.QueueCapacity}, rd, wr)
	}
	if runErr == nil && stage != nil {
		runErr = targetWriter.PublishStage(ctx, stage)
	}
	if stage != nil && stage.Published {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		if cleanupErr := targetWriter.DropStage(cleanupCtx, stage); cleanupErr != nil {
			log.Printf("run=%s old target cleanup pending: %v", id, cleanupErr)
		}
		cleanupCancel()
	}
	status := "succeeded"
	message := ""
	if runErr != nil {
		status = "failed"
		message = runErr.Error()
	}
	persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err = s.pg.Exec(persistCtx, `UPDATE bootstrap_runs SET status=$2,rows_written=$3,batches_written=$4,error_message=$5,finished_at=now() WHERE id=$1`, id, status, stats.Rows, stats.Batches, message); err != nil {
		log.Printf("run=%s persist: %v", id, err)
	}
	w.Header().Set("Content-Type", "application/json")
	if runErr != nil {
		w.WriteHeader(502)
	}
	upper := ""
	if incremental != nil && runErr == nil {
		upper = incremental.Upper.String()
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"run_id": id, "status": status, "rows_written": stats.Rows, "batches_written": stats.Batches, "upper_exclusive": upper, "elapsed_seconds": time.Since(started).Seconds(), "error": message})
}
func (s *server) status(w http.ResponseWriter, r *http.Request) {
	if !s.validBearer(r) {
		http.Error(w, "unauthorized", 401)
		return
	}
	id := r.URL.Query().Get("id")
	var status, source, target string
	var rows, batches int64
	var msg *string
	err := s.pg.QueryRow(r.Context(), `SELECT status,source_table,target_table,rows_written,batches_written,error_message FROM bootstrap_runs WHERE id::text=$1`, id).Scan(&status, &source, &target, &rows, &batches, &msg)
	if err != nil {
		http.Error(w, "run not found", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"run_id": id, "status": status, "source": source, "target": target, "rows_written": rows, "batches_written": batches, "error": msg})
}
func main() {
	required := []string{"DATAFLOW_POSTGRES_DSN", "DATAFLOW_SQLSERVER_DSN", "DATAFLOW_CH_URL", "DATAFLOW_API_TOKEN"}
	for _, k := range required {
		if os.Getenv(k) == "" {
			log.Fatal(errors.New("missing " + k))
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pg, err := pgxpool.New(ctx, os.Getenv("DATAFLOW_POSTGRES_DSN"))
	if err != nil {
		log.Fatal(err)
	}
	defer pg.Close()
	if err = pg.Ping(ctx); err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("sqlserver", os.Getenv("DATAFLOW_SQLSERVER_DSN"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	if err = db.PingContext(ctx); err != nil {
		log.Fatal(err)
	}
	svc := &server{pg: pg, sql: db, chURL: os.Getenv("DATAFLOW_CH_URL"), chUser: os.Getenv("DATAFLOW_CH_USER"), chPass: os.Getenv("DATAFLOW_CH_PASSWORD"), token: os.Getenv("DATAFLOW_API_TOKEN"), managedSlots: make(chan struct{}, 2), sqlDAGSlots: make(chan struct{}, 2)}
	// Fail startup if the opt-in isolated OIDC view lacks a pinned key or migration 018.
	// No other API handler, connection or job uses this independent JWT path.
	secureView, secureErr := connectionsecurity.NewSecureViewHandlerFromEnv(ctx, pg)
	if secureErr != nil { log.Fatal(secureErr) }
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/api/v1/bootstrap/runs", svc.run)
	mux.HandleFunc("/api/v1/bootstrap/status", svc.status)
	mux.HandleFunc("/api/v1/connections", svc.connectionsAPI)
	mux.HandleFunc("/api/v1/connections/definitions", svc.connectionDefinitionsAPI)
	mux.HandleFunc("/api/v1/connections/runtime", svc.runtimeProfilesAPI)
	mux.HandleFunc("/api/v1/connections/runtime/test", svc.runtimeTestAPI)
	mux.HandleFunc("/api/v1/connections/runtime/bind", svc.runtimeBindAPI)
	mux.Handle("/api/v1/secure/connections/definitions", secureView)
	mux.HandleFunc("/api/v1/source/tables", svc.sourceAPI)
	mux.HandleFunc("/api/v1/source/columns", svc.sourceAPI)
	mux.HandleFunc("/api/v1/source/rowversion-upper", svc.sourceAPI)
	mux.HandleFunc("/api/v1/pipelines/validate", svc.pipelineAPI)
	mux.HandleFunc("/api/v1/bootstrap/history", svc.history)
	mux.HandleFunc("/api/v1/managed/streams", svc.streamAPI)
	mux.HandleFunc("/api/v1/managed/runs", svc.managedRunAPI)
	mux.HandleFunc("/api/v1/core/graphs", svc.coreGraphsAPI)
	mux.HandleFunc("/api/v1/core/runs", svc.coreRunsAPI)
	mux.HandleFunc("/api/v1/core/actions", svc.coreActionAPI)
	mux.HandleFunc("/api/v1/managed/schedules", svc.scheduleAPI)
	mux.HandleFunc("/api/v1/reconcile/runs", svc.reconciliationAPI)
	mux.HandleFunc("/api/v1/reconcile/hard-delete", svc.hardDeleteAPI)
	mux.HandleFunc("/api/v1/sql/jobs", svc.sqlJobsAPI)
	mux.HandleFunc("/api/v1/sql/dags", svc.sqlDagsAPI)
	mux.HandleFunc("/api/v1/sql/dag-runs", svc.sqlDagRunsAPI)
	mux.HandleFunc("/api/v1/transform/compile", svc.transformCompile)
	mux.HandleFunc("/api/v1/transform/preview", svc.transformPreview)
	mux.HandleFunc("/api/v1/transform/columns", svc.transformColumns)
	mux.HandleFunc("/api/v1/transform/models", svc.transformModels)
	mux.HandleFunc("/api/v1/transform/dbt/runs", svc.dbtRunsAPI)
	mux.HandleFunc("/api/v1/transform/table/runs", svc.tableRunsAPI)
	mux.HandleFunc("/api/v1/transform/table/action", svc.tableActionAPI)
	mux.HandleFunc("/api/v1/transform/incremental/runs", svc.incrementalRunsAPI)
	mux.HandleFunc("/api/v1/transform/incremental/action", svc.incrementalActionAPI)
	mux.HandleFunc("/api/v1/transform/lineage", svc.lineageAPI)
	mux.HandleFunc("/api/v1/transform/auto-build", svc.autoBuildAPI)
	port := 8080
	if p := os.Getenv("PORT"); p != "" {
		if v, e := strconv.Atoi(p); e == nil && v > 0 && v < 65536 {
			port = v
		}
	}
	tlsCert,tlsKey,tlsConfig,tlsErr := directTLSFromEnv()
	if tlsErr != nil { log.Fatal(tlsErr) }
	srv := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: mux, ReadHeaderTimeout: 5 * time.Second, TLSConfig: tlsConfig}
	checkCtx, checkCancel := context.WithTimeout(context.Background(), 10*time.Second)
	schemaErr := svc.checkSQLDAGSchema(checkCtx)
	checkCancel()
	if schemaErr != nil {
		log.Fatal(schemaErr)
	}
	managedCtx, managedStop := context.WithTimeout(context.Background(), 10*time.Second)
	managedSchemaErr := svc.checkManagedSchema(managedCtx)
	managedStop()
	if managedSchemaErr != nil {
		log.Fatal(managedSchemaErr)
	}
	flowCtx, flowCancel := context.WithTimeout(context.Background(), 10*time.Second)
	flowErr := svc.checkFlowSchema(flowCtx)
	flowCancel()
	if flowErr != nil {
		log.Fatal(flowErr)
	}
	registryCtx, registryCancel := context.WithTimeout(context.Background(), 10*time.Second)
	registryErr := svc.checkConnectionRegistrySchema(registryCtx)
	registryCancel()
	if registryErr != nil {log.Fatal(registryErr)}
	coreCtx, coreStop := context.WithTimeout(context.Background(), 10*time.Second)
 coreErr := svc.checkCoreSchema(coreCtx)
 coreStop()
 if coreErr != nil { log.Fatal(coreErr) }
 root, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	svc.startManagedDispatcher(root)
	svc.startManagedScheduler(root)
	svc.startSQLDAGDispatcher(root)
	svc.startAutoBuildCoordinator(root)
	svc.startCoreCoordinator(root)
	go func() {
		<-root.Done()
		shutdownCtx, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown: %v", err)
		}
	}()
	var serveErr error
	if tlsConfig != nil {
		log.Printf("DataFlow isolated TLS 1.3 listener on :%d", port)
		serveErr = srv.ListenAndServeTLS(tlsCert, tlsKey)
	} else {
		log.Printf("DataFlow bootstrap listening on :%d", port)
		serveErr = srv.ListenAndServe()
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		log.Fatal(serveErr)
	}
}
