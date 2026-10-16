package main

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "log"
 "net/http"
 "os"
 "strings"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/connectors/clickhouse"
 "github.com/dinhvansh/dataflow-engine/internal/jobs/dag"
 "github.com/dinhvansh/dataflow-engine/internal/jobs/executor"
 "github.com/jackc/pgx/v5"
)

type sqlJob struct{
 ID string `json:"id"`
 Name string `json:"name"`
 Database string `json:"database"`
 Query string `json:"query"`
 Enabled bool `json:"enabled"`
}
type sqlDAG struct{
 ID string `json:"id"`
 Name string `json:"name"`
 Enabled bool `json:"enabled"`
 Nodes []dag.Node `json:"nodes"`
}
type frozenJob struct{Database string `json:"database"`;Query string `json:"query"`}
type frozenDAG struct{Plan dag.Plan `json:"plan"`;Jobs map[string]frozenJob `json:"jobs"`}
func sqlDAGEnabled()bool{
 return os.Getenv("DATAFLOW_ENABLE_SQL_JOBS_TEST")=="true" &&
        os.Getenv("DATAFLOW_CH_READONLY_USER")!="" &&
        os.Getenv("DATAFLOW_ALLOW_TEST_LOAD")=="true"
}
func requireSQLDev(w http.ResponseWriter)bool{
 if sqlDAGEnabled(){return true}
 http.Error(w,"SQL jobs disabled; configure an explicit read-only ClickHouse account and dev flags",403)
 return false
}
func(s *server)sqlJobsAPI(w http.ResponseWriter,r *http.Request){
 if !s.validBearer(r){http.Error(w,"unauthorized",401);return}
 if !requireSQLDev(w){return}
 ctx,cancel:=context.WithTimeout(r.Context(),10*time.Second);defer cancel()
 switch r.Method{
 case http.MethodGet:
  rows,err:=s.pg.Query(ctx,`SELECT id::text,name,database_name,query_text,enabled FROM sql_jobs ORDER BY name LIMIT 200`)
  if err!=nil{http.Error(w,"job registry unavailable",500);return}
  defer rows.Close()
  result:=make([]sqlJob,0)
  for rows.Next(){
   var job sqlJob
   if err=rows.Scan(&job.ID,&job.Name,&job.Database,&job.Query,&job.Enabled);err!=nil{http.Error(w,"cannot read SQL jobs",500);return}
   result=append(result,job)
  }
  if err=rows.Err();err!=nil{http.Error(w,"cannot read SQL jobs",500);return}
  jsonResponse(w,200,map[string]any{"jobs":result})
 case http.MethodPost:
  r.Body=http.MaxBytesReader(w,r.Body,20000)
  dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields()
  var job sqlJob
  if err:=dec.Decode(&job);err!=nil{http.Error(w,"invalid JSON",400);return}
  job.Name=strings.TrimSpace(job.Name)
  if len(job.Name)<1||len(job.Name)>120||job.Database!=scratchDatabase(){
   http.Error(w,"name must be 1..120 and database must match configured scratch database",400);return
  }
  if err:=clickhouse.ValidateReadQuery(job.Query);err!=nil{http.Error(w,err.Error(),400);return}
  err:=s.pg.QueryRow(ctx,`INSERT INTO sql_jobs(name,database_name,query_text,enabled)
   VALUES($1,$2,$3,$4)
   ON CONFLICT(name) DO UPDATE SET database_name=EXCLUDED.database_name,
   query_text=EXCLUDED.query_text,enabled=EXCLUDED.enabled,updated_at=now()
   RETURNING id::text`,job.Name,job.Database,job.Query,job.Enabled).Scan(&job.ID)
  if err!=nil{http.Error(w,"could not save SQL job",500);return}
  jsonResponse(w,201,job)
 default:http.Error(w,"method not allowed",405)
 }
}
func(s *server)sqlDagsAPI(w http.ResponseWriter,r *http.Request){
 if !s.validBearer(r){http.Error(w,"unauthorized",401);return}
 if !requireSQLDev(w){return}
 ctx,cancel:=context.WithTimeout(r.Context(),10*time.Second);defer cancel()
 switch r.Method{
 case http.MethodGet:
  rows,err:=s.pg.Query(ctx,`SELECT id::text,name,enabled,definition FROM dag_definitions ORDER BY name LIMIT 100`)
  if err!=nil{http.Error(w,"pipeline registry unavailable",500);return}
  defer rows.Close()
  result:=make([]sqlDAG,0)
  for rows.Next(){
   var p sqlDAG;var raw []byte
   if err=rows.Scan(&p.ID,&p.Name,&p.Enabled,&raw);err!=nil{http.Error(w,"cannot read pipeline",500);return}
   var definition dag.Plan
   if err=json.Unmarshal(raw,&definition);err!=nil{http.Error(w,"invalid stored pipeline",500);return}
   p.Nodes=definition.Nodes
   result=append(result,p)
  }
  if err=rows.Err();err!=nil{http.Error(w,"pipeline query failed",500);return}
  jsonResponse(w,200,map[string]any{"pipelines":result})
 case http.MethodPost:
  r.Body=http.MaxBytesReader(w,r.Body,65536)
  dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields()
  var definition sqlDAG
  if err:=dec.Decode(&definition);err!=nil{http.Error(w,"invalid JSON",400);return}
  definition.Name=strings.TrimSpace(definition.Name)
  if len(definition.Name)<1||len(definition.Name)>120{http.Error(w,"name must be 1..120",400);return}
  plan:=dag.Plan{Nodes:definition.Nodes}
  if err:=plan.Validate();err!=nil{http.Error(w,err.Error(),422);return}
  for _,n:=range plan.Nodes{if !validUUID.MatchString(n.JobID){http.Error(w,"job_id must be UUID",422);return}}
  data,err:=json.Marshal(plan);if err!=nil{http.Error(w,"invalid DAG",400);return}
  err=s.pg.QueryRow(ctx,`INSERT INTO dag_definitions(name,definition,enabled)
   VALUES($1,$2::jsonb,$3)
   ON CONFLICT(name) DO UPDATE SET definition=EXCLUDED.definition,enabled=EXCLUDED.enabled,updated_at=now()
   RETURNING id::text`,definition.Name,string(data),definition.Enabled).Scan(&definition.ID)
  if err!=nil{http.Error(w,"cannot save pipeline",500);return}
  jsonResponse(w,201,definition)
 default:http.Error(w,"method not allowed",405)
 }
}
func(s *server)sqlDagRunsAPI(w http.ResponseWriter,r *http.Request){
 if !s.validBearer(r){http.Error(w,"unauthorized",401);return}
 if !requireSQLDev(w){return}
 switch r.Method{
 case http.MethodPost:
  r.Body=http.MaxBytesReader(w,r.Body,4096)
  dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields()
  var input struct{DAGID string `json:"dag_id"`}
  if dec.Decode(&input)!=nil||!validUUID.MatchString(input.DAGID){http.Error(w,"valid dag_id required",400);return}
  select{case s.managedSlots<-struct{}{}:default:http.Error(w,"runner is busy",429);return}
  ctx,cancel:=context.WithTimeout(r.Context(),15*time.Second)
  runID,err:=s.queueSQLDag(ctx,input.DAGID)
  cancel()
  if err!=nil{<-s.managedSlots;http.Error(w,"pipeline not enabled or one of its SQL jobs is disabled: "+err.Error(),422);return}
  go func(){defer func(){<-s.managedSlots}();s.executeSQLDag(runID)}()
  jsonResponse(w,202,map[string]any{"run_id":runID,"status":"queued"})
 case http.MethodGet:
  ctx,cancel:=context.WithTimeout(r.Context(),10*time.Second);defer cancel()
  id:=r.URL.Query().Get("id")
  if id!=""{
   if !validUUID.MatchString(id){http.Error(w,"invalid run_id",400);return}
   var runID,dagID,status,message string;var raw []byte
   err:=s.pg.QueryRow(ctx,`SELECT id::text,dag_id::text,status,COALESCE(result,'{}'::jsonb),COALESCE(error_message,'') FROM dag_runs WHERE id=$1::uuid`,id).Scan(&runID,&dagID,&status,&raw,&message)
   if err!=nil{http.Error(w,"run not found",404);return}
   jsonResponse(w,200,map[string]any{"run_id":runID,"dag_id":dagID,"status":status,"result":json.RawMessage(raw),"error":message});return
  }
  rows,err:=s.pg.Query(ctx,`SELECT id::text,dag_id::text,status,COALESCE(error_message,'') FROM dag_runs ORDER BY created_at DESC LIMIT 50`)
  if err!=nil{http.Error(w,"cannot list pipeline runs",500);return}
  defer rows.Close()
  result:=make([]map[string]any,0)
  for rows.Next(){var id,dagID,status,msg string
   if err=rows.Scan(&id,&dagID,&status,&msg);err!=nil{http.Error(w,"cannot scan pipeline runs",500);return}
   result=append(result,map[string]any{"run_id":id,"dag_id":dagID,"status":status,"error":msg})
  }
  if err=rows.Err();err!=nil{http.Error(w,"cannot scan pipeline runs",500);return}
  jsonResponse(w,200,map[string]any{"runs":result})
 default:http.Error(w,"method not allowed",405)
 }
}
func(s *server)queueSQLDag(ctx context.Context,dagID string)(string,error){
 tx,err:=s.pg.BeginTx(ctx,pgx.TxOptions{IsoLevel:pgx.RepeatableRead})
 if err!=nil{return "",err}
 defer tx.Rollback(ctx)
 var raw []byte
 err=tx.QueryRow(ctx,`SELECT definition FROM dag_definitions WHERE id=$1::uuid AND enabled=true`,dagID).Scan(&raw)
 if err!=nil{return "",err}
 var plan dag.Plan
 if err=json.Unmarshal(raw,&plan);err!=nil{return "",err}
 if err=plan.Validate();err!=nil{return "",err}
 snapshot:=frozenDAG{Plan:plan,Jobs:make(map[string]frozenJob)}
 for _,n:=range plan.Nodes{
  if _,exists:=snapshot.Jobs[n.JobID];exists{continue}
  if !validUUID.MatchString(n.JobID){return "",errors.New("invalid job UUID")}
  var j frozenJob
  err=tx.QueryRow(ctx,`SELECT database_name,query_text FROM sql_jobs WHERE id=$1::uuid AND enabled=true`,n.JobID).Scan(&j.Database,&j.Query)
  if err!=nil{return "",fmt.Errorf("required job %s is missing/disabled: %w",n.JobID,err)}
  if j.Database!=scratchDatabase(){return "",errors.New("job targets non-scratch database")}
  if err=clickhouse.ValidateReadQuery(j.Query);err!=nil{return "",err}
  snapshot.Jobs[n.JobID]=j
 }
 frozen,err:=json.Marshal(snapshot);if err!=nil{return "",err}
 var runID string
 err=tx.QueryRow(ctx,`INSERT INTO dag_runs(dag_id,status,snapshot)
  VALUES($1::uuid,'queued',$2::jsonb) RETURNING id::text`,dagID,string(frozen)).Scan(&runID)
 if err!=nil{return "",err}
 if err=tx.Commit(ctx);err!=nil{return "",err}
 return runID,nil
}
func(s *server)executeSQLDag(runID string){
 ctx,cancel:=context.WithTimeout(context.Background(),30*time.Minute)
 defer cancel()
 var snapshotRaw []byte
 var err error
 err=s.pg.QueryRow(ctx,`UPDATE dag_runs SET status='running',started_at=now()
  WHERE id=$1::uuid AND status='queued' RETURNING snapshot`,runID).Scan(&snapshotRaw)
 if err!=nil{log.Printf("sql dag %s failed to start: %v",runID,err);return}
 var frozen frozenDAG
 if err=json.Unmarshal(snapshotRaw,&frozen);err!=nil{
  log.Printf("sql dag %s corrupt snapshot: %v",runID,err);return
 }
 opts:=executor.Options{Concurrency:2,Retries:1,Timeout:35*time.Second}
 result,execErr:=executor.Run(ctx,frozen.Plan,func(stepCtx context.Context,jobID string)error{
  job,ok:=frozen.Jobs[jobID];if !ok{return fmt.Errorf("missing frozen job %s",jobID)}
  reader:=clickhouse.QueryJob{Endpoint:s.chURL,Database:job.Database,
   Username:os.Getenv("DATAFLOW_CH_READONLY_USER"),
   Password:os.Getenv("DATAFLOW_CH_READONLY_PASSWORD")}
  _,err:=reader.Execute(stepCtx,job.Query)
  return err
 },opts)
 status:="succeeded";message:=""
 if execErr!=nil{status="failed";message=execErr.Error()}
 body,e:=json.Marshal(result);if e!=nil{log.Printf("SQL DAG result marshal: %v",e);return}
 persistedCtx,stop:=context.WithTimeout(context.Background(),10*time.Second);defer stop()
 _,err=s.pg.Exec(persistedCtx,`UPDATE dag_runs SET status=$2,result=$3::jsonb,error_message=$4,finished_at=now()
   WHERE id=$1::uuid`,runID,status,string(body),message)
 if err!=nil{log.Printf("SQL DAG %s persist failure: %v",runID,err)}
}
