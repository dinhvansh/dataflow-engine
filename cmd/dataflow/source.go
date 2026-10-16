package main

import(
 "context"
 "crypto/subtle"
 "encoding/json"
 "net/http"
 "time"
 "github.com/dinhvansh/dataflow-engine/internal/connectors/sqlserver"
)
func (s *server) validBearer(r *http.Request)bool{
 expected:="Bearer "+s.token; actual:=r.Header.Get("Authorization")
 return s.token!="" && len(actual)==len(expected) && subtle.ConstantTimeCompare([]byte(actual),[]byte(expected))==1
}
func (s *server) sourceAPI(w http.ResponseWriter,r *http.Request){
 if r.Method!=http.MethodGet{http.Error(w,"GET required",http.StatusMethodNotAllowed);return}
 if !s.validBearer(r){http.Error(w,"unauthorized",http.StatusUnauthorized);return}
 ctx,cancel:=context.WithTimeout(r.Context(),20*time.Second);defer cancel()
 w.Header().Set("Content-Type","application/json")
 var result any
 var err error
 switch r.URL.Path{
 case "/api/v1/source/tables":
  result,err=sqlserver.DiscoverTables(ctx,s.sql,r.URL.Query().Get("schema"))
 case "/api/v1/source/columns":
  result,err=sqlserver.DiscoverColumns(ctx,s.sql,r.URL.Query().Get("schema"),r.URL.Query().Get("table"))
 case "/api/v1/source/rowversion-upper":
  var upper interface{}
  rv,e:=sqlserver.SafeRowVersionUpper(ctx,s.sql);err=e
  if err==nil{upper=map[string]string{"upper_exclusive":rv.String()}}
  result=upper
 default:http.NotFound(w,r);return
 }
 if err!=nil{http.Error(w,err.Error(),http.StatusBadRequest);return}
 _=json.NewEncoder(w).Encode(result)
}
