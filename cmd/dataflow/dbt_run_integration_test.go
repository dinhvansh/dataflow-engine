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
 "github.com/jackc/pgx/v5/pgxpool"
)
func TestDBTRunAPIIntegration(t *testing.T){
 dsn:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
 if dsn==""{t.Skip("requires PostgreSQL")}
 if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true"{t.Fatal("test DB isolation required")}
 ctx,stop:=context.WithTimeout(context.Background(),40*time.Second);defer stop()
 pg,err:=pgxpool.New(ctx,dsn);if err!=nil{t.Fatal(err)};defer pg.Close()
 for _,file:=range []string{"010_transform_models.sql","011_dbt_runs.sql"}{
  data,err:=os.ReadFile(filepath.Join("..","..","migrations","postgres",file));if err!=nil{t.Fatal(err)}
  if _,err=pg.Exec(ctx,string(data));err!=nil{t.Fatal(err)}
 }
 t.Setenv("DATAFLOW_ALLOW_TEST_LOAD","true")
 t.Setenv("DATAFLOW_ENABLE_SQL_JOBS_TEST","true")
 t.Setenv("DATAFLOW_ENABLE_DBT_TEST","true")
 t.Setenv("DATAFLOW_TEST_TARGET_DATABASE","scratch")
 s:=&server{pg:pg,token:"local-ci-token"}
 modelName:=fmt.Sprintf("dbt_api_ci_%d",time.Now().UnixNano())
 definition:=fmt.Sprintf(`{"name":%q,"mode":"visual","source":{"database":"scratch","table":"events"},"steps":[]}`,modelName)
 var id string
 err=pg.QueryRow(ctx,`INSERT INTO transform_models(name,definition,compiled_sql)
 VALUES($1,$2::jsonb,'SELECT * FROM scratch.events') RETURNING id::text`,modelName,definition).Scan(&id)
 if err!=nil{t.Fatal(err)}
 call:=func(method,body,uri string)*httptest.ResponseRecorder{
  req:=httptest.NewRequest(method,uri,bytes.NewBufferString(body))
  req.Header.Set("Authorization","Bearer local-ci-token")
  rec:=httptest.NewRecorder();s.dbtRunsAPI(rec,req);return rec
 }
 body:=`{"model_id":"`+id+`","materialization":"view"}`
 queued:=call(http.MethodPost,body,"/api/v1/transform/dbt/runs")
 if queued.Code!=202{t.Fatalf("queue failed %d %s",queued.Code,queued.Body.String())}
 if again:=call(http.MethodPost,body,"/api/v1/transform/dbt/runs");again.Code!=409{t.Fatalf("duplicate run allowed %d",again.Code)}
 if blocked:=call(http.MethodPost,`{"model_id":"`+id+`","materialization":"table"}`,"/api/v1/transform/dbt/runs");blocked.Code!=422{t.Fatalf("table build allowed %d",blocked.Code)}
 history:=call(http.MethodGet,"","/api/v1/transform/dbt/runs")
 if history.Code!=200||!strings.Contains(history.Body.String(),"queued"){t.Fatalf("missing persisted run %s",history.Body.String())}
 req:=httptest.NewRequest(http.MethodGet,"/api/v1/transform/dbt/runs",nil)
 noAuth:=httptest.NewRecorder();s.dbtRunsAPI(noAuth,req)
 if noAuth.Code!=401{t.Fatalf("unauthenticated access %d",noAuth.Code)}
 // Remove the intentionally QUEUED API test run so the following worker
 // package test in the same CI PostgreSQL database claims only its own jobs.
 // This changes TEST fixtures only, never production queue behavior.
 if _,err=pg.Exec(ctx,"DELETE FROM dbt_runs WHERE model_id=$1::uuid",id);err!=nil{t.Fatal(err)}

}
