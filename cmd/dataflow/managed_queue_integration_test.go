package main

import(
 "context"
 "errors"
 "fmt"
 "net/http"
 "net/url"
 "os"
 "path/filepath"
 "testing"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/connectors/clickhouse"
 "github.com/dinhvansh/dataflow-engine/internal/cursor"
 "github.com/dinhvansh/dataflow-engine/internal/engine"
 "github.com/dinhvansh/dataflow-engine/internal/state/postgres"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)

func TestManagedQueueIntegration(t *testing.T){
 dsn:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
 chURL:=os.Getenv("DATAFLOW_INTEGRATION_CH_URL")
 if dsn==""||chURL==""{t.Skip("requires real isolated PostgreSQL and ClickHouse")}
 if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true"{t.Fatal("test requires disposable PostgreSQL")}
 ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second);defer cancel()
 pool,err:=pgxpool.New(ctx,dsn);if err!=nil{t.Fatal(err)};defer pool.Close()
 targetTable:=fmt.Sprintf("dataflow_raw_events_%d",time.Now().UnixNano())
 for _,migration:=range []string{
  "001_bootstrap.sql","002_sync_checkpoint.sql","003_managed_runs.sql","004_stream_tuning.sql","009_managed_queue.sql",
  "032_sync_stream_selected_columns.sql","033_qa_stream_modes_destinations.sql","034_source_key_and_bootstrap_cursor.sql","035_typed_destinations.sql",
 }{
  raw,readErr:=os.ReadFile(filepath.Join("..","..","migrations","postgres",migration));if readErr!=nil{t.Fatal(readErr)}
  if _,err=pool.Exec(ctx,string(raw));err!=nil{t.Fatalf("migration %s failed: %v",migration,err)}
 }
 t.Setenv("DATAFLOW_ALLOW_TEST_LOAD","true")
 t.Setenv("DATAFLOW_ENABLE_MANAGED_TEST","true")
 s:=&server{pg:pool,managedSlots:make(chan struct{},2)}
 if err=s.checkManagedSchema(ctx);err!=nil{t.Fatal(err)}
 var streamID string
 err=pool.QueryRow(ctx,`INSERT INTO sync_streams(source_schema,source_table,target_database,target_table,
  batch_size,workers,queue_capacity) VALUES('Erp','Part','scratch',$1,100,1,1)
  RETURNING id::text`,targetTable).Scan(&streamID)
 if err!=nil{t.Fatal(err)}
 if _,err=pool.Exec(ctx,"INSERT INTO sync_checkpoints(stream_id) VALUES($1::uuid)",streamID);err!=nil{t.Fatal(err)}
 var runID string
 err=pool.QueryRow(ctx,`INSERT INTO managed_runs(stream_id,status) VALUES($1::uuid,'queued') RETURNING id::text`,streamID).Scan(&runID)
 if err!=nil{t.Fatal(err)}
 // A second run for the same stream must be rejected even across processes.
 _,err=pool.Exec(ctx,`INSERT INTO managed_runs(stream_id,status) VALUES($1::uuid,'queued')`,streamID)
 if err==nil{t.Fatal("duplicate active run allowed")}
 first,err:=s.claimManagedRun(ctx,runID);if err!=nil{t.Fatal(err)}
 if first.Attempts!=1{t.Fatalf("first attempt=%d",first.Attempts)}
 if err=s.renewManagedRun(ctx,first);err!=nil{t.Fatal(err)}
 if _,err=s.claimManagedRun(ctx,runID);!errors.Is(err,pgx.ErrNoRows){t.Fatalf("second simultaneous claim allowed: %v",err)}
 // Process dies: both leases eventually expire. Simulate expiry in isolated PostgreSQL.
 if _,err=pool.Exec(ctx,`UPDATE managed_runs SET lease_expires_at=clock_timestamp()-interval '1 second'
    WHERE id=$1::uuid`,runID);err!=nil{t.Fatal(err)}
 if _,err=pool.Exec(ctx,`UPDATE sync_checkpoints SET lease_expires_at=clock_timestamp()-interval '1 second'
    WHERE stream_id=$1::uuid`,streamID);err!=nil{t.Fatal(err)}
 second,err:=s.claimManagedRun(ctx,runID);if err!=nil{t.Fatal(err)}
 if second.OwnerID==first.OwnerID||second.Fence<=first.Fence||second.Attempts!=2{
  t.Fatalf("recovery claim invalid: first=%+v second=%+v",first,second)
 }
 next,err:=cursor.ParseRowVersion("0x000000000000000A");if err!=nil{t.Fatal(err)}
 stats:=engine.Stats{Rows:1,Batches:1}
 if err=s.finishManagedSuccess(ctx,first,next,stats);!errors.Is(err,postgres.ErrLeaseLost){
  t.Fatalf("stale worker advanced checkpoint: %v",err)
 }

 // Real ClickHouse test: prior attempt's partial events must NOT inflate
 // the count for the new attempt, while latest-state view can deduplicate.
 endpoint,err:=url.Parse(chURL);if err!=nil{t.Fatal(err)}
 httpClient:=&http.Client{Timeout:15*time.Second}
 runDDL:=func(sql string){
  t.Helper()
  u:=*endpoint;q:=u.Query();q.Set("query",sql);u.RawQuery=q.Encode()
  req,requestErr:=http.NewRequestWithContext(ctx,http.MethodPost,u.String(),nil)
  if requestErr!=nil{t.Fatal(requestErr)}
  if user:=os.Getenv("DATAFLOW_INTEGRATION_CH_WRITER_USER");user!=""{
   req.SetBasicAuth(user,os.Getenv("DATAFLOW_INTEGRATION_CH_WRITER_PASSWORD"))
  }
  res,reqErr:=httpClient.Do(req);if reqErr!=nil{t.Fatal(reqErr)}
  res.Body.Close()
  if res.StatusCode<200||res.StatusCode>=300{t.Fatalf("ClickHouse DDL status %d",res.StatusCode)}
 }
 runDDL(`CREATE TABLE IF NOT EXISTS scratch.dataflow_raw_events
 (_stream_id String,_pk String,_sysrevid String,_payload String,_run_id String,
 _batch_seq UInt64,_ingested_at DateTime64(3,'UTC') DEFAULT now64(3))
 ENGINE=MergeTree ORDER BY (_stream_id,_pk,_sysrevid)`)
 writer:=&clickhouse.Writer{Endpoint:chURL,Database:"scratch",Table:"dataflow_raw_events",
  Username:os.Getenv("DATAFLOW_INTEGRATION_CH_WRITER_USER"),Password:os.Getenv("DATAFLOW_INTEGRATION_CH_WRITER_PASSWORD")}
 makeEvent:=func(attempt string)engine.Batch{return engine.Batch{Rows:[]map[string]any{{
  "_stream_id":streamID,"_pk":"00000000-0000-0000-0000-000000000001",
  "_sysrevid":"0x0000000000000009","_payload":"{\"v\":9}",
  "_run_id":attempt,"_batch_seq":0,
 }}}}
 if err=writer.Write(ctx,makeEvent(first.OwnerID));err!=nil{t.Fatal(err)}
 if err=writer.Write(ctx,makeEvent(second.OwnerID));err!=nil{t.Fatal(err)}
 if err=verifyManagedAttempt(ctx,writer,second.OwnerID,1);err!=nil{t.Fatal(err)}
 if err=verifyManagedAttempt(ctx,writer,second.OwnerID,2);err==nil{t.Fatal("incorrect destination count accepted")}
 if err=s.finishManagedSuccess(ctx,second,next,stats);err!=nil{t.Fatal(err)}
 cp,err:=postgres.Store{Pool:pool}.Load(ctx,streamID);if err!=nil{t.Fatal(err)}
 if cp.Lower!=next||cp.Generation!=1{t.Fatalf("checkpoint wrong: %+v",cp)}
 var status string;var written uint64
 if err=pool.QueryRow(ctx,"SELECT status,rows_written FROM managed_runs WHERE id=$1::uuid",runID).Scan(&status,&written);err!=nil{t.Fatal(err)}
 if status!="succeeded"||written!=1{t.Fatalf("atomic finish failed: status=%s rows=%d",status,written)}

 // A destination mismatch is a terminal run failure and MUST NOT advance cursor.
 var failedID string
 if err=pool.QueryRow(ctx,"INSERT INTO managed_runs(stream_id,status) VALUES($1::uuid,'queued') RETURNING id::text",streamID).Scan(&failedID);err!=nil{t.Fatal(err)}
 mismatch,err:=s.claimManagedRun(ctx,failedID);if err!=nil{t.Fatal(err)}
 verifyErr:=verifyManagedAttempt(ctx,writer,mismatch.OwnerID,1)
 if verifyErr==nil{t.Fatal("missing target rows did not fail")}
 if err=s.finishManagedFailure(ctx,mismatch,verifyErr,engine.Stats{Rows:1});err!=nil{t.Fatal(err)}
 after,err:=postgres.Store{Pool:pool}.Load(ctx,streamID);if err!=nil{t.Fatal(err)}
 if after.Lower!=next||after.Generation!=1{t.Fatalf("failed attempt altered checkpoint %+v",after)}

 // Bounded crash retries prevent unbounded replay storms.
 var cappedID string
 if err=pool.QueryRow(ctx,"INSERT INTO managed_runs(stream_id,status) VALUES($1::uuid,'queued') RETURNING id::text",streamID).Scan(&cappedID);err!=nil{t.Fatal(err)}
 for i:=1;i<=managedMaxAttempts;i++{
  claim,claimErr:=s.claimManagedRun(ctx,cappedID)
  if claimErr!=nil||claim.Attempts!=i{t.Fatalf("claim %d: %v %+v",i,claimErr,claim)}
  if _,err=pool.Exec(ctx,`UPDATE managed_runs SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1::uuid`,cappedID);err!=nil{t.Fatal(err)}
  if _,err=pool.Exec(ctx,`UPDATE sync_checkpoints SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE stream_id=$1::uuid`,streamID);err!=nil{t.Fatal(err)}
 }
 if _,err=s.claimManagedRun(ctx,cappedID);!errors.Is(err,pgx.ErrNoRows){t.Fatalf("over-cap claim: %v",err)}
 if err=pool.QueryRow(ctx,"SELECT status FROM managed_runs WHERE id=$1::uuid",cappedID).Scan(&status);err!=nil||status!="failed"{
  t.Fatalf("exhausted run status=%s error=%v",status,err)
 }
 again,err:=postgres.Store{Pool:pool}.Load(ctx,streamID)
 if err!=nil||again.Lower!=next{t.Fatalf("capped run modified cursor: %v %v",again,err)}
 t.Log(fmt.Sprintf("PASS: claims %d, cursor fence, isolated attempts, count validation, atomic checkpoint and bounded recovery",managedMaxAttempts))
}
