package main

import(
 "context"
 "encoding/json"
 "net/http"
 "os"
 "regexp"
 "strings"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/safetable"
 "github.com/dinhvansh/dataflow-engine/internal/transform"
)
var safeDQColumn=regexp.MustCompile("^[A-Za-z_][A-Za-z0-9_]*$")
func safeTableAPIEnabled()bool{
 return os.Getenv("DATAFLOW_ALLOW_TEST_LOAD")=="true"&&
 os.Getenv("DATAFLOW_ENABLE_DBT_TEST")=="true"&&
 os.Getenv("DATAFLOW_ENABLE_SAFE_TABLE_TEST")=="true"&&
 os.Getenv("DATAFLOW_TEST_TARGET_DATABASE")=="scratch"
}
func(s *server)tableRunsAPI(w http.ResponseWriter,r *http.Request){
 if !s.validBearer(r){http.Error(w,"unauthorized",401);return}
 if !safeTableAPIEnabled(){http.Error(w,"safe table is disabled outside isolated TEST scratch mode",403);return}
 ctx,stop:=context.WithTimeout(r.Context(),10*time.Second);defer stop()
 switch r.Method{
 case http.MethodPost:
  r.Body=http.MaxBytesReader(w,r.Body,4096)
  var input struct{ModelID string `json:"model_id"`;UniqueColumn string `json:"unique_column"`}
  d:=json.NewDecoder(r.Body);d.DisallowUnknownFields()
  if d.Decode(&input)!=nil||!validUUID.MatchString(input.ModelID){http.Error(w,"invalid model_id",400);return}
  if input.UniqueColumn!=""&&!safeDQColumn.MatchString(input.UniqueColumn){http.Error(w,"invalid unique column",422);return}
  tx,err:=s.pg.Begin(ctx);if err!=nil{http.Error(w,"database unavailable",503);return}
  defer tx.Rollback(ctx)
  if _,err=tx.Exec(ctx,"SELECT pg_advisory_xact_lock(84721, 1)");err!=nil{http.Error(w,"lock unavailable",503);return}
  var name string;var data []byte
  err=tx.QueryRow(ctx,`SELECT name,definition FROM transform_models WHERE id=$1::uuid`,input.ModelID).Scan(&name,&data)
  if err!=nil{http.Error(w,"model not found",404);return}
  if len(name)>80||name==""||name[0]<'a'||name[0]>'z'||strings.ToLower(name)!=name{
   http.Error(w,"model name must begin lowercase and contain only safe identifiers",422);return
  }
  if _,err=transform.ValidateModelName(name);err!=nil{http.Error(w,"unsafe model name",422);return}
  var model transform.Model
  if json.Unmarshal(data,&model)!=nil||model.Mode!="visual"{
   http.Error(w,"only saved Visual transformations can be materialized",422);return
  }
  if model.Source.Database!="scratch"||
   strings.HasPrefix(model.Source.Table,"dfv_")||strings.HasPrefix(model.Source.Table,"dft_"){
   http.Error(w,"v1.1 safe table accepts direct scratch raw source only (no downstream model dependencies yet)",422);return
  }
  compiled,err:=transform.Compile(model,"scratch");if err!=nil{http.Error(w,err.Error(),422);return}
  if len(compiled.SQL)>16000{http.Error(w,"SQL too large",422);return}
  var id string
  if err=tx.QueryRow(ctx,"SELECT gen_random_uuid()::text").Scan(&id);err!=nil{http.Error(w,"cannot allocate run id",503);return}
  stage:=safetable.StageName(id)
  if err=safetable.SafeNames(name,stage);err!=nil{http.Error(w,"unsafe stage name",422);return}
  _,err=tx.Exec(ctx,`INSERT INTO table_runs(id,model_id,model_name,compiled_sql,source_definition,
    stage_name,target_name,unique_column)
    VALUES($1::uuid,$2::uuid,$3,$4,$5::jsonb,$6,$7,NULLIF($8,''))`,
    id,input.ModelID,name,compiled.SQL,string(data),stage,safetable.TargetName(name),input.UniqueColumn)
  if err!=nil{http.Error(w,"active table job exists or migration 012 missing",409);return}
  if err=tx.Commit(ctx);err!=nil{http.Error(w,"table queue transaction failed",503);return}
  jsonResponse(w,202,map[string]any{"run_id":id,"state":"queued","stage":"scratch."+stage,"target":"scratch."+safetable.TargetName(name)})
 case http.MethodGet:
  id:=r.URL.Query().Get("id")
  if id!=""&&!validUUID.MatchString(id){http.Error(w,"invalid id",400);return}
  query:=`SELECT id::text,model_name,state,stage_name,target_name,
    COALESCE(rows_verified,0),COALESCE(error_message,''),COALESCE(old_target_uuid,''),
    attempt_count,COALESCE(log_tail,'')
    FROM table_runs`
  args:=[]any{}
  if id!=""{query+=" WHERE id=$1::uuid";args=append(args,id)}
  query+=" ORDER BY created_at DESC LIMIT 50"
  rows,err:=s.pg.Query(ctx,query,args...);if err!=nil{http.Error(w,"table registry unavailable; apply migration 012",503);return}
  defer rows.Close()
  items:=make([]map[string]any,0)
  for rows.Next(){
   var id,name,status,stage,target,issue,old,logTail string;var count int64;var attempts int
   if rows.Scan(&id,&name,&status,&stage,&target,&count,&issue,&old,&attempts,&logTail)!=nil{http.Error(w,"table metadata invalid",500);return}
   items=append(items,map[string]any{"run_id":id,"model_name":name,"state":status,
    "stage":"scratch."+stage,"target":"scratch."+target,"rows":count,
    "error":issue,"has_backup":old!="","attempts":attempts,"log_tail":logTail})
  }
  if rows.Err()!=nil{http.Error(w,"table registry read failed",500);return}
  jsonResponse(w,200,map[string]any{"runs":items})
 default:http.Error(w,"method not allowed",405)
 }
}
func(s *server)tableActionAPI(w http.ResponseWriter,r *http.Request){
 if !s.validBearer(r){http.Error(w,"unauthorized",401);return}
 if !safeTableAPIEnabled(){http.Error(w,"safe table disabled",403);return}
 if r.Method!=http.MethodPost{http.Error(w,"method not allowed",405);return}
 r.Body=http.MaxBytesReader(w,r.Body,4096)
 d:=json.NewDecoder(r.Body);d.DisallowUnknownFields()
 var input struct{RunID string `json:"run_id"`;Action string `json:"action"`;Confirm string `json:"confirm"`}
 if d.Decode(&input)!=nil||!validUUID.MatchString(input.RunID){http.Error(w,"invalid request",400);return}
 if input.Action!="publish"&&input.Action!="rollback"{http.Error(w,"unsupported action",422);return}
 if (input.Action=="publish"&&input.Confirm!="PUBLISH")||(input.Action=="rollback"&&input.Confirm!="ROLLBACK"){
  http.Error(w,"explicit confirmation required",422);return
 }
 ctx,stop:=context.WithTimeout(r.Context(),10*time.Second);defer stop()
 tx,err:=s.pg.Begin(ctx);if err!=nil{http.Error(w,"database unavailable",503);return}
 defer tx.Rollback(ctx)
 if _,err=tx.Exec(ctx,"SELECT pg_advisory_xact_lock(84721, 1)");err!=nil{http.Error(w,"lock unavailable",503);return}
 var status,modelID,sql,old,target string;var count int64
 err=tx.QueryRow(ctx,`SELECT state,model_id::text,compiled_sql,
 COALESCE(old_target_uuid,''),target_name,COALESCE(rows_verified,0)
 FROM table_runs WHERE id=$1::uuid FOR UPDATE`,input.RunID).
 Scan(&status,&modelID,&sql,&old,&target,&count)
 if err!=nil{http.Error(w,"run not found",404);return}
 next:=""
 if input.Action=="publish"{
  if status!="ready"||count<1{http.Error(w,"table not validated/ready for publish",409);return}
  var currentSQL string
  if err=tx.QueryRow(ctx,`SELECT compiled_sql FROM transform_models WHERE id=$1::uuid`,modelID).Scan(&currentSQL);err!=nil||currentSQL!=sql{
   http.Error(w,"saved model SQL changed since staging; rebuild first",409);return
  }
  next="publishing"
 }else{
  if status!="published"||old==""{http.Error(w,"no previously published backup available for rollback",409);return}
  var latestID string
  if err=tx.QueryRow(ctx,`SELECT id::text FROM table_runs
    WHERE target_name=$1 AND state='published' ORDER BY created_at DESC,id DESC LIMIT 1`,target).Scan(&latestID);err!=nil||latestID!=input.RunID{
   http.Error(w,"another table publication superseded this backup",409);return
  }
  next="rolling_back"
 }
 tag,err:=tx.Exec(ctx,`UPDATE table_runs
 SET state=$2,attempt_count=0,lease_owner=NULL,lease_until=NULL,
 error_message=NULL WHERE id=$1::uuid AND state=$3`,input.RunID,next,status)
 if err!=nil||tag.RowsAffected()!=1{http.Error(w,"table state changed",409);return}
 if err=tx.Commit(ctx);err!=nil{http.Error(w,"cannot queue action",503);return}
 jsonResponse(w,202,map[string]any{"run_id":input.RunID,"state":next})
}
