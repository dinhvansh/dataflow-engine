package main

import(
 "bytes"
 "context"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/dbtworker"
 "github.com/dinhvansh/dataflow-engine/internal/safetable"
 "github.com/dinhvansh/dataflow-engine/internal/transform"
 "github.com/jackc/pgx/v5/pgxpool"
)
func TestAutoBuildRealDBTIntegration(t *testing.T){
 dsn:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
 clickhouseURL:=os.Getenv("DATAFLOW_INTEGRATION_CH_URL")
 dbtBin:=os.Getenv("DATAFLOW_INTEGRATION_DBT_BIN")
 if dsn==""||clickhouseURL==""||dbtBin==""{t.Skip("needs isolated PostgreSQL, ClickHouse and dbt")}
 if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true"{t.Fatal("non-isolated PostgreSQL is forbidden")}
 ctx,done:=context.WithTimeout(context.Background(),4*time.Minute);defer done()
 pg,err:=pgxpool.New(ctx,dsn);if err!=nil{t.Fatal(err)};defer pg.Close()
 for _,name:=range []string{"010_transform_models.sql","011_dbt_runs.sql","013_auto_build_flows.sql"}{
  content,e:=os.ReadFile(filepath.Join("..","..","migrations","postgres",name))
  if e!=nil{t.Fatal(e)}
  if _,e=pg.Exec(ctx,string(content));e!=nil{t.Fatalf("migration %s: %v",name,e)}
 }
 t.Setenv("DATAFLOW_ALLOW_TEST_LOAD","true")
 t.Setenv("DATAFLOW_ENABLE_SQL_JOBS_TEST","true")
 t.Setenv("DATAFLOW_ENABLE_DBT_TEST","true")
 t.Setenv("DATAFLOW_ENABLE_AUTO_BUILD_TEST","true")
 t.Setenv("DATAFLOW_TEST_TARGET_DATABASE","scratch")
 t.Setenv("DATAFLOW_CH_READONLY_USER","dataflow_reader")
 t.Setenv("DATAFLOW_CH_READONLY_PASSWORD","ci-read-password")
 svc:=&server{pg:pg,chURL:clickhouseURL,token:"ci-flow-token"}
 if err=svc.checkFlowSchema(ctx);err!=nil{t.Fatal(err)}
 stamp:=time.Now().UnixNano()
 prefix:=fmt.Sprintf("flowci%d",stamp)
 source:=prefix+"raw"
 a,b,c:=prefix+"a",prefix+"b",prefix+"c"
 writer:=safetable.Client{Endpoint:clickhouseURL,
  User:os.Getenv("DATAFLOW_INTEGRATION_CH_WRITER_USER"),Password:os.Getenv("DATAFLOW_INTEGRATION_CH_WRITER_PASSWORD")}
 if _,err=writer.Query(ctx,fmt.Sprintf("CREATE TABLE scratch.%s (Customer String, Revenue UInt32) ENGINE=MergeTree ORDER BY Customer",source));err!=nil{t.Fatal(err)}
 defer func(){
  cleanup,stop:=context.WithTimeout(context.Background(),20*time.Second);defer stop()
  for _,name:=range []string{a,b,c}{_,_=writer.Query(cleanup,"DROP VIEW IF EXISTS scratch.dfv_"+name)}
  _,_=writer.Query(cleanup,"DROP TABLE IF EXISTS scratch."+source)
 }()
 if _,err=writer.Query(ctx,"INSERT INTO scratch."+source+" VALUES ('A',10),('B',20)");err!=nil{t.Fatal(err)}
 modelIDs:=map[string]string{}
 add:=func(model transform.Model){
  t.Helper()
  snapshot,e:=transform.Compile(model,"scratch");if e!=nil{t.Fatal(e)}
  data,_:=json.Marshal(model)
  var id string
  if e=pg.QueryRow(ctx,`INSERT INTO transform_models(name,definition,compiled_sql)
    VALUES($1,$2::jsonb,$3) RETURNING id::text`,model.Name,string(data),snapshot.SQL).Scan(&id);e!=nil{t.Fatal(e)}
  modelIDs[model.Name]=id
 }
 add(transform.Model{Name:a,Mode:"visual",Source:transform.Source{Database:"scratch",Table:source}})
 add(transform.Model{Name:b,Mode:"visual",Source:transform.Source{Database:"scratch",Table:"dfv_"+a},
  Steps:[]transform.Step{{ID:"calc",Type:"calculate",Column:"Revenue",Alias:"Adjusted",FormulaOperator:"multiply",Value:"2"}}})
 add(transform.Model{Name:c,Mode:"visual",Source:transform.Source{Database:"scratch",Table:"dfv_"+b},
  Steps:[]transform.Step{{ID:"aggregate",Type:"aggregate",GroupBy:[]string{"Customer"},Function:"sum",Column:"Adjusted",Alias:"Total"}}})
 call:=func(id string)*httptest.ResponseRecorder{
  raw,_:=json.Marshal(map[string]string{"model_id":id})
  r:=httptest.NewRequest(http.MethodPost,"/api/v1/transform/auto-build",bytes.NewReader(raw))
  r.Header.Set("Authorization","Bearer ci-flow-token")
  result:=httptest.NewRecorder();svc.autoBuildAPI(result,r);return result
 }
 queued:=call(modelIDs[c])
 if queued.Code!=202{t.Fatalf("auto build enqueue %d %s",queued.Code,queued.Body.String())}
 var result struct{RunID string `json:"run_id"`;Order []string `json:"order"`}
 if err=json.Unmarshal(queued.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
 if len(result.Order)!=3||result.Order[0]!=a||result.Order[1]!=b||result.Order[2]!=c{t.Fatal(result.Order)}
 if r:=call(modelIDs[b]);r.Code!=409{t.Fatalf("concurrent flow permitted: %d",r.Code)}
 config:=dbtworker.Config{Binary:dbtBin,Host:"127.0.0.1",Port:8123,Database:"scratch",
  Username:writer.User,Password:writer.Password}
 for _,name:=range []string{a,b,c}{
  if err=svc.reconcileOneFlow(ctx);err!=nil{t.Fatal(err)}
  var childID,childName,childSQL,childTarget string
  err=pg.QueryRow(ctx,`SELECT d.id::text,d.model_name,d.compiled_sql,d.target_relation FROM dbt_runs d
   JOIN model_flow_steps f ON d.id=f.dbt_run_id
   WHERE f.flow_id=$1::uuid AND f.model_name=$2`,result.RunID,name).
   Scan(&childID,&childName,&childSQL,&childTarget)
  if err!=nil{t.Fatalf("dbt job for %s not queued: %v",name,err)}
  out,e:=dbtworker.Execute(ctx,dbtworker.Plan{Name:childName,SQL:childSQL,Target:childTarget},config)
  if e!=nil{t.Fatalf("real dbt step %s: %v (%s)",name,e,out.Log)}
  if _,err=pg.Exec(ctx,`UPDATE dbt_runs SET status='succeeded',
   run_results=$2::jsonb,finished_at=clock_timestamp() WHERE id=$1::uuid`,
   childID,string(out.Results));err!=nil{t.Fatal(err)}
  if err=svc.reconcileOneFlow(ctx);err!=nil{t.Fatalf("DQ failed for %s: %v",name,err)}
 }
 var finalStatus string;var completed int
 if err=pg.QueryRow(ctx,`SELECT status,current_step FROM model_flow_runs WHERE id=$1::uuid`,
  result.RunID).Scan(&finalStatus,&completed);err!=nil{t.Fatal(err)}
 if finalStatus!="succeeded"||completed!=3{t.Fatalf("flow result=%s step=%d",finalStatus,completed)}
 output,e:=writer.Query(ctx,"SELECT Customer,Total FROM scratch.dfv_"+c+" ORDER BY Customer FORMAT TabSeparatedRaw")
 if e!=nil||!strings.Contains(output,"A\t20")||!strings.Contains(output,"B\t40"){t.Fatalf("wrong final result %s %v",output,e)}
 var successful int
 if err=pg.QueryRow(ctx,`SELECT count(*) FROM model_flow_steps
  WHERE flow_id=$1::uuid AND status='succeeded' AND rows_verified=2`,result.RunID).Scan(&successful);err!=nil||successful!=3{t.Fatalf("DQ check status %d %v",successful,err)}
 t.Log("PASS real dbt A → B → C, View DQ gate, durable status and output")

 // A new flow must stop before enqueueing B if A fails.
 stopFlow:=call(modelIDs[b])
 if stopFlow.Code!=202{t.Fatalf("failure flow enqueue %d %s",stopFlow.Code,stopFlow.Body.String())}
 var failure struct{RunID string `json:"run_id"`}
 if err=json.Unmarshal(stopFlow.Body.Bytes(),&failure);err!=nil{t.Fatal(err)}
 if err=svc.reconcileOneFlow(ctx);err!=nil{t.Fatal(err)}
 var firstRun string
 if err=pg.QueryRow(ctx,`SELECT dbt_run_id::text FROM model_flow_steps
  WHERE flow_id=$1::uuid AND step_index=0`,failure.RunID).Scan(&firstRun);err!=nil{t.Fatal(err)}
 if _,err=pg.Exec(ctx,`UPDATE dbt_runs SET status='failed',error_message='deliberate source error' WHERE id=$1::uuid`,firstRun);err!=nil{t.Fatal(err)}
 if err=svc.reconcileOneFlow(ctx);err!=nil{t.Fatal(err)}
 var state string;var nextRuns int
 if err=pg.QueryRow(ctx,`SELECT status FROM model_flow_runs WHERE id=$1::uuid`,failure.RunID).Scan(&state);err!=nil||state!="failed"{t.Fatalf("failed upstream did not stop flow: %s %v",state,err)}
 if err=pg.QueryRow(ctx,`SELECT count(*) FROM model_flow_steps
   WHERE flow_id=$1::uuid AND step_index>0 AND dbt_run_id IS NOT NULL`,failure.RunID).Scan(&nextRuns);err!=nil||nextRuns!=0{t.Fatalf("downstream ran despite upstream failure %d %v",nextRuns,err)}
 t.Log("PASS failure stops downstream")

 // Modifying model code after enqueue must block the flow before its first dbt run.
 stale:=call(modelIDs[a])
 if stale.Code!=202{t.Fatalf("stale test enqueue %d %s",stale.Code,stale.Body.String())}
 if _,err=pg.Exec(ctx,"UPDATE transform_models SET compiled_sql='SELECT 123' WHERE id=$1::uuid",modelIDs[a]);err!=nil{t.Fatal(err)}
 if err=svc.reconcileOneFlow(ctx);err!=nil{t.Fatal(err)}
 var staleRun struct{RunID string `json:"run_id"`}
 _=json.Unmarshal(stale.Body.Bytes(),&staleRun)
 if err=pg.QueryRow(ctx,"SELECT status FROM model_flow_runs WHERE id=$1::uuid",staleRun.RunID).Scan(&state);err!=nil||state!="failed"{t.Fatalf("changed snapshot not rejected %s %v",state,err)}
 t.Log("PASS stale model SQL rejects auto build")
}
