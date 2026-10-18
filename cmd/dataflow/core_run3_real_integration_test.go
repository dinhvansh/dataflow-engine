package main

import (
 "context"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strconv"
 "strings"
 "testing"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/dbtworker"
 "github.com/dinhvansh/dataflow-engine/internal/safetable"
 "github.com/dinhvansh/dataflow-engine/internal/transform"
)

// Genuine four-service QA: an actual authenticated HTTP core Run enqueues
// PG managed sync, then materializes a real ClickHouse dbt A->B->C DAG,
// checks real source-derived values and publishes a physical immutable
// ClickHouse target. Reconcile calls below simulate independent poll ticks,
// with a NEW server/worker object after the sync boundary.
func TestCoreRun3UnifiedRealFourServiceIntegration(t *testing.T){realCoreIntegration(t,false)}
func TestCoreRun4FaultRollbackFourServiceIntegration(t *testing.T){realCoreIntegration(t,true)}

func realCoreIntegration(t *testing.T,withFaults bool){
 if os.Getenv("DATAFLOW_INTEGRATION_DBT_BIN")==""{
  t.Skip("real dbt executable is required on SQL Server+PG+ClickHouse QA runner")
 }
 ctx,svc,cleanup:=coreRealFixture(t);defer cleanup()
 t.Setenv("DATAFLOW_ENABLE_SQL_JOBS_TEST","true")
 t.Setenv("DATAFLOW_ENABLE_DBT_TEST","true")
 t.Setenv("DATAFLOW_ENABLE_AUTO_BUILD_TEST","true")
 t.Setenv("DATAFLOW_ENABLE_SAFE_TABLE_TEST","true")
 t.Setenv("DATAFLOW_ENABLE_CORE_PIPELINE_TEST","true")
 for _,name:=range []string{
  "010_transform_models.sql","011_dbt_runs.sql",
  "013_auto_build_flows.sql","028_core_pipeline_runs.sql",
  "029_core_pipeline_rollback.sql",
 }{
  content,e:=os.ReadFile(filepath.Join("..","..","migrations","postgres",name))
  if e!=nil{t.Fatal(e)}
  if _,e=svc.pg.Exec(ctx,string(content));e!=nil{t.Fatalf("migration %s: %v",name,e)}
 }
 if err:=svc.checkCoreSchema(ctx);err!=nil{t.Fatal(err)}
 if err:=svc.checkFlowSchema(ctx);err!=nil{t.Fatal(err)}
 streamRaw:=map[string]any{"source_schema":"Erp","source_table":"PartTran",
   "target_database":"scratch","target_table":"dataflow_raw_events",
   "batch_size":500,"workers":2,"queue_capacity":2}
 mux:=http.NewServeMux()
 mux.HandleFunc("/api/v1/managed/streams",svc.streamAPI)
 mux.HandleFunc("/api/v1/core/runs",svc.coreRunsAPI)
 mux.HandleFunc("/api/v1/core/actions",svc.coreActionAPI)
 api:=httptest.NewServer(mux);defer api.Close()
 status,stream:=coreHTTP(t,api.URL,"POST","/api/v1/managed/streams",streamRaw)
 streamID,_:=stream["id"].(string)
 if status!=201||!validUUID.MatchString(streamID){t.Fatalf("stream API %d %+v",status,stream)}
 coreRawFixture(t,ctx,svc,streamID)
 cl:=safetable.Client{Endpoint:svc.chURL,User:svc.chUser,Password:svc.chPass}
 // A separate SQL-restricted ClickHouse reader is used by the existing
 // auto-build DQ gate; never silently use the API writer as its DQ identity.
 if _,e:=cl.Query(ctx,"CREATE USER IF NOT EXISTS core3_reader IDENTIFIED WITH sha256_password BY 'ci-core3-reader-password'");e!=nil{
  t.Fatalf("disposable read-only ClickHouse user: %v",e)
 }
 if _,e:=cl.Query(ctx,"GRANT SELECT ON scratch.* TO core3_reader");e!=nil{t.Fatal(e)}
 t.Setenv("DATAFLOW_CH_READONLY_USER","core3_reader")
 t.Setenv("DATAFLOW_CH_READONLY_PASSWORD","ci-core3-reader-password")
 stamp:=time.Now().UnixNano()
 pre:=fmt.Sprintf("cr%d",stamp)
 a,b,c:=pre+"a",pre+"b",pre+"c"
 for _,name:=range []string{a,b,c}{
  if len(name)>70{t.Fatal("unsafe model name")}
 }
 targetName:=safetable.TargetName("core_"+c)
 defer func(){
  cctx,done:=context.WithTimeout(context.Background(),25*time.Second);defer done()
  for _,n:=range []string{a,b,c}{_,_=cl.Query(cctx,"DROP VIEW IF EXISTS scratch.dfv_"+n)}
  // Every table here was created by this exact disposable, single-test
  // scratch run, not by production or any pre-existing customer workspace.
  _,_=cl.Query(cctx,"DROP TABLE IF EXISTS scratch."+targetName)
  _,_=cl.Query(cctx,"DROP USER IF EXISTS core3_reader")
  _,_=svc.pg.Exec(cctx,"DELETE FROM core_pipeline_runs WHERE stream_id=$1::uuid",streamID)
  _,_=svc.pg.Exec(cctx,"DELETE FROM managed_runs WHERE stream_id=$1::uuid",streamID)
  _,_=svc.pg.Exec(cctx,"DELETE FROM sync_checkpoints WHERE stream_id=$1::uuid",streamID)
  _,_=svc.pg.Exec(cctx,"DELETE FROM sync_streams WHERE id=$1::uuid",streamID)
 }()
 models:=[]transform.Model{
  {Name:a,Mode:"visual",Source:transform.Source{Database:"scratch",Table:"dataflow_raw_events"},
   Steps:[]transform.Step{{ID:"rename_stream",Type:"rename",Column:"_stream_id",Alias:"stream"}}},
  {Name:b,Mode:"visual",Source:transform.Source{Database:"scratch",Table:"dfv_"+a},
   Steps:[]transform.Step{{ID:"add_batch",Type:"calculate",Column:"_batch_seq",
    Alias:"batch_plus_one",FormulaOperator:"add",Value:"1"}}},
  {Name:c,Mode:"visual",Source:transform.Source{Database:"scratch",Table:"dfv_"+b},
   Steps:[]transform.Step{{ID:"count_stream",Type:"aggregate",GroupBy:[]string{"stream"},
    Function:"count",Alias:"events"}}},
 }
 var modelID string
 for _,model:=range models{
  compiled,e:=transform.Compile(model,"scratch");if e!=nil{t.Fatal(e)}
  raw,e:=json.Marshal(model);if e!=nil{t.Fatal(e)}
  var id string
  e=svc.pg.QueryRow(ctx,`INSERT INTO transform_models(name,definition,compiled_sql)
    VALUES($1,$2::jsonb,$3) RETURNING id::text`,
    model.Name,string(raw),compiled.SQL).Scan(&id)
  if e!=nil{t.Fatal(e)}
  if model.Name==c{modelID=id}
 }
 defer func(){
  cctx,done:=context.WithTimeout(context.Background(),10*time.Second);defer done()
  _,_=svc.pg.Exec(cctx,"DELETE FROM core_pipeline_runs WHERE stream_id=$1::uuid",streamID)
  _,_=svc.pg.Exec(cctx,"DELETE FROM model_flow_steps WHERE model_name LIKE $1",pre+"%")
  _,_=svc.pg.Exec(cctx,"DELETE FROM model_flow_runs WHERE target_model_name=$1",c)
  _,_=svc.pg.Exec(cctx,"DELETE FROM dbt_runs WHERE model_name LIKE $1",pre+"%")
  _,_=svc.pg.Exec(cctx,"DELETE FROM transform_models WHERE name LIKE $1",pre+"%")
 }()
 start,details:=coreHTTP(t,api.URL,"POST","/api/v1/core/runs",
  map[string]any{"stream_id":streamID,"model_id":modelID,"unique_column":"stream"})
 runID,_:=details["run_id"].(string)
 syncID,_:=details["managed_run_id"].(string)
 if start!=202||!validUUID.MatchString(runID)||!validUUID.MatchString(syncID)||
  details["status"]!="waiting_sync"{
  t.Fatalf("core API failed: %d %+v",start,details)
 }
 // Do not permit a second conflicting source/model core run.
 // No second run can be accepted while this core chain is active.
 var concurrentCount int
 if e:=svc.pg.QueryRow(ctx,`SELECT count(*) FROM core_pipeline_runs
   WHERE status IN ('waiting_sync','waiting_flow','publishing')`).Scan(&concurrentCount);
   e!=nil||concurrentCount!=1{
  t.Fatalf("core active-run invariant broken: %d %v",concurrentCount,e)
 }
 // Workers can restart without losing the API-created persisted run.
 svc2:=&server{pg:svc.pg,sql:svc.sql,chURL:svc.chURL,chUser:svc.chUser,
  chPass:svc.chPass,token:svc.token,managedSlots:make(chan struct{},2)}
 claim,e:=svc2.claimManagedRun(ctx,syncID);if e!=nil{t.Fatal(e)}
 svc2.runClaimedManaged(ctx,claim)
 var syncRows int64
 if e=svc.pg.QueryRow(ctx,"SELECT rows_written FROM managed_runs WHERE id=$1::uuid AND status='succeeded'",
  syncID).Scan(&syncRows);e!=nil||syncRows!=10000{
  t.Fatalf("real SQL→CH managed step not proven: %d %v",syncRows,e)
 }
 if e=svc2.reconcileOneCore(ctx);e!=nil{t.Fatalf("core sync→dbt transition: %v",e)}
 var flowID string
 if e=svc.pg.QueryRow(ctx,`SELECT flow_id::text FROM core_pipeline_runs WHERE id=$1::uuid`,runID).
  Scan(&flowID);e!=nil||!validUUID.MatchString(flowID){t.Fatalf("flow not durable: %v",e)}
 port,e:=strconv.Atoi(strings.TrimPrefix(strings.Split(svc.chURL,":")[2],"/"));if e!=nil{t.Fatal(e)}
 conf:=dbtworker.Config{Binary:os.Getenv("DATAFLOW_INTEGRATION_DBT_BIN"),Host:"127.0.0.1",
  Port:port,Database:"scratch",Username:svc.chUser,Password:svc.chPass}
 for _,name:=range []string{a,b,c}{
  if e=svc2.reconcileOneFlow(ctx);e!=nil{t.Fatalf("dbt dispatch %s: %v",name,e)}
  var dbtID,dbtSQL,dbtTarget string
  e=svc.pg.QueryRow(ctx,`SELECT d.id::text,d.compiled_sql,d.target_relation
   FROM dbt_runs d JOIN model_flow_steps st ON st.dbt_run_id=d.id
   WHERE st.flow_id=$1::uuid AND st.model_name=$2`,
    flowID,name).Scan(&dbtID,&dbtSQL,&dbtTarget)
  if e!=nil{t.Fatalf("durable dbt step %s absent: %v",name,e)}
  result,runErr:=dbtworker.Execute(ctx,dbtworker.Plan{
   Name:name,SQL:dbtSQL,Target:dbtTarget,
  },conf)
  if runErr!=nil{t.Fatalf("REAL dbt %s: %v, log=%s",name,runErr,result.Log)}
  if _,e=svc.pg.Exec(ctx,`UPDATE dbt_runs SET status='succeeded',
   run_results=$2::jsonb,finished_at=clock_timestamp() WHERE id=$1::uuid`,
   dbtID,string(result.Results));e!=nil{t.Fatal(e)}
  if e=svc2.reconcileOneFlow(ctx);e!=nil{t.Fatalf("DQ gate %s: %v",name,e)}
 }
 var flowStatus string;var steps int
 e=svc.pg.QueryRow(ctx,`SELECT status,current_step FROM model_flow_runs
   WHERE id=$1::uuid`,flowID).Scan(&flowStatus,&steps)
 if e!=nil||flowStatus!="succeeded"||steps!=3{
  t.Fatalf("A→B→C not completed: %s steps=%d err=%v",flowStatus,steps,e)
 }
 if e=svc2.reconcileOneCore(ctx);e!=nil{t.Fatal(e)} // waiting_flow -> publishing
 if e=svc2.reconcileOneCore(ctx);e!=nil{t.Fatal(e)} // stage and persist UUID
 if e=svc2.reconcileOneCore(ctx);e!=nil{t.Fatal(e)} // actual publish by UUID
 _,finished:=coreHTTP(t,api.URL,"GET","/api/v1/core/runs?id="+runID,nil)
 if finished["status"]!="succeeded"||finished["rows_verified"]!=float64(1)||
  finished["managed_status"]!="succeeded"||finished["model_flow_status"]!="succeeded"{
  t.Fatalf("unified final product state not successful: %+v",finished)
 }
 actual,e:=cl.Query(ctx,`SELECT stream,events FROM scratch.`+targetName+` FORMAT TabSeparatedRaw`)
 if e!=nil||actual!=fmt.Sprintf("%s\t10000",streamID){
  t.Fatalf("real SQL-source-derived published result mismatch: %q err=%v",actual,e)
 }
 var staged string
 if e=svc.pg.QueryRow(ctx,`SELECT stage_name FROM core_pipeline_runs
    WHERE id=$1::uuid AND status='succeeded'`,runID).Scan(&staged);e!=nil{t.Fatal(e)}
 // Reconcile after success must not republish or mutate immutable target UUID.
 before,e:=cl.UUID(ctx,targetName);if e!=nil||before==""{t.Fatal(e)}
 if e=svc2.reconcileOneCore(ctx);e!=nil{
  // No other pending core run exists. pgx.ErrNoRows is the correct
  // replay-idempotent no-op result.
  if !strings.Contains(e.Error(),"no rows"){t.Fatal(e)}
 }
 after,e:=cl.UUID(ctx,targetName)
 if e!=nil||after!=before{t.Fatal("repeated core poll published a new table")}
 t.Logf("CORE-RUN-3 REAL PASS: authenticated operator core POST → PG managed 10000 SQL Server rows → ClickHouse raw → PG dbt A→B→C (real dbt) → DQ 3 steps → UUID stage → atomic publish → GET succeeded / 10000 events; immutable target unchanged on reconcile")
 if !withFaults{return}
 // CORE-RUN-4: a second real operator Run produces a different physical
 // table version. After the external EXCHANGE has succeeded we intentionally
 // abandon the PG terminal transition and restart the coordinator.
 code,secondResponse:=coreHTTP(t,api.URL,"POST","/api/v1/core/runs",
  map[string]any{"stream_id":streamID,"model_id":modelID,"unique_column":"stream"})
 secondRun,_:=secondResponse["run_id"].(string)
 secondManaged,_:=secondResponse["managed_run_id"].(string)
 if code!=202||!validUUID.MatchString(secondRun)||!validUUID.MatchString(secondManaged){
  t.Fatalf("second core run not queued: %d %+v",code,secondResponse)
 }
 nextClaim,e:=svc2.claimManagedRun(ctx,secondManaged);if e!=nil{t.Fatal(e)}
 svc2.runClaimedManaged(ctx,nextClaim)
 var zeroRows int64
 if e=svc.pg.QueryRow(ctx,`SELECT rows_written FROM managed_runs
   WHERE id=$1::uuid AND status='succeeded'`,secondManaged).Scan(&zeroRows);
   e!=nil||zeroRows!=0{
  t.Fatalf("incremental second run should transfer zero new source rows: %d %v",zeroRows,e)
 }
 if e=svc2.reconcileOneCore(ctx);e!=nil{t.Fatal(e)}
 var flowTwo string
 if e=svc.pg.QueryRow(ctx,`SELECT flow_id::text FROM core_pipeline_runs
  WHERE id=$1::uuid`,secondRun).Scan(&flowTwo);e!=nil{t.Fatal(e)}
 for _,name:=range []string{a,b,c}{
  if e=svc2.reconcileOneFlow(ctx);e!=nil{t.Fatal(e)}
  var child,sqlText,target string
  e=svc.pg.QueryRow(ctx,`SELECT r.id::text,r.compiled_sql,r.target_relation
   FROM dbt_runs r JOIN model_flow_steps st ON r.id=st.dbt_run_id
   WHERE st.flow_id=$1::uuid AND st.model_name=$2`,
   flowTwo,name).Scan(&child,&sqlText,&target)
  if e!=nil{t.Fatal(e)}
  result,runErr:=dbtworker.Execute(ctx,dbtworker.Plan{
   Name:name,SQL:sqlText,Target:target,
  },conf)
  if runErr!=nil{t.Fatalf("CORE4 REAL second dbt %s: %v (%s)",name,runErr,result.Log)}
  if _,e=svc.pg.Exec(ctx,`UPDATE dbt_runs SET status='succeeded',
   run_results=$2::jsonb,finished_at=clock_timestamp() WHERE id=$1::uuid`,
   child,string(result.Results));e!=nil{t.Fatal(e)}
  if e=svc2.reconcileOneFlow(ctx);e!=nil{t.Fatalf("second flow DQ %s: %v",name,e)}
 }
 if e=svc2.reconcileOneCore(ctx);e!=nil{t.Fatal(e)} // flow -> publishing
 if e=svc2.reconcileOneCore(ctx);e!=nil{t.Fatal(e)} // durable UUID journal
 var stageTwo,stageUUID,oldUUID,statusBefore string
 e=svc.pg.QueryRow(ctx,`SELECT stage_name,stage_uuid,old_uuid,status
   FROM core_pipeline_runs WHERE id=$1::uuid`,secondRun).
  Scan(&stageTwo,&stageUUID,&oldUUID,&statusBefore)
 if e!=nil||statusBefore!="publishing"||oldUUID!=before||stageUUID==""{
  t.Fatalf("unsafe pre-publication journal: %s %s %s %s error=%v",
   stageTwo,stageUUID,oldUUID,statusBefore,e)
 }
 publication:=safetable.Publication{Target:targetName,Stage:stageTwo,
  StageUUID:stageUUID,OldUUID:oldUUID}
 if e=cl.Publish(ctx,publication);e!=nil{t.Fatalf("real uncertain external publish: %v",e)}
 publishedUUID,e:=cl.UUID(ctx,targetName)
 if e!=nil||publishedUUID!=stageUUID{t.Fatal("external publish not actually committed")}
 // A brand NEW worker object sees PG still at 'publishing' and safely
 // replays the already completed ClickHouse EXCHANGE without double-swap.
 recoveredCore:=&server{pg:svc.pg,sql:svc.sql,chURL:svc.chURL,chUser:svc.chUser,
  chPass:svc.chPass,token:svc.token}
 if e=recoveredCore.reconcileOneCore(ctx);e!=nil{
  t.Fatalf("publication crash-window replay failed: %v",e)
 }
 _,state:=coreHTTP(t,api.URL,"GET","/api/v1/core/runs?id="+secondRun,nil)
 if state["status"]!="succeeded"||state["rows_verified"]!=float64(1){
  t.Fatalf("replayed publication not acknowledged: %+v",state)
 }
 afterReplay,e:=cl.UUID(ctx,targetName)
 if e!=nil||afterReplay!=publishedUUID{t.Fatal("unsafe double-exchange after crash recovery")}
 // Explicit rollback requires an existing physical backup and operator
 // confirmation; only test ClickHouse scratch is modified, never source SQL.
 actionCode,rolled:=coreHTTP(t,api.URL,"POST","/api/v1/core/actions",
  map[string]string{"run_id":secondRun,"action":"rollback","confirm":"ROLLBACK"})
 if actionCode!=200||rolled["status"]!="rolled_back"{
  t.Fatalf("operator rollback failed: %d %+v",actionCode,rolled)
 }
 rolledUUID,e:=cl.UUID(ctx,targetName)
 if e!=nil||rolledUUID!=before{t.Fatal("rollback did not restore precise prior physical table")}
 actionCode,rolled=coreHTTP(t,api.URL,"POST","/api/v1/core/actions",
  map[string]string{"run_id":secondRun,"action":"rollback","confirm":"ROLLBACK"})
 if actionCode!=200||rolled["status"]!="rolled_back"{
  t.Fatalf("idempotent rollback replay failed: %d %+v",actionCode,rolled)
 }
 rolledUUID,e=cl.UUID(ctx,targetName)
 if e!=nil||rolledUUID!=before{t.Fatal("repeated rollback double-swapped target")}
 // Negative test: mutate only saved PostgreSQL model metadata after a
 // third API enqueue. The source SQL is still SELECT-only and a complete
 // managed sync does not permit stale model execution or new publication.
 failCode,failedArgs:=coreHTTP(t,api.URL,"POST","/api/v1/core/runs",
  map[string]any{"stream_id":streamID,"model_id":modelID,"unique_column":"stream"})
 failedID,_:=failedArgs["run_id"].(string)
 childID,_:=failedArgs["managed_run_id"].(string)
 if failCode!=202||!validUUID.MatchString(failedID){t.Fatal("third core run rejected")}
 claimThree,e:=svc2.claimManagedRun(ctx,childID);if e!=nil{t.Fatal(e)}
 svc2.runClaimedManaged(ctx,claimThree)
 var compiledOriginal string
 if e=svc.pg.QueryRow(ctx,"SELECT compiled_sql FROM transform_models WHERE name=$1",b).
  Scan(&compiledOriginal);e!=nil{t.Fatal(e)}
 if _,e=svc.pg.Exec(ctx,`UPDATE transform_models SET compiled_sql='SELECT 123 AS tampered'
  WHERE name=$1`,b);e!=nil{t.Fatal(e)}
 if e=svc2.reconcileOneCore(ctx);e!=nil{t.Fatal(e)}
 _,failure:=coreHTTP(t,api.URL,"GET","/api/v1/core/runs?id="+failedID,nil)
 if failure["status"]!="failed"||failure["model_flow_id"]!=""{
  t.Fatalf("model drift permitted a new dbt chain: %+v",failure)
 }
 if _,e=svc.pg.Exec(ctx,`UPDATE transform_models SET compiled_sql=$2 WHERE name=$1`,
  b,compiledOriginal);e!=nil{t.Fatal(e)}
 finalUUID,e:=cl.UUID(ctx,targetName)
 if e!=nil||finalUUID!=before{t.Fatal("failed model drift modified published result")}
 _,_=cl.Query(ctx,"DROP TABLE IF EXISTS scratch."+stageTwo)
 var checkpointGen int64
 if e=svc.pg.QueryRow(ctx,`SELECT generation FROM sync_checkpoints WHERE stream_id=$1::uuid`,
   streamID).Scan(&checkpointGen);e!=nil||checkpointGen!=3{
  t.Fatalf("rollback altered incremental cursor: generation=%d, err=%v",checkpointGen,e)
 }
 t.Logf("CORE-RUN-4 REAL PASS: 10000-row repeat real dbt E2E → external ClickHouse publish crash window → new worker replay preserves UUID → explicit rollback restores old UUID → rollback retry idempotent → model SQL drift blocks new flow/publish → 3 committed SQL-only checkpoints preserved")
}
}
