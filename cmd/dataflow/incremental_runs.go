package main

import(
 "context"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "os"
 "regexp"
 "strconv"
 "strings"
 "time"
 "github.com/dinhvansh/dataflow-engine/internal/safetable"
)
var incrementalName=regexp.MustCompile("^[a-z][a-z0-9_]{0,75}$")
var incrementalKey=regexp.MustCompile("^[A-Za-z_][A-Za-z0-9_]*$")
func incrementalEnabled()bool{
 return os.Getenv("DATAFLOW_ALLOW_TEST_LOAD")=="true"&&
 os.Getenv("DATAFLOW_ENABLE_DBT_TEST")=="true"&&
 os.Getenv("DATAFLOW_ENABLE_SAFE_TABLE_TEST")=="true"&&
 os.Getenv("DATAFLOW_ENABLE_INCREMENTAL_TEST")=="true"&&
 os.Getenv("DATAFLOW_TEST_TARGET_DATABASE")=="scratch"
}
func(s *server)incrementalRunsAPI(w http.ResponseWriter,r *http.Request){
 if !s.validBearer(r){http.Error(w,"unauthorized",401);return}
 if !incrementalEnabled(){http.Error(w,"incremental disabled outside isolated scratch TEST",403);return}
 ctx,stop:=context.WithTimeout(r.Context(),10*time.Second);defer stop()
 switch r.Method{
 case http.MethodPost:
  r.Body=http.MaxBytesReader(w,r.Body,4096)
  d:=json.NewDecoder(r.Body);d.DisallowUnknownFields()
  var input struct{Target string `json:"target"`;DeltaTable string `json:"delta_table"`;Key string `json:"key"`;InitialCursor string `json:"initial_cursor"`}
  if d.Decode(&input)!=nil{http.Error(w,"invalid JSON",400);return}
  var extra any
  if e:=d.Decode(&extra);!errors.Is(e,io.EOF){http.Error(w,"only one JSON object allowed",400);return}
  if !strings.HasPrefix(input.Target,"dft_")||!incrementalName.MatchString(input.Target)||
   !incrementalName.MatchString(input.DeltaTable)||!incrementalKey.MatchString(input.Key){
   http.Error(w,"invalid scratch target, delta table or key",422);return
  }
  if input.InitialCursor==""{input.InitialCursor="0"}
  if _,err:=strconv.ParseUint(input.InitialCursor,10,64);err!=nil{http.Error(w,"initial cursor must be a UInt64 decimal",422);return}
  tx,err:=s.pg.Begin(ctx);if err!=nil{http.Error(w,"database unavailable",503);return}
  defer tx.Rollback(ctx)
  if _,err=tx.Exec(ctx,`SELECT pg_advisory_xact_lock(84721,2)`);err!=nil{http.Error(w,"target registry lock unavailable",503);return}
  // Registration captures the CDC starting version exactly once. Retrying
  // requests cannot silently reset an existing target watermark.
  _,err=tx.Exec(ctx,`INSERT INTO incremental_targets(target_name,delta_table,key_column,cursor_value)
   VALUES($1,$2,$3,$4::numeric) ON CONFLICT(target_name) DO NOTHING`,
   input.Target,input.DeltaTable,input.Key,input.InitialCursor)
  if err!=nil{http.Error(w,"missing migration 014",503);return}
  var delta,key,lower string
  err=tx.QueryRow(ctx,`SELECT delta_table,key_column,cursor_value::text
   FROM incremental_targets WHERE target_name=$1 FOR UPDATE`,input.Target).Scan(&delta,&key,&lower)
  if err!=nil{http.Error(w,"target registration unavailable",503);return}
  if delta!=input.DeltaTable||key!=input.Key{
   http.Error(w,"target already registered with different delta/key; do not reset checkpoint",409);return
  }
  id:=""
  err=tx.QueryRow(ctx,`SELECT gen_random_uuid()::text`).Scan(&id)
  if err!=nil{http.Error(w,"run ID unavailable",503);return}
  stage:=safetable.StageName(id)
  _,err=tx.Exec(ctx,`INSERT INTO incremental_runs(id,target_name,delta_table,key_column,stage_name,cursor_lower)
   VALUES($1::uuid,$2,$3,$4,$5,$6::numeric)`,id,input.Target,delta,key,stage,lower)
  if err!=nil{http.Error(w,"an incremental run is active or migration 014 is missing",409);return}
  if err=tx.Commit(ctx);err!=nil{http.Error(w,"incremental enqueue commit failed",503);return}
  jsonResponse(w,202,map[string]any{"run_id":id,"state":"queued","target":"scratch."+input.Target,"cursor_lower":lower})
 case http.MethodGet:
  id:=r.URL.Query().Get("id")
  if id!=""&&!validUUID.MatchString(id){http.Error(w,"invalid run ID",400);return}
  query:=`SELECT r.id::text,r.target_name,r.delta_table,r.key_column,r.state,
   r.cursor_lower::text,COALESCE(r.cursor_upper::text,''),COALESCE(r.delta_rows,0),
   COALESCE(r.rows_verified,0),COALESCE(r.error_message,''),COALESCE(r.old_target_uuid,'')
   FROM incremental_runs r`
  args:=[]any{}
  if id!=""{query+=" WHERE r.id=$1::uuid";args=append(args,id)}
  query+=" ORDER BY r.created_at DESC LIMIT 50"
  rows,err:=s.pg.Query(ctx,query,args...)
  if err!=nil{http.Error(w,"incremental registry missing; apply migration 014",503);return}
  defer rows.Close()
  result:=make([]map[string]any,0)
  for rows.Next(){
   var id,target,delta,key,state,lower,upper,issue,backup string
   var changes,verified int64
   if err=rows.Scan(&id,&target,&delta,&key,&state,&lower,&upper,&changes,&verified,&issue,&backup);err!=nil{http.Error(w,"incremental metadata corrupt",500);return}
   result=append(result,map[string]any{
    "run_id":id,"target":"scratch."+target,"delta_table":"scratch."+delta,"key":key,
    "state":state,"cursor_lower":lower,"cursor_upper":upper,"delta_rows":changes,
    "rows_verified":verified,"error":issue,"has_backup":backup!=""})
  }
  if rows.Err()!=nil{http.Error(w,"incremental query failed",503);return}
  jsonResponse(w,200,map[string]any{"runs":result})
 default:http.Error(w,"method not allowed",405)
 }
}
func(s *server)incrementalActionAPI(w http.ResponseWriter,r *http.Request){
 if !s.validBearer(r){http.Error(w,"unauthorized",401);return}
 if !incrementalEnabled(){http.Error(w,"incremental disabled outside scratch TEST",403);return}
 if r.Method!=http.MethodPost{http.Error(w,"method not allowed",405);return}
 r.Body=http.MaxBytesReader(w,r.Body,4096)
 d:=json.NewDecoder(r.Body);d.DisallowUnknownFields()
 var input struct{RunID string `json:"run_id"`;Action string `json:"action"`;Confirm string `json:"confirm"`}
 if d.Decode(&input)!=nil||!validUUID.MatchString(input.RunID){http.Error(w,"invalid run",400);return}
 if input.Action!="publish"&&input.Action!="rollback"{http.Error(w,"action must be publish or rollback",422);return}
 if input.Confirm!=strings.ToUpper(input.Action){http.Error(w,"explicit confirmation required",422);return}
 ctx,stop:=context.WithTimeout(r.Context(),10*time.Second);defer stop()
 tx,err:=s.pg.Begin(ctx);if err!=nil{http.Error(w,"database unavailable",503);return}
 defer tx.Rollback(ctx)
 if _,err=tx.Exec(ctx,`SELECT pg_advisory_xact_lock(84721,2)`);err!=nil{http.Error(w,"incremental lock unavailable",503);return}
 var state,target,lower,upper,backup string
 var rowsVerified int64
 err=tx.QueryRow(ctx,`SELECT state,target_name,cursor_lower::text,
 COALESCE(cursor_upper::text,''),COALESCE(old_target_uuid,''),COALESCE(rows_verified,0)
 FROM incremental_runs WHERE id=$1::uuid FOR UPDATE`,input.RunID).
 Scan(&state,&target,&lower,&upper,&backup,&rowsVerified)
 if err!=nil{http.Error(w,"run not found",404);return}
 var cursor,lastID string
 err=tx.QueryRow(ctx,`SELECT cursor_value::text,COALESCE(latest_run_id::text,'')
 FROM incremental_targets WHERE target_name=$1 FOR UPDATE`,target).Scan(&cursor,&lastID)
 if err!=nil{http.Error(w,"target checkpoint missing",503);return}
 next:=""
 if input.Action=="publish"{
  if state!="ready"||upper==""||cursor!=lower{http.Error(w,"stage is not ready or cursor changed",409);return}
  // Empty output is allowed for tombstones; a DQ-ready stage must have its
  // checked row count persisted (even zero).
  if rowsVerified<0{http.Error(w,"invalid DQ count",409);return}
  next="publishing"
 }else{
  if state!="published"||backup==""||lastID!=input.RunID||cursor!=upper{
   http.Error(w,"rollback unavailable: no matching latest published version",409);return
  }
  var competing int
  err=tx.QueryRow(ctx,`SELECT count(*) FROM incremental_runs
   WHERE target_name=$1 AND id<>$2::uuid AND state IN ('queued','building','ready','publishing','rolling_back')`,
   target,input.RunID).Scan(&competing)
  if err!=nil||competing!=0{http.Error(w,"another increment is active",409);return}
  next="rolling_back"
 }
 tag,err:=tx.Exec(ctx,`UPDATE incremental_runs SET state=$2,attempt_count=0,
 lease_owner=NULL,lease_until=NULL,error_message=NULL
 WHERE id=$1::uuid AND state=$3`,input.RunID,next,state)
 if err!=nil||tag.RowsAffected()!=1{http.Error(w,"run state changed",409);return}
 if err=tx.Commit(ctx);err!=nil{http.Error(w,"action commit failed",503);return}
 jsonResponse(w,202,map[string]any{"run_id":input.RunID,"state":next})
}
