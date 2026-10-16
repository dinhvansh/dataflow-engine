package main

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "log"
 "net/http"
 "os"
 "strconv"
 "strings"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/lineage"
 "github.com/dinhvansh/dataflow-engine/internal/safetable"
 "github.com/dinhvansh/dataflow-engine/internal/transform"
 "github.com/jackc/pgx/v5"
)

func autoBuildEnabled()bool{
 return dbtPreviewEnabled()&&os.Getenv("DATAFLOW_ENABLE_AUTO_BUILD_TEST")=="true"
}
func(s *server)checkFlowSchema(ctx context.Context)error{
 if !autoBuildEnabled(){return nil}
 rows,err:=s.pg.Query(ctx,`SELECT id,current_step FROM model_flow_runs LIMIT 0`)
 if err!=nil{return fmt.Errorf("auto-build requires migration 013: %w",err)}
 rows.Close()
 if err=rows.Err();err!=nil{return err}
 rows,err=s.pg.Query(ctx,`SELECT flow_id,step_index,dbt_run_id FROM model_flow_steps LIMIT 0`)
 if err!=nil{return fmt.Errorf("auto-build steps require migration 013: %w",err)}
 rows.Close()
 return rows.Err()
}
func(s *server)autoBuildAPI(w http.ResponseWriter,r *http.Request){
 if !s.validBearer(r){http.Error(w,"unauthorized",401);return}
 if !autoBuildEnabled(){http.Error(w,"auto-build is disabled outside scratch TEST",403);return}
 ctx,cancel:=context.WithTimeout(r.Context(),10*time.Second);defer cancel()
 switch r.Method{
 case http.MethodPost:
  r.Body=http.MaxBytesReader(w,r.Body,4096)
  d:=json.NewDecoder(r.Body);d.DisallowUnknownFields()
  var input struct{ModelID string `json:"model_id"`}
  if err:=d.Decode(&input);err!=nil||!validUUID.MatchString(input.ModelID){http.Error(w,"model_id must be a UUID",400);return}
  var extra any
  if err:=d.Decode(&extra);!errors.Is(err,io.EOF){http.Error(w,"only one JSON object allowed",400);return}
  tx,err:=s.pg.Begin(ctx);if err!=nil{http.Error(w,"database unavailable",503);return}
  defer tx.Rollback(ctx)
  if _,err=tx.Exec(ctx,`SELECT pg_advisory_xact_lock(84721,1)`);err!=nil{http.Error(w,"model catalog locked",503);return}
  models,err:=s.readLineageModels(ctx,tx)
  if err!=nil{http.Error(w,"cannot read saved models",503);return}
  target:=""
  for _,model:=range models{if model.ID==input.ModelID{target=model.Name;break}}
  if target==""{http.Error(w,"selected model not found",404);return}
  chain,err:=lineage.PlanChain(models,target)
  if err!=nil{http.Error(w,err.Error(),422);return}
  if len(chain)>30{http.Error(w,"maximum 30 models in auto-build chain",422);return}
  if len(chain)==0{http.Error(w,"empty model chain",422);return}
  type snapshot struct{m lineage.Model;definition []byte;sql string}
  planned:=make([]snapshot,0,len(chain))
  for _,node:=range chain{
   if len(node.Name)>85||len(node.Name)==0||node.Name[0]<'a'||node.Name[0]>'z'||strings.ToLower(node.Name)!=node.Name{
    http.Error(w,"only safe lower-case dbt model names are supported",422);return
   }
   if _,err=transform.ValidateModelName(node.Name);err!=nil{http.Error(w,"invalid model name",422);return}
   var data []byte;var storedSQL string
   err=tx.QueryRow(ctx,`SELECT definition,compiled_sql FROM transform_models WHERE id=$1::uuid`,node.ID).Scan(&data,&storedSQL)
   if err!=nil{http.Error(w,"saved model metadata unavailable",503);return}
   var model transform.Model
   if err=json.Unmarshal(data,&model);err!=nil||model.Mode!="visual"{http.Error(w,"Visual model required",422);return}
   if model.Source.Database!="scratch"||model.Source.Table==""{http.Error(w,"scratch source required",422);return}
   compiled,e:=transform.Compile(model,"scratch")
   if e!=nil||compiled.SQL!=storedSQL{
    http.Error(w,"model SQL snapshot out of date; save the model first",409);return
   }
   if model.Name!=node.Name{http.Error(w,"saved model name mismatch",409);return}
   planned=append(planned,snapshot{node,data,storedSQL})
  }
  var flowID string
  err=tx.QueryRow(ctx,`INSERT INTO model_flow_runs(target_model_id,target_model_name,step_count)
   VALUES($1::uuid,$2,$3) RETURNING id::text`,input.ModelID,target,len(planned)).Scan(&flowID)
  if err!=nil{http.Error(w,"another model flow is active, or migration 013 is missing",409);return}
  for i,step:=range planned{
   _,err=tx.Exec(ctx,`INSERT INTO model_flow_steps(
    flow_id,step_index,model_id,model_name,source_definition,compiled_sql)
    VALUES($1::uuid,$2,$3::uuid,$4,$5::jsonb,$6)`,
    flowID,i,step.m.ID,step.m.Name,string(step.definition),step.sql)
   if err!=nil{http.Error(w,"failed to save flow snapshot",503);return}
  }
  if err=tx.Commit(ctx);err!=nil{http.Error(w,"flow enqueue commit failed",503);return}
  names:=make([]string,0,len(planned));for _,step:=range planned{names=append(names,step.m.Name)}
  jsonResponse(w,202,map[string]any{"run_id":flowID,"status":"queued","target_model":target,"order":names})
 case http.MethodGet:
  id:=r.URL.Query().Get("id")
  if id==""{http.Error(w,"run id required",400);return}
  if !validUUID.MatchString(id){http.Error(w,"invalid run id",400);return}
  var status,target,message string;var current,total int
  err:=s.pg.QueryRow(ctx,`SELECT status,target_model_name,current_step,step_count,
    COALESCE(error_message,'') FROM model_flow_runs WHERE id=$1::uuid`,id).
    Scan(&status,&target,&current,&total,&message)
  if err!=nil{http.Error(w,"flow run not found",404);return}
  rows,err:=s.pg.Query(ctx,`SELECT step_index,model_name,status,COALESCE(dbt_run_id::text,''),
    COALESCE(rows_verified,0),COALESCE(error_message,'')
    FROM model_flow_steps WHERE flow_id=$1::uuid ORDER BY step_index`,id)
  if err!=nil{http.Error(w,"flow steps unavailable",503);return}
  defer rows.Close()
  steps:=make([]map[string]any,0)
  for rows.Next(){
   var idx int;var name,stepStatus,dbtID,issue string;var count int64
   if err=rows.Scan(&idx,&name,&stepStatus,&dbtID,&count,&issue);err!=nil{http.Error(w,"flow metadata corrupt",500);return}
   steps=append(steps,map[string]any{"index":idx,"model_name":name,"status":stepStatus,
    "dbt_run_id":dbtID,"rows_verified":count,"error":issue})
  }
  if rows.Err()!=nil{http.Error(w,"flow status unavailable",503);return}
  jsonResponse(w,200,map[string]any{"run_id":id,"status":status,"target_model":target,
   "current_step":current,"step_count":total,"error":message,"steps":steps})
 default:http.Error(w,"method not allowed",405)
 }
}

// reconcileOneFlow performs one short transactional state transition.
// A crash cannot orphan the current dbt job: its UUID is committed in the
// same transaction as the step status. The existing durable dbt worker
// performs the actual materialization and owns its own recovery/fencing.
func(s *server)reconcileOneFlow(ctx context.Context)error{
 tx,err:=s.pg.Begin(ctx);if err!=nil{return err}
 defer tx.Rollback(ctx)
 var id,status string
 var current,total int
 err=tx.QueryRow(ctx,`SELECT id::text,status,current_step,step_count FROM model_flow_runs
  WHERE status IN ('queued','running') ORDER BY created_at,id
  LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&id,&status,&current,&total)
 if err!=nil{return err}
 if current>=total{return errors.New("flow progress exceeds plan")}
 var modelID,name,sqlText string
 var definition []byte
 var childID *string
 err=tx.QueryRow(ctx,`SELECT model_id::text,model_name,compiled_sql,source_definition,
  dbt_run_id::text FROM model_flow_steps WHERE flow_id=$1::uuid AND step_index=$2 FOR UPDATE`,
  id,current).Scan(&modelID,&name,&sqlText,&definition,&childID)
 if err!=nil{return err}
 fail:=func(message string)error{
  if len(message)>800{message=message[:800]}
  if _,e:=tx.Exec(ctx,`UPDATE model_flow_steps SET status='failed',error_message=$3
   WHERE flow_id=$1::uuid AND step_index=$2`,id,current,message);e!=nil{return e}
  if _,e:=tx.Exec(ctx,`UPDATE model_flow_runs SET status='failed',error_message=$2,
   finished_at=clock_timestamp() WHERE id=$1::uuid`,id,message);e!=nil{return e}
  return tx.Commit(ctx)
 }
 // Validate every already-completed ancestor and the current model
 // against the original flow plan. This prevents a successful flow from
 // silently combining stale upstream Views with newly edited definitions.
 rows,err:=tx.Query(ctx,`SELECT f.model_name,f.compiled_sql,m.compiled_sql
  FROM model_flow_steps f JOIN transform_models m ON m.id=f.model_id
  WHERE f.flow_id=$1::uuid AND f.step_index<=$2 ORDER BY f.step_index`,id,current)
 if err!=nil{return err}
 changed:=""
 for rows.Next(){
  var stepName,snapshotSQL,currentSQL string
  if err=rows.Scan(&stepName,&snapshotSQL,&currentSQL);err!=nil{break}
  if snapshotSQL!=currentSQL{changed=stepName;break}
 }
 scanErr:=rows.Err()
 rows.Close()
 if err!=nil||scanErr!=nil{return fail("cannot validate saved model snapshots")}
 if changed!=""{return fail("model "+changed+" changed during Auto Build; restart a new flow")}
 // Explicitly track missing models as failed, not indefinitely blocked.
 var modelExists bool
 err=tx.QueryRow(ctx,`SELECT EXISTS(SELECT 1 FROM transform_models WHERE id=$1::uuid)`,modelID).Scan(&modelExists)
 if err!=nil||!modelExists{return fail("current model no longer exists")}
 if childID==nil{
  // A manual dbt job can already be operating on this model. Wait rather
  // than inserting conflicting jobs or stealing its worker lease.
  var busyID string
  e:=tx.QueryRow(ctx,`SELECT id::text FROM dbt_runs
    WHERE model_id=$1::uuid AND status IN ('queued','running') LIMIT 1`,modelID).Scan(&busyID)
  if e==nil{return tx.Commit(ctx)}
  if !errors.Is(e,pgx.ErrNoRows){return e}
  var queued string
  err=tx.QueryRow(ctx,`INSERT INTO dbt_runs(model_id,model_name,materialization,
    target_database,target_relation,source_definition,compiled_sql)
    VALUES($1::uuid,$2,'view','scratch',$3,$4::jsonb,$5) RETURNING id::text`,
    modelID,name,"dfv_"+name,string(definition),sqlText).Scan(&queued)
  if err!=nil{return err}
  if _,err=tx.Exec(ctx,`UPDATE model_flow_steps SET dbt_run_id=$3::uuid,
   status='queued' WHERE flow_id=$1::uuid AND step_index=$2`,id,current,queued);err!=nil{return err}
  if _,err=tx.Exec(ctx,`UPDATE model_flow_runs SET status='running',
   started_at=COALESCE(started_at,clock_timestamp()) WHERE id=$1::uuid`,id);err!=nil{return err}
  return tx.Commit(ctx)
 }
 var childStatus,childError string
 err=tx.QueryRow(ctx,`SELECT status,COALESCE(error_message,'') FROM dbt_runs
  WHERE id=$1::uuid`,*childID).Scan(&childStatus,&childError)
 if err!=nil{return fail("linked dbt job is missing")}
 switch childStatus{
 case "failed":
  return fail("dbt model "+name+" failed: "+childError)
 case "queued","running":
  if _,err=tx.Exec(ctx,`UPDATE model_flow_steps SET status=$3
     WHERE flow_id=$1::uuid AND step_index=$2`,id,current,childStatus);err!=nil{return err}
  return tx.Commit(ctx)
 case "succeeded":
  // Data Quality is checked after dbt completes and before unlocking the
  // downstream step. It uses the existing SELECT-only ClickHouse identity.
  if s.chURL==""{return fail("read-only ClickHouse endpoint missing for DQ")}
  reader:=safetable.Client{Endpoint:s.chURL,User:os.Getenv("DATAFLOW_CH_READONLY_USER"),
   Password:os.Getenv("DATAFLOW_CH_READONLY_PASSWORD")}
  if reader.User==""||reader.Password==""{return fail("read-only ClickHouse credentials missing for DQ")}
  dqCtx,cancel:=context.WithTimeout(ctx,15*time.Second)
  // Name has already been validated by enqueue and frozen into step metadata.
  raw,e:=reader.Query(dqCtx,"SELECT count() FROM scratch.dfv_"+name+" FORMAT TabSeparatedRaw")
  cancel()
  if e!=nil{return fail("DQ query failed for "+name+"; check view output and SELECT grants")}
  count,e:=strconv.ParseInt(strings.TrimSpace(raw),10,64)
  if e!=nil||count<1{return fail("DQ check failed for "+name+": expected at least one output row")}
  if count>1000000{return fail("TEST DQ cap exceeded: more than 1m output rows")}
  if _,err=tx.Exec(ctx,`UPDATE model_flow_steps SET status='succeeded',
    rows_verified=$3 WHERE flow_id=$1::uuid AND step_index=$2`,id,current,count);err!=nil{return err}
  if current+1==total{
   if _,err=tx.Exec(ctx,`UPDATE model_flow_runs SET status='succeeded',
     current_step=$2,finished_at=clock_timestamp()
     WHERE id=$1::uuid`,id,total);err!=nil{return err}
  }else{
   if _,err=tx.Exec(ctx,`UPDATE model_flow_runs SET current_step=$2
     WHERE id=$1::uuid`,id,current+1);err!=nil{return err}
  }
  return tx.Commit(ctx)
 default:return fail("unexpected dbt run status")
 }
}
func(s *server)startAutoBuildCoordinator(root context.Context){
 if !autoBuildEnabled(){return}
 go func(){
  ticker:=time.NewTicker(2*time.Second);defer ticker.Stop()
  for{
   workCtx,cancel:=context.WithTimeout(root,25*time.Second)
   err:=s.reconcileOneFlow(workCtx)
   cancel()
   if err!=nil&&!errors.Is(err,pgx.ErrNoRows)&&root.Err()==nil{
    log.Printf("auto-build coordinator: %v",err)
   }
   select{case <-root.Done():return;case <-ticker.C:}
  }
 }()
}

