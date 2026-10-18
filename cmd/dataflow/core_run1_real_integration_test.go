package main

import (
 "bytes"
 "context"
 "database/sql"
 "encoding/json"
 "fmt"
 "io"
 "net/http"
 "net/http/httptest"
 "net/url"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/safetable"
 "github.com/jackc/pgx/v5/pgxpool"
)

// This is the real Core-Run-1 entrypoint test: authenticated HTTP enqueue
// -> durable PostgreSQL queue -> actual dispatcher -> SELECT-only SQL Server
// rowversion read -> real ClickHouse raw events -> fenced PG checkpoint.
// It intentionally does NOT claim to be a mixed-step Canvas/dbt pipeline.
func coreRealFixture(t *testing.T)(context.Context,*server,func()){
 t.Helper()
 pgDSN,sqlDSN,chURL:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN"),
  os.Getenv("DATAFLOW_INTEGRATION_SQLSERVER_DSN"),os.Getenv("DATAFLOW_INTEGRATION_CH_URL")
 if pgDSN==""||sqlDSN==""||chURL==""{
  t.Skip("requires disposable PostgreSQL, SQL Server and ClickHouse in same QA runner")
 }
 if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true"||
  os.Getenv("DATAFLOW_INTEGRATION_MSSQL_ISOLATED")!="true"||
  os.Getenv("DATAFLOW_INTEGRATION_CH_ISOLATED")!="true"{
  t.Fatal("all three disposable service attestations required")
 }
 u,err:=url.Parse(chURL)
 if err!=nil||u.Scheme!="http"||u.Hostname()!="127.0.0.1"||
  u.User!=nil||u.RawQuery!=""||u.Fragment!=""||(u.Path!=""&&u.Path!="/"){
  t.Fatal("ClickHouse MUST be disposable loopback fixture")
 }
 t.Setenv("DATAFLOW_ALLOW_TEST_LOAD","true")
 t.Setenv("DATAFLOW_ENABLE_MANAGED_TEST","true")
 t.Setenv("DATAFLOW_TEST_TARGET_DATABASE","scratch")
 maxDuration:=3*time.Minute
 if os.Getenv("DATAFLOW_BENCHMARK_7M")=="true" &&
   os.Getenv("DATAFLOW_QA_LOCAL_OWNER_VERIFIED")=="true" {
  maxDuration=35*time.Minute
 }
 ctx,cancel:=context.WithTimeout(context.Background(),maxDuration)
 pg,err:=pgxpool.New(ctx,pgDSN);if err!=nil{cancel();t.Fatal(err)}
 var pgName string
 if err=pg.QueryRow(ctx,"SELECT current_database()").Scan(&pgName);err!=nil||
  (pgName!="dataflow_test_ci" &&
   !(pgName=="dataflow_qa" &&
    os.Getenv("DATAFLOW_QA_LOCAL_OWNER_VERIFIED")=="true" &&
    os.Getenv("DATAFLOW_BENCHMARK_7M")=="true")){
  pg.Close();cancel();t.Fatal("PostgreSQL must be disposable dataflow_test_ci")
 }
 for _,name:=range []string{"001_bootstrap.sql","002_sync_checkpoint.sql",
  "003_managed_runs.sql","004_stream_tuning.sql","009_managed_queue.sql"}{
  raw,e:=os.ReadFile(filepath.Join("..","..","migrations","postgres",name))
  if e!=nil{pg.Close();cancel();t.Fatal(e)}
  if _,e=pg.Exec(ctx,string(raw));e!=nil{pg.Close();cancel();t.Fatalf("PG migration %s: %v",name,e)}
 }
 source,err:=sql.Open("sqlserver",sqlDSN);if err!=nil{pg.Close();cancel();t.Fatal(err)}
 source.SetMaxOpenConns(3)
 var sqlName,login string
 var hasSelect,hasInsert,hasUpdate,hasDelete,hasAlter sql.NullInt64
 err=source.QueryRowContext(ctx,`SELECT DB_NAME(),ORIGINAL_LOGIN(),
 HAS_PERMS_BY_NAME(N'Erp.Part',N'OBJECT',N'SELECT'),
 HAS_PERMS_BY_NAME(N'Erp.Part',N'OBJECT',N'INSERT'),
 HAS_PERMS_BY_NAME(N'Erp.Part',N'OBJECT',N'UPDATE'),
 HAS_PERMS_BY_NAME(N'Erp.Part',N'OBJECT',N'DELETE'),
 HAS_PERMS_BY_NAME(N'Erp.Part',N'OBJECT',N'ALTER')`).
 Scan(&sqlName,&login,&hasSelect,&hasInsert,&hasUpdate,&hasDelete,&hasAlter)
 if err!=nil||sqlName!="DataFlowFixtureQA"||login!="qa_erp_reader"||
  !hasSelect.Valid||hasSelect.Int64!=1||
  !hasInsert.Valid||hasInsert.Int64!=0||
  !hasUpdate.Valid||hasUpdate.Int64!=0||
  !hasDelete.Valid||hasDelete.Int64!=0||
  !hasAlter.Valid||hasAlter.Int64!=0{
  source.Close();pg.Close();cancel();t.Fatalf("refusing source without exact QA SELECT-only grants: %v",err)
 }
 var partCount int64
 if err=source.QueryRowContext(ctx,"SELECT COUNT_BIG(*) FROM [Erp].[Part]").Scan(&partCount);err!=nil||partCount!=200{
  source.Close();pg.Close();cancel();t.Fatal("unexpected SQL Server Part fixture size")
 }
 cli:=safetable.Client{Endpoint:chURL,User:os.Getenv("DATAFLOW_INTEGRATION_CH_USER"),
  Password:os.Getenv("DATAFLOW_INTEGRATION_CH_PASSWORD")}
 if err=cli.RequireAtomic(ctx);err!=nil{
  source.Close();pg.Close();cancel();t.Fatalf("scratch ClickHouse must be Atomic: %v",err)
 }
 svc:=&server{pg:pg,sql:source,chURL:chURL,chUser:cli.User,chPass:cli.Password,
  token:"isolated-core-run-qa-token",managedSlots:make(chan struct{},2)}
 if err=svc.checkManagedSchema(ctx);err!=nil{source.Close();pg.Close();cancel();t.Fatal(err)}
 cleanup:=func(){source.Close();pg.Close();cancel()}
 return ctx,svc,cleanup
}

func coreHTTP(t *testing.T,base string,method,path string,body any)(int,map[string]any){
 t.Helper()
 var reader io.Reader
 if body!=nil{
  raw,e:=json.Marshal(body);if e!=nil{t.Fatal(e)}
  reader=bytes.NewReader(raw)
 }
 req,e:=http.NewRequest(method,base+path,reader);if e!=nil{t.Fatal(e)}
 req.Header.Set("Authorization","Bearer isolated-core-run-qa-token")
 req.Header.Set("Content-Type","application/json")
 res,e:=(&http.Client{Timeout:15*time.Second}).Do(req);if e!=nil{t.Fatal(e)}
 defer res.Body.Close()
 out:=map[string]any{}
 if e=json.NewDecoder(res.Body).Decode(&out);e!=nil{t.Fatalf("HTTP %s %s failed %d: %v",method,path,res.StatusCode,e)}
 return res.StatusCode,out
}

func coreRawFixture(t *testing.T,ctx context.Context,svc *server,streamID string){
 t.Helper()
 cl:=safetable.Client{Endpoint:svc.chURL,User:svc.chUser,Password:svc.chPass}
 const ddl=`CREATE TABLE IF NOT EXISTS scratch.dataflow_raw_events (
 _stream_id String,_pk String,_sysrevid String,_payload String,
 _run_id String,_batch_seq UInt64,_ingested_at DateTime64(3,'UTC') DEFAULT now64(3))
 ENGINE=MergeTree ORDER BY (_stream_id,_pk,_sysrevid)`
 if _,e:=cl.Query(ctx,ddl);e!=nil{t.Fatal(e)}
 t.Cleanup(func(){
  cleanCtx,stop:=context.WithTimeout(context.Background(),20*time.Second);defer stop()
  _,_=cl.Query(cleanCtx,"DROP TABLE IF EXISTS scratch.dataflow_raw_events")
 })
 // This table exists only in a freshly started, isolated, loopback CI ClickHouse.
 // Do not drop unrelated scratch tables and do not truncate production data.
}

func TestCoreRun1RealHTTPManagedPipelineIntegration(t *testing.T){
 ctx,svc,cleanup:=coreRealFixture(t);defer cleanup()
 api:=http.NewServeMux()
 api.HandleFunc("/api/v1/managed/streams",svc.streamAPI)
 api.HandleFunc("/api/v1/managed/runs",svc.managedRunAPI)
 httpSrv:=httptest.NewServer(api);defer httpSrv.Close()

 status,configured:=coreHTTP(t,httpSrv.URL,"POST","/api/v1/managed/streams",
  map[string]any{"source_schema":"Erp","source_table":"Part",
   "target_database":"scratch","target_table":"dataflow_raw_events",
   "batch_size":100,"workers":2,"queue_capacity":2})
 if status!=201{t.Fatalf("API create stream returned %d: %+v",status,configured)}
 streamID,ok:=configured["id"].(string);if !ok||!validUUID.MatchString(streamID){
  t.Fatalf("invalid persisted stream ID: %+v",configured)
 }
 coreRawFixture(t,ctx,svc,streamID)
 defer func(){
  clean,done:=context.WithTimeout(context.Background(),10*time.Second);defer done()
  _,_=svc.pg.Exec(clean,"DELETE FROM managed_runs WHERE stream_id=$1::uuid",streamID)
  _,_=svc.pg.Exec(clean,"DELETE FROM sync_checkpoints WHERE stream_id=$1::uuid",streamID)
  _,_=svc.pg.Exec(clean,"DELETE FROM sync_streams WHERE id=$1::uuid",streamID)
 }()
 enqueue:=func()string{
  t.Helper()
  code,response:=coreHTTP(t,httpSrv.URL,"POST","/api/v1/managed/runs",
   map[string]any{"stream_id":streamID})
  runID,_:=response["run_id"].(string)
  if code!=202||!validUUID.MatchString(runID)||response["status"]!="queued"{
   t.Fatalf("durable API enqueue failed code=%d body=%+v",code,response)
  }
  return runID
 }
 await:=func(id string,wantRows float64){
  t.Helper()
  deadline:=time.Now().Add(50*time.Second)
  for time.Now().Before(deadline){
   _,body:=coreHTTP(t,httpSrv.URL,"GET","/api/v1/managed/runs?id="+id,nil)
   if body["status"]=="succeeded"{
    if body["rows_written"]!=wantRows||body["dispatch_attempts"]!=float64(1){
     t.Fatalf("incorrect persisted result: %+v",body)
    }
    if body["upper_exclusive"]==""||body["lease_expires_at"]!=nil{
     t.Fatalf("checkpoint/lease not committed: %+v",body)
    }
    return
   }
   if body["status"]=="failed"{t.Fatalf("run failed: %+v",body)}
   time.Sleep(150*time.Millisecond)
  }
  t.Fatalf("timed out waiting for actual API run %s",id)
 }
 first:=enqueue()
 // Real API calls enqueue durably; second concurrent run MUST be denied.
 req,_:=json.Marshal(map[string]string{"stream_id":streamID})
 denied:=httptest.NewRecorder()
 denyReq:=httptest.NewRequest("POST","/api/v1/managed/runs",bytes.NewReader(req))
 denyReq.Header.Set("Authorization","Bearer isolated-core-run-qa-token")
 svc.managedRunAPI(denied,denyReq)
 if denied.Code!=409{t.Fatalf("duplicate in-flight run accepted: %d",denied.Code)}
 // Construct NEW worker object, preserving only PostgreSQL / DB connections.
 // This proves the queue does not depend on HTTP handler's Go memory.
 worker:=&server{pg:svc.pg,sql:svc.sql,chURL:svc.chURL,
  chUser:svc.chUser,chPass:svc.chPass,managedSlots:make(chan struct{},2)}
 worker.dispatchManaged(ctx)
 await(first,200)
 cl:=safetable.Client{Endpoint:svc.chURL,User:svc.chUser,Password:svc.chPass}
 rows,e:=cl.Query(ctx,fmt.Sprintf(`SELECT count(),uniqExact(_pk),uniqExact(_sysrevid)
 FROM scratch.dataflow_raw_events WHERE _stream_id='%s' FORMAT TabSeparatedRaw`,streamID))
 if e!=nil||!strings.HasPrefix(rows,"200\t200\t200"){
  t.Fatalf("real destination lacks 200 distinct source rows: %s, err=%v",rows,e)
 }
 var firstGen int64
 var firstCursor []byte
 e=svc.pg.QueryRow(ctx,"SELECT generation,lower_cursor FROM sync_checkpoints WHERE stream_id=$1::uuid",streamID).Scan(&firstGen,&firstCursor)
 if e!=nil||firstGen!=1||len(firstCursor)!=8||bytes.Equal(firstCursor,make([]byte,8)){
  t.Fatalf("checkpoint was not committed: gen=%d bytes=%x err=%v",firstGen,firstCursor,e)
 }
 // A second operator POST must read a real incremental empty window, not
 // re-insert all source rows after a successful committed checkpoint.
 second:=enqueue()
 worker.dispatchManaged(ctx)
 await(second,0)
 var secondGen int64
 var secondCursor []byte
 e=svc.pg.QueryRow(ctx,"SELECT generation,lower_cursor FROM sync_checkpoints WHERE stream_id=$1::uuid",streamID).Scan(&secondGen,&secondCursor)
 if e!=nil||secondGen!=2||!bytes.Equal(firstCursor,secondCursor){
  t.Fatalf("no-op incremental cursor did not remain stable: gen=%d err=%v",secondGen,e)
 }
 rows,e=cl.Query(ctx,fmt.Sprintf("SELECT count(),uniqExact(_pk) FROM scratch.dataflow_raw_events WHERE _stream_id='%s' FORMAT TabSeparatedRaw",streamID))
 if e!=nil||rows!="200\t200"{t.Fatalf("incremental replay inserted duplicates: %q %v",rows,e)}
 t.Logf("CORE-RUN-1 REAL PASS: HTTP create stream → API queued → new worker dispatch → SQL Server SELECT-only 200 rows → ClickHouse 200 keys → persisted cursor gen1 → no-op re-run gen2 zero new rows; status GET persisted")
}
