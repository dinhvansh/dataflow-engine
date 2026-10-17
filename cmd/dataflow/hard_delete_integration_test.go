package main

import(
 "context"
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "github.com/jackc/pgx/v5/pgxpool"
)

// Only disposable PostgreSQL is modified. ClickHouse is mocked to prove that
// a historical run can NEVER emit a delete after a newer baseline exists.
func TestHardDeleteRejectsStaleSnapshotIntegration(t *testing.T){
 dsn:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
 if dsn==""{t.Skip("isolated PostgreSQL required")}
 if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true"{t.Fatal("disposable PostgreSQL required")}
 ctx,cancel:=context.WithTimeout(context.Background(),60*time.Second);defer cancel()
 pg,err:=pgxpool.New(ctx,dsn);if err!=nil{t.Fatal(err)};defer pg.Close()
 for _,name:=range []string{"001_bootstrap.sql","002_sync_checkpoint.sql","006_reconciliation.sql","016_hard_delete_reconciliation.sql"}{
  raw,e:=os.ReadFile(filepath.Join("..","..","migrations","postgres",name));if e!=nil{t.Fatal(e)}
  if _,e=pg.Exec(ctx,string(raw));e!=nil{t.Fatalf("%s: %v",name,e)}
 }
 source:=fmt.Sprintf("hard_delete_review_%d",time.Now().UnixNano())
 var stream,oldRun,newRun string
 err=pg.QueryRow(ctx,`INSERT INTO sync_streams(source_schema,source_table,target_database,target_table)
 VALUES('Erp',$1,'scratch','dataflow_raw_events') RETURNING id::text`,source).Scan(&stream)
 if err!=nil{t.Fatal(err)}
 defer func(){
  clean,done:=context.WithTimeout(context.Background(),10*time.Second);defer done()
  _,_=pg.Exec(clean,"DELETE FROM reconcile_delete_candidates WHERE stream_id=$1::uuid",stream)
  _,_=pg.Exec(clean,"DELETE FROM reconcile_delete_sequences WHERE stream_id=$1::uuid",stream)
  _,_=pg.Exec(clean,"DELETE FROM reconcile_baselines WHERE stream_id=$1::uuid",stream)
  _,_=pg.Exec(clean,"DELETE FROM reconcile_runs WHERE stream_id=$1::uuid",stream)
  _,_=pg.Exec(clean,"DELETE FROM sync_streams WHERE id=$1::uuid",stream)
 }()
 for _,target:=range []*string{&oldRun,&newRun}{
  if err=pg.QueryRow(ctx,`INSERT INTO reconcile_runs(stream_id,status,scanned_rows,verified_rows)
   VALUES($1::uuid,'succeeded',0,0) RETURNING id::text`,stream).Scan(target);err!=nil{t.Fatal(err)}
 }
 if _,err=pg.Exec(ctx,`INSERT INTO reconcile_baselines(stream_id,snapshot_id,generation) VALUES($1::uuid,$2::uuid,2)`,stream,newRun);err!=nil{t.Fatal(err)}
 var inserts int
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  q:=r.URL.Query().Get("query")
  if strings.Contains(q,"INSERT INTO"){inserts++}
  if strings.Contains(q,"max("){fmt.Fprint(w,"0\n")}
 }))
 defer server.Close()
 svc:=&server{pg:pg,chURL:server.URL}
 if _,err=svc.emitHardDeleteTombstones(ctx,oldRun);err==nil||!strings.Contains(err.Error(),"stale delete confirmation"){
  t.Fatalf("stale baseline was not rejected: %v",err)
 }
 if inserts!=0{t.Fatalf("stale run emitted %d tombstone INSERTs",inserts)}
 if count,err:=svc.emitHardDeleteTombstones(ctx,newRun);err!=nil||count!=0{
  t.Fatalf("latest empty baseline should be idempotent: count=%d err=%v",count,err)
 }
}
