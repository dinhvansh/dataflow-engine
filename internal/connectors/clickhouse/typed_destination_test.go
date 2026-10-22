package clickhouse

import(
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"

 "github.com/dinhvansh/dataflow-engine/internal/engine"
)
const fixtureUUID="cc000000-0000-4000-8000-000000000003"
func sampleTyped(w *Writer)*TypedDestination{
 return &TypedDestination{Writer:w,StreamID:fixtureUUID,RunID:fixtureUUID,KeyColumn:"OrderID",
  Columns:[]TypedColumn{{Source:"OrderID",Target:"OrderID",CHType:"Int64"},
   {Source:"Amount",Target:"TotalAmount",CHType:"Nullable(Decimal(18,2))"}}}
}
func TestTypedDestinationRejectsInjectionAndUnowned(t *testing.T){
 w:=&Writer{Database:"scratch",Table:"invoices"}
 d:=sampleTyped(w)
 if err:=d.Validate();err!=nil{t.Fatal(err)}
 for _,change:=range []func(*TypedDestination){
  func(x *TypedDestination){x.Writer.Table="x;DROP"},
  func(x *TypedDestination){x.Columns[0].Target="_df_key"},
  func(x *TypedDestination){x.Columns[0].CHType="String) ENGINE=Memory"},
  func(x *TypedDestination){x.Columns[0].Target="TotalAmount"},
  func(x *TypedDestination){x.KeyColumn="Unknown"},
 }{
  candidate:=sampleTyped(&Writer{Database:"scratch",Table:"invoices"})
  change(candidate)
  if e:=candidate.Validate();e==nil{t.Fatal("unsafe typed configuration accepted")}
 }
 srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  q:=r.URL.Query().Get("query")
  if strings.HasPrefix(q,"SELECT comment FROM system.tables"){_,_=w.Write([]byte("other-owner"))}
 }))
 defer srv.Close()
 d.Writer.Endpoint=srv.URL
 if e:=d.EnsureOwned(context.Background());e==nil{t.Fatal("adopted an unrelated table")}
}
func TestTypedDestinationFormatsVersionedRows(t *testing.T){
 var inserts int
 srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  q:=r.URL.Query().Get("query")
  if !strings.HasPrefix(q,"INSERT INTO"){t.Errorf("not an insert %q",q);return}
  var record map[string]any
  if e:=json.NewDecoder(r.Body).Decode(&record);e!=nil{t.Error(e);return}
  inserts++
  if record["TotalAmount"]!=12.5||record["_df_key"]!="1001"||
     record["_df_version"]!=float64(7){t.Errorf("bad typed row %#v",record)}
 }))
 defer srv.Close()
 d:=sampleTyped(&Writer{Endpoint:srv.URL,Database:"scratch",Table:"invoices"})
 batch:=engine.Batch{Rows:[]map[string]any{{
  "OrderID":int64(1001),"Amount":12.5,
  "SysRowID":"1001","SysRevID":"0x0000000000000007",
 }}}
 if e:=d.Write(context.Background(),batch);e!=nil{t.Fatal(e)}
 if inserts!=1{t.Fatal("typed write was not executed")}
}
