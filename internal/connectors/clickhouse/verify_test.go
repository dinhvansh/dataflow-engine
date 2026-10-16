package clickhouse

import (
 "context"
 "fmt"
 "net/http"
 "net/http/httptest"
 "testing"
)
func TestCountRunRows(t *testing.T){
 const run="12345678-1234-1234-1234-123456789012"
 srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.URL.Query().Get("query")==""{t.Error("missing query")}
  _,_ = fmt.Fprintln(w,"42")
 }));defer srv.Close()
 writer:=&Writer{Endpoint:srv.URL,Database:"scratch",Table:"dataflow_raw_events"}
 n,err:=writer.CountRunRows(context.Background(),run)
 if err!=nil||n!=42{t.Fatalf("count %d err %v",n,err)}
 if _,err=writer.CountRunRows(context.Background(),"unsafe'");err==nil{t.Fatal("invalid ID accepted")}
}
