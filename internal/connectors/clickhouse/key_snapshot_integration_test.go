package clickhouse

import(
 "context"
 "fmt"
 "os"
 "testing"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/engine"
)

// Runs against REAL ClickHouse only when DATAFLOW_INTEGRATION_CH_URL is set.
// This test creates and drops a dedicated test table; never point it at an
// existing or production ClickHouse database.
func TestSnapshotDiffIntegration(t *testing.T){
 endpoint:=os.Getenv("DATAFLOW_INTEGRATION_CH_URL")
 if endpoint==""{t.Skip("real ClickHouse test disabled; set DATAFLOW_INTEGRATION_CH_URL")}
 ctx,cancel:=context.WithTimeout(context.Background(),2*time.Minute);defer cancel()
 db:="scratch"
 table:=fmt.Sprintf("df_diff_ci_%d",time.Now().UnixNano())
 w:=&Writer{Endpoint:endpoint,Database:db,Table:table,Username:os.Getenv("DATAFLOW_INTEGRATION_CH_USER"),Password:os.Getenv("DATAFLOW_INTEGRATION_CH_PASSWORD")}
 create:=fmt.Sprintf("CREATE TABLE `%s`.`%s` (_stream_id String,_snapshot_id String,_pk String,_sysrevid String) ENGINE=MergeTree ORDER BY (_stream_id,_snapshot_id,_pk)",db,table)
 if err:=w.exec(ctx,create);err!=nil{t.Fatalf("create ClickHouse fixture: %v",err)}
 defer func(){cleanupCtx,stop:=context.WithTimeout(context.Background(),30*time.Second);defer stop();if err:=w.exec(cleanupCtx,fmt.Sprintf("DROP TABLE IF EXISTS `%s`.`%s`",db,table));err!=nil{t.Logf("cleanup failed: %v",err)}}()
 stream:="12345678-1234-1234-1234-123456789012"
 old:="12345678-1234-1234-1234-123456789013"
 newer:="12345678-1234-1234-1234-123456789014"
 pk1:="12345678-0000-0000-0000-000000000001"
 pk2:="12345678-0000-0000-0000-000000000002"
 pk3:="12345678-0000-0000-0000-000000000003"
 pk4:="12345678-0000-0000-0000-000000000004"
 version:="0x0000000000000001"
 insert:=func(snapshot string,keys []string,versions []string){
  t.Helper()
  rows:=make([]map[string]any,0,len(keys))
  for i,k:=range keys{rows=append(rows,map[string]any{"_stream_id":stream,"_snapshot_id":snapshot,"_pk":k,"_sysrevid":versions[i]})}
  if err:=w.Write(ctx,engine.Batch{Rows:rows});err!=nil{t.Fatalf("insert snapshot %s: %v",snapshot,err)}
 }
 insert(old,[]string{pk1,pk2,pk3},[]string{version,version,version})
 insert(newer,[]string{pk1,pk2,pk4},[]string{"0x0000000000000002",version,version})
 stats,err:=w.SnapshotStats(ctx,stream,newer)
 if err!=nil{t.Fatal(err)}
 if stats.Rows!=3||stats.DistinctKeys!=3{t.Fatalf("invalid stored snapshot: %+v",stats)}
 diff,err:=w.SnapshotDiff(ctx,stream,old,newer)
 if err!=nil{t.Fatalf("real ClickHouse diff query failed: %v",err)}
 if diff.Added!=1||diff.Deleted!=1||diff.Changed!=1{t.Fatalf("unexpected diff %+v; want +1/-1/~1",diff)}
}
