package main

import (
 "context"
 "encoding/json"
 "net/http"
 "os"
 "regexp"

 "github.com/dinhvansh/dataflow-engine/internal/connectors/sqlserver"
)

var validUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type managedStream struct {
 ID string `json:"id"`
 SourceSchema string `json:"source_schema"`
 SourceTable string `json:"source_table"`
 TargetDatabase string `json:"target_database"`
 TargetTable string `json:"target_table"`
 BatchSize int `json:"batch_size"`
 Workers int `json:"workers"`
 QueueCapacity int `json:"queue_capacity"`
 LowerCursor string `json:"lower_cursor,omitempty"`
}
func(s *server) streamAPI(w http.ResponseWriter,r *http.Request){
 if !s.validBearer(r){http.Error(w,"unauthorized",401);return}
 switch r.Method{
 case http.MethodGet:
  ctx,cancel:=context.WithTimeout(r.Context(),10*time.Second);defer cancel()
  rows,err:=s.pg.Query(ctx,`SELECT s.id::text,s.source_schema,s.source_table,s.target_database,s.target_table,s.batch_size,s.workers,s.queue_capacity,encode(c.lower_cursor,'hex')
  FROM sync_streams s JOIN sync_checkpoints c ON c.stream_id=s.id ORDER BY s.created_at DESC LIMIT 100`)
  if err!=nil{http.Error(w,"cannot list streams",500);return}
  defer rows.Close()
  out:=make([]managedStream,0)
  for rows.Next(){
   var item managedStream;var hex string
   if err=rows.Scan(&item.ID,&item.SourceSchema,&item.SourceTable,&item.TargetDatabase,&item.TargetTable,&item.BatchSize,&item.Workers,&item.QueueCapacity,&hex);err!=nil{http.Error(w,"cannot scan streams",500);return}
   item.LowerCursor="0x"+hex;out=append(out,item)
  }
  if err=rows.Err();err!=nil{http.Error(w,"cannot fetch streams",500);return}
  jsonResponse(w,200,map[string]any{"streams":out})
 case http.MethodPost:
  r.Body=http.MaxBytesReader(w,r.Body,16384)
  dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields()
  var cfg managedStream
  if err:=dec.Decode(&cfg);err!=nil{http.Error(w,"invalid JSON configuration",400);return}
  if cfg.BatchSize<100||cfg.BatchSize>100000||cfg.Workers<1||cfg.Workers>32||cfg.QueueCapacity<1||cfg.QueueCapacity>100{
   http.Error(w,"invalid batch/worker/queue limits",400);return
  }
  if cfg.TargetDatabase!=scratchDatabase()||cfg.TargetTable!="dataflow_raw_events"{
   http.Error(w,"managed incremental requires configured scratch database and dataflow_raw_events",403);return
  }
  ctx,cancel:=context.WithTimeout(r.Context(),30*time.Second);defer cancel()
  cols,err:=sqlserver.DiscoverColumns(ctx,s.sql,cfg.SourceSchema,cfg.SourceTable)
  if err!=nil||len(cols)==0{http.Error(w,"source metadata unavailable",400);return}
  rev,id:=false,false
  for _,col:=range cols{
   if col.Name=="SysRevID"&&col.CursorCandidate{rev=true}
   if col.Name=="SysRowID"&&col.SQLType=="uniqueidentifier"{id=true}
  }
  if !rev||!id{http.Error(w,"source requires native SysRevID rowversion and SysRowID uniqueidentifier",422);return}
  unique,uniqueErr:=sqlserver.HasUniqueSysRowID(ctx,s.sql,cfg.SourceSchema,cfg.SourceTable)
  if uniqueErr!=nil||!unique{http.Error(w,"source SysRowID must have a single-column unique index (read-only inspection)",422);return}
  var isolation string
  if err=s.sql.QueryRowContext(ctx,`SELECT snapshot_isolation_state_desc FROM sys.databases WHERE name=DB_NAME()`).Scan(&isolation);err!=nil||isolation!="ON"{
   http.Error(w,"SQL Server SNAPSHOT isolation must already be ON (DBA-managed setting)",422);return
  }
  tx,err:=s.pg.Begin(ctx)
  if err!=nil{http.Error(w,"metadata storage unavailable",500);return}
  defer tx.Rollback(ctx)
  err=tx.QueryRow(ctx,`INSERT INTO sync_streams(source_schema,source_table,target_database,target_table,batch_size,workers,queue_capacity)
    VALUES($1,$2,$3,$4,$5,$6,$7)
    ON CONFLICT(source_schema,source_table,target_database,target_table)
    DO UPDATE SET batch_size=EXCLUDED.batch_size,workers=EXCLUDED.workers,queue_capacity=EXCLUDED.queue_capacity
    RETURNING id::text`,cfg.SourceSchema,cfg.SourceTable,cfg.TargetDatabase,cfg.TargetTable,cfg.BatchSize,cfg.Workers,cfg.QueueCapacity).Scan(&cfg.ID)
  if err!=nil{http.Error(w,"cannot save stream",500);return}
  if _,err=tx.Exec(ctx,`INSERT INTO sync_checkpoints(stream_id) VALUES ($1::uuid) ON CONFLICT DO NOTHING`,cfg.ID);err!=nil{http.Error(w,"cannot initialize cursor",500);return}
  if err=tx.Commit(ctx);err!=nil{http.Error(w,"cannot commit stream",500);return}
  var lastHex string
  if err=s.pg.QueryRow(ctx,`SELECT encode(lower_cursor,'hex') FROM sync_checkpoints WHERE stream_id=$1::uuid`,cfg.ID).Scan(&lastHex);err!=nil{
   http.Error(w,"cannot read committed checkpoint",500);return
  }
  cfg.LowerCursor=formatHex(lastHex)
  jsonResponse(w,201,cfg)
 default:http.Error(w,"method not allowed",405)
 }
}

func(s *server)managedRunAPI(w http.ResponseWriter,r *http.Request){
 if !s.validBearer(r){http.Error(w,"unauthorized",401);return}
 switch r.Method{
 case http.MethodPost:
  r.Body=http.MaxBytesReader(w,r.Body,4096)
  var args struct{StreamID string `json:"stream_id"`}
  dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields()
  if dec.Decode(&args)!=nil||!validUUID.MatchString(args.StreamID){http.Error(w,"valid stream_id required",400);return}
  if !managedEnabled(){http.Error(w,"managed test jobs disabled",403);return}
  // Durable enqueue; worker claims through PostgreSQL after API response.
  // Unique active-stream index guarantees one pending/run per stream.
  ctx,cancel:=context.WithTimeout(r.Context(),10*time.Second);defer cancel()
  var runID string
  err:=s.pg.QueryRow(ctx,`INSERT INTO managed_runs(stream_id,status)
      SELECT id,'queued' FROM sync_streams WHERE id=$1::uuid RETURNING id::text`,args.StreamID).Scan(&runID)
  if err!=nil{http.Error(w,"cannot enqueue: unknown stream or another run already active",409);return}
  jsonResponse(w,202,map[string]any{"run_id":runID,"status":"queued"})
 case http.MethodGet:
  id:=r.URL.Query().Get("id")
  if !validUUID.MatchString(id){http.Error(w,"valid run id required",400);return}
  ctx,cancel:=context.WithTimeout(r.Context(),10*time.Second);defer cancel()
  var streamID,status,lowerHex,upperHex,errMessage string
  var count,batches int64
  err:=s.pg.QueryRow(ctx,`SELECT stream_id::text,status,COALESCE(encode(lower_cursor,'hex'),''),COALESCE(encode(upper_cursor,'hex'),''),rows_written,batches_written,COALESCE(error_message,'')
  FROM managed_runs WHERE id=$1::uuid`,id).Scan(&streamID,&status,&lowerHex,&upperHex,&count,&batches,&errMessage)
  if err!=nil{http.Error(w,"run not found",404);return}
  jsonResponse(w,200,map[string]any{"run_id":id,"stream_id":streamID,"status":status,"lower_cursor":formatHex(lowerHex),"upper_exclusive":formatHex(upperHex),"rows_written":count,"batches_written":batches,"error":errMessage})
 default:http.Error(w,"method not allowed",405)
 }
}
func formatHex(x string)string{if x==""{return ""};return "0x"+x}
func jsonResponse(w http.ResponseWriter,status int,value any){w.Header().Set("Content-Type","application/json");w.WriteHeader(status);_ = json.NewEncoder(w).Encode(value)}


func scratchDatabase()string{
 if v:=os.Getenv("DATAFLOW_TEST_TARGET_DATABASE");v!=""{return v}
 return "scratch"
}
func managedEnabled()bool{
 return os.Getenv("DATAFLOW_ALLOW_TEST_LOAD")=="true" &&
        os.Getenv("DATAFLOW_ENABLE_MANAGED_TEST")=="true"
}
