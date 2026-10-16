package clickhouse

import(
 "context"
 "os"
 "testing"
 "time"
)
func TestReadOnlySQLIntegration(t *testing.T){
 endpoint:=os.Getenv("DATAFLOW_INTEGRATION_CH_URL")
 if endpoint==""{t.Skip("requires real ClickHouse environment")}
 ctx,cancel:=context.WithTimeout(context.Background(),45*time.Second);defer cancel()
 client:=QueryJob{Endpoint:endpoint,Database:"scratch",Username:os.Getenv("DATAFLOW_INTEGRATION_CH_USER"),Password:os.Getenv("DATAFLOW_INTEGRATION_CH_PASSWORD")}
 result,err:=client.Execute(ctx,"SELECT 42 AS answer")
 if err!=nil{t.Fatalf("real ClickHouse SELECT failed: %v",err)}
 if result.Rows!=1||len(result.Data)!=1{t.Fatalf("unexpected result: %+v",result)}
 if result.Data[0]["answer"]==nil{t.Fatalf("missing SELECT result: %+v",result)}
}
