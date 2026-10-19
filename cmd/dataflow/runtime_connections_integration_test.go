package main

import (
 "bytes"
 "context"
 "database/sql"
 "encoding/base64"
 "encoding/json"
 "net"
 "net/http"
 "net/http/httptest"
 "net/url"
 "os"
 "path/filepath"
 "strconv"
 "strings"
 "testing"
 "time"

 "github.com/jackc/pgx/v5/pgxpool"
)

// An independently verified disposable owner/CI Docker stack uses dataflow_qa,
// whereas coreRealFixture accepts only dataflow_test_ci. Do not weaken the
// shared Core acceptance fixture merely to exercise the new connection lab.
func runtimeConnectionRealFixture(t *testing.T)(context.Context,*server,func()){
 if os.Getenv("DATAFLOW_RUNTIME_QA_OPERATOR_VERIFIED")!="true"{return coreRealFixture(t)}
 if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true"||
 os.Getenv("DATAFLOW_INTEGRATION_MSSQL_ISOLATED")!="true"||
 os.Getenv("DATAFLOW_INTEGRATION_CH_ISOLATED")!="true"{t.Fatal("all disposable labels required")}
 pgDSN,sqlDSN,chURL:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN"),
 os.Getenv("DATAFLOW_INTEGRATION_SQLSERVER_DSN"),os.Getenv("DATAFLOW_INTEGRATION_CH_URL")
 if pgDSN==""||sqlDSN==""||chURL==""{t.Fatal("disposable fixture settings required")}
 u,e:=url.Parse(chURL)
 if e!=nil||u.Scheme!="http"||u.Hostname()!="127.0.0.1"||u.User!=nil||u.RawQuery!=""||u.Fragment!=""{
  t.Fatal("refusing non-loopback ClickHouse target")
 }
 ctx,stop:=context.WithTimeout(context.Background(),3*time.Minute)
 pg,e:=pgxpool.New(ctx,pgDSN);if e!=nil{stop();t.Fatal("QA PostgreSQL connection unavailable")}
 var dbName string
 if e=pg.QueryRow(ctx,"SELECT current_database()").Scan(&dbName);e!=nil||dbName!="dataflow_qa"{
  pg.Close();stop();t.Fatal("refusing non-DataFlow disposable PostgreSQL")}
 source,e:=sql.Open("sqlserver",sqlDSN)
 if e!=nil{pg.Close();stop();t.Fatal("QA SQL driver unavailable")}
 if !checkLabSQLSource(ctx,source){source.Close();pg.Close();stop();t.Fatal("refusing SQL reader lacking exact disposable SELECT-only permissions")}
 svc:=&server{pg:pg,sql:source,chURL:chURL,
  chUser:os.Getenv("DATAFLOW_INTEGRATION_CH_USER"),
  chPass:os.Getenv("DATAFLOW_INTEGRATION_CH_PASSWORD"),
  token:"synthetic-runtime-lab-token"}
 return ctx,svc,func(){source.Close();pg.Close();stop()}
}

// This test exercises actual disposable PostgreSQL + SQL Server driver +
// ClickHouse credentials; it is never permitted to dial company ERP.
func TestRuntimeProfilesRealDisposableQA(t *testing.T) {
 ctx,svc,cleanup:=runtimeConnectionRealFixture(t)
 defer cleanup()
 sqlDSN:=os.Getenv("DATAFLOW_INTEGRATION_SQLSERVER_DSN")
 sqlURL,e:=url.Parse(sqlDSN)
 if e!=nil||sqlURL.Scheme!="sqlserver"||sqlURL.User==nil{
  t.Skip("requires disposable QA SQL Server URL credentials")
 }
 sqlUser:=sqlURL.User.Username()
 sqlPass,hasPass:=sqlURL.User.Password()
 if !hasPass||sqlUser!="qa_erp_reader"{t.Fatal("requires exactly disposable SELECT-only reader")}
 chURL,e:=url.Parse(svc.chURL)
 if e!=nil||chURL.Hostname()!="127.0.0.1"{t.Fatal("requires loopback ClickHouse fixture")}
 sqlHost,sqlPort,e:=net.SplitHostPort(sqlURL.Host)
 if e!=nil{t.Fatal("QA SQL host must specify port")}
 chHost,chPort,e:=net.SplitHostPort(chURL.Host)
 if e!=nil{t.Fatal("QA ClickHouse host must specify port")}
 srcPort,e:=strconv.Atoi(sqlPort);if e!=nil{t.Fatal(e)}
 dstPort,e:=strconv.Atoi(chPort);if e!=nil{t.Fatal(e)}
 t.Setenv("DATAFLOW_ENABLE_RUNTIME_CONNECTIONS_LAB","true")
 t.Setenv("DATAFLOW_INTEGRATION_PG_ISOLATED","true")
 t.Setenv("DATAFLOW_INTEGRATION_MSSQL_ISOLATED","true")
 t.Setenv("DATAFLOW_INTEGRATION_CH_ISOLATED","true")
 t.Setenv("DATAFLOW_ALLOW_TEST_LOAD","true")
 t.Setenv("DATAFLOW_TEST_TARGET_DATABASE","scratch")
 t.Setenv("DATAFLOW_QA_SQL_TRUST_CERT","true")
 t.Setenv("DATAFLOW_QA_CLICKHOUSE_ALLOW_HTTP","true")
 t.Setenv("DATAFLOW_QA_SQLSERVER_ALLOWED_ENDPOINT",sqlURL.Host)
 t.Setenv("DATAFLOW_QA_CLICKHOUSE_ALLOWED_ENDPOINT",chURL.Host)
 t.Setenv("DATAFLOW_QA_CONNECTION_KEY_B64",base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
 t.Setenv("DATAFLOW_CONNECTION_ADMIN_TOKEN","different-synthetic-qa-admin-only")
 raw,e:=os.ReadFile(filepath.Join("..","..","migrations","postgres","031_connection_runtime_profiles.sql"))
 if e!=nil{t.Fatal(e)}
 if _,e=svc.pg.Exec(ctx,string(raw));e!=nil{t.Fatalf("isolated migration unavailable: %v",e)}
 var createdIDs []string
 var createdStreamID string
 defer func(){
  x,stop:=context.WithTimeout(context.Background(),15*time.Second);defer stop()
  if createdStreamID!=""{
   _,_=svc.pg.Exec(x,`DELETE FROM connection_runtime_bindings WHERE stream_id=$1::uuid`,createdStreamID)
   _,_=svc.pg.Exec(x,`DELETE FROM sync_checkpoints WHERE stream_id=$1::uuid`,createdStreamID)
   _,_=svc.pg.Exec(x,`DELETE FROM sync_streams WHERE id=$1::uuid`,createdStreamID)
  }
  for _,id:=range createdIDs{
   _,_=svc.pg.Exec(x,`DELETE FROM connection_runtime_bindings WHERE source_profile_id=$1::uuid OR target_profile_id=$1::uuid`,id)
   _,_=svc.pg.Exec(x,`DELETE FROM connection_runtime_profiles WHERE id=$1::uuid`,id)
  }
 }()
 do:=func(method,path string,payload any)(int,map[string]any,string){
  t.Helper()
  var body bytes.Buffer
  if payload!=nil{if e:=json.NewEncoder(&body).Encode(payload);e!=nil{t.Fatal(e)}}
  req:=httptest.NewRequest(method,path,&body)
  req.Header.Set("Authorization","Bearer "+svc.token)
  req.Header.Set("X-DataFlow-Connection-Admin","different-synthetic-qa-admin-only")
  rec:=httptest.NewRecorder()
  switch path {
  case "/api/v1/connections/runtime":svc.runtimeProfilesAPI(rec,req)
  case "/api/v1/connections/runtime/test":svc.runtimeTestAPI(rec,req)
  case "/api/v1/connections/runtime/bind":svc.runtimeBindAPI(rec,req)
  default:t.Fatal("unexpected test path")
  }
  var result map[string]any
  _=json.Unmarshal(rec.Body.Bytes(),&result)
  return rec.Code,result,rec.Body.String()
 }
 suffix:=time.Now().UTC().Format("150405.000000000")
 sourcePayload:=createRuntimeConnection{Name:"QA Runtime SQL "+suffix,Driver:"sqlserver",Host:sqlHost,Port:srcPort,Database:"DataFlowFixtureQA",Username:sqlUser,Password:sqlPass}
 status,sourceResp,body:=do(http.MethodPost,"/api/v1/connections/runtime",sourcePayload)
 if status!=201{t.Fatalf("source create code=%d response redacted=%t",status,!strings.Contains(body,sqlPass))}
 sourceID,_:=sourceResp["id"].(string);if sourceID==""{t.Fatal("missing source UUID")}
 createdIDs=append(createdIDs,sourceID)
 chUser:=os.Getenv("DATAFLOW_INTEGRATION_CH_USER")
 chPass:=os.Getenv("DATAFLOW_INTEGRATION_CH_PASSWORD")
 if chUser==""||chPass==""{t.Skip("requires disposable ClickHouse writer credentials")}
 targetPayload:=createRuntimeConnection{Name:"QA Runtime CH "+suffix,Driver:"clickhouse",Host:chHost,Port:dstPort,Database:"scratch",Username:chUser,Password:chPass}
 status,targetResp,body:=do(http.MethodPost,"/api/v1/connections/runtime",targetPayload)
 if status!=201{t.Fatalf("destination create code=%d",status)}
 targetID,_:=targetResp["id"].(string);if targetID==""{t.Fatal("missing destination UUID")}
 createdIDs=append(createdIDs,targetID)
 status,_,body=do(http.MethodGet,"/api/v1/connections/runtime",nil)
 if status!=200||strings.Contains(body,sqlPass)||strings.Contains(body,chPass){t.Fatal("GET leaked credentials or failed")}
 var encrypted []byte
 if e=svc.pg.QueryRow(ctx,`SELECT secret_encrypted FROM connection_runtime_profiles WHERE id=$1::uuid`,sourceID).Scan(&encrypted);e!=nil{t.Fatal(e)}
 if bytes.Contains(encrypted,[]byte(sqlPass)){t.Fatal("plaintext credential in PostgreSQL")}
 for _,id:=range []string{sourceID,targetID}{
  status,resp,_:=do(http.MethodPost,"/api/v1/connections/runtime/test",map[string]string{"id":id})
  if status!=200||resp["passed"]!=true{t.Fatalf("real driver test failed for verified disposable profile")}
 }
 status,streams,_:=func()(int,map[string]any,string){
  cfg:=map[string]any{"source_schema":"Erp","source_table":"Part","target_database":"scratch","target_table":"dataflow_raw_events","batch_size":100,"workers":2,"queue_capacity":2}
  raw,_:=json.Marshal(cfg)
  req:=httptest.NewRequest(http.MethodPost,"/api/v1/managed/streams",bytes.NewReader(raw))
  req.Header.Set("Authorization","Bearer "+svc.token)
  rec:=httptest.NewRecorder();svc.streamAPI(rec,req)
  var obj map[string]any;_ = json.Unmarshal(rec.Body.Bytes(),&obj)
  return rec.Code,obj,rec.Body.String()
 }()
 if status!=201{t.Fatalf("could not create isolated scratch stream: code=%d",status)}
 streamID,_:=streams["id"].(string)
 if streamID==""{t.Fatal("missing QA stream ID")}
 createdStreamID=streamID
 status,resp,_:=do(http.MethodPost,"/api/v1/connections/runtime/bind",connectionBindingInput{StreamID:streamID,SourceID:sourceID,TargetID:targetID})
 if status!=200||resp["bound"]!=true{t.Fatalf("binding refused in isolated QA")}
 db,endpoint,user,pass,release,e:=svc.runtimeForManagedStream(ctx,streamID)
 if e!=nil{t.Fatal("worker failed to load saved verified binding")}
 defer release()
 if !checkLabSQLSource(ctx,db)||endpoint!=svc.chURL||user!=chUser||pass!=chPass{
  t.Fatal("stream-bound runtime differs from verified QA resources")
 }
}
