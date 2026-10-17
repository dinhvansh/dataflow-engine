package main

import (
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

func TestConnectionDefinitionsInputHardGate(t *testing.T) {
 good:=[]definitionInput{
  {Name:"QA SQL Server 01",Driver:"sqlserver",Kind:"source"},
  {Name:"QA ClickHouse 02",Driver:"clickhouse",Kind:"destination"},
 }
 for _,d:=range good{if !validDefinitionInput(d){t.Fatalf("rejected valid definition %+v",d)}}
 bad:=[]definitionInput{
  {Name:"",Driver:"sqlserver",Kind:"source"},
  {Name:"https://private.local",Driver:"sqlserver",Kind:"source"},
  {Name:"qa@example.com",Driver:"sqlserver",Kind:"source"},
  {Name:"SQL secret:abc",Driver:"sqlserver",Kind:"source"},
  {Name:strings.Repeat("N",81),Driver:"sqlserver",Kind:"source"},
  {Name:"QA DB",Driver:"postgresql",Kind:"internal"},
  {Name:"QA CH",Driver:"clickhouse",Kind:"source"},
  {Name:"QA SQL",Driver:"sqlserver",Kind:"destination"},
 }
 for _,d:=range bad{if validDefinitionInput(d){t.Fatalf("accepted invalid definition %+v",d)}}
}
func TestConnectionDefinitionsDisabledAndAdminFailClosed(t *testing.T){
 t.Setenv("DATAFLOW_ENABLE_CONNECTION_REGISTRY","")
 t.Setenv("DATAFLOW_CONNECTION_ADMIN_TOKEN","")
 s:=&server{token:"synthetic-user"}
 call:=func(method,apiToken,adminToken,body string) *httptest.ResponseRecorder {
  req:=httptest.NewRequest(method,"/api/v1/connections/definitions",strings.NewReader(body))
  if apiToken!=""{req.Header.Set("Authorization","Bearer "+apiToken)}
  if adminToken!=""{req.Header.Set("X-DataFlow-Connection-Admin",adminToken)}
  w:=httptest.NewRecorder();s.connectionDefinitionsAPI(w,req);return w
 }
 if a:=call(http.MethodGet,"","", "");a.Code!=401{t.Fatalf("GET unauth=%d",a.Code)}
 if a:=call(http.MethodPost,"synthetic-user","",`{"name":"QA","driver":"sqlserver","kind":"source"}`);a.Code!=403{t.Fatalf("POST disabled=%d",a.Code)}
 if a:=call(http.MethodPut,"synthetic-user","", "");a.Code!=405{t.Fatalf("PUT=%d",a.Code)}
 resp:=call(http.MethodGet,"synthetic-user","", "")
 if resp.Code!=200{t.Fatalf("disabled GET=%d",resp.Code)}
 var v struct{Definitions []connectionDefinition `json:"definitions"`;WriteEnabled bool `json:"write_enabled"`}
 if err:=json.Unmarshal(resp.Body.Bytes(),&v);err!=nil{t.Fatal(err)}
 if len(v.Definitions)!=0||v.WriteEnabled{t.Fatalf("unexpected registry data %+v",v)}
 if resp.Header().Get("Cache-Control")!="no-store"{t.Fatal("missing no-store")}
 t.Setenv("DATAFLOW_ENABLE_CONNECTION_REGISTRY","true")
 t.Setenv("DATAFLOW_CONNECTION_ADMIN_TOKEN","different-synthetic-admin")
 if a:=call(http.MethodPost,"synthetic-user","incorrect","{}");a.Code!=403{t.Fatalf("POST unauthorized=%d",a.Code)}
 if a:=call(http.MethodPost,"synthetic-user","different-synthetic-admin","{}");a.Code!=503{t.Fatalf("POST nil PG=%d",a.Code)}
 if a:=call(http.MethodGet,"synthetic-user","", "");a.Code!=503{t.Fatalf("GET nil PG=%d",a.Code)}
 if !adminConnectionAllowed(httptest.NewRequest(http.MethodGet,"/",nil)){
  // Expected no admin grant without header.
 }else{t.Fatal("unexpected admin permission")}
}
