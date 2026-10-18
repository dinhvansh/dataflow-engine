package main

import (
 "context"
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "runtime"
 "testing"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/connectors/clickhouse"
)

// This deliberately does not run on GitHub hosted CI. An owner-verified
// local, labeled disposable SQL Server fixture seeded with 7,000,000 rows
// and adequate disk/RAM is mandatory. A skipped test NEVER counts as a
// passing 7m benchmark or as a production performance certificate.
func TestCoreRun7mRealManagedBenchmarkIntegration(t *testing.T){
 if os.Getenv("DATAFLOW_BENCHMARK_7M")!="true"||
  os.Getenv("DATAFLOW_QA_LOCAL_OWNER_VERIFIED")!="true"||
  os.Getenv("DATAFLOW_BENCHMARK_QA_CONFIRM")!="RUN_SEVEN_MILLION_DISPOSABLE_ROWS"{
  t.Skip("explicit owner-verified local 7m disposable SQL benchmark only")
 }
 ctx,svc,cleanup:=coreRealFixture(t);defer cleanup()
 var count int64
 err:=svc.sql.QueryRowContext(ctx,`SELECT COUNT_BIG(*) FROM [Erp].[PartTranScale7M]`).Scan(&count)
 if err!=nil||count!=7000000{
  t.Fatalf("refusing unseeded 7m fixture: count=%d err=%v",count,err)
 }
 api:=http.NewServeMux()
 api.HandleFunc("/api/v1/managed/streams",svc.streamAPI)
 api.HandleFunc("/api/v1/managed/runs",svc.managedRunAPI)
 app:=httptest.NewServer(api);defer app.Close()
 status,response:=coreHTTP(t,app.URL,"POST","/api/v1/managed/streams",
  map[string]any{
   "source_schema":"Erp","source_table":"PartTranScale7M",
   "target_database":"scratch","target_table":"dataflow_raw_events",
   "batch_size":10000,"workers":2,"queue_capacity":2,
  })
 id,_:=response["id"].(string)
 if status!=201||!validUUID.MatchString(id){t.Fatalf("7m stream refused: %d %+v",status,response)}
 coreRawFixture(t,ctx,svc,id)
 defer func(){
  c,done:=context.WithTimeout(context.Background(),20*time.Second);defer done()
  _,_=svc.pg.Exec(c,"DELETE FROM managed_runs WHERE stream_id=$1::uuid",id)
  _,_=svc.pg.Exec(c,"DELETE FROM sync_checkpoints WHERE stream_id=$1::uuid",id)
  _,_=svc.pg.Exec(c,"DELETE FROM sync_streams WHERE id=$1::uuid",id)
 }()
 code,queued:=coreHTTP(t,app.URL,"POST","/api/v1/managed/runs",
  map[string]any{"stream_id":id})
 runID,_:=queued["run_id"].(string)
 if code!=202||!validUUID.MatchString(runID){t.Fatalf("7m queue refused: %+v",queued)}
 worker:=&serverWorkerHolder{
  svc:&server{pg:svc.pg,sql:svc.sql,chURL:svc.chURL,chUser:svc.chUser,
   chPass:svc.chPass,managedSlots:make(chan struct{},2)},
 }
 attempt,e:=worker.svc.claimManagedRun(ctx,runID);if e!=nil{t.Fatal(e)}
 begin:=time.Now()
 worker.svc.runClaimedManaged(ctx,attempt)
 elapsed:=time.Since(begin)
 _,result:=coreHTTP(t,app.URL,"GET","/api/v1/managed/runs?id="+runID,nil)
 if result["status"]!="succeeded"||result["rows_written"]!=float64(7000000){
  t.Fatalf("7m actual QA sync not complete: %+v",result)
 }
 rawDest:=&clickhouse.Writer{Endpoint:svc.chURL,Username:svc.chUser,
  Password:svc.chPass,Database:"scratch",Table:"dataflow_raw_events"}
 persisted,e:=rawDest.CountRunRows(ctx,attempt.OwnerID)
 if e!=nil||persisted!=7000000{t.Fatalf("7m physical destination mismatch: %d %v",persisted,e)}
 var gen int64
 if e=svc.pg.QueryRow(ctx,"SELECT generation FROM sync_checkpoints WHERE stream_id=$1::uuid",
  id).Scan(&gen);e!=nil||gen!=1{t.Fatalf("7m checkpoint not committed: %d %v",gen,e)}
 var memory runtime.MemStats;runtime.ReadMemStats(&memory)
 rps:=float64(persisted)/elapsed.Seconds()
 t.Logf("QA_REAL_7M_BENCH_PASS source=SQLServer2022_readonly destination=ClickHouse25.3 checkpoint=PG16 rows=%d batches=%v elapsed_seconds=%.3f rows_per_second=%.2f go_heap_alloc_bytes=%d",
  persisted,result["batches_written"],elapsed.Seconds(),rps,memory.Alloc)
 t.Log(fmt.Sprintf("Benchmark is not production-certified; save runner hardware and Docker peak RAM/CPU evidence with this log"))
}
type serverWorkerHolder struct{svc *server}
