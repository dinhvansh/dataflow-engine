package clickhouse

import(
 "context"
 "encoding/json"
 "io"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"

 "github.com/dinhvansh/dataflow-engine/internal/engine"
)
func TestEventEnvelope(t *testing.T){
 var stored map[string]any
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  data,_:=io.ReadAll(r.Body)
  if err:=json.Unmarshal([]byte(strings.TrimSpace(string(data))),&stored);err!=nil{t.Error(err)}
  w.WriteHeader(200)
 }));defer server.Close()
 writer:=&EventWriter{Destination:&Writer{Endpoint:server.URL,Database:"scratch",Table:"dataflow_raw_events"},StreamID:"stream1",RunID:"run1"}
 input:=engine.Batch{Sequence:4,Rows:[]map[string]any{{"SysRowID":"ABCD-1234","SysRevID":"0x0000000000000010","PartNum":"PART-01"}}}
 if err:=writer.Write(context.Background(),input);err!=nil{t.Fatal(err)}
 if stored["_pk"]!="abcd-1234"||stored["_sysrevid"]!="0x0000000000000010"||stored["_batch_seq"]!=float64(4){t.Fatalf("bad envelope: %v",stored)}
 var decoded map[string]any
 if err:=json.Unmarshal([]byte(stored["_payload"].(string)),&decoded);err!=nil||decoded["PartNum"]!="PART-01"{t.Fatalf("payload: %v %v",decoded,err)}
}
func TestEventInvalidVersion(t *testing.T){
 writer:=&EventWriter{Destination:&Writer{Endpoint:"http://127.0.0.1",Database:"scratch",Table:"raw"}}
 err:=writer.Write(context.Background(),engine.Batch{Rows:[]map[string]any{{"SysRowID":"x","SysRevID":"invalid"}}})
 if err==nil{t.Fatal("expected validation error before network")}
}
