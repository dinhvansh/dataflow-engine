package main

import (
 "context"
 "encoding/json"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/jobs/dag"
 "github.com/dinhvansh/dataflow-engine/internal/jobs/executor"
 "github.com/jackc/pgx/v5/pgxpool"
)

// SQL DAG integration requires a disposable Postgres database AND a running
// ClickHouse scratch database. The CI job supplies these explicitly.
func TestSQLDAGIntegration(t *testing.T){
 pgDSN:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
 chURL:=os.Getenv("DATAFLOW_INTEGRATION_CH_URL")
 if pgDSN==""||chURL==""{t.Skip("requires disposable PostgreSQL + ClickHouse")}
 if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true"{t.Fatal("refusing to mutate PostgreSQL without isolated test database marker")}
 ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second);defer cancel()
 pool,err:=pgxpool.New(ctx,pgDSN);if err!=nil{t.Fatal(err)};defer pool.Close()
 schema,err:=os.ReadFile(filepath.Join("..","..","migrations","postgres","007_sql_dag.sql"));if err!=nil{t.Fatal(err)}
 for _,statement:=range strings.Split(string(schema),";"){
  if strings.TrimSpace(statement)==""{continue}
  if _,err=pool.Exec(ctx,statement);err!=nil{t.Fatalf("migration 007: %v",err)}
 }
 t.Setenv("DATAFLOW_TEST_TARGET_DATABASE","scratch")
 t.Setenv("DATAFLOW_ALLOW_TEST_LOAD","true")
 t.Setenv("DATAFLOW_ENABLE_SQL_JOBS_TEST","true")
 t.Setenv("DATAFLOW_CH_READONLY_USER",os.Getenv("DATAFLOW_INTEGRATION_CH_USER"))
 t.Setenv("DATAFLOW_CH_READONLY_PASSWORD",os.Getenv("DATAFLOW_INTEGRATION_CH_PASSWORD"))
 s:=&server{pg:pool,chURL:chURL,managedSlots:make(chan struct{},2)}
 var id1,id2,dagID string
 if err=pool.QueryRow(ctx,"INSERT INTO sql_jobs(name,database_name,query_text,enabled) VALUES('ci_first','scratch','SELECT 1 AS answer',true) RETURNING id::text").Scan(&id1);err!=nil{t.Fatal(err)}
 if err=pool.QueryRow(ctx,"INSERT INTO sql_jobs(name,database_name,query_text,enabled) VALUES('ci_second','scratch','SELECT 2 AS answer',true) RETURNING id::text").Scan(&id2);err!=nil{t.Fatal(err)}
 plan:=dag.Plan{Nodes:[]dag.Node{{ID:"first",JobID:id1},{ID:"second",JobID:id2,Requires:[]string{"first"}}}}
 encoded,err:=json.Marshal(plan);if err!=nil{t.Fatal(err)}
 if err=pool.QueryRow(ctx,"INSERT INTO dag_definitions(name,definition,enabled) VALUES('ci_pipeline',$1::jsonb,true) RETURNING id::text",string(encoded)).Scan(&dagID);err!=nil{t.Fatal(err)}
 runID,err:=s.queueSQLDag(ctx,dagID);if err!=nil{t.Fatal(err)}
 // Mutate registry between queue and execution: immutable run snapshot wins.
 if _,err=pool.Exec(ctx,"UPDATE sql_jobs SET query_text='SELECT imaginary_missing_function()' WHERE id=$1::uuid",id1);err!=nil{t.Fatal(err)}
 s.executeSQLDag(runID)
 var status string;var raw []byte
 if err=pool.QueryRow(ctx,"SELECT status,result FROM dag_runs WHERE id=$1::uuid",runID).Scan(&status,&raw);err!=nil{t.Fatal(err)}
 if status!="succeeded"{t.Fatalf("DAG did not succeed: %s %s",status,raw)}
 var result executor.Result
 if err=json.Unmarshal(raw,&result);err!=nil{t.Fatal(err)}
 if result.State["first"]!=dag.Succeeded||result.State["second"]!=dag.Succeeded{t.Fatalf("DAG results %+v",result)}
 if result.Attempts["first"]!=1||result.Attempts["second"]!=1{t.Fatalf("unexpected retries: %+v",result.Attempts)}
}
