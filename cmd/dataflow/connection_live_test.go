package main

import (
 "context"
 "encoding/json"
 "net/http"
 "os"
 "strings"
 "time"
)

// An intentionally narrow operational check: no user-supplied endpoint or
// credentials, no DDL/DML, no production ERP probing. Arbitrary connection
// profiles must be implemented behind separate server-side egress and Vault
// controls; this route MUST NOT become a generic URL dialer.
func (s *server) configuredConnectionTestAPI(w http.ResponseWriter, r *http.Request) {
 w.Header().Set("Cache-Control","no-store")
 w.Header().Set("X-Content-Type-Options","nosniff")
 if r.Method!=http.MethodPost {http.Error(w,"POST required",http.StatusMethodNotAllowed);return}
 if !s.validBearer(r) {http.Error(w,"unauthorized",http.StatusUnauthorized);return}
 if os.Getenv("DATAFLOW_ENABLE_LIVE_CONNECTION_TEST")!="true" ||
    os.Getenv("DATAFLOW_INTEGRATION_MSSQL_ISOLATED")!="true" ||
    os.Getenv("DATAFLOW_INTEGRATION_CH_ISOLATED")!="true" {
    http.Error(w,"isolated live connection checks disabled",http.StatusForbidden);return
 }
 // The SQL Server DB must be precisely the known disposable fixture.
 // Never dial real Epicor or a client-controlled destination.
 if s.sql==nil || s.chURL=="" {http.Error(w,"QA connection unavailable",http.StatusServiceUnavailable);return}
 r.Body=http.MaxBytesReader(w,r.Body,512)
 var input struct{ID string `json:"id"`}
 dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields()
 if err:=dec.Decode(&input);err!=nil {http.Error(w,"invalid request",http.StatusBadRequest);return}
 if input.ID!="env-sqlserver-source" && input.ID!="env-clickhouse-target" {http.Error(w,"unsupported connection",http.StatusUnprocessableEntity);return}
 ctx,cancel:=context.WithTimeout(r.Context(),5*time.Second);defer cancel()
 passed:=false
 switch input.ID {
 case "env-sqlserver-source":
  var dbName string
  // Metadata-only query; fixed SQL with no injected fields.
  if err:=s.sql.QueryRowContext(ctx,"SELECT DB_NAME()").Scan(&dbName);err==nil && dbName=="DataFlowFixtureQA" {
   // Require effective SELECT and deny all source table modifications.
   var rd,wr,up,del,alter int
   err=s.sql.QueryRowContext(ctx,`SELECT
    HAS_PERMS_BY_NAME(N'Erp.Part',N'OBJECT',N'SELECT'),
    HAS_PERMS_BY_NAME(N'Erp.Part',N'OBJECT',N'INSERT'),
    HAS_PERMS_BY_NAME(N'Erp.Part',N'OBJECT',N'UPDATE'),
    HAS_PERMS_BY_NAME(N'Erp.Part',N'OBJECT',N'DELETE'),
    HAS_PERMS_BY_NAME(N'Erp.Part',N'OBJECT',N'ALTER')`).Scan(&rd,&wr,&up,&del,&alter)
   passed=err==nil && rd==1 && wr==0 && up==0 && del==0 && alter==0
  }
 case "env-clickhouse-target":
  // Only a preconfigured isolated target; the existing environment credentials
  // are never returned. No URL is accepted from the incoming request.
  if strings.HasPrefix(s.chURL,"http://") || strings.HasPrefix(s.chURL,"https://") {
   req,err:=http.NewRequestWithContext(ctx,http.MethodPost,s.chURL,strings.NewReader("SELECT 1"))
   if err==nil {
    req.Header.Set("Content-Type","text/plain; charset=utf-8")
    if s.chUser!="" {req.SetBasicAuth(s.chUser,s.chPass)}
    client:=&http.Client{Timeout:5*time.Second,CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}}
    resp,e:=client.Do(req)
    if e==nil {
     defer resp.Body.Close()
     passed=resp.StatusCode==http.StatusOK
    }
   }
  }
 }
 w.Header().Set("Content-Type","application/json")
 _=json.NewEncoder(w).Encode(map[string]any{"id":input.ID,"passed":passed,"environment":"isolated-qa","tested_at":time.Now().UTC().Format(time.RFC3339)})
}
