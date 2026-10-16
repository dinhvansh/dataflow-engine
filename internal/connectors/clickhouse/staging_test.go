package clickhouse

import(
 "context"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)
func TestStageName(t *testing.T){
 got,err:=stageName("Part","12345678-1234-1234-1234-123456789012")
 if err!=nil||!strings.HasPrefix(got,"Part_df_"){t.Fatalf("%s %v",got,err)}
 for _,run:=range []string{"","../123","gggggggg-1111-1111-1111-111111111111"}{if _,err:=stageName("Part",run);err==nil{t.Fatalf("accepted %q",run)}}
}
func TestStageExchange(t *testing.T){
 var q []string
 srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){q=append(q,r.URL.Query().Get("query"));w.WriteHeader(200)}));defer srv.Close()
 wr:=&Writer{Endpoint:srv.URL,Database:"scratch",Table:"part_test"}
 stage,err:=wr.PrepareStage(context.Background(),"12345678-1234-1234-1234-123456789012")
 if err!=nil{t.Fatal(err)}
 if err=wr.PublishStage(context.Background(),stage);err!=nil{t.Fatal(err)}
 if err=wr.DropStage(context.Background(),stage);err!=nil{t.Fatal(err)}
 if len(q)!=3||!strings.HasPrefix(q[0],"CREATE TABLE")||!strings.HasPrefix(q[1],"EXCHANGE TABLES")||!strings.HasPrefix(q[2],"DROP TABLE"){t.Fatal(q)}
}
func TestDDLFailure(t *testing.T){
 srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){http.Error(w,"denied",403)}));defer srv.Close()
 wr:=&Writer{Endpoint:srv.URL,Database:"scratch",Table:"part"}
 _,err:=wr.PrepareStage(context.Background(),"12345678-1234-1234-1234-123456789012")
 if err==nil{t.Fatal("expected DDL refusal")}
}
