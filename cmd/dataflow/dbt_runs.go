package main

import(
 "context"
 "encoding/json"
 "net/http"
 "os"
 "strings"
 "time"
 "github.com/dinhvansh/dataflow-engine/internal/transform"
 "github.com/dinhvansh/dataflow-engine/internal/lineage"
 "github.com/jackc/pgx/v5"
)
func dbtPreviewEnabled()bool{
 return os.Getenv("DATAFLOW_ALLOW_TEST_LOAD")=="true" &&
  os.Getenv("DATAFLOW_ENABLE_DBT_TEST")=="true" &&
  os.Getenv("DATAFLOW_ENABLE_SQL_JOBS_TEST")=="true" &&
  os.Getenv("DATAFLOW_TEST_TARGET_DATABASE")=="scratch"
}
func(s *server)dbtRunsAPI(w http.ResponseWriter,r *http.Request){
 if !s.validBearer(r){http.Error(w,"unauthorized",401);return}
 if !dbtPreviewEnabled(){http.Error(w,"dbt materialization is disabled outside isolated scratch test mode",403);return}
 ctx,stop:=context.WithTimeout(r.Context(),10*time.Second);defer stop()
 switch r.Method{
 case http.MethodPost:
  r.Body=http.MaxBytesReader(w,r.Body,4096)
  d:=json.NewDecoder(r.Body);d.DisallowUnknownFields()
  var input struct{ModelID string `json:"model_id"`;Materialization string `json:"materialization"`}
  if d.Decode(&input)!=nil||!validUUID.MatchString(input.ModelID){http.Error(w,"valid model_id required",400);return}
  if input.Materialization!="view"{http.Error(w,"only view materialization is allowed in v0.9",422);return}
  // Hold the same advisory transaction lock used by model Save so we
  // cannot queue a stale graph while another process edits dependencies.
  tx,err:=s.pg.BeginTx(ctx,pgx.TxOptions{})
  if err!=nil{http.Error(w,"dbt model registry unavailable",503);return}
  defer tx.Rollback(ctx)
  if _,err=tx.Exec(ctx,`SELECT pg_advisory_xact_lock(84721, 1)`);err!=nil{http.Error(w,"model lock unavailable",503);return}
  var raw []byte;var name string
  err=tx.QueryRow(ctx,`SELECT name,definition FROM transform_models WHERE id=$1::uuid`,input.ModelID).Scan(&name,&raw)
  if err!=nil{http.Error(w,"model not found",404);return}
  var model transform.Model
  if json.Unmarshal(raw,&model)!=nil||model.Mode!="visual"{http.Error(w,"dbt worker accepts visual models only",422);return}
  if len(name)>85||len(name)==0||name[0]<'a'||name[0]>'z'||strings.ToLower(name)!=name{http.Error(w,"model name must be lower-case and <=85 characters",422);return}
  if _,err=transform.ValidateModelName(name);err!=nil{http.Error(w,"invalid model name",422);return}
  if model.Source.Database!="scratch"||strings.EqualFold(model.Source.Table,"dfv_"+name){
   http.Error(w,"only scratch source supported; destination cannot equal source",422);return
  }
  compiled,err:=transform.Compile(model,"scratch")
  if err!=nil{http.Error(w,err.Error(),422);return}
  if len(compiled.SQL)>16000{http.Error(w,"compiled SQL too long",422);return}
  all,err:=s.readLineageModels(ctx,tx)
  if err!=nil{http.Error(w,"lineage catalog unavailable",503);return}
  graph,err:=lineage.Build(all)
  if err!=nil||lineage.CheckCycle(graph)!=nil{http.Error(w,"invalid cyclic lineage; repair dependencies before running dbt",422);return}
  known:=make(map[string]lineage.Model,len(all))
  for _,item:=range all{known[item.Name]=item}
  // Validate the ENTIRE chain, not just the immediate parent. A changed
  // ancestor that has not been rebuilt must block downstream jobs.
  checked:=map[string]bool{}
  parent:=lineage.DependsOn(lineage.Model{Name:name,Mode:model.Mode,Database:model.Source.Database,Table:model.Source.Table})
  for parent!=""{
   if checked[parent]{http.Error(w,"cyclic lineage",422);return}
   checked[parent]=true
   item,exists:=known[parent]
   if !exists{http.Error(w,"upstream model "+parent+" is missing: save and build it first",409);return}
   var currentSQL string
   if err=tx.QueryRow(ctx,`SELECT compiled_sql FROM transform_models WHERE name=$1`,parent).Scan(&currentSQL);err!=nil{http.Error(w,"upstream SQL not found",409);return}
   var latestStatus,latestSQL string
   err=tx.QueryRow(ctx,`SELECT status,compiled_sql FROM dbt_runs
    WHERE model_name=$1 ORDER BY created_at DESC,id DESC LIMIT 1`,parent).Scan(&latestStatus,&latestSQL)
   if err!=nil||latestStatus!="succeeded"||latestSQL!=currentSQL{
    http.Error(w,"upstream model "+parent+" needs a successful, up-to-date Build View",409);return
   }
   parent=lineage.DependsOn(item)
  }
  var id string
  err=tx.QueryRow(ctx,`INSERT INTO dbt_runs(model_id,model_name,materialization,target_database,target_relation,source_definition,compiled_sql)
   VALUES($1::uuid,$2,'view','scratch',$3,$4::jsonb,$5) RETURNING id::text`,
   input.ModelID,name,"dfv_"+name,string(raw),compiled.SQL).Scan(&id)
  if err!=nil{http.Error(w,"cannot queue: active model run exists, or migration 011 missing",409);return}
  if err=tx.Commit(ctx);err!=nil{http.Error(w,"dbt queue transaction failed",503);return}
  jsonResponse(w,202,map[string]any{"run_id":id,"status":"queued","target":"scratch.dfv_"+name,"materialization":"view"})
 case http.MethodGet:
  id:=r.URL.Query().Get("id")
  if id!=""{
   if !validUUID.MatchString(id){http.Error(w,"invalid run ID",400);return}
   var result struct{
    RunID string `json:"run_id"`;ModelName string `json:"model_name"`
    Status string `json:"status"`;Target string `json:"target"`
    Attempts int `json:"attempts"`;Error string `json:"error"`
    LogTail string `json:"log_tail"`;Details json.RawMessage `json:"run_results"`
   }
   err:=s.pg.QueryRow(ctx,`SELECT id::text,model_name,status,target_database||'.'||target_relation,
     attempt_count,COALESCE(error_message,''),COALESCE(log_tail,''),COALESCE(run_results,'{}'::jsonb)
     FROM dbt_runs WHERE id=$1::uuid`,id).Scan(&result.RunID,&result.ModelName,&result.Status,&result.Target,&result.Attempts,&result.Error,&result.LogTail,&result.Details)
   if err!=nil{http.Error(w,"run not found",404);return}
   jsonResponse(w,200,result);return
  }
  rows,err:=s.pg.Query(ctx,`SELECT id::text,model_name,status,target_database||'.'||target_relation,
    attempt_count,COALESCE(error_message,'') FROM dbt_runs ORDER BY created_at DESC LIMIT 50`)
  if err!=nil{http.Error(w,"dbt registry unavailable; apply migration 011",503);return}
  defer rows.Close()
  list:=make([]map[string]any,0)
  for rows.Next(){var id,name,status,target,msg string;var attempts int
   if rows.Scan(&id,&name,&status,&target,&attempts,&msg)!=nil{http.Error(w,"invalid run metadata",500);return}
   list=append(list,map[string]any{"run_id":id,"model_name":name,"status":status,"target":target,"attempts":attempts,"error":msg})
  }
  if rows.Err()!=nil{http.Error(w,"run query failed",500);return}
  jsonResponse(w,200,map[string]any{"runs":list})
 default:http.Error(w,"method not allowed",405)
 }
}

