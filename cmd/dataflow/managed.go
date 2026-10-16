package main

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "log"
 "net/http"
 "os"
 "regexp"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/connectors/clickhouse"
 "github.com/dinhvansh/dataflow-engine/internal/connectors/sqlserver"
 "github.com/dinhvansh/dataflow-engine/internal/engine"
 pgstate "github.com/dinhvansh/dataflow-engine/internal/state/postgres"
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
  select{case s.managedSlots<-struct{}{}:default:http.Error(w,"managed runner capacity reached",429);return}
  ctx,cancel:=context.WithTimeout(r.Context(),10*time.Second);defer cancel()
  var runID string
  err:=s.pg.QueryRow(ctx,`INSERT INTO managed_runs(stream_id,status)
      SELECT id,'queued' FROM sync_streams WHERE id=$1::uuid RETURNING id::text`,args.StreamID).Scan(&runID)
  if err!=nil{<-s.managedSlots;http.Error(w,"stream not found or cannot enqueue",404);return}
  go func(){defer func(){<-s.managedSlots}();s.executeManaged(runID,args.StreamID)}()
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

func(s *server)executeManaged(runID,streamID string){
 ctx,cancel:=context.WithTimeout(context.Background(),2*time.Hour)
 defer cancel()
 stats:=engine.Stats{}
 var upper []byte
 var lower []byte
 var runError error
 completed:=false
 defer func(){
  finalizeCtx,done:=context.WithTimeout(context.Background(),10*time.Second);defer done()
  status:="failed"
  if completed{status="succeeded"}
  message:=""
  if runError!=nil{message=runError.Error()}
  _,err:=s.pg.Exec(finalizeCtx,`UPDATE managed_runs SET status=$2,rows_written=$3,batches_written=$4,
     lower_cursor=$5,upper_cursor=$6,error_message=$7,finished_at=now() WHERE id=$1::uuid`,
    runID,status,stats.Rows,stats.Batches,lower,upper,message)
  if err!=nil{log.Printf("managed run %s finalization failed: %v",runID,err)}
 }()
 if _,err:=s.pg.Exec(ctx,`UPDATE managed_runs SET status='running',started_at=now() WHERE id=$1::uuid`,runID);err!=nil{runError=err;return}
 var cfg managedStream
 err:=s.pg.QueryRow(ctx,`SELECT source_schema,source_table,target_database,target_table,batch_size,workers,queue_capacity
  FROM sync_streams WHERE id=$1::uuid`,streamID).Scan(&cfg.SourceSchema,&cfg.SourceTable,&cfg.TargetDatabase,&cfg.TargetTable,&cfg.BatchSize,&cfg.Workers,&cfg.QueueCapacity)
 if err!=nil{runError=err;return}
 if cfg.TargetDatabase!=scratchDatabase()||cfg.TargetTable!="dataflow_raw_events"{runError=errors.New("managed target outside scratch raw event table");return}
 store:=pgstate.Store{Pool:s.pg}
 lease,err:=store.Claim(ctx,streamID,runID,5*time.Minute)
 if err!=nil{runError=fmt.Errorf("stream lease unavailable: %w",err);return}
 lower=lease.Lower.Bytes()
 defer func(){releaseCtx,done:=context.WithTimeout(context.Background(),5*time.Second);defer done();_ = store.Release(releaseCtx,lease)}()
 // A lease heartbeat stops the stream if ownership is lost. SQL Server scan
 // may run for a long time, so never rely on initial lease TTL alone.
 heartbeatDone:=make(chan struct{})
 go func(){
  defer close(heartbeatDone)
  ticker:=time.NewTicker(30*time.Second);defer ticker.Stop()
  for{select{
   case <-ctx.Done():return
   case <-ticker.C:
    hctx,hcancel:=context.WithTimeout(ctx,10*time.Second)
    heartbeatErr:=store.Renew(hctx,lease,5*time.Minute)
    hcancel()
    if heartbeatErr!=nil{log.Printf("managed run %s lease lost: %v",runID,heartbeatErr);cancel();return}
  }}
 }()
 defer func(){cancel();<-heartbeatDone}()
 reader:=&sqlserver.RowVersionReader{DB:s.sql,Schema:cfg.SourceSchema,Table:cfg.SourceTable,BatchSize:cfg.BatchSize,Lower:lease.Lower}
 target:=&clickhouse.Writer{Endpoint:s.chURL,Database:cfg.TargetDatabase,Table:cfg.TargetTable,Username:s.chUser,Password:s.chPass}
 writer:=&clickhouse.EventWriter{Destination:target,StreamID:streamID,RunID:runID}
 stats,err=engine.Run(ctx,engine.Config{Workers:cfg.Workers,QueueCapacity:cfg.QueueCapacity},reader,writer)
 if err!=nil{runError=fmt.Errorf("stream load: %w",err);return}
 if err=ctx.Err();err!=nil{runError=err;return}
 upper=reader.Upper.Bytes()
 // Require ClickHouse to return the exact number of events for this run.
 // A mismatched or unverifiable write NEVER advances the cursor.
 observed,verifyErr:=target.CountRunRows(ctx,runID)
 if verifyErr!=nil{runError=fmt.Errorf("cannot verify destination count: %w",verifyErr);return}
 if observed!=stats.Rows{runError=fmt.Errorf("destination mismatch: expected %d events, found %d",stats.Rows,observed);return}
 // Engine success also means source snapshot committed and each batch returned
 // a successful destination acknowledgment.
 if err=store.Commit(ctx,lease,reader.Upper);err!=nil{runError=fmt.Errorf("checkpoint not committed: %w",err);return}
 completed=true
}

func scratchDatabase()string{
 if v:=os.Getenv("DATAFLOW_TEST_TARGET_DATABASE");v!=""{return v}
 return "scratch"
}
func managedEnabled()bool{
 return os.Getenv("DATAFLOW_ALLOW_TEST_LOAD")=="true" &&
        os.Getenv("DATAFLOW_ENABLE_MANAGED_TEST")=="true"
}
