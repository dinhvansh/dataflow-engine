package main

import (
 "net/http"
 "net/http/httptest"
 "os"
 "strings"
 "testing"
)

func TestConfiguredConnectionTestGuard(t *testing.T) {
 old:=os.Getenv("DATAFLOW_ENABLE_LIVE_CONNECTION_TEST")
 t.Cleanup(func(){_ = os.Setenv("DATAFLOW_ENABLE_LIVE_CONNECTION_TEST",old)})
 s:=&server{token:"unit-test-token"}
 cases:=[]struct{method,path,body,auth string;status int}{
  {http.MethodGet,"/api/v1/connections/test-configured", "", "Bearer unit-test-token",http.StatusMethodNotAllowed},
  {http.MethodPost,"/api/v1/connections/test-configured",`{"id":"env-sqlserver-source"}`,"",http.StatusUnauthorized},
  {http.MethodPost,"/api/v1/connections/test-configured",`{"id":"env-sqlserver-source"}`,"Bearer unit-test-token",http.StatusForbidden},
 }
 _=os.Setenv("DATAFLOW_ENABLE_LIVE_CONNECTION_TEST","false")
 for i,tc:=range cases {
  req:=httptest.NewRequest(tc.method,tc.path,strings.NewReader(tc.body))
  if tc.auth!=""{req.Header.Set("Authorization",tc.auth)}
  rec:=httptest.NewRecorder()
  s.configuredConnectionTestAPI(rec,req)
  if rec.Code!=tc.status {t.Fatalf("case %d status=%d expected=%d",i,rec.Code,tc.status)}
  if rec.Header().Get("Cache-Control")!="no-store"{t.Fatalf("case %d missing no-store",i)}
 }
}
