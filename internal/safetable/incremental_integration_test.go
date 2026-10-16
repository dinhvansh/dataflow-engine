package safetable
import("context";"fmt";"os";"testing";"time")
func TestDeltaUpdateDeleteReplayIntegration(t *testing.T){
 endpoint:=os.Getenv("DATAFLOW_INTEGRATION_CH_URL");if endpoint==""{t.Skip("real ClickHouse only")}
 ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second);defer cancel()
 client:=Client{Endpoint:endpoint,User:os.Getenv("DATAFLOW_INTEGRATION_CH_USER"),Password:os.Getenv("DATAFLOW_INTEGRATION_CH_PASSWORD")}
 base:=fmt.Sprintf("inc%d",time.Now().UnixNano())
 target:=TargetName(base);delta:="dfd_"+base;stage:=StageName("12345678-1234-1234-1234-1234567890ab")
 for _,n:=range []string{target,delta,stage}{_,_=client.Query(ctx,"DROP TABLE IF EXISTS scratch."+n)}
 defer func(){for _,n:=range []string{target,delta,stage}{_,_=client.Query(context.Background(),"DROP TABLE IF EXISTS scratch."+n)}}()
 if _,err:=client.Query(ctx,"CREATE TABLE scratch."+target+" (ID String, Revenue UInt64) ENGINE=MergeTree ORDER BY ID");err!=nil{t.Fatal(err)}
 if _,err:=client.Query(ctx,"INSERT INTO scratch."+target+" VALUES ('A',10),('B',20),('C',30)");err!=nil{t.Fatal(err)}
 if _,err:=client.Query(ctx,"CREATE TABLE scratch."+delta+" (ID String, Revenue UInt64, _df_version UInt64, _df_deleted UInt8) ENGINE=MergeTree ORDER BY (ID,_df_version)");err!=nil{t.Fatal(err)}
 if _,err:=client.Query(ctx,"INSERT INTO scratch."+delta+" VALUES ('A',99,1,0),('B',20,2,1),('D',40,3,0),('A',100,4,0)");err!=nil{t.Fatal(err)}
 uuid,err:=client.UUID(ctx,target);if err!=nil{t.Fatal(err)}
 p:=DeltaPlan{Target:target,Delta:delta,Stage:stage,Key:"ID",Lower:0,Upper:4,BaselineUUID:uuid}
 stats,err:=client.BuildDeltaStage(ctx,p);if err!=nil{t.Fatal(err)}
 if stats.Rows!=3||stats.Events!=4{t.Fatalf("wrong counts %+v",stats)}
 actual,err:=client.Query(ctx,"SELECT ID,Revenue FROM scratch."+stage+" ORDER BY ID FORMAT TabSeparatedRaw")
 if err!=nil||actual!="A\t100\nC\t30\nD\t40"{t.Fatalf("upsert/delete wrong %q %v",actual,err)}
 // Replay build after a crash must not duplicate the copied data.
 if _,err=client.BuildDeltaStage(ctx,p);err!=nil{t.Fatal(err)}
 pub:=Publication{Target:target,Stage:stage,StageUUID:"",OldUUID:uuid}
 pub.StageUUID,err=client.UUID(ctx,stage);if err!=nil{t.Fatal(err)}
 if err=client.Publish(ctx,pub);err!=nil{t.Fatal(err)}
 if err=client.Publish(ctx,pub);err!=nil{t.Fatal(err)}
 actual,err=client.Query(ctx,"SELECT ID,Revenue FROM scratch."+target+" ORDER BY ID FORMAT TabSeparatedRaw")
 if err!=nil||actual!="A\t100\nC\t30\nD\t40"{t.Fatalf("publish result mismatch %q %v",actual,err)}
 if err=client.Rollback(ctx,pub);err!=nil{t.Fatal(err)}
 actual,err=client.Query(ctx,"SELECT ID,Revenue FROM scratch."+target+" ORDER BY ID FORMAT TabSeparatedRaw")
 if err!=nil||actual!="A\t10\nB\t20\nC\t30"{t.Fatalf("rollback mismatch %q %v",actual,err)}
}
func TestDeltaRejectDuplicateVersionIntegration(t *testing.T){
 endpoint:=os.Getenv("DATAFLOW_INTEGRATION_CH_URL");if endpoint==""{t.Skip("real ClickHouse only")}
 ctx,cancel:=context.WithTimeout(context.Background(),60*time.Second);defer cancel()
 cli:=Client{Endpoint:endpoint,User:os.Getenv("DATAFLOW_INTEGRATION_CH_USER"),Password:os.Getenv("DATAFLOW_INTEGRATION_CH_PASSWORD")}
 base:=fmt.Sprintf("dup%d",time.Now().UnixNano());target:=TargetName(base);delta:="dfd_"+base
 defer func(){for _,n:=range []string{target,delta}{_,_=cli.Query(context.Background(),"DROP TABLE IF EXISTS scratch."+n)}}()
 _,err:=cli.Query(ctx,"CREATE TABLE scratch."+target+" (ID String, Revenue UInt64) ENGINE=MergeTree ORDER BY ID");if err!=nil{t.Fatal(err)}
 _,err=cli.Query(ctx,"CREATE TABLE scratch."+delta+" (ID String, Revenue UInt64,_df_version UInt64,_df_deleted UInt8) ENGINE=MergeTree ORDER BY ID");if err!=nil{t.Fatal(err)}
 _,err=cli.Query(ctx,"INSERT INTO scratch."+delta+" VALUES ('A',1,7,0),('A',2,7,0)");if err!=nil{t.Fatal(err)}
 if _,err=cli.DeltaStats(ctx,DeltaPlan{Target:target,Delta:delta,Key:"ID",Lower:0,Upper:7});err==nil{t.Fatal("duplicate key/version must be rejected")}
}
