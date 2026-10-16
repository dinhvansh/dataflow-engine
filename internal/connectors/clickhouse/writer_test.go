package clickhouse

import (
 "context"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "github.com/dinhvansh/dataflow-engine/internal/engine"
)

func TestWriterSendsJSONEachRow(t *testing.T){
 var body,query string
 srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.Method!="POST"{t.Errorf("method %s",r.Method)}
  data:=make([]byte,r.ContentLength);_,_ = r.Body.Read(data)
  body=string(data);query=r.URL.Query().Get("query")
  w.WriteHeader(200)
 }));defer srv.Close()
 wr:=&Writer{Endpoint:srv.URL,Database:"scratch",Table:"test"}
 if err:=wr.Write(context.Background(),engine.Batch{Rows:[]map[string]any{{"id":1}}});err!=nil {t.Fatal(err)}
 if !strings.Contains(body,"\"id\":1") || !strings.Contains(query,"INSERT INTO"){t.Fatalf("unexpected query/body: %s %s",query,body)}
}
func TestRejectUnsafeTarget(t *testing.T){
 w:=&Writer{Database:"db;drop",Table:"table"}
 if err:=w.Write(context.Background(),engine.Batch{Rows:[]map[string]any{{"id":1}}});err==nil{t.Fatal("expected rejection")}
}
