package transform

import (
 "context"
 "fmt"
 "net/http"
 "net/url"
 "os"
 "testing"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/connectors/clickhouse"
 "github.com/dinhvansh/dataflow-engine/internal/engine"
)
func TestVisualTransformClickHouseIntegration(t *testing.T){
 endpoint:=os.Getenv("DATAFLOW_INTEGRATION_CH_URL")
 if endpoint==""{t.Skip("requires disposable ClickHouse container")}
 ctx,cancel:=context.WithTimeout(context.Background(),2*time.Minute);defer cancel()
 name:=fmt.Sprintf("df_transform_ci_%d",time.Now().UnixNano())
 writerUser,writerPass:=os.Getenv("DATAFLOW_INTEGRATION_CH_USER"),os.Getenv("DATAFLOW_INTEGRATION_CH_PASSWORD")
 ddl:=func(statement string)error{
  u,err:=url.Parse(endpoint);if err!=nil{return err}
  q:=u.Query();q.Set("query",statement);u.RawQuery=q.Encode()
  req,err:=http.NewRequestWithContext(ctx,http.MethodPost,u.String(),nil);if err!=nil{return err}
  if writerUser!=""{req.SetBasicAuth(writerUser,writerPass)}
  client:=http.Client{Timeout:30*time.Second}
  res,err:=client.Do(req);if err!=nil{return err}
  defer res.Body.Close()
  if res.StatusCode<200||res.StatusCode>=300{return fmt.Errorf("ClickHouse DDL HTTP %d",res.StatusCode)}
  return nil
 }
 create:=fmt.Sprintf("CREATE TABLE scratch.%s (Customer String, Amount Float64, Units Int64) ENGINE=MergeTree ORDER BY Customer",name)
 if err:=ddl(create);err!=nil{t.Fatal(err)}
 defer func(){_ = ddl(fmt.Sprintf("DROP TABLE IF EXISTS scratch.%s",name))
 }()
 writer:=clickhouse.Writer{Endpoint:endpoint,Database:"scratch",Table:name,Username:writerUser,Password:writerPass}
 if err:=writer.Write(ctx,engine.Batch{Rows:[]map[string]any{
  {"Customer":"A","Amount":100,"Units":2},
  {"Customer":"B","Amount":50,"Units":3},
  {"Customer":"A","Amount":75,"Units":1},
 }});err!=nil{t.Fatal(err)}
 m:=Model{Source:Source{Database:"scratch",Table:name},Steps:[]Step{
  {ID:"a",Type:"filter",Column:"Amount",Operator:"greater",Value:"50",ValueType:"number"},
  {ID:"b",Type:"calculate",Column:"Amount",Alias:"Adjusted",FormulaOperator:"multiply",Value:"1.2"},
  {ID:"c",Type:"aggregate",GroupBy:[]string{"Customer"},Function:"sum",Column:"Adjusted",Alias:"Revenue"},
  {ID:"d",Type:"sort",Column:"Revenue",Direction:"DESC"},
  {ID:"e",Type:"limit",Limit:10},
 }}
 compiled,err:=Compile(m,"scratch");if err!=nil{t.Fatal(err)}
 q,err:=PreviewSQL(compiled.SQL,50);if err!=nil{t.Fatal(err)}
 reader:=clickhouse.QueryJob{Endpoint:endpoint,Database:"scratch",Username:writerUser,Password:writerPass}
 result,err:=reader.Execute(ctx,q)
 if err!=nil{t.Fatalf("ClickHouse visual SQL invalid: %v\n%s",err,q)}
 if result.Rows!=1||len(result.Data)!=1{t.Fatalf("unexpected grouped rows %+v",result)}
 if result.Data[0]["Customer"]!="A"{t.Fatalf("wrong result: %+v",result.Data)}
 if len(result.Meta)!=2{t.Fatalf("missing schema metadata: %+v",result.Meta)}
 // Rename should generate valid ClickHouse syntax and preserve other columns.
 ren,err:=Compile(Model{Source:Source{Database:"scratch",Table:name},Steps:[]Step{{ID:"r",Type:"rename",Column:"Amount",Alias:"InvoiceAmount"}}},"scratch")
 if err!=nil{t.Fatal(err)}
 rq,_:=PreviewSQL(ren.SQL,3)
 rr,err:=reader.Execute(ctx,rq)
 if err!=nil{t.Fatalf("ClickHouse rename failed: %v\n%s",err,rq)}
 if rr.Rows!=3{t.Fatalf("unexpected rename row count: %d",rr.Rows)}
 if _,ok:=rr.Data[0]["InvoiceAmount"];!ok{t.Fatalf("renamed field missing: %+v",rr.Data[0])}
}
