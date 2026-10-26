package main

import (
 "encoding/base64"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

func TestRuntimeConnectionAEAD(t *testing.T){
 t.Setenv("DATAFLOW_QA_CONNECTION_KEY_B64",base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
 id:="86b0483f-1c2a-45b6-8d7b-78b38ce371a4"
 ciphertext,e:=encryptConnectionSecret(id,"synthetic-qa-secret")
 if e!=nil{t.Fatal(e)}
 if strings.Contains(string(ciphertext),"synthetic-qa-secret"){t.Fatal("plaintext stored")}
 secret,e:=decryptConnectionSecret(id,ciphertext)
 if e!=nil||secret!="synthetic-qa-secret"{t.Fatalf("decrypt failed")}
 if _,e=decryptConnectionSecret("different-id",ciphertext);e==nil{t.Fatal("AAD binding bypass")}
 ciphertext[len(ciphertext)-1]^=0x01
 if _,e=decryptConnectionSecret(id,ciphertext);e==nil{t.Fatal("tamper accepted")}
}
func TestRuntimeConnectionFailClosedAdmission(t *testing.T){
 t.Setenv("DATAFLOW_QA_SQLSERVER_ALLOWED_ENDPOINT","approved-sql:1433")
 t.Setenv("DATAFLOW_QA_CLICKHOUSE_ALLOWED_ENDPOINT","approved-ch:8123")
 approved:=createRuntimeConnection{Name:"QA Source",Driver:"sqlserver",Host:"approved-sql",Port:1433,Database:"DataFlowFixtureQA",Username:"qa_erp_reader",Password:"test-fixture-only"}
 if !validRuntimeProfile(approved){t.Fatal("expected approved isolated SQL profile")}
 for _,change:=range []func(*createRuntimeConnection){
  func(p *createRuntimeConnection){p.Host="not-approved-sql"},
  func(p *createRuntimeConnection){p.Port=9999},
  func(p *createRuntimeConnection){p.Database="ERPDBTEST"},
  func(p *createRuntimeConnection){p.Username="sa"},
  func(p *createRuntimeConnection){p.Driver="clickhouse"},
 }{
  bad:=approved;change(&bad)
  if validRuntimeProfile(bad){t.Fatalf("admitted unsafe lab source: %+v",bad)}
 }
 ch:=createRuntimeConnection{Name:"QA Destination",Driver:"clickhouse",Host:"approved-ch",Port:8123,Database:"scratch",Username:"qa_ch_writer",Password:"dummy"}
 if !validRuntimeProfile(ch){t.Fatal("expected approved scratch target")}
 ch.Database="default"
 if validRuntimeProfile(ch){t.Fatal("non-scratch destination admitted")}
}
func TestRuntimeConnectionAPIRequiresIsolationAndIndependentAdmin(t *testing.T){
 s:=&server{token:"api-secret"}
 t.Setenv("DATAFLOW_ENABLE_RUNTIME_CONNECTIONS_LAB","false")
 req:=httptest.NewRequest(http.MethodGet,"/api/v1/connections/runtime",nil)
 req.Header.Set("Authorization","Bearer api-secret")
 rec:=httptest.NewRecorder()
 s.runtimeProfilesAPI(rec,req)
 if rec.Code!=http.StatusForbidden{t.Fatalf("expected blocked disabled runtime got %d",rec.Code)}
 t.Setenv("DATAFLOW_ENABLE_RUNTIME_CONNECTIONS_LAB","true")
 t.Setenv("DATAFLOW_INTEGRATION_PG_ISOLATED","true")
 t.Setenv("DATAFLOW_INTEGRATION_MSSQL_ISOLATED","true")
 t.Setenv("DATAFLOW_INTEGRATION_CH_ISOLATED","true")
 t.Setenv("DATAFLOW_ALLOW_TEST_LOAD","true")
 t.Setenv("DATAFLOW_TEST_TARGET_DATABASE","scratch")
 t.Setenv("DATAFLOW_CONNECTION_ADMIN_TOKEN","different-admin")
 rec=httptest.NewRecorder();s.runtimeProfilesAPI(rec,req)
 if rec.Code!=http.StatusForbidden{t.Fatalf("missing independent admin denied: %d",rec.Code)}
 for _,method:=range []string{http.MethodPatch,http.MethodDelete}{
  mutation:=httptest.NewRequest(method,"/api/v1/connections/runtime",strings.NewReader(`{"id":"86b0483f-1c2a-45b6-8d7b-78b38ce371a4","revision":1}`))
  mutation.Header.Set("Authorization","Bearer api-secret")
  denied:=httptest.NewRecorder();s.runtimeProfilesAPI(denied,mutation)
  if denied.Code!=http.StatusForbidden{t.Fatalf("%s without independent admin accepted: %d",method,denied.Code)}
 }
}
