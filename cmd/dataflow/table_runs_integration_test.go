package main
import(
 "bytes"
 "context"
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
func TestSafeTableAPIIntegration(t *testing.T){
 dsn:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
 if dsn==""{t.Skip("needs disposable PostgreSQL")}
 if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true"{t.Fatal("requires isolated database")}
 ctx,stop:=context.WithTimeout(context.Background(),50*time.Second);defer stop()
 pg,err:=pgxpool.New(ctx,dsn);if err!=nil{t.Fatal(err)};defer pg.Close()
 for _,name:=range []string{"010_transform_models.sql","011_dbt_runs.sql","012_safe_table.sql"}{
  raw,e:=os.ReadFile(filepath.Join("..","..","migrations","postgres",name));if e!=nil{t.Fatal(e)}
  if _,e=pg.Exec(ctx,string(raw));e!=nil{t.Fatal(e)}
 }
 t.Setenv("DATAFLOW_ALLOW_TEST_LOAD","true")
 t.Setenv("DATAFLOW_ENABLE_DBT_TEST","true")
 t.Setenv("DATAFLOW_ENABLE_SAFE_TABLE_TEST","true")
 t.Setenv("DATAFLOW_TEST_TARGET_DATABASE","scratch")
 svc:=&server{pg:pg,token:"ci-table-token"}
 name:=fmt.Sprintf("safeapici%d",time.Now().UnixNano())
 m:=transform.Model{Name:name,Mode:"visual",Source:transform.Source{Database:"scratch",Table:"test_source"}}
 compiled,e:=transform.Compile(m,"scratch");if e!=nil{t.Fatal(e)}
 definition:=fmt.Sprintf(`{"name":"%s","mode":"visual","source":{"database":"scratch","table":"test_source"}}`,name)
 var modelID string
 if err=pg.QueryRow(ctx,`INSERT INTO transform_models(name,definition,compiled_sql)
  VALUES($1,$2::jsonb,$3) RETURNING id::text`,name,definition,compiled.SQL).Scan(&modelID);err!=nil{t.Fatal(err)}
 // These tests share a disposable PostgreSQL container. Never leave a
 // queued publish/rollback fixture for the separate dbt worker test package.
 defer func(){
  cleanup,done:=context.WithTimeout(context.Background(),10*time.Second)
  defer done()
  if _,err:=pg.Exec(cleanup,"DELETE FROM table_runs WHERE model_id=$1::uuid",modelID);err!=nil{t.Logf("cleanup table_run fixture: %v",err)}
 }()

 call:=func(path string,handler http.HandlerFunc,body string,auth bool)*httptest.ResponseRecorder{
  req:=httptest.NewRequest(http.MethodPost,path,bytes.NewBufferString(body))
  if auth{req.Header.Set("Authorization","Bearer ci-table-token")}
  rec:=httptest.NewRecorder();handler(rec,req);return rec
 }
 payload:=fmt.Sprintf(`{"model_id":"%s","unique_column":"id"}`,modelID)
 if rec:=call("/api/v1/transform/table/runs",svc.tableRunsAPI,payload,false);rec.Code!=401{t.Fatal("unauthenticated queue accepted")}
 if rec:=call("/api/v1/transform/table/runs",svc.tableRunsAPI,fmt.Sprintf(`{"model_id":"%s","unique_column":"id;DROP"}`,modelID),true);rec.Code!=422{t.Fatal("unsafe key accepted")}
 rec:=call("/api/v1/transform/table/runs",svc.tableRunsAPI,payload,true)
 if rec.Code!=202{t.Fatalf("queue failed %d %s",rec.Code,rec.Body.String())}
 var id string
 if err=pg.QueryRow(ctx,"SELECT id::text FROM table_runs WHERE model_id=$1::uuid",modelID).Scan(&id);err!=nil{t.Fatal(err)}
 if rec=call("/api/v1/transform/table/runs",svc.tableRunsAPI,payload,true);rec.Code!=409{t.Fatal("concurrent active run accepted")}
 act:=func(action,confirm string)*httptest.ResponseRecorder{
  b:=fmt.Sprintf(`{"run_id":"%s","action":"%s","confirm":"%s"}`,id,action,confirm)
  return call("/api/v1/transform/table/action",svc.tableActionAPI,b,true)
 }
 if rec=act("publish","PUBLISH");rec.Code!=409{t.Fatal("unverified stage publish accepted")}
 if _,err=pg.Exec(ctx,`UPDATE table_runs SET state='ready',stage_uuid='test-stage-uuid',rows_verified=5 WHERE id=$1::uuid`,id);err!=nil{t.Fatal(err)}
 if rec=act("publish","yes");rec.Code!=422{t.Fatal("no confirmation accepted")}
 if _,err=pg.Exec(ctx,`UPDATE transform_models SET compiled_sql='SELECT changed' WHERE id=$1::uuid`,modelID);err!=nil{t.Fatal(err)}
 if rec=act("publish","PUBLISH");rec.Code!=409||!strings.Contains(rec.Body.String(),"changed"){t.Fatalf("stale SQL allowed %s",rec.Body.String())}
 if _,err=pg.Exec(ctx,`UPDATE transform_models SET compiled_sql=$2 WHERE id=$1::uuid`,modelID,compiled.SQL);err!=nil{t.Fatal(err)}
 if rec=act("publish","PUBLISH");rec.Code!=202{t.Fatalf("ready publication rejected %d %s",rec.Code,rec.Body.String())}
 if _,err=pg.Exec(ctx,`UPDATE table_runs SET state='published',old_target_uuid=NULL WHERE id=$1::uuid`,id);err!=nil{t.Fatal(err)}
 if rec=act("rollback","ROLLBACK");rec.Code!=409{t.Fatal("initial publication rollback accepted")}
 if _,err=pg.Exec(ctx,`UPDATE table_runs SET old_target_uuid='previous-uuid' WHERE id=$1::uuid`,id);err!=nil{t.Fatal(err)}
 if rec=act("rollback","ROLLBACK");rec.Code!=202{t.Fatalf("rollback queue failed %d %s",rec.Code,rec.Body.String())}
}
