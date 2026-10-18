package engine_test

import (
 "context"
 "database/sql"
 "errors"
 "fmt"
 "net/url"
 "os"
 "strconv"
 "strings"
 "testing"
 "time"

 "github.com/google/uuid"
 "github.com/dinhvansh/dataflow-engine/internal/connectors/clickhouse"
 "github.com/dinhvansh/dataflow-engine/internal/connectors/sqlserver"
 "github.com/dinhvansh/dataflow-engine/internal/engine"
 "github.com/dinhvansh/dataflow-engine/internal/safetable"
)

// This is intentionally a REAL cross-system smoke test (not a mock):
// disposable Microsoft SQL Server SELECT-only -> bounded Go engine ->
// scratch ClickHouse stage -> actual DQ -> UUID-protected publication.
// It is NOT a persistent scheduled pipeline, exactly-once runtime or a
// production Vault / IdP / network-verified connection.
type partTranProjection struct { sink *clickhouse.Writer }
func (w partTranProjection) Write(ctx context.Context,b engine.Batch)error{
 out:=engine.Batch{Sequence:b.Sequence,Rows:make([]map[string]any,0,len(b.Rows))}
 for _,row:=range b.Rows {
  company,ok:=row["Company"].(string);if !ok||company==""{return errors.New("invalid SQL Company")}
  n,ok:=row["TranNum"].(int64);if !ok||n<=0{return errors.New("invalid SQL TranNum")}
  part,ok:=row["PartNum"].(string);if !ok||part==""{return errors.New("invalid SQL PartNum")}
  id,ok:=row["SysRowID"].(string);if !ok||id==""{return errors.New("invalid SQL SysRowID")}
  rev,ok:=row["SysRevID"].(string);if !ok||!strings.HasPrefix(rev,"0x")||len(rev)!=18{
   return errors.New("invalid SQL SysRevID")}
  comment:=row["CommentText"]
  if comment!=nil{if _,ok:=comment.(string);!ok{return errors.New("invalid SQL CommentText")}}
  out.Rows=append(out.Rows,map[string]any{
   "company":company,"trannum":n,"partnum":part,"comment_text":comment,
   "sysrowid":id,"sysrevid":rev,
  })
 }
 return w.sink.Write(ctx,out)
}

func TestRealSQLServerToClickHousePublishIntegration(t *testing.T){
 dsn,endpoint:=os.Getenv("DATAFLOW_INTEGRATION_SQLSERVER_DSN"),os.Getenv("DATAFLOW_INTEGRATION_CH_URL")
 if dsn==""||endpoint==""{t.Skip("requires BOTH real disposable Microsoft SQL Server and ClickHouse")}
 if os.Getenv("DATAFLOW_INTEGRATION_MSSQL_ISOLATED")!="true"||
  os.Getenv("DATAFLOW_INTEGRATION_CH_ISOLATED")!="true"{
  t.Fatal("both isolated source and destination gates are required")
 }
 origin,err:=url.Parse(endpoint)
 if err!=nil||origin.Scheme!="http"||origin.Hostname()!="127.0.0.1"||
  origin.User!=nil||origin.RawQuery!=""||origin.Fragment!=""||
  (origin.Path!=""&&origin.Path!="/"){
  t.Fatal("refusing any non-loopback/non-disposable ClickHouse endpoint")
 }
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Minute)
 defer cancel()
 source,err:=sql.Open("sqlserver",dsn);if err!=nil{t.Fatal("SQL driver unavailable")}
 defer source.Close()
 source.SetMaxOpenConns(2)
 var database,login string
 var selectPerm,insertPerm,updatePerm,deletePerm,alterPerm sql.NullInt64
 err=source.QueryRowContext(ctx,`SELECT DB_NAME(),ORIGINAL_LOGIN(),
 HAS_PERMS_BY_NAME(N'Erp.PartTran',N'OBJECT',N'SELECT'),
 HAS_PERMS_BY_NAME(N'Erp.PartTran',N'OBJECT',N'INSERT'),
 HAS_PERMS_BY_NAME(N'Erp.PartTran',N'OBJECT',N'UPDATE'),
 HAS_PERMS_BY_NAME(N'Erp.PartTran',N'OBJECT',N'DELETE'),
 HAS_PERMS_BY_NAME(N'Erp.PartTran',N'OBJECT',N'ALTER')`).
 Scan(&database,&login,&selectPerm,&insertPerm,&updatePerm,&deletePerm,&alterPerm)
 if err!=nil||database!="DataFlowFixtureQA"||login!="qa_erp_reader"||
  !selectPerm.Valid||selectPerm.Int64!=1||
  !insertPerm.Valid||insertPerm.Int64!=0||
  !updatePerm.Valid||updatePerm.Int64!=0||
  !deletePerm.Valid||deletePerm.Int64!=0||
  !alterPerm.Valid||alterPerm.Int64!=0 {
  t.Fatal("source must be the exact disposable SELECT-only fixture reader")
 }
 var sourceRows,sourceSum,sourceKeys,sourceVietnamese int64
 err=source.QueryRowContext(ctx,`SELECT COUNT_BIG(*),COALESCE(SUM(TranNum),0),
  COUNT_BIG(DISTINCT TranNum),
  SUM(CASE WHEN CommentText=N'Kiểm thử tiếng Việt' THEN CONVERT(bigint,1) ELSE CONVERT(bigint,0) END)
 FROM [Erp].[PartTran]`).Scan(&sourceRows,&sourceSum,&sourceKeys,&sourceVietnamese)
 if err!=nil||sourceRows!=10000||sourceKeys!=sourceRows{
  t.Fatalf("refusing unseeded/non-disposable source counts: %v, rows=%d keys=%d",err,sourceRows,sourceKeys)
 }
 cli:=safetable.Client{Endpoint:endpoint,User:os.Getenv("DATAFLOW_INTEGRATION_CH_USER"),
  Password:os.Getenv("DATAFLOW_INTEGRATION_CH_PASSWORD")}
 if err=cli.RequireAtomic(ctx);err!=nil{t.Fatalf("scratch must be Atomic: %v",err)}
 runID:=uuid.NewString()
 stage:=safetable.StageName(runID)
 target:=safetable.TargetName("qa_e2e_"+strings.ReplaceAll(runID[:8],"-",""))
 if err=safetable.SafeNames(target,stage);err!=nil{t.Fatal(err)}
 tables:=[]string{stage,target}
 defer func(){
  cleanup,done:=context.WithTimeout(context.Background(),20*time.Second);defer done()
  for _,table:=range tables{
   if _,e:=cli.Query(cleanup,"DROP TABLE IF EXISTS scratch."+table);e!=nil{
    t.Logf("disposable ClickHouse cleanup for %s failed: %v",table,e)
   }
  }
 }()
 schema:=` (company String,trannum Int64,partnum String,comment_text Nullable(String),
  sysrowid String,sysrevid String) ENGINE=MergeTree ORDER BY (company,trannum)`
 if _,err=cli.Query(ctx,"CREATE TABLE scratch."+stage+schema);err!=nil{t.Fatal(err)}
 sink:=&clickhouse.Writer{Endpoint:endpoint,Database:"scratch",Table:stage,
  Username:os.Getenv("DATAFLOW_INTEGRATION_CH_USER"),
  Password:os.Getenv("DATAFLOW_INTEGRATION_CH_PASSWORD")}
 stats,err:=engine.Run(ctx,engine.Config{Workers:3,QueueCapacity:3},
  &sqlserver.Reader{DB:source,Schema:"Erp",Table:"PartTran",BatchSize:250,Snapshot:true},
  partTranProjection{sink:sink})
 if err!=nil{t.Fatalf("real SQL Server -> ClickHouse stream failed: %v",err)}
 if stats.Rows!=uint64(sourceRows)||stats.Batches<2{
  t.Fatalf("stream lost rows or batching not exercised: %+v SQL=%d",stats,sourceRows)
 }
 dq,err:=cli.ValidateStage(ctx,stage,"trannum")
 if err!=nil||dq.Rows!=uint64(sourceRows)||dq.Unique!=uint64(sourceKeys){
  t.Fatalf("real staged DQ failed: %+v %v",dq,err)
 }
 verify:=func(table string){
  t.Helper()
  actual,err:=cli.Query(ctx,"SELECT count(),sum(trannum),uniqExact(trannum),countIf(comment_text='Kiểm thử tiếng Việt') FROM scratch."+table+" FORMAT TabSeparatedRaw")
  if err!=nil{t.Fatal(err)}
  expected:=fmt.Sprintf("%d\t%d\t%d\t%d",sourceRows,sourceSum,sourceKeys,sourceVietnamese)
  if actual!=expected{t.Fatalf("source/destination mismatch: got %q want %q",actual,expected)}
 }
 verify(stage)
 physicalUUID,err:=cli.UUID(ctx,stage)
 if err!=nil||physicalUUID==""{t.Fatalf("stage UUID missing: %v",err)}
 publication:=safetable.Publication{Target:target,Stage:stage,StageUUID:physicalUUID}
 if err=cli.Publish(ctx,publication);err!=nil{t.Fatalf("DQ-approved real publication failed: %v",err)}
 if err=cli.Publish(ctx,publication);err!=nil{t.Fatalf("publish replay unsafe: %v",err)}
 verify(target)
 t.Logf("REAL QA E2E PASS SQLServer->ClickHouse: rows=%d batches=%d distinct=%d sumTranNum=%d Vietnamese=%d DQ=PASS publish=PASS replay=PASS",
  stats.Rows,stats.Batches,dq.Unique,sourceSum,sourceVietnamese)

 // A deliberately duplicate synthetic stage MUST FAIL DQ and not replace
 // the existing published target. This only proves the explicit test
 // orchestration gate, not that every future production caller enforces it.
 bad:=safetable.StageName(uuid.NewString())
 tables=append(tables,bad)
 if _,err=cli.Query(ctx,"CREATE TABLE scratch."+bad+schema);err!=nil{t.Fatal(err)}
 badWriter:=&clickhouse.Writer{Endpoint:endpoint,Database:"scratch",Table:bad,
  Username:os.Getenv("DATAFLOW_INTEGRATION_CH_USER"),
  Password:os.Getenv("DATAFLOW_INTEGRATION_CH_PASSWORD")}
 duplicate:=map[string]any{"company":"VN01","trannum":1,"partnum":"P-000001",
  "comment_text":"qa-only-duplicate","sysrowid":uuid.NewString(),"sysrevid":"0x0000000000000001"}
 if err=badWriter.Write(ctx,engine.Batch{Rows:[]map[string]any{duplicate,duplicate}});err!=nil{
  t.Fatal(err)
 }
 if _,err=cli.ValidateStage(ctx,bad,"trannum");err==nil{
  t.Fatal("bad duplicate stage unexpectedly passed DQ")
 }
 verify(target) // no publication happened on failed DQ
 actualUUID,err:=cli.UUID(ctx,target)
 if err!=nil||actualUUID!=physicalUUID{t.Fatal("DQ failure changed published target")}
 if _,err=strconv.ParseInt(fmt.Sprint(stats.Rows),10,64);err!=nil{t.Fatal(err)}
}
