package main

import (
 "context"
 "errors"
 "fmt"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/connectors/clickhouse"
 "github.com/dinhvansh/dataflow-engine/internal/cursor"
 "github.com/dinhvansh/dataflow-engine/internal/engine"
 "github.com/dinhvansh/dataflow-engine/internal/safetable"
 pgstate "github.com/dinhvansh/dataflow-engine/internal/state/postgres"
)

// CORE-RUN-2 is verified with real SQL Server source reads, ClickHouse
// persisted partial+recovered writes and PostgreSQL fenced recovery.
// Fault is injected by expiring the ONLY disposable PG leases and creating
// a fresh Go worker object; it is crash-equivalent lease recovery, not an
// actual process SIGKILL test nor full multi-host fault certification.
func TestCoreRun2RealCrashRetryCheckpointIntegration(t *testing.T){
 ctx,svc,cleanup:=coreRealFixture(t);defer cleanup()
 mux:=http.NewServeMux()
 mux.HandleFunc("/api/v1/managed/streams",svc.streamAPI)
 mux.HandleFunc("/api/v1/managed/runs",svc.managedRunAPI)
 app:=httptest.NewServer(mux);defer app.Close()
 status,cfg:=coreHTTP(t,app.URL,"POST","/api/v1/managed/streams",
  map[string]any{"source_schema":"Erp","source_table":"Part",
   "target_database":"scratch","target_table":"dataflow_raw_events",
   "batch_size":100,"workers":2,"queue_capacity":2})
 streamID,_:=cfg["id"].(string)
 if status!=201||!validUUID.MatchString(streamID){t.Fatalf("setup failed: %d %v",status,cfg)}
 coreRawFixture(t,ctx,svc,streamID)
 defer func(){
  c,done:=context.WithTimeout(context.Background(),10*time.Second);defer done()
  _,_=svc.pg.Exec(c,"DELETE FROM managed_runs WHERE stream_id=$1::uuid",streamID)
  _,_=svc.pg.Exec(c,"DELETE FROM sync_checkpoints WHERE stream_id=$1::uuid",streamID)
  _,_=svc.pg.Exec(c,"DELETE FROM sync_streams WHERE id=$1::uuid",streamID)
 }()
 enqueue:=func()string{
  t.Helper()
  code,res:=coreHTTP(t,app.URL,"POST","/api/v1/managed/runs",map[string]string{"stream_id":streamID})
  id,_:=res["run_id"].(string)
  if code!=202||!validUUID.MatchString(id){t.Fatalf("API enqueue rejected %d %+v",code,res)}
  return id
 }
 logicalRun:=enqueue()
 first,err:=svc.claimManagedRun(ctx,logicalRun)
 if err!=nil||first.Attempts!=1{t.Fatalf("initial PG lease failed: %+v %v",first,err)}
 // Source row comes from the ACTUAL disposable SQL Server driver, never a
 // handcrafted fake. We write only the destination scratch table.
 var pk,revision,part string
 err=svc.sql.QueryRowContext(ctx,`SELECT TOP (1)
  CONVERT(varchar(36),SysRowID),
  CONVERT(varchar(18),CONVERT(varbinary(8),SysRevID),1),
  PartNum FROM [Erp].[Part] ORDER BY PartNum`).Scan(&pk,&revision,&part)
 if err!=nil{t.Fatalf("real source SELECT unavailable: %v",err)}
 dest:=&clickhouse.Writer{Endpoint:svc.chURL,Database:"scratch",Table:"dataflow_raw_events",
  Username:svc.chUser,Password:svc.chPass}
 partial:=clickhouse.EventWriter{Destination:dest,StreamID:streamID,RunID:first.OwnerID}
 if err=partial.Write(ctx,engine.Batch{Sequence:0,Rows:[]map[string]any{{
  "SysRowID":pk,"SysRevID":revision,"PartNum":part,
 }}});err!=nil{t.Fatalf("cannot persist partial real source event: %v",err)}
 oldCount,err:=dest.CountRunRows(ctx,first.OwnerID)
 if err!=nil||oldCount!=1{t.Fatalf("partial persisted event missing: %d %v",oldCount,err)}

 // Simulate worker process disappearing after its partial INSERT: only QA
 // lease metadata expires, source remains untouched.
 for _,statement:=range []string{
  `UPDATE managed_runs SET lease_expires_at=clock_timestamp()-interval '1 second'
    WHERE id=$1::uuid AND lease_owner=$2::uuid`,
  `UPDATE sync_checkpoints SET lease_expires_at=clock_timestamp()-interval '1 second'
    WHERE stream_id=$1::uuid AND lease_owner=$2::uuid`,
 }{
  if strings.Contains(statement,"managed_runs"){
   _,err=svc.pg.Exec(ctx,statement,first.RunID,first.OwnerID)
  }else{
   _,err=svc.pg.Exec(ctx,statement,first.StreamID,first.OwnerID)
  }
  if err!=nil{t.Fatal(err)}
 }
 // Fresh worker reconstructs all execution state solely from PostgreSQL.
 recovered:=&server{pg:svc.pg,sql:svc.sql,chURL:svc.chURL,
  chUser:svc.chUser,chPass:svc.chPass,managedSlots:make(chan struct{},2)}
 second,err:=recovered.claimManagedRun(ctx,logicalRun)
 if err!=nil||second.Attempts!=2||second.Fence<=first.Fence||second.OwnerID==first.OwnerID{
  t.Fatalf("fenced crash recovery invalid: first=%+v, second=%+v, err=%v",first,second,err)
 }
 upper,err:=cursor.ParseRowVersion("0x0000000000000001");if err!=nil{t.Fatal(err)}
 if err=recovered.finishManagedSuccess(ctx,first,upper,engine.Stats{Rows:1,Batches:1});
  !errors.Is(err,pgstate.ErrLeaseLost){
  t.Fatalf("stale crashed worker was allowed to advance checkpoint: %v",err)
 }
 recovered.runClaimedManaged(ctx,second) // actual real SQL incremental ingest
 _,out:=coreHTTP(t,app.URL,"GET","/api/v1/managed/runs?id="+logicalRun,nil)
 if out["status"]!="succeeded"||out["rows_written"]!=float64(200)||
   out["dispatch_attempts"]!=float64(2)||out["lease_expires_at"]!=nil{
  t.Fatalf("real recovered worker did not complete: %+v",out)
 }
 nowCount,err:=dest.CountRunRows(ctx,second.OwnerID)
 if err!=nil||nowCount!=200{t.Fatalf("recovered attempt wrong rows %d %v",nowCount,err)}
 oldCount,err=dest.CountRunRows(ctx,first.OwnerID)
 if err!=nil||oldCount!=1{t.Fatalf("old partial attempt unexpectedly mutated: %d %v",oldCount,err)}
 cl:=safetable.Client{Endpoint:svc.chURL,User:svc.chUser,Password:svc.chPass}
 physical,e:=cl.Query(ctx,fmt.Sprintf(
  "SELECT count(),uniqExact(_pk),uniqExact((_pk,_sysrevid)) FROM scratch.dataflow_raw_events WHERE _stream_id='%s' FORMAT TabSeparatedRaw",streamID))
 if e!=nil||physical!="201\t200\t200"{
  t.Fatalf("raw log must preserve partial evidence but deduplicate by source key/version: %q %v",physical,e)
 }
 var gen int64
 var checkpoint []byte
 if err=svc.pg.QueryRow(ctx,"SELECT generation,lower_cursor FROM sync_checkpoints WHERE stream_id=$1::uuid",streamID).
  Scan(&gen,&checkpoint);err!=nil||gen!=1||len(checkpoint)!=8{
  t.Fatalf("durable checkpoint was not committed safely: gen=%d error=%v",gen,err)
 }
 // Exercise the new bounded TRANSIENT retry transition on the same real PG
 // stream, then perform a successful no-op incremental retry over real SQL.
 retryRun:=enqueue()
 interrupted,err:=recovered.claimManagedRun(ctx,retryRun)
 if err!=nil||interrupted.Attempts!=1{t.Fatalf("transient claim: %v %+v",err,interrupted)}
 if !retryableManagedFailure(&clickhouse.WriteHTTPError{StatusCode:503}){
  t.Fatal("ClickHouse temporary 503 must be retryable")
 }
 if err=recovered.finishManagedRetry(ctx,interrupted);err!=nil{
  t.Fatalf("transient error did not requeue without checkpoint commit: %v",err)
 }
 var stillGen int64;var stillCursor []byte;var queued string
 err=svc.pg.QueryRow(ctx,`SELECT r.status,c.generation,c.lower_cursor
  FROM managed_runs r JOIN sync_checkpoints c ON c.stream_id=r.stream_id
  WHERE r.id=$1::uuid`,retryRun).Scan(&queued,&stillGen,&stillCursor)
 if err!=nil||queued!="queued"||stillGen!=gen||string(stillCursor)!=string(checkpoint){
  t.Fatalf("transient retry changed checkpoint: queued=%s gen=%d err=%v",queued,stillGen,err)
 }
 retried,err:=recovered.claimManagedRun(ctx,retryRun)
 if err!=nil||retried.Attempts!=2||retried.OwnerID==interrupted.OwnerID||
  retried.Fence<=interrupted.Fence{t.Fatalf("retry lease invalid %+v %v",retried,err)}
 if err=recovered.finishManagedSuccess(ctx,interrupted,upper,engine.Stats{});
  !errors.Is(err,pgstate.ErrLeaseLost){t.Fatalf("stale retried worker advanced cursor: %v",err)}
 recovered.runClaimedManaged(ctx,retried)
 _,out=coreHTTP(t,app.URL,"GET","/api/v1/managed/runs?id="+retryRun,nil)
 if out["status"]!="succeeded"||out["rows_written"]!=float64(0)||out["dispatch_attempts"]!=float64(2){
  t.Fatalf("bounded no-op retry failed: %+v",out)
 }
 var finalGen int64
 err=svc.pg.QueryRow(ctx,"SELECT generation FROM sync_checkpoints WHERE stream_id=$1::uuid",streamID).Scan(&finalGen)
 if err!=nil||finalGen!=2{t.Fatalf("checkpoint after no-op retry incorrect gen=%d err=%v",finalGen,err)}
 t.Logf("CORE-RUN-2 REAL PASS: persisted SQL Server partial 1 row → crash-equivalent expired lease → NEW worker fence → 200 real rows → PG checkpoint gen1 → stale worker denied → 503 bounded retry → fresh attempt → zero-row incremental gen2; 201 physical/200 deduped records")
}
