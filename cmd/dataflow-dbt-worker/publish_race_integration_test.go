package main

import (
 "context"
 "fmt"
 "os"
 "path/filepath"
 "sync"
 "testing"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/safetable"
 "github.com/jackc/pgx/v5/pgxpool"
)
// Two independent table runs can target the same relation; row locks alone
// are insufficient. Two real dbt worker goroutines compete across PG clients
// for one CH atomic EXCHANGE. Exactly ONE is allowed to win.
func TestRealConcurrentPublishUsesTargetScopedLock(t *testing.T) {
 dsn,endpoint:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN"),os.Getenv("DATAFLOW_INTEGRATION_CH_URL")
 if dsn==""||endpoint==""{t.Skip("needs actual isolated PG+ClickHouse")}
 if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true"{t.Fatal("non-isolated PG forbidden")}
 ctx,cancel:=context.WithTimeout(context.Background(),100*time.Second);defer cancel()
 pg,e:=pgxpool.New(ctx,dsn);if e!=nil{t.Fatal(e)};defer pg.Close()
 for _,n:=range []string{"010_transform_models.sql","012_safe_table.sql"} {
  s,err:=os.ReadFile(filepath.Join("..","..","migrations","postgres",n));if err!=nil{t.Fatal(err)}
  if _,err=pg.Exec(ctx,string(s));err!=nil{t.Fatal(err)}
 }
 client:=safetable.Client{Endpoint:endpoint,User:os.Getenv("DATAFLOW_INTEGRATION_CH_WRITER_USER"),Password:os.Getenv("DATAFLOW_INTEGRATION_CH_WRITER_PASSWORD")}
 var suffix string
 if e=pg.QueryRow(ctx,"SELECT substr(replace(gen_random_uuid()::text,'-',''),1,14)").Scan(&suffix);e!=nil{t.Fatal(e)}
 target:="dft_lock_"+suffix
 stages:=[]string{}
 clean:=func(){
  cl,cancel:=context.WithTimeout(context.Background(),20*time.Second);defer cancel()
  for _,name:=range append(stages,target){_,_=client.Query(cl,"DROP TABLE IF EXISTS scratch."+name)}
 }
 defer clean()
 mkTable:=func(name string,value int)string{
  t.Helper()
  if _,err:=client.Query(ctx,fmt.Sprintf("CREATE TABLE scratch.%s (id UInt32,amount UInt32) ENGINE=MergeTree ORDER BY id",name));err!=nil{t.Fatal(err)}
  if _,err:=client.Query(ctx,fmt.Sprintf("INSERT INTO scratch.%s VALUES(1,%d)",name,value));err!=nil{t.Fatal(err)}
  uid,err:=client.UUID(ctx,name);if err!=nil||uid==""{t.Fatalf("UUID %s: %v",name,err)}
  return uid
 }
 oldUUID:=mkTable(target,100)
 claims:=make([]tableClaim,0,2)
 for i,v:=range []int{201,202}{
  var id string
  if e=pg.QueryRow(ctx,"SELECT gen_random_uuid()::text").Scan(&id);e!=nil{t.Fatal(e)}
  stage:=safetable.StageName(id);stages=append(stages,stage)
  uuid:=mkTable(stage,v)
  name:=fmt.Sprintf("race%s%d",suffix,i)
  var modelID string
  if e=pg.QueryRow(ctx,`INSERT INTO transform_models(name,definition,compiled_sql) VALUES($1,'{}'::jsonb,'SELECT 1 AS id') RETURNING id::text`,name).Scan(&modelID);e!=nil{t.Fatal(e)}
  _,e=pg.Exec(ctx,`INSERT INTO table_runs(id,model_id,model_name,compiled_sql,source_definition,stage_name,target_name,unique_column,state,publish_prepared,stage_uuid,old_target_uuid)
   VALUES($1::uuid,$2::uuid,$3,'SELECT 1 AS id','{}'::jsonb,$4,$5,'id','publishing',true,$6,$7)`,
   id,modelID,name,stage,target,uuid,oldUUID)
  if e!=nil{t.Fatal(e)}
 }
 for i:=0;i<2;i++{
  c,err:=claimTable(ctx,pg);if err!=nil||c.State!="publishing"{t.Fatalf("claim %d %+v %v",i,c,err)}
  claims=append(claims,c)
 }
 results:=make(chan error,2)
 gate:=make(chan struct{})
 var wg sync.WaitGroup
 for _,cl:=range claims {
  c:=cl;wg.Add(1)
  go func(){
   defer wg.Done();<-gate
   results<-guardedTableOperation(ctx,pg,c,func()error{return client.Publish(ctx,c.publication())})
  }()
 }
 close(gate);wg.Wait();close(results)
 succeeded,refused:=0,0
 for err:=range results{if err==nil{succeeded++}else{refused++}}
 if succeeded!=1||refused!=1{t.Fatalf("exactly 1 safe physical exchange: winner=%d refused=%d",succeeded,refused)}
 actual,e:=client.UUID(ctx,target);if e!=nil{t.Fatal(e)}
 if actual!=claims[0].StageUUID&&actual!=claims[1].StageUUID{t.Fatalf("wrong physical target UUID %s",actual)}
 raw,e:=client.Query(ctx,"SELECT amount FROM scratch."+target+" FORMAT TabSeparatedRaw")
 if e!=nil||(raw!="201"&&raw!="202"){t.Fatalf("wrong persisted value %q %v",raw,e)}
 // Re-invoke BOTH old snapshots: neither can reverse already published UUID.
 for _,c:=range claims{
  if err:=guardedTableOperation(ctx,pg,c,func()error{return client.Publish(ctx,c.publication())});err==nil&&c.StageUUID!=actual{
   t.Fatal("loser replay unexpectedly swapped physical target")
  }
 }
 after,e:=client.UUID(ctx,target);if e!=nil||after!=actual{t.Fatal("replay mutated published physical UUID")}
 t.Logf("CORE_RUN6B_REAL_PUBLISH_RACE_PASS target=%s one_winner=1 rejected=1 immutable_UUID=%s",target,actual)
}
