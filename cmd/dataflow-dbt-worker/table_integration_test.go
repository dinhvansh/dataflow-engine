package main
import(
 "context"
 "fmt"
 "os"
 "path/filepath"
 "testing"
 "time"
 "github.com/dinhvansh/dataflow-engine/internal/dbtworker"
 "github.com/dinhvansh/dataflow-engine/internal/safetable"
 "github.com/jackc/pgx/v5/pgxpool"
)
func TestSafeTableFullIntegration(t *testing.T){
 dsn:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
 bin:=os.Getenv("DATAFLOW_INTEGRATION_DBT_BIN")
 endpoint:=os.Getenv("DATAFLOW_INTEGRATION_CH_URL")
 if dsn==""||bin==""||endpoint==""{t.Skip("requires actual isolated Postgres+ClickHouse+dbt")}
 if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true"{t.Fatal("not a disposable database")}
 ctx,cancel:=context.WithTimeout(context.Background(),4*time.Minute);defer cancel()
 pg,err:=pgxpool.New(ctx,dsn);if err!=nil{t.Fatal(err)};defer pg.Close()
 for _,file:=range []string{"010_transform_models.sql","011_dbt_runs.sql","012_safe_table.sql"}{
  raw,e:=os.ReadFile(filepath.Join("..","..","migrations","postgres",file));if e!=nil{t.Fatal(e)}
  if _,e=pg.Exec(ctx,string(raw));e!=nil{t.Fatalf("migration %s: %v",file,e)}
 }
 t.Setenv("DATAFLOW_ALLOW_TEST_LOAD","true");t.Setenv("DATAFLOW_ENABLE_DBT_TEST","true")
 t.Setenv("DATAFLOW_ENABLE_SAFE_TABLE_TEST","true");t.Setenv("DATAFLOW_TEST_TARGET_DATABASE","scratch")
 if err=checkTableSchema(ctx,pg);err!=nil{t.Fatal(err)}
 config:=dbtworker.Config{Binary:bin,Host:"127.0.0.1",Port:8123,Database:"scratch",Username:os.Getenv("DATAFLOW_INTEGRATION_CH_WRITER_USER"),Password:os.Getenv("DATAFLOW_INTEGRATION_CH_WRITER_PASSWORD")}
 client:=tableClickhouse(config)
 name:=fmt.Sprintf("tc%d",time.Now().UnixNano())
 target:=safetable.TargetName(name)
 var modelID string
 if err=pg.QueryRow(ctx,`INSERT INTO transform_models(name,definition,compiled_sql) VALUES($1,'{}'::jsonb,'SELECT 101 AS amount') RETURNING id::text`,name).Scan(&modelID);err!=nil{t.Fatal(err)}
 queue:=func(value int)string{
  t.Helper()
  var id string
  if e:=pg.QueryRow(ctx,"SELECT gen_random_uuid()::text").Scan(&id);e!=nil{t.Fatal(e)}
  sql:=fmt.Sprintf("SELECT 1 AS id, %d AS amount",value)
  _,e:=pg.Exec(ctx,`INSERT INTO table_runs(id,model_id,model_name,compiled_sql,source_definition,stage_name,target_name,unique_column)
  VALUES($1::uuid,$2::uuid,$3,$4,'{}'::jsonb,$5,$6,'id')`,id,modelID,name,sql,safetable.StageName(id),target)
  if e!=nil{t.Fatal(e)}
  return id
 }
 getState:=func(id string)string{
  t.Helper();var state string
  if e:=pg.QueryRow(ctx,"SELECT state FROM table_runs WHERE id=$1::uuid",id).Scan(&state);e!=nil{t.Fatal(e)}
  return state
 }
 first:=queue(101)
 claim,err:=claimTable(ctx,pg);if err!=nil||claim.ID!=first{t.Fatalf("first claim %+v %v",claim,err)}
 runTableClaim(ctx,pg,claim,config,client)
 if getState(first)!="ready"{t.Fatalf("expected staging ready, got %s",getState(first))}
 if targetUUID,_:=client.UUID(ctx,target);targetUUID!=""{t.Fatalf("target changed before approval %s",targetUUID)}
 if _,err=pg.Exec(ctx,"UPDATE table_runs SET state='publishing',attempt_count=0 WHERE id=$1::uuid",first);err!=nil{t.Fatal(err)}
 claim,err=claimTable(ctx,pg);if err!=nil||claim.State!="publishing"{t.Fatal(claim,err)}
 runTableClaim(ctx,pg,claim,config,client)
 if getState(first)!="published"{t.Fatalf("not published: %s",getState(first))}
 value,err:=client.Query(ctx,"SELECT amount FROM scratch."+target+" FORMAT TabSeparatedRaw")
 if err!=nil||value!="101"{t.Fatalf("initial table incorrect %s %v",value,err)}
 second:=queue(202)
 claim,err=claimTable(ctx,pg);if err!=nil||claim.ID!=second{t.Fatal(claim,err)}
 runTableClaim(ctx,pg,claim,config,client)
 if getState(second)!="ready"{t.Fatalf("second staging failed %s",getState(second))}
 // Before approval, the published result must still equal the last revision.
 value,err=client.Query(ctx,"SELECT amount FROM scratch."+target+" FORMAT TabSeparatedRaw")
 if err!=nil||value!="101"{t.Fatalf("unapproved build overwrote data %s %v",value,err)}
 if _,err=pg.Exec(ctx,"UPDATE table_runs SET state='publishing',attempt_count=0 WHERE id=$1::uuid",second);err!=nil{t.Fatal(err)}
 claim,err=claimTable(ctx,pg);if err!=nil{t.Fatal(err)}
 runTableClaim(ctx,pg,claim,config,client)
 if getState(second)!="published"{t.Fatalf("second publish failed %s",getState(second))}
 value,err=client.Query(ctx,"SELECT amount FROM scratch."+target+" FORMAT TabSeparatedRaw")
 if err!=nil||value!="202"{t.Fatalf("second table incorrect %s %v",value,err)}
 // Retry the exact EXCHANGE after PG committed must not revert the table.
 var stageUUID,oldUUID,stage string
 if err=pg.QueryRow(ctx,`SELECT stage_uuid,old_target_uuid,stage_name FROM table_runs WHERE id=$1::uuid`,second).Scan(&stageUUID,&oldUUID,&stage);err!=nil{t.Fatal(err)}
 if err=client.Publish(ctx,safetable.Publication{Target:target,Stage:stage,StageUUID:stageUUID,OldUUID:oldUUID});err!=nil{t.Fatal(err)}
 value,err=client.Query(ctx,"SELECT amount FROM scratch."+target+" FORMAT TabSeparatedRaw")
 if err!=nil||value!="202"{t.Fatalf("replay reversed publish %s %v",value,err)}
 if _,err=pg.Exec(ctx,"UPDATE table_runs SET state='rolling_back',attempt_count=0 WHERE id=$1::uuid",second);err!=nil{t.Fatal(err)}
 claim,err=claimTable(ctx,pg);if err!=nil{t.Fatal(err)}
 runTableClaim(ctx,pg,claim,config,client)
 if getState(second)!="rolled_back"{t.Fatalf("rollback failed %s",getState(second))}
 value,err=client.Query(ctx,"SELECT amount FROM scratch."+target+" FORMAT TabSeparatedRaw")
 if err!=nil||value!="101"{t.Fatalf("rollback wrong %s %v",value,err)}
 // Only isolated CI ephemeral relations are cleaned up.
 _,_=client.Query(ctx,"DROP TABLE IF EXISTS scratch."+target)
 _,_=client.Query(ctx,"DROP TABLE IF EXISTS scratch."+stage)
}
