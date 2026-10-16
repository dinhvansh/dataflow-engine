package main

import (
 "bytes"
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "github.com/jackc/pgx/v5/pgxpool"
)
func TestTransformStudioIntegration(t *testing.T){
 dsn:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
 url:=os.Getenv("DATAFLOW_INTEGRATION_CH_URL")
 if dsn==""||url==""{t.Skip("requires isolated PostgreSQL and ClickHouse")}
 if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true"{t.Fatal("refusing unisolated database")}
 ctx,stop:=context.WithTimeout(context.Background(),60*time.Second);defer stop()
 pg,err:=pgxpool.New(ctx,dsn);if err!=nil{t.Fatal(err)};defer pg.Close()
 migration,err:=os.ReadFile(filepath.Join("..","..","migrations","postgres","010_transform_models.sql"))
 if err!=nil{t.Fatal(err)}
 if _,err=pg.Exec(ctx,string(migration));err!=nil{t.Fatal(err)}
 t.Setenv("DATAFLOW_ALLOW_TEST_LOAD","true")
 t.Setenv("DATAFLOW_ENABLE_SQL_JOBS_TEST","true")
 t.Setenv("DATAFLOW_CH_READONLY_USER",os.Getenv("DATAFLOW_INTEGRATION_CH_USER"))
 t.Setenv("DATAFLOW_CH_READONLY_PASSWORD",os.Getenv("DATAFLOW_INTEGRATION_CH_PASSWORD"))
 s:=&server{pg:pg,chURL:url,token:"local-ci-bearer"}
 call:=func(handler http.HandlerFunc,path,method,body string)*httptest.ResponseRecorder{
  t.Helper()
  req:=httptest.NewRequest(method,path,bytes.NewBufferString(body))
  req.Header.Set("Authorization","Bearer local-ci-bearer")
  recorder:=httptest.NewRecorder()
  handler(recorder,req)
  return recorder
 }
 savedBody:=\`{"model":{"name":"ci_visual_model","mode":"visual","source":{"database":"scratch","table":"dataflow_raw_events"},"steps":[{"id":"a","type":"filter","column":"_pk","operator":"contains","value":"abc"}]}}\`
 saved:=call(s.transformModels,"/api/v1/transform/models","POST",savedBody)
 if saved.Code!=201{t.Fatalf("save returned %d: %s",saved.Code,saved.Body.String())}
 list:=call(s.transformModels,"/api/v1/transform/models","GET","")
 if list.Code!=200||!strings.Contains(list.Body.String(),"ci_visual_model"){t.Fatalf("list failed %d %s",list.Code,list.Body.String())}
 compile:=call(s.transformCompile,"/api/v1/transform/compile","POST",savedBody)
 if compile.Code!=200||!strings.Contains(compile.Body.String(),"positionUTF8"){t.Fatalf("compile failed %d %s",compile.Code,compile.Body.String())}
 preview:=call(s.transformPreview,"/api/v1/transform/preview","POST",\`{"model":{"name":"ci_sql","mode":"sql","source":{"database":"scratch","table":"dataflow_raw_events"},"steps":[],"sql":"SELECT 42 AS answer"},"limit":5}\`)
 if preview.Code!=200{t.Fatalf("ClickHouse read-only preview failed %d %s",preview.Code,preview.Body.String())}
 var data struct{Rows int \`json:"rows"\`;Data []map[string]any \`json:"data"\`}
 if err=json.Unmarshal(preview.Body.Bytes(),&data);err!=nil{t.Fatal(err)}
 if data.Rows!=1||len(data.Data)!=1{t.Fatalf("preview: %+v",data)}
 invalid:=call(s.transformCompile,"/api/v1/transform/compile","POST",\`{"model":{"name":"bad","mode":"visual","source":{"database":"prod","table":"secret"}}}\`)
 if invalid.Code!=422{t.Fatalf("production table should be blocked, got %d",invalid.Code)}
 noAuth:=httptest.NewRecorder()
 s.transformModels(noAuth,httptest.NewRequest(http.MethodGet,"/api/v1/transform/models",nil))
 if noAuth.Code!=401{t.Fatalf("missing token accepted: %d",noAuth.Code)}
}
