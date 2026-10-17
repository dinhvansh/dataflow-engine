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

// Live scratch-only regression: A->missing->still missing must confirm the
// delete, while missing->reappeared must cancel any pending candidate.
func TestHardDeleteThreeGenerationSnapshotIntegration(t *testing.T){
 endpoint:=os.Getenv("DATAFLOW_INTEGRATION_CH_URL")
 if endpoint==""{t.Skip("disposable ClickHouse required")}
 ctx,cancel:=context.WithTimeout(context.Background(),2*time.Minute);defer cancel()
 table:=fmt.Sprintf("df_harddelete_qa_%d",time.Now().UnixNano())
 w:=&Writer{Endpoint:endpoint,Database:"scratch",Table:table,Username:os.Getenv("DATAFLOW_INTEGRATION_CH_USER"),Password:os.Getenv("DATAFLOW_INTEGRATION_CH_PASSWORD")}
 if err:=w.exec(ctx,"CREATE TABLE scratch."+table+" (_stream_id String,_snapshot_id String,_pk String,_sysrevid String) ENGINE=MergeTree ORDER BY (_stream_id,_snapshot_id,_pk)");err!=nil{t.Fatal(err)}
 defer func(){cleanup,done:=context.WithTimeout(context.Background(),20*time.Second);defer done();_=w.exec(cleanup,"DROP TABLE IF EXISTS scratch."+table)}()
 const stream="12345678-1234-1234-1234-123456789012"
 const a="12345678-1234-1234-1234-123456789013"
 const b="12345678-1234-1234-1234-123456789014"
 const c="12345678-1234-1234-1234-123456789015"
 const d="12345678-1234-1234-1234-123456789016"
 const kept="12345678-0000-0000-0000-000000000001"
 const missing="12345678-0000-0000-0000-000000000002"
 appendSnapshot:=func(id string,keys ...string){
  t.Helper()
  rows:=make([]map[string]any,0,len(keys))
  for _,k:=range keys{rows=append(rows,map[string]any{"_stream_id":stream,"_snapshot_id":id,"_pk":k,"_sysrevid":"0x0000000000000001"})}
  if err:=w.Write(ctx,engine.Batch{Rows:rows});err!=nil{t.Fatal(err)}
 }
 appendSnapshot(a,kept,missing)
 appendSnapshot(b,kept)
 appendSnapshot(c,kept)
 appendSnapshot(d,kept,missing)
 assertKeys:=func(label string,actual []string,err error,want ...string){
  t.Helper()
  if err!=nil||len(actual)!=len(want){t.Fatalf("%s: keys=%v want=%v err=%v",label,actual,want,err)}
  for i:=range want{if actual[i]!=want[i]{t.Fatalf("%s: keys=%v want=%v",label,actual,want)}}
 }
 candidate,e:=w.MissingSnapshotKeys(ctx,stream,a,b);assertKeys("first omission",candidate,e,missing)
 newMissing,e:=w.MissingSnapshotKeys(ctx,stream,b,c);assertKeys("second omission is not NEW deletion",newMissing,e)
 stillMissing,e:=w.AddedSnapshotKeys(ctx,stream,b,c);assertKeys("second omission is not reappearance",stillMissing,e)
 confirmed,e:=w.ConfirmedMissingSnapshotKeys(ctx,stream,a,b,c);assertKeys("confirmed after two omissions",confirmed,e,missing)
 reappeared,e:=w.AddedSnapshotKeys(ctx,stream,c,d);assertKeys("reappeared in fourth generation",reappeared,e,missing)
 noDelete,e:=w.ConfirmedMissingSnapshotKeys(ctx,stream,a,b,d);assertKeys("reappearance cannot be confirmed",noDelete,e)
}
