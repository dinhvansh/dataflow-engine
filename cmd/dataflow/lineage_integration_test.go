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
 "github.com/dinhvansh/dataflow-engine/internal/transform"
 "github.com/jackc/pgx/v5/pgxpool"
)
func TestLineageAPIIntegration(t *testing.T){
 dsn:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
 if dsn==""{t.Skip("requires isolated PostgreSQL")}
 if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true"{t.Fatal("refusing non-isolated test")}
 ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second);defer cancel()
 pool,err:=pgxpool.New(ctx,dsn);if err!=nil{t.Fatal(err)};defer pool.Close()
 for _,file:=range []string{"010_transform_models.sql","011_dbt_runs.sql"}{
  data,e:=os.ReadFile(filepath.Join("..","..","migrations","postgres",file));if e!=nil{t.Fatal(e)}
  if _,e=pool.Exec(ctx,string(data));e!=nil{t.Fatal(e)}
 }
 t.Setenv("DATAFLOW_ALLOW_TEST_LOAD","true");t.Setenv("DATAFLOW_ENABLE_SQL_JOBS_TEST","true")
 t.Setenv("DATAFLOW_CH_READONLY_USER","dataflow_reader");t.Setenv("DATAFLOW_CH_READONLY_PASSWORD","ci-read-password")
 t.Setenv("DATAFLOW_ENABLE_DBT_TEST","true");t.Setenv("DATAFLOW_TEST_TARGET_DATABASE","scratch")
 svc:=&server{pg:pool,token:"lineage-test-secret"}
 postModel:=func(m transform.Model)(int,string){
  t.Helper();payload,_:=json.Marshal(map[string]any{"model":m})
  req:=httptest.NewRequest(http.MethodPost,"/api/v1/transform/models",bytes.NewReader(payload))
  req.Header.Set("Authorization","Bearer lineage-test-secret")
  w:=httptest.NewRecorder();svc.transformModels(w,req)
  return w.Code,w.Body.String()
 }
 getGraph:=func(impact string)(int,string){
  t.Helper();url:="/api/v1/transform/lineage"
  if impact!=""{url+="?impact="+impact}
  req:=httptest.NewRequest(http.MethodGet,url,nil);req.Header.Set("Authorization","Bearer lineage-test-secret")
  w:=httptest.NewRecorder();svc.lineageAPI(w,req)
  return w.Code,w.Body.String()
 }
 prefix:=fmt.Sprintf("l%d",time.Now().UnixNano())
 a,b,c:=prefix+"a",prefix+"b",prefix+"c"
 model:=func(name,table string)transform.Model{
  return transform.Model{Name:name,Mode:"visual",Source:transform.Source{Database:"scratch",Table:table}}
 }
 for _,m:=range []transform.Model{model(a,"invoices"),model(b,"dfv_"+a),model(c,"dfv_"+b)}{
  status,body:=postModel(m);if status!=201{t.Fatalf("save %s status %d: %s",m.Name,status,body)}
 }
 status,body:=getGraph(a)
 if status!=200{t.Fatalf("lineage GET %d %s",status,body)}
 var graph struct{
  Impact []string `json:"impact"`
  ModelOrder []string `json:"model_order"`
  Edges []struct{From string `json:"from"`;To string `json:"to"`} `json:"edges"`
 }
 if e:=json.Unmarshal([]byte(body),&graph);e!=nil{t.Fatal(e)}
 if len(graph.Impact)!=2||graph.Impact[0]!=b||graph.Impact[1]!=c{t.Fatalf("impact wrong %+v",graph.Impact)}
 if len(graph.Edges)<3{t.Fatalf("lineage missing edges: %+v",graph.Edges)}
 // This update would create A -> C -> B -> A. Save must reject and not
 // corrupt the previous A source in PostgreSQL.
 status,body=postModel(model(a,"dfv_"+c))
 if status!=422||!strings.Contains(body,"cycle"){t.Fatalf("cycle allowed: %d %s",status,body)}
 var sourceTable string
 err=pool.QueryRow(ctx,`SELECT definition->'source'->>'table' FROM transform_models WHERE name=$1`,a).Scan(&sourceTable)
 if err!=nil||sourceTable!="invoices"{t.Fatalf("rejected mutation persisted: %q %v",sourceTable,err)}
 // Build C requires B and A to have completed latest successful View builds.
 var modelID string
 if err=pool.QueryRow(ctx,"SELECT id::text FROM transform_models WHERE name=$1",c).Scan(&modelID);err!=nil{t.Fatal(err)}
 runBody:=fmt.Sprintf(`{"model_id":"%s","materialization":"view"}`,modelID)
 build:=func()(int,string){
  req:=httptest.NewRequest(http.MethodPost,"/api/v1/transform/dbt/runs",strings.NewReader(runBody))
  req.Header.Set("Authorization","Bearer lineage-test-secret")
  w:=httptest.NewRecorder();svc.dbtRunsAPI(w,req);return w.Code,w.Body.String()
 }
 status,body=build()
 if status!=409||!strings.Contains(body,"upstream"){t.Fatalf("allowed missing upstream: %d %s",status,body)}
 // Simulate successfully built ancestor versions in the disposable test DB.
 for _,name:=range []string{a,b}{
  _,e:=pool.Exec(ctx,`INSERT INTO dbt_runs(model_id,model_name,target_database,target_relation,materialization,source_definition,compiled_sql,status)
    SELECT id,name,'scratch','dfv_'||name,'view',definition,compiled_sql,'succeeded'
    FROM transform_models WHERE name=$1`,name)
  if e!=nil{t.Fatal(e)}
 }
 status,body=build();if status!=202{t.Fatalf("valid upstream chain rejected: %d %s",status,body)}
 // Complete pending C run so its active unique index does not hide a stale-parent error.
 if _,err=pool.Exec(ctx,"UPDATE dbt_runs SET status='failed' WHERE model_name=$1 AND status='queued'",c);err!=nil{t.Fatal(err)}
 changed:=model(a,"invoices");changed.Steps=[]transform.Step{{ID:"limit_a",Type:"limit",Limit:10}}
 status,body=postModel(changed);if status!=201{t.Fatalf("cannot update upstream model: %d %s",status,body)}
 status,body=build()
 if status!=409||!strings.Contains(body,"upstream"){t.Fatalf("stale upstream build accepted: %d %s",status,body)}
}
