package clickhouse

import(
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)
func TestValidateReadQuery(t *testing.T){
 for _,sql:=range []string{"SELECT 1"," select count() FROM scratch.events","SELECT _pk FROM scratch.latest LIMIT 5"}{
  if err:=ValidateReadQuery(sql);err!=nil{t.Fatalf("unexpected rejected %q: %v",sql,err)}
 }
 for _,sql:=range []string{"DROP TABLE t","SELECT 1; DROP TABLE t","SELECT 1 -- comment","WITH q AS (SELECT 1) SELECT * FROM q","SELECT /*hi*/ 1"}{
  if err:=ValidateReadQuery(sql);err==nil{t.Fatalf("dangerous unsupported query accepted %q",sql)}
 }
}
func TestReadQueryRestrictedHTTP(t *testing.T){
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  q:=r.URL.Query()
  if q.Get("readonly")!="1"||q.Get("max_execution_time")!="30"||q.Get("database")!="scratch"{t.Errorf("missing security settings: %s",r.URL)}
  if !strings.HasSuffix(q.Get("query"),"FORMAT JSON"){t.Errorf("missing JSON format")}
  _=json.NewEncoder(w).Encode(map[string]any{"rows":1,"data":[]map[string]any{{"answer":42}}})
 }))
 defer server.Close()
 j:=QueryJob{Endpoint:server.URL,Database:"scratch"}
 result,err:=j.Execute(context.Background(),"SELECT 42 AS answer")
 if err!=nil||result.Rows!=1||len(result.Data)!=1{t.Fatalf("result %+v error %v",result,err)}
}
func TestReadQueryRejectUnsafe(t *testing.T){
 q:=QueryJob{Endpoint:"http://127.0.0.1",Database:"scratch;DROP"}
 if _,err:=q.Execute(context.Background(),"SELECT 1");err==nil{t.Fatal("unsafe database allowed")}
}
