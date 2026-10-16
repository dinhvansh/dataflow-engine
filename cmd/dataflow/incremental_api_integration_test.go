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
func TestSafeIncrementalAPIIntegration(t *testing.T){
 dsn:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
 if dsn==""{t.Skip("requires disposable PostgreSQL")}
 if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true"{t.Fatal("not isolated")}
 ctx,done:=context.WithTimeout(context.Background(),45*time.Second);defer done()
 pg,err:=pgxpool.New(ctx,dsn);if err!=nil{t.Fatal(err)};defer pg.Close()
 data,e:=os.ReadFile(filepath.Join("..","..","migrations","postgres","014_safe_incremental.sql"));if e!=nil{t.Fatal(e)}
 if _,err=pg.Exec(ctx,string(data));err!=nil{t.Fatal(err)}
 t.Setenv("DATAFLOW_ALLOW_TEST_LOAD","true");t.Setenv("DATAFLOW_ENABLE_DBT_TEST","true")
 t.Setenv("DATAFLOW_ENABLE_SAFE_TABLE_TEST","true");t.Setenv("DATAFLOW_ENABLE_INCREMENTAL_TEST","true")
 t.Setenv("DATAFLOW_TEST_TARGET_DATABASE","scratch")
 svc:=&server{pg:pg,token:"incremental-ci-token"}
 call:=func(handler http.HandlerFunc,path,body string,auth bool)*httptest.ResponseRecorder{
  req:=httptest.NewRequest(http.MethodPost,path,bytes.NewBufferString(body))
  if auth{req.Header.Set("Authorization","Bearer incremental-ci-token")}
  r:=httptest.NewRecorder();handler(r,req);return r
 }
 suffix:=fmt.Sprintf("%d",time.Now().UnixNano())
 target:="dft_citest"+suffix;delta:="dfd_citest"+suffix
 body:=fmt.Sprintf(`{"target":"%s","delta_table":"%s","key":"ID","initial_cursor":"0"}`,target,delta)
 if r:=call(svc.incrementalRunsAPI,"/api/v1/transform/incremental/runs",body,false);r.Code!=401{t.Fatal("no auth accepted")}
 if r:=call(svc.incrementalRunsAPI,"/api/v1/transform/incremental/runs",
  fmt.Sprintf(`{"target":"%s","delta_table":"%s","key":"ID;DROP"}`,target,delta),true);r.Code!=422{t.Fatal("unsafe key accepted")}
 created:=call(svc.incrementalRunsAPI,"/api/v1/transform/incremental/runs",body,true)
 if created.Code!=202{t.Fatalf("enqueue returned %d %s",created.Code,created.Body.String())}
 var runID string
 if err=pg.QueryRow(ctx,"SELECT id::text FROM incremental_runs WHERE target_name=$1",target).Scan(&runID);err!=nil{t.Fatal(err)}
 defer func(){cleanup,stop:=context.WithTimeout(context.Background(),10*time.Second);defer stop()
  _,_=pg.Exec(cleanup,"DELETE FROM incremental_runs WHERE target_name=$1",target)
  _,_=pg.Exec(cleanup,"DELETE FROM incremental_targets WHERE target_name=$1",target)
 }()
 if r:=call(svc.incrementalRunsAPI,"/api/v1/transform/incremental/runs",body,true);r.Code!=409{t.Fatal("duplicate active run accepted")}
 incompatible:=fmt.Sprintf(`{"target":"%s","delta_table":"%s","key":"Different"}`,target,delta)
 if r:=call(svc.incrementalRunsAPI,"/api/v1/transform/incremental/runs",incompatible,true);r.Code!=409{t.Fatal("checkpoint reset by changing key")}
 action:=func(kind,confirmation string)*httptest.ResponseRecorder{
  payload:=fmt.Sprintf(`{"run_id":"%s","action":"%s","confirm":"%s"}`,runID,kind,confirmation)
  return call(svc.incrementalActionAPI,"/api/v1/transform/incremental/action",payload,true)
 }
 if r:=action("publish","PUBLISH");r.Code!=409{t.Fatal("unvalidated run published")}
 if _,err=pg.Exec(ctx,`UPDATE incremental_runs SET state='ready',cursor_upper=4,
 rows_verified=2,stage_uuid='test-stage' WHERE id=$1::uuid`,runID);err!=nil{t.Fatal(err)}
 if r:=action("publish","yes");r.Code!=422{t.Fatal("missing confirm allowed")}
 if _,err=pg.Exec(ctx,"UPDATE incremental_targets SET cursor_value=3 WHERE target_name=$1",target);err!=nil{t.Fatal(err)}
 if r:=action("publish","PUBLISH");r.Code!=409{t.Fatal("stale lower cursor allowed")}
 if _,err=pg.Exec(ctx,"UPDATE incremental_targets SET cursor_value=0 WHERE target_name=$1",target);err!=nil{t.Fatal(err)}
 if r:=action("publish","PUBLISH");r.Code!=202{t.Fatalf("ready request rejected %d %s",r.Code,r.Body.String())}
 if _,err=pg.Exec(ctx,`UPDATE incremental_runs SET state='published',old_target_uuid=NULL
 WHERE id=$1::uuid`,runID);err!=nil{t.Fatal(err)}
 if r:=action("rollback","ROLLBACK");r.Code!=409{t.Fatal("no backup rollback accepted")}
 if _,err=pg.Exec(ctx,`UPDATE incremental_runs SET old_target_uuid='old-uuid' WHERE id=$1::uuid`,runID);err!=nil{t.Fatal(err)}
 if _,err=pg.Exec(ctx,`UPDATE incremental_targets SET cursor_value=4,
 target_uuid='test-stage',latest_run_id=$2::uuid WHERE target_name=$1`,target,runID);err!=nil{t.Fatal(err)}
 if r:=action("rollback","ROLLBACK");r.Code!=202{t.Fatalf("approved rollback rejected %d %s",r.Code,r.Body.String())}
 get:=httptest.NewRequest(http.MethodGet,"/api/v1/transform/incremental/runs?id="+runID,nil)
 get.Header.Set("Authorization","Bearer incremental-ci-token")
 view:=httptest.NewRecorder();svc.incrementalRunsAPI(view,get)
 if view.Code!=200||!strings.Contains(view.Body.String(),runID){t.Fatal("run status missing")}
}
