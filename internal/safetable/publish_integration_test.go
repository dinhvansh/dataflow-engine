package safetable

import(
 "context"
 "fmt"
 "os"
 "testing"
 "time"
)
func TestAtomicPublishAndRollbackIntegration(t *testing.T){
 endpoint:=os.Getenv("DATAFLOW_INTEGRATION_CH_URL")
 if endpoint==""{t.Skip("requires disposable real ClickHouse")}
 ctx,stop:=context.WithTimeout(context.Background(),90*time.Second);defer stop()
 client:=Client{Endpoint:endpoint,User:os.Getenv("DATAFLOW_INTEGRATION_CH_USER"),Password:os.Getenv("DATAFLOW_INTEGRATION_CH_PASSWORD")}
 stamp:=time.Now().UnixNano()
 model:=fmt.Sprintf("safe%d",stamp)
 target:=TargetName(model)
 stage1:=StageName("00000000-0000-0000-0000-000000000001")
 stage2:=StageName("00000000-0000-0000-0000-000000000002")
 // Isolated ClickHouse CI container, clean up fixed stage IDs before use.
 for _,name:=range []string{target,stage1,stage2}{
  if _,err:=client.Query(ctx,"DROP TABLE IF EXISTS scratch."+name);err!=nil{t.Fatal(err)}
 }
 defer func(){
  for _,name:=range []string{target,stage1,stage2}{_,_=client.Query(ctx,"DROP TABLE IF EXISTS scratch."+name)}
 }()
 create:=func(name string,amount int) {
  t.Helper()
  if _,err:=client.Query(ctx,fmt.Sprintf("CREATE TABLE scratch.%s (id UInt32, revenue UInt32) ENGINE=MergeTree ORDER BY id",name));err!=nil{t.Fatal(err)}
  if _,err:=client.Query(ctx,fmt.Sprintf("INSERT INTO scratch.%s VALUES (1,%d)",name,amount));err!=nil{t.Fatal(err)}
 }
 create(stage1,100)
 firstUUID,err:=client.UUID(ctx,stage1);if err!=nil||firstUUID==""{t.Fatal(firstUUID,err)}
 if _,err=client.ValidateStage(ctx,stage1,"id");err!=nil{t.Fatal(err)}
 p1:=Publication{Target:target,Stage:stage1,StageUUID:firstUUID}
 if err=client.Publish(ctx,p1);err!=nil{t.Fatal(err)}
 if err=client.Publish(ctx,p1);err!=nil{t.Fatalf("initial replay must be idempotent: %v",err)}
 create(stage2,200)
 nextUUID,err:=client.UUID(ctx,stage2);if err!=nil{t.Fatal(err)}
 p2:=Publication{Target:target,Stage:stage2,StageUUID:nextUUID,OldUUID:firstUUID}
 if err=client.Publish(ctx,p2);err!=nil{t.Fatal(err)}
 if err=client.Publish(ctx,p2);err!=nil{t.Fatalf("retry after ambiguous exchange switched back: %v",err)}
 value,err:=client.Query(ctx,"SELECT revenue FROM scratch."+target+" FORMAT TabSeparatedRaw")
 if err!=nil||value!="200"{t.Fatalf("published rows corrupted %s %v",value,err)}
 old,err:=client.Query(ctx,"SELECT revenue FROM scratch."+stage2+" FORMAT TabSeparatedRaw")
 if err!=nil||old!="100"{t.Fatalf("backup lost %s %v",old,err)}
 if err=client.Rollback(ctx,p2);err!=nil{t.Fatal(err)}
 if err=client.Rollback(ctx,p2);err!=nil{t.Fatalf("rollback replay should be idempotent: %v",err)}
 value,err=client.Query(ctx,"SELECT revenue FROM scratch."+target+" FORMAT TabSeparatedRaw")
 if err!=nil||value!="100"{t.Fatalf("rollback failed %s %v",value,err)}
 // Last old UUID does not match current target if a new external publication
 // changed it. No destructive exchange may occur.
 if err=client.Publish(ctx,p2);err!=nil{t.Fatal(err)}
 if err=client.Rollback(ctx,Publication{Target:target,Stage:stage2,StageUUID:nextUUID,OldUUID:"wrong-uuid"});err==nil{t.Fatal("wrong backup UUID accepted")}
 if err=client.Rollback(ctx,p1);err==nil{t.Fatal("initial no-backup rollback accepted")}
 // Row-count QA rejects accidentally empty and duplicate-key stages.
 bad:=StageName("00000000-0000-0000-0000-000000000003")
 defer func(){_,_=client.Query(ctx,"DROP TABLE IF EXISTS scratch."+bad)}()
 if _,err=client.Query(ctx,"CREATE TABLE scratch."+bad+" (id UInt32, revenue UInt32) ENGINE=MergeTree ORDER BY id");err!=nil{t.Fatal(err)}
 if _,err=client.ValidateStage(ctx,bad,"id");err==nil{t.Fatal("empty stage passed DQ")}
 if _,err=client.Query(ctx,"INSERT INTO scratch."+bad+" VALUES (1,10),(1,20)");err!=nil{t.Fatal(err)}
 if _,err=client.ValidateStage(ctx,bad,"id");err==nil{t.Fatal("duplicate key passed DQ")}
}
