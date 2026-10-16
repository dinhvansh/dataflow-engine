package main

import(
 "context"
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "testing"
 "time"
 "github.com/dinhvansh/dataflow-engine/internal/dbtworker"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)
func TestDBTQueueIntegration(t *testing.T){
 dsn:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
 if dsn==""{t.Skip("requires isolated PostgreSQL")}
 if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true"{t.Fatal("refusing non-isolated database")}
 ctx,cancel:=context.WithTimeout(context.Background(),45*time.Second);defer cancel()
 pool,err:=pgxpool.New(ctx,dsn);if err!=nil{t.Fatal(err)};defer pool.Close()
 for _,m:=range []string{"010_transform_models.sql","011_dbt_runs.sql"}{
  schema,e:=os.ReadFile(filepath.Join("..","..","migrations","postgres",m));if e!=nil{t.Fatal(e)}
  if _,e=pool.Exec(ctx,string(schema));e!=nil{t.Fatal(e)}
 }
 if err:=ensureSchema(ctx,pool);err!=nil{t.Fatal(err)}
 var modelID string
 err=pool.QueryRow(ctx,`INSERT INTO transform_models(name,definition,compiled_sql)
 VALUES('dbt_ci_model','{"name":"dbt_ci_model"}'::jsonb,'SELECT 1') RETURNING id::text`).Scan(&modelID)
 if err!=nil{t.Fatal(err)}
 queue:=func()string{
  t.Helper();var id string
  if e:=pool.QueryRow(ctx,`INSERT INTO dbt_runs(model_id,model_name,target_database,target_relation,materialization,source_definition,compiled_sql)
 VALUES($1::uuid,'dbt_ci_model','scratch','dfv_dbt_ci_model','view','{}'::jsonb,'SELECT 1') RETURNING id::text`,modelID).Scan(&id);e!=nil{t.Fatal(e)}
  return id
 }
 runID:=queue()
 if _,err=pool.Exec(ctx,`INSERT INTO dbt_runs(model_id,model_name,target_database,target_relation,materialization,source_definition,compiled_sql)
 VALUES($1::uuid,'dbt_ci_model','scratch','dfv_dbt_ci_model','view','{}'::jsonb,'SELECT 1')`,modelID);err==nil{t.Fatal("duplicate active model accepted")}
 first,err:=claimNext(ctx,pool);if err!=nil{t.Fatal(err)}
 if first.ID!=runID{t.Fatalf("unexpected claim %+v",first)}
 if _,err=claimNext(ctx,pool);!errors.Is(err,pgx.ErrNoRows){t.Fatal("double claim permitted")}
 if _,err=pool.Exec(ctx,`UPDATE dbt_runs SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1::uuid`,runID);err!=nil{t.Fatal(err)}
 second,err:=claimNext(ctx,pool);if err!=nil{t.Fatal(err)}
 if second.Fence<=first.Fence||first.Owner==second.Owner||second.Attempts!=2{t.Fatal("recovery fencing failed")}
 ok:=dbtworker.Outcome{Success:true,Results:json.RawMessage(`{"results":[{"status":"success"}]}`),Log:"success"}
 if err=finish(ctx,pool,first,ok,nil);err==nil{t.Fatal("stale worker finalized")}
 if err=finish(ctx,pool,second,ok,nil);err!=nil{t.Fatal(err)}
 var status string
 if err=pool.QueryRow(ctx,"SELECT status FROM dbt_runs WHERE id=$1::uuid",runID).Scan(&status);err!=nil||status!="succeeded"{t.Fatalf("run: %s %v",status,err)}
 exhausted:=queue()
 for n:=1;n<=2;n++{
  claimed,e:=claimNext(ctx,pool);if e!=nil||claimed.ID!=exhausted||claimed.Attempts!=n{t.Fatalf("retry %d %+v %v",n,claimed,e)}
  if _,err=pool.Exec(ctx,`UPDATE dbt_runs SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1::uuid`,exhausted);err!=nil{t.Fatal(err)}
 }
 if _,err=claimNext(ctx,pool);!errors.Is(err,pgx.ErrNoRows){t.Fatal("retry limit bypassed")}
 if err=pool.QueryRow(ctx,"SELECT status FROM dbt_runs WHERE id=$1::uuid",exhausted).Scan(&status);err!=nil||status!="failed"{t.Fatalf("exhausted status=%s %v",status,err)}
}
