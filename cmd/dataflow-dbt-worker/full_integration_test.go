package main
import(
 "context"
 "fmt"
 "os"
 "path/filepath"
 "testing"
 "time"
 "github.com/dinhvansh/dataflow-engine/internal/connectors/clickhouse"
 "github.com/dinhvansh/dataflow-engine/internal/dbtworker"
 "github.com/jackc/pgx/v5/pgxpool"
)
func TestDBTFullQueueIntegration(t *testing.T){
 dsn:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
 endpoint:=os.Getenv("DATAFLOW_INTEGRATION_CH_URL")
 bin:=os.Getenv("DATAFLOW_INTEGRATION_DBT_BIN")
 if dsn==""||endpoint==""||bin==""{t.Skip("real PostgreSQL, ClickHouse and dbt required")}
 if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true"{t.Fatal("non-isolated database")}
 ctx,cancel:=context.WithTimeout(context.Background(),2*time.Minute);defer cancel()
 pg,err:=pgxpool.New(ctx,dsn);if err!=nil{t.Fatal(err)};defer pg.Close()
 for _,name:=range []string{"010_transform_models.sql","011_dbt_runs.sql"}{
  schema,e:=os.ReadFile(filepath.Join("..","..","migrations","postgres",name));if e!=nil{t.Fatal(e)}
  if _,e=pg.Exec(ctx,string(schema));e!=nil{t.Fatal(e)}
 }
 name:=fmt.Sprintf("ci_endtoend_%d",time.Now().UnixNano())
 var modelID string
 if err=pg.QueryRow(ctx,`INSERT INTO transform_models(name,definition,compiled_sql)
 VALUES($1,'{}'::jsonb,'SELECT 42 AS Answer') RETURNING id::text`,name).Scan(&modelID);err!=nil{t.Fatal(err)}
 var id string
 target:="dfv_"+name
 if err=pg.QueryRow(ctx,`INSERT INTO dbt_runs(model_id,model_name,target_database,target_relation,source_definition,compiled_sql)
 VALUES($1::uuid,$2,'scratch',$3,'{}'::jsonb,'SELECT 42 AS Answer') RETURNING id::text`,modelID,name,target).Scan(&id);err!=nil{t.Fatal(err)}
 claimed,err:=claimNext(ctx,pg);if err!=nil||claimed.ID!=id{t.Fatalf("claim %+v %v",claimed,err)}
 cfg:=dbtworker.Config{Binary:bin,Host:"127.0.0.1",Port:8123,Database:"scratch",Username:os.Getenv("DATAFLOW_INTEGRATION_CH_WRITER_USER"),Password:os.Getenv("DATAFLOW_INTEGRATION_CH_WRITER_PASSWORD")}
 executeClaim(ctx,pg,claimed,cfg)
 var status string;var detail []byte
 if err=pg.QueryRow(ctx,"SELECT status,run_results FROM dbt_runs WHERE id=$1::uuid",id).Scan(&status,&detail);err!=nil{t.Fatal(err)}
 if status!="succeeded"||len(detail)<20{t.Fatalf("materialization failed: %s %s",status,detail)}
 reader:=clickhouse.QueryJob{Endpoint:endpoint,Database:"scratch",Username:cfg.Username,Password:cfg.Password}
 result,err:=reader.Execute(ctx,"SELECT Answer FROM scratch."+target)
 if err!=nil||result.Rows!=1{t.Fatalf("view not published %+v %v",result,err)}
 t.Log("PASS dbt queue -> worker -> materialized ClickHouse View -> PostgreSQL terminal status")
}
