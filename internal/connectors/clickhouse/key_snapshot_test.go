package clickhouse

import(
 "context"
 "fmt"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "github.com/dinhvansh/dataflow-engine/internal/engine"
)
func TestKeySnapshotWriterValidation(t *testing.T){
 w:=&KeySnapshotWriter{Destination:&Writer{Database:"scratch",Table:"dataflow_key_snapshots"},StreamID:"12345678-1234-1234-1234-123456789012",SnapshotID:"12345678-1234-1234-1234-123456789013"}
 bad:=engine.Batch{Rows:[]map[string]any{{"SysRowID":"not-a-uuid","SysRevID":"0x0000000000000001"}}}
 if err:=w.Write(context.Background(),bad);err==nil{t.Fatal("invalid key accepted")}
}
func TestSnapshotStatsAndDiffQueries(t *testing.T){
 var queries []string
 svr:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  q:=r.URL.Query().Get("query");queries=append(queries,q)
  if strings.Contains(q,"uniqExact") {fmt.Fprintln(w,"3");return}
  fmt.Fprintln(w,"3")
 }))
 defer svr.Close()
 w:=&Writer{Endpoint:svr.URL,Database:"scratch",Table:"dataflow_key_snapshots"}
 stream:="12345678-1234-1234-1234-123456789012"
 old:="12345678-1234-1234-1234-123456789013"
 newID:="12345678-1234-1234-1234-123456789014"
 stats,err:=w.SnapshotStats(context.Background(),stream,newID)
 if err!=nil||stats.Rows!=3||stats.DistinctKeys!=3{t.Fatalf("stats=%+v err=%v",stats,err)}
 diff,err:=w.SnapshotDiff(context.Background(),stream,old,newID)
 if err!=nil||diff.Added!=3||diff.Deleted!=3||diff.Changed!=3{t.Fatalf("diff=%+v err=%v",diff,err)}
 if len(queries)!=5{t.Fatalf("got %d queries",len(queries))}
 if !strings.Contains(queries[2],"LEFT ANTI JOIN")||!strings.Contains(queries[4],"INNER JOIN"){t.Fatal(queries)}
}
func TestSnapshotRejectsIdentifiers(t *testing.T){
 w:=&Writer{Database:"unsafe;db",Table:"snapshots"}
 _,err:=w.SnapshotStats(context.Background(),"12345678-1234-1234-1234-123456789012","12345678-1234-1234-1234-123456789012")
 if err==nil{t.Fatal("unsafe identifier")}
}
