package main

import (
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

// Mutates ONLY the identity-verified, disposable integration PostgreSQL.
func TestConnectionRegistryDraftIntegration(t *testing.T){
 dsn:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
 if dsn==""{t.Skip("requires disposable PostgreSQL fixture")}
 if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true"{t.Fatal("refusing non-isolated PostgreSQL")}
 ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second);defer cancel()
 pool,err:=pgxpool.New(ctx,dsn);if err!=nil{t.Fatal(err)};t.Cleanup(pool.Close)
 sql,err:=os.ReadFile(filepath.Join("..","..","migrations","postgres","017_connection_definitions.sql"));if err!=nil{t.Fatal(err)}
 for i:=0;i<2;i++{if _,err=pool.Exec(ctx,string(sql));err!=nil{t.Fatalf("migration pass %d: %v",i+1,err)}}
 t.Setenv("DATAFLOW_ENABLE_CONNECTION_REGISTRY","true")
 t.Setenv("DATAFLOW_CONNECTION_ADMIN_TOKEN","qa-only-admin-synthetic")
 service:=&server{token:"qa-only-user-synthetic",pg:pool}
 if err=service.checkConnectionRegistrySchema(ctx);err!=nil{t.Fatalf("migration check: %v",err)}
 name:="QA Draft "+time.Now().UTC().Format("20060102150405.000000000")
 do:=func(method,payload,admin string)*httptest.ResponseRecorder{
  req:=httptest.NewRequest(method,"/api/v1/connections/definitions",strings.NewReader(payload))
  req.Header.Set("Authorization","Bearer qa-only-user-synthetic")
  if admin!=""{req.Header.Set("X-DataFlow-Connection-Admin",admin)}
  rec:=httptest.NewRecorder();service.connectionDefinitionsAPI(rec,req);return rec
 }
 payload,err:=json.Marshal(map[string]string{"name":name,"driver":"sqlserver","kind":"source"});if err!=nil{t.Fatal(err)}
 created:=do(http.MethodPost,string(payload),"qa-only-admin-synthetic")
 if created.Code!=http.StatusCreated{t.Fatalf("create=%d %s",created.Code,created.Body.String())}
 var definition connectionDefinition
 if err=json.Unmarshal(created.Body.Bytes(),&definition);err!=nil{t.Fatal(err)}
 if definition.Status!="draft"||definition.Workspace!="lab"||definition.Revision!=1||definition.ID==""{t.Fatalf("bad definition %+v",definition)}
 t.Cleanup(func(){
  cleanupCtx,done:=context.WithTimeout(context.Background(),15*time.Second);defer done()
  if _,err:=pool.Exec(cleanupCtx,`DELETE FROM connection_definition_audit WHERE definition_id=$1::uuid`,definition.ID);err!=nil{t.Errorf("cleanup audit: %v",err)}
  if _,err:=pool.Exec(cleanupCtx,`DELETE FROM connection_definitions WHERE id=$1::uuid AND workspace='lab' AND status='draft'`,definition.ID);err!=nil{t.Errorf("cleanup definition: %v",err)}
 })
 dup:=do(http.MethodPost,string(payload),"qa-only-admin-synthetic")
 if dup.Code!=http.StatusConflict{t.Fatalf("duplicate=%d",dup.Code)}
 blocked:=do(http.MethodPost,`{"name":"another","driver":"sqlserver","kind":"source","host":"localhost"}`,"qa-only-admin-synthetic")
 if blocked.Code!=http.StatusBadRequest{t.Fatalf("host injection=%d",blocked.Code)}
 unauthorized:=do(http.MethodPost,string(payload),"wrong-admin")
 if unauthorized.Code!=http.StatusForbidden{t.Fatalf("admin gate=%d",unauthorized.Code)}
 listed:=do(http.MethodGet,"","")
 if listed.Code!=http.StatusOK{t.Fatalf("GET=%d",listed.Code)}
 var payloadList struct{Definitions []connectionDefinition `json:"definitions"`}
 if err=json.Unmarshal(listed.Body.Bytes(),&payloadList);err!=nil{t.Fatal(err)}
 found:=false
 for _,d:=range payloadList.Definitions{if d.ID==definition.ID{found=true}}
 if !found{t.Fatal("created draft missing from registry list")}
 var audited int
 if err=pool.QueryRow(ctx,`SELECT count(*) FROM connection_definition_audit
  WHERE definition_id=$1::uuid AND action='created'`,definition.ID).Scan(&audited);err!=nil{t.Fatal(err)}
 if audited!=1{t.Fatalf("audit count=%d",audited)}
}
