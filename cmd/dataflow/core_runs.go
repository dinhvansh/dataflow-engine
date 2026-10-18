package main

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "log"
 "net/http"
 "os"
 "strings"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/lineage"
 "github.com/dinhvansh/dataflow-engine/internal/safetable"
 "github.com/dinhvansh/dataflow-engine/internal/transform"
 "github.com/jackc/pgx/v5"
)

func corePipelineEnabled()bool{
 return managedEnabled()&&autoBuildEnabled()&&safeTableAPIEnabled()&&
  os.Getenv("DATAFLOW_ENABLE_CORE_PIPELINE_TEST")=="true"&&scratchDatabase()=="scratch"
}
type corePlannedStep struct{ ID,Name,SQL string; Definition []byte }
type corePlan struct{ Name,Digest string; Steps []corePlannedStep }

// Preserve immutable compiled SQL and definitions across the ingestion/dbt
// boundary. The first node MUST consume the local raw event table; arbitrary
// SQL/remote sources cannot be smuggled into this isolated pipeline.
func(s *server)corePlan(ctx context.Context,tx pgx.Tx,targetID string)(corePlan,error){
 var p corePlan
 models,err:=s.readLineageModels(ctx,tx);if err!=nil{return p,err}
 for _,m:=range models{if m.ID==targetID{p.Name=m.Name;break}}
 if p.Name==""{return p,errors.New("saved target model does not exist")}
 nodes,err:=lineage.PlanChain(models,p.Name);if err!=nil{return p,err}
 if len(nodes)<1||len(nodes)>30{return p,errors.New("model chain must be 1..30 steps")}
 hash:=sha256.New()
 for i,node:=range nodes{
  if len(node.Name)>80||node.Name==""||node.Name[0]<'a'||node.Name[0]>'z'||
    strings.ToLower(node.Name)!=node.Name{
   return p,errors.New("unsafe saved model name")
  }
  if _,err=transform.ValidateModelName(node.Name);err!=nil{return p,err}
  var def []byte;var sqlText string
  err=tx.QueryRow(ctx,`SELECT definition,compiled_sql FROM transform_models
    WHERE id=$1::uuid`,node.ID).Scan(&def,&sqlText)
  if err!=nil{return p,err}
  var m transform.Model
  if err=json.Unmarshal(def,&m);err!=nil||m.Mode!="visual"||
    m.Name!=node.Name||m.Source.Database!="scratch"{
   return p,errors.New("model definition is not a saved scratch Visual model")
  }
  if i==0&&m.Source.Table!="dataflow_raw_events"{
   return p,errors.New("first model must read scratch.dataflow_raw_events")
  }
  compiled,err:=transform.Compile(m,"scratch")
  if err!=nil||compiled.SQL!=sqlText{return p,errors.New("stored model SQL drift; re-save model")}
  if i>0&&m.Source.Table!="dfv_"+nodes[i-1].Name{
   return p,errors.New("model must depend on previous saved scratch View")
  }
  // Length-prefixed JSON bytes are already canonical pg JSONB serialization
  // for this snapshot; do not rely on map iteration order.
  record,_:=json.Marshal(struct{ID,Name,SQL string;Definition json.RawMessage}{
   node.ID,node.Name,sqlText,json.RawMessage(def),
  })
  _,_=hash.Write(record);_,_=hash.Write([]byte{0})
  p.Steps=append(p.Steps,corePlannedStep{node.ID,node.Name,sqlText,def})
 }
 p.Digest=hex.EncodeToString(hash.Sum(nil))
 return p,nil
}

func(s *server)checkCoreSchema(ctx context.Context)error{
 if !corePipelineEnabled(){return nil}
 rows,err:=s.pg.Query(ctx,`SELECT id,managed_run_id,flow_id,stage_uuid,old_uuid,rolled_back_at
 FROM core_pipeline_runs LIMIT 0`)
 if err!=nil{return fmt.Errorf("CORE-RUN-3 requires migrations 028 and 029: %w",err)}
 rows.Close();return rows.Err()
}
func(s *server)coreRunsAPI(w http.ResponseWriter,r *http.Request){
 if !s.validBearer(r){http.Error(w,"unauthorized",401);return}
 if !corePipelineEnabled(){http.Error(w,"isolated core pipeline disabled",403);return}
 ctx,stop:=context.WithTimeout(r.Context(),15*time.Second);defer stop()
 switch r.Method{
 case http.MethodPost:
  r.Body=http.MaxBytesReader(w,r.Body,4096)
  d:=json.NewDecoder(r.Body);d.DisallowUnknownFields()
  var arg struct{
   StreamID string `json:"stream_id"`
   ModelID string `json:"model_id"`
   UniqueColumn string `json:"unique_column"`
  }
  if d.Decode(&arg)!=nil||!validUUID.MatchString(arg.StreamID)||
    !validUUID.MatchString(arg.ModelID)||!safeDQColumn.MatchString(arg.UniqueColumn)||
    len(arg.UniqueColumn)>85 {
   http.Error(w,"valid stream_id/model_id/unique_column required",400);return
  }
  var extra any
  if err:=d.Decode(&extra);!errors.Is(err,io.EOF){
   http.Error(w,"extra JSON not permitted",400);return
  }
  tx,err:=s.pg.Begin(ctx);if err!=nil{http.Error(w,"queue unavailable",503);return}
  defer tx.Rollback(ctx)
  if _,err=tx.Exec(ctx,`SELECT pg_advisory_xact_lock(84721,1)`);err!=nil{
   http.Error(w,"catalog lock unavailable",503);return
  }
  var db,table string
  err=tx.QueryRow(ctx,`SELECT target_database,target_table
   FROM sync_streams WHERE id=$1::uuid`,arg.StreamID).Scan(&db,&table)
  if err!=nil{http.Error(w,"stream not found",404);return}
  if db!="scratch"||table!="dataflow_raw_events"{
   http.Error(w,"non-scratch managed stream refused",403);return
  }
  plan,err:=s.corePlan(ctx,tx,arg.ModelID)
  if err!=nil{http.Error(w,"saved dependency chain unavailable or changed: "+err.Error(),422);return}
  var runID,managedID string
  err=tx.QueryRow(ctx,`INSERT INTO managed_runs(stream_id,status)
   VALUES($1::uuid,'queued') RETURNING id::text`,arg.StreamID).Scan(&managedID)
  if err!=nil{http.Error(w,"active sync or missing migration",409);return}
  err=tx.QueryRow(ctx,`INSERT INTO core_pipeline_runs(
   stream_id,managed_run_id,target_model_id,target_model_name,
   plan_digest,unique_column,stage_name,target_name)
   VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,
    'dft_stage_'||replace(gen_random_uuid()::text,'-',''),$7)
   RETURNING id::text`,
    arg.StreamID,managedID,arg.ModelID,plan.Name,plan.Digest,arg.UniqueColumn,
    safetable.TargetName("core_"+plan.Name)).Scan(&runID)
  if err!=nil{http.Error(w,"another core pipeline active",409);return}
  // Stage identity must be derived from run ID; random stage GUID above
  // is stored as immutable per-run identity in PG before any ClickHouse DDL.
  if err=tx.Commit(ctx);err!=nil{http.Error(w,"cannot commit core enqueue",503);return}
  names:=make([]string,0,len(plan.Steps))
  for _,step:=range plan.Steps{names=append(names,step.Name)}
  jsonResponse(w,202,map[string]any{"run_id":runID,"managed_run_id":managedID,
    "status":"waiting_sync","dependency_order":names})
 case http.MethodGet:
  id:=r.URL.Query().Get("id")
  if !validUUID.MatchString(id){http.Error(w,"valid run id required",400);return}
  var row struct{
   ID,ManagedID,FlowID,Status,Target,Stage,Issue string
   Rows int64
  }
  err:=s.pg.QueryRow(ctx,`SELECT id::text,managed_run_id::text,
    COALESCE(flow_id::text,''),status,target_name,stage_name,
    COALESCE(rows_verified,0),COALESCE(error_message,'')
   FROM core_pipeline_runs WHERE id=$1::uuid`,id).
    Scan(&row.ID,&row.ManagedID,&row.FlowID,&row.Status,&row.Target,
      &row.Stage,&row.Rows,&row.Issue)
  if err!=nil{http.Error(w,"core run not found",404);return}
  var managedStatus string
  if e:=s.pg.QueryRow(ctx,`SELECT status FROM managed_runs WHERE id=$1::uuid`,
    row.ManagedID).Scan(&managedStatus);e!=nil{
   http.Error(w,"missing child sync run",503);return
  }
  var flowStatus string
  if row.FlowID!=""{
   if e:=s.pg.QueryRow(ctx,`SELECT status FROM model_flow_runs WHERE id=$1::uuid`,
     row.FlowID).Scan(&flowStatus);e!=nil{
    http.Error(w,"missing child model flow",503);return
   }
  }
  jsonResponse(w,200,map[string]any{
   "run_id":row.ID,"status":row.Status,"managed_run_id":row.ManagedID,
   "managed_status":managedStatus,"model_flow_id":row.FlowID,
   "model_flow_status":flowStatus,"stage":"scratch."+row.Stage,
   "target":"scratch."+row.Target,"rows_verified":row.Rows,"error":row.Issue,
  })
 default:http.Error(w,"method not allowed",405)
 }
}

// One short PG-locked transition; externally visible ClickHouse state is
// reconciled with immutable UUID publication, not assumed from HTTP success.
func(s *server)reconcileOneCore(ctx context.Context)error{
 tx,err:=s.pg.Begin(ctx);if err!=nil{return err};defer tx.Rollback(ctx)
 var id,managedID,targetID,planDigest,status,modelName,unique,stage,target string
 var flowID,stageUUID,oldUUID *string
 err=tx.QueryRow(ctx,`SELECT id::text,managed_run_id::text,
   target_model_id::text,plan_digest,status,target_model_name,unique_column,
   stage_name,target_name,flow_id::text,stage_uuid,old_uuid
   FROM core_pipeline_runs WHERE status IN('waiting_sync','waiting_flow','publishing')
   ORDER BY created_at,id LIMIT 1 FOR UPDATE SKIP LOCKED`).
   Scan(&id,&managedID,&targetID,&planDigest,&status,&modelName,&unique,
    &stage,&target,&flowID,&stageUUID,&oldUUID)
 if err!=nil{return err}
 fail:=func(message string)error{
  if len(message)>300{message=message[:300]}
  _,e:=tx.Exec(ctx,`UPDATE core_pipeline_runs SET status='failed',
    error_message=$2,finished_at=clock_timestamp()
    WHERE id=$1::uuid`,id,message)
  if e!=nil{return e}
  return tx.Commit(ctx)
 }
 switch status{
 case "waiting_sync":
  var syncStatus string
  err=tx.QueryRow(ctx,`SELECT status FROM managed_runs WHERE id=$1::uuid`,
    managedID).Scan(&syncStatus)
  if err!=nil{return fail("missing managed sync job")}
  if syncStatus=="failed"{return fail("source sync failed")}
  if syncStatus!="succeeded"{return tx.Commit(ctx)}
  if _,err=tx.Exec(ctx,`SELECT pg_advisory_xact_lock(84721,1)`);err!=nil{return err}
  plan,e:=s.corePlan(ctx,tx,targetID)
  if e!=nil||plan.Digest!=planDigest||plan.Name!=modelName{
   return fail("model plan changed between enqueue and dbt execution")
  }
  var next string
  err=tx.QueryRow(ctx,`INSERT INTO model_flow_runs(
   target_model_id,target_model_name,step_count)
   VALUES($1::uuid,$2,$3) RETURNING id::text`,
   targetID,modelName,len(plan.Steps)).Scan(&next)
  if err!=nil{return err} // retain waiting_sync on an active external flow
  for i,step:=range plan.Steps{
   if _,err=tx.Exec(ctx,`INSERT INTO model_flow_steps(
     flow_id,step_index,model_id,model_name,source_definition,compiled_sql)
     VALUES($1::uuid,$2,$3::uuid,$4,$5::jsonb,$6)`,
     next,i,step.ID,step.Name,string(step.Definition),step.SQL);err!=nil{return err}
  }
  _,err=tx.Exec(ctx,`UPDATE core_pipeline_runs SET status='waiting_flow',
   flow_id=$2::uuid WHERE id=$1::uuid`,id,next)
  if err!=nil{return err}
  return tx.Commit(ctx)
 case "waiting_flow":
  if flowID==nil{return fail("missing immutable dbt flow ID")}
  var flowState string;var stepCount int
  err=tx.QueryRow(ctx,`SELECT status,current_step FROM model_flow_runs
    WHERE id=$1::uuid`,*flowID).Scan(&flowState,&stepCount)
  if err!=nil{return fail("lost model flow")}
  if flowState=="failed"{return fail("dependent dbt or DQ step failed")}
  if flowState!="succeeded"{return tx.Commit(ctx)}
  if stepCount<1{return fail("flow lacks completed dependencies")}
  _,err=tx.Exec(ctx,`UPDATE core_pipeline_runs
   SET status='publishing' WHERE id=$1::uuid`,id)
  if err!=nil{return err}
  return tx.Commit(ctx)
 case "publishing":
  plan,e:=s.corePlan(ctx,tx,targetID)
  if e!=nil||plan.Digest!=planDigest{return fail("saved model changed before publication")}
  if flowID==nil{return fail("missing flow evidence")}
  var flowState string;var count int64
  err=tx.QueryRow(ctx,`SELECT status FROM model_flow_runs WHERE id=$1::uuid`,
    *flowID).Scan(&flowState)
  if err!=nil||flowState!="succeeded"{return fail("upstream model flow not successful")}
  err=tx.QueryRow(ctx,`SELECT rows_verified FROM model_flow_steps
   WHERE flow_id=$1::uuid ORDER BY step_index DESC LIMIT 1`,*flowID).Scan(&count)
  if err!=nil||count<1{return fail("missing trusted final dbt DQ row count")}
  if len(stage)!=42||!strings.HasPrefix(stage,"dft_stage_")||
    !safeDQColumn.MatchString(unique){
   return fail("invalid immutable staging configuration")
  }
  writer:=safetable.Client{Endpoint:s.chURL,User:s.chUser,Password:s.chPass}
  if stageUUID==nil{
   chCtx,stop:=context.WithTimeout(ctx,20*time.Second)
   defer stop()
   if err=writer.RequireAtomic(chCtx);err!=nil{return err}
   // Only compiler-saved, validated names from the immutable chain can
   // influence this SQL, never user-entered raw SQL.
   _,err=writer.Query(chCtx,`CREATE TABLE IF NOT EXISTS scratch.`+stage+
    ` ENGINE=MergeTree ORDER BY tuple() AS SELECT * FROM scratch.dfv_`+modelName)
   if err!=nil{return err}
   dq,checkErr:=writer.ValidateStage(chCtx,stage,unique)
   if checkErr!=nil||dq.Rows!=uint64(count){
    return fail("core publication DQ rejected staged row count or uniqueness")
   }
   stageID,e:=writer.UUID(chCtx,stage);if e!=nil||stageID==""{return errors.New("stage identity lookup failed")}
   oldID,e:=writer.UUID(chCtx,target);if e!=nil{return e}
   _,err=tx.Exec(ctx,`UPDATE core_pipeline_runs
     SET stage_uuid=$2,old_uuid=$3,rows_verified=$4 WHERE id=$1::uuid`,
      id,stageID,oldID,count)
   if err!=nil{return err}
   return tx.Commit(ctx) // persist immutable UUIDs BEFORE external rename/exchange
  }
  previous:=""
  if oldUUID!=nil{previous=*oldUUID}
  chCtx,stop:=context.WithTimeout(ctx,20*time.Second);defer stop()
  err=writer.Publish(chCtx,safetable.Publication{
   Target:target,Stage:stage,StageUUID:*stageUUID,OldUUID:previous,
  })
  if err!=nil{return err} // uncertain external commit: replay safely next tick
  _,err=tx.Exec(ctx,`UPDATE core_pipeline_runs SET status='succeeded',
   error_message=NULL,finished_at=clock_timestamp() WHERE id=$1::uuid`,id)
  if err!=nil{return err}
  return tx.Commit(ctx)
 default:return fail("invalid core pipeline state")
 }
}
func(s *server)startCoreCoordinator(ctx context.Context){
 if !corePipelineEnabled(){return}
 go func(){
  t:=time.NewTicker(2*time.Second);defer t.Stop()
  for{
   stepCtx,stop:=context.WithTimeout(ctx,30*time.Second)
   err:=s.reconcileOneCore(stepCtx)
   stop()
   if err!=nil&&!errors.Is(err,pgx.ErrNoRows)&&ctx.Err()==nil{
    log.Printf("core pipeline coordinator: %v",err)
   }
   select{case <-ctx.Done():return;case <-t.C:}
  }
 }()
}
