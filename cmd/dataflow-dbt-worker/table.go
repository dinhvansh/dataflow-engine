package main

import(
 "context"
 "errors"
 "fmt"
 "log"
 "os"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/dbtworker"
 "github.com/dinhvansh/dataflow-engine/internal/safetable"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)
type tableClaim struct{
 ID,Owner,Model,SQL,Stage,Target,UniqueKey,State,StageUUID,OldUUID string
 Fence int64
 Prepared bool
}
func tableEnabled()bool{
 return os.Getenv("DATAFLOW_ALLOW_TEST_LOAD")=="true" &&
 os.Getenv("DATAFLOW_ENABLE_DBT_TEST")=="true" &&
 os.Getenv("DATAFLOW_ENABLE_SAFE_TABLE_TEST")=="true" &&
 os.Getenv("DATAFLOW_TEST_TARGET_DATABASE")=="scratch"
}
func checkTableSchema(ctx context.Context,pg *pgxpool.Pool)error{
 if !tableEnabled(){return nil}
 rows,err:=pg.Query(ctx,"SELECT publish_prepared,stage_uuid,old_target_uuid FROM table_runs LIMIT 0")
 if err!=nil{return fmt.Errorf("safe table requires migration 012: %w",err)}
 rows.Close();return rows.Err()
}
func claimTable(ctx context.Context,pg *pgxpool.Pool)(tableClaim,error){
 var c tableClaim
 // Terminalize work only after it has exhausted attempts. This separate
 // statement must commit even if the claim SELECT finds no rows.
 _,err:=pg.Exec(ctx,`UPDATE table_runs SET state='failed',
  error_message='worker lease expired after 3 attempts',lease_owner=NULL,lease_until=NULL,
  finished_at=clock_timestamp()
  WHERE state IN ('building','publishing','rolling_back')
  AND lease_until<clock_timestamp() AND attempt_count>=3`)
 if err!=nil{return c,err}
 err=pg.QueryRow(ctx,`WITH candidate AS (
  SELECT id FROM table_runs
   WHERE (state='queued' OR (state IN ('building','publishing','rolling_back')
    AND lease_until<clock_timestamp()))
    AND attempt_count<3
  ORDER BY created_at,id LIMIT 1 FOR UPDATE SKIP LOCKED
 )
 UPDATE table_runs t SET
   state=CASE WHEN t.state='queued' THEN 'building' ELSE t.state END,
   attempt_count=t.attempt_count+1,
   lease_owner=gen_random_uuid(),lease_until=clock_timestamp()+interval '10 minutes',
   fence_token=t.fence_token+1
 FROM candidate c WHERE t.id=c.id
 RETURNING t.id::text,t.lease_owner::text,t.fence_token,t.model_name,t.compiled_sql,
 t.stage_name,t.target_name,COALESCE(t.unique_column,''),t.state,
 COALESCE(t.stage_uuid,''),COALESCE(t.old_target_uuid,''),t.publish_prepared`).
 Scan(&c.ID,&c.Owner,&c.Fence,&c.Model,&c.SQL,&c.Stage,&c.Target,&c.UniqueKey,&c.State,
 &c.StageUUID,&c.OldUUID,&c.Prepared)
 return c,err
}
func updateTable(ctx context.Context,pg *pgxpool.Pool,c tableClaim,status string,stageUUID,oldUUID string,rows uint64,prepared bool,reason string,logs string)error{
 tag,err:=pg.Exec(ctx,`UPDATE table_runs SET state=$4,stage_uuid=NULLIF($5,''),
 old_target_uuid=NULLIF($6,''),rows_verified=CASE WHEN $7::bigint>0 THEN $7 ELSE rows_verified END,
 publish_prepared=$8,error_message=NULLIF($9,''),log_tail=NULLIF($10,''),
 finished_at=CASE WHEN $4 IN ('published','rolled_back','failed') THEN clock_timestamp() ELSE NULL END,
 lease_owner=NULL,lease_until=NULL
 WHERE id=$1::uuid AND lease_owner=$2::uuid AND fence_token=$3
 AND lease_until>clock_timestamp() AND state=$11`,
 c.ID,c.Owner,c.Fence,status,stageUUID,oldUUID,rows,prepared,reason,logs,c.State)
 if err!=nil{return err}
 if tag.RowsAffected()!=1{return errors.New("safe table lease lost; state not persisted")}
 return nil
}
func(c tableClaim)publication()safetable.Publication{
 return safetable.Publication{Target:c.Target,Stage:c.Stage,StageUUID:c.StageUUID,OldUUID:c.OldUUID}
}
func runTableClaim(parent context.Context,pg *pgxpool.Pool,c tableClaim,config dbtworker.Config,client safetable.Client){
 ctx,stop:=context.WithTimeout(parent,6*time.Minute);defer stop()
 switch c.State{
 case "building":
  p:=dbtworker.Plan{Name:c.Model,SQL:c.SQL,Target:c.Stage,Materialization:"table"}
  out,err:=dbtworker.Execute(ctx,p,config)
  uuid:="";var count safetable.Counts
  if err==nil{uuid,err=client.UUID(ctx,c.Stage)}
  if err==nil&&uuid==""{err=errors.New("dbt returned success but stage table missing")}
  if err==nil{count,err=client.ValidateStage(ctx,c.Stage,c.UniqueKey)}
  if parent.Err()!=nil||ctx.Err()!=nil{return}
  finalCtx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel()
  if err!=nil{
   if e:=updateTable(finalCtx,pg,c,"failed","","",0,false,err.Error(),out.Log);e!=nil{log.Printf("stage %s failed to persist: %v",c.ID,e)}
   return
  }
  if e:=updateTable(finalCtx,pg,c,"ready",uuid,"",count.Rows,false,"",out.Log);e!=nil{log.Printf("stage %s completion rejected: %v",c.ID,e)}
 case "publishing":
  pub:=c.publication()
  if !c.Prepared{
   if err:=client.RequireAtomic(ctx);err!=nil{log.Printf("table publish prerequisite: %v",err);return}
   before,err:=client.UUID(ctx,c.Target)
   if err!=nil{log.Printf("table preflight: %v",err);return}
   stage,err:=client.UUID(ctx,c.Stage)
   if err!=nil||stage!=c.StageUUID{log.Printf("table stage uuid changed for %s",c.ID);return}
   // PREPARE is durable. Crash after atomic EXCHANGE and before PG commit
   // cannot accidentally exchange the two tables back on replay.
   tag,err:=pg.Exec(ctx,`UPDATE table_runs SET old_target_uuid=NULLIF($4,''),
    publish_prepared=true WHERE id=$1::uuid AND lease_owner=$2::uuid AND fence_token=$3
     AND state='publishing' AND lease_until>clock_timestamp()`,c.ID,c.Owner,c.Fence,before)
   if err!=nil||tag.RowsAffected()!=1{return}
   pub.OldUUID=before
  }
  if err:=client.Publish(ctx,pub);err!=nil{
   log.Printf("table publish %s uncertain/rejected: %v (recovery will re-check UUID)",c.ID,err)
   return
  }
  if parent.Err()!=nil||ctx.Err()!=nil{return}
  finCtx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel()
  if err:=updateTable(finCtx,pg,c,"published",c.StageUUID,pub.OldUUID,0,true,"","");err!=nil{log.Printf("table %s publish metadata uncertain: %v",c.ID,err)}
 case "rolling_back":
  if err:=client.Rollback(ctx,c.publication());err!=nil{
   log.Printf("table rollback %s blocked: %v",c.ID,err);return
  }
  if parent.Err()!=nil||ctx.Err()!=nil{return}
  finCtx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel()
  if err:=updateTable(finCtx,pg,c,"rolled_back",c.StageUUID,c.OldUUID,0,true,"","");err!=nil{log.Printf("table %s rollback metadata uncertain: %v",c.ID,err)}
 }
}
func tableClickhouse(config dbtworker.Config)safetable.Client{
 proto:="http";if config.Secure{proto="https"}
 return safetable.Client{Endpoint:fmt.Sprintf("%s://%s:%d",proto,config.Host,config.Port),User:config.Username,Password:config.Password}
}
func dispatchTable(ctx context.Context,pg *pgxpool.Pool,config dbtworker.Config){
 claimCtx,cancel:=context.WithTimeout(ctx,10*time.Second)
 c,err:=claimTable(claimCtx,pg);cancel()
 if err==nil{runTableClaim(ctx,pg,c,config,tableClickhouse(config));return}
 if !errors.Is(err,pgx.ErrNoRows)&&ctx.Err()==nil{log.Printf("table queue claim: %v",err)}
}

