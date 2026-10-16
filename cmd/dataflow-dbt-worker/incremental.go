package main

import(
 "context"
 "errors"
 "fmt"
 "log"
 "os"
 "strconv"
 "time"
 "github.com/dinhvansh/dataflow-engine/internal/dbtworker"
 "github.com/dinhvansh/dataflow-engine/internal/safetable"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)
type incrementalClaim struct{
 ID,Owner,Target,Delta,Key,Stage,State,Lower,Upper,BaselineUUID,StageUUID,OldUUID string
 Fence int64
 Prepared bool
}
func incrementalWorkerEnabled()bool{
 return tableEnabled()&&os.Getenv("DATAFLOW_ENABLE_INCREMENTAL_TEST")=="true"
}
func checkIncrementalSchema(ctx context.Context,pg *pgxpool.Pool)error{
 if !incrementalWorkerEnabled(){return nil}
 rows,err:=pg.Query(ctx,`SELECT stage_uuid,publish_prepared FROM incremental_runs LIMIT 0`)
 if err!=nil{return fmt.Errorf("incremental requires migration 014: %w",err)}
 rows.Close();return rows.Err()
}
func claimIncremental(ctx context.Context,pg *pgxpool.Pool)(incrementalClaim,error){
 var c incrementalClaim
 _,err:=pg.Exec(ctx,`UPDATE incremental_runs SET state='failed',
 error_message='incremental lease expired after 3 attempts',lease_owner=NULL,
 lease_until=NULL,finished_at=clock_timestamp()
 WHERE state IN ('building','publishing','rolling_back')
 AND lease_until<clock_timestamp() AND attempt_count>=3`)
 if err!=nil{return c,err}
 err=pg.QueryRow(ctx,`WITH candidate AS(
 SELECT id FROM incremental_runs
 WHERE (state='queued' OR (state IN ('publishing','rolling_back') AND lease_owner IS NULL)
 OR (state IN ('building','publishing','rolling_back') AND lease_until<clock_timestamp()))
 AND attempt_count<3
 ORDER BY created_at,id LIMIT 1 FOR UPDATE SKIP LOCKED
 )
 UPDATE incremental_runs r SET
 state=CASE WHEN r.state='queued' THEN 'building' ELSE r.state END,
 attempt_count=r.attempt_count+1,fence_token=r.fence_token+1,
 lease_owner=gen_random_uuid(),lease_until=clock_timestamp()+interval '10 minutes'
 FROM candidate c WHERE c.id=r.id
 RETURNING r.id::text,r.lease_owner::text,r.fence_token,r.target_name,r.delta_table,
 r.key_column,r.stage_name,r.state,r.cursor_lower::text,COALESCE(r.cursor_upper::text,''),
 COALESCE(r.target_uuid,''),COALESCE(r.stage_uuid,''),
 COALESCE(r.old_target_uuid,''),r.publish_prepared`).
 Scan(&c.ID,&c.Owner,&c.Fence,&c.Target,&c.Delta,&c.Key,&c.Stage,&c.State,
 &c.Lower,&c.Upper,&c.BaselineUUID,&c.StageUUID,&c.OldUUID,&c.Prepared)
 return c,err
}
func finishIncremental(ctx context.Context,pg *pgxpool.Pool,c incrementalClaim,state,reason string,stageUUID string,stats safetable.DeltaStats)error{
 tag,err:=pg.Exec(ctx,`UPDATE incremental_runs SET state=$4,error_message=NULLIF($5,''),
 stage_uuid=COALESCE(NULLIF($6,''),stage_uuid),
 delta_rows=CASE WHEN $7>=0 THEN $7 ELSE delta_rows END,
 rows_verified=CASE WHEN $8>=0 THEN $8 ELSE rows_verified END,
 lease_owner=NULL,lease_until=NULL,
 finished_at=CASE WHEN $4 IN ('failed','no_changes','published','rolled_back')
 THEN clock_timestamp() ELSE NULL END
 WHERE id=$1::uuid AND lease_owner=$2::uuid AND fence_token=$3
 AND state=$9 AND lease_until>clock_timestamp()`,
 c.ID,c.Owner,c.Fence,state,reason,stageUUID,int64(stats.Events),int64(stats.Rows),c.State)
 if err!=nil{return err}
 if tag.RowsAffected()!=1{return errors.New("incremental lease lost before result persisted")}
 return nil
}
func runIncrementalBuild(ctx context.Context,pg *pgxpool.Pool,c incrementalClaim,client safetable.Client){
 if !incrementalWorkerEnabled(){return}
 lower,err:=strconv.ParseUint(c.Lower,10,64);if err!=nil{return}
 targetUUID,err:=client.UUID(ctx,c.Target)
 if err!=nil||targetUUID==""{
  _=finishIncremental(ctx,pg,c,"failed","baseline physical table missing", "",safetable.DeltaStats{})
  return
 }
 // Use row locking to register the baseline UUID only once. The watermark
 // may already exist from an earlier approved run.
 tx,e:=pg.Begin(ctx);if e!=nil{return}
 var checkpointUUID,checkpointValue string
 e=tx.QueryRow(ctx,`SELECT COALESCE(target_uuid,''),cursor_value::text
 FROM incremental_targets WHERE target_name=$1 FOR UPDATE`,c.Target).Scan(&checkpointUUID,&checkpointValue)
 if e==nil&&checkpointValue!=c.Lower{e=errors.New("baseline cursor changed")}
 if e==nil&&checkpointUUID!=""&&checkpointUUID!=targetUUID{e=errors.New("baseline UUID changed")}
 if e==nil{
  _,e=tx.Exec(ctx,`UPDATE incremental_targets SET target_uuid=$2
    WHERE target_name=$1 AND (target_uuid IS NULL OR target_uuid=$2)`,c.Target,targetUUID)
 }
 if e==nil{_,e=tx.Exec(ctx,`UPDATE incremental_runs SET target_uuid=$4
   WHERE id=$1::uuid AND lease_owner=$2::uuid AND fence_token=$3
   AND state='building' AND (target_uuid IS NULL OR target_uuid=$4)`,
   c.ID,c.Owner,c.Fence,targetUUID)}
 if e==nil{e=tx.Commit(ctx)}else{_ = tx.Rollback(ctx)}
 if e!=nil{
  _=finishIncremental(ctx,pg,c,"failed","baseline changed; cannot copy data", "",safetable.DeltaStats{})
  return
 }
 if c.BaselineUUID!=""&&c.BaselineUUID!=targetUUID{
  _=finishIncremental(ctx,pg,c,"failed","run baseline UUID mismatch", "",safetable.DeltaStats{})
  return
 }
 upper:=uint64(0)
 if c.Upper!=""{upper,err=strconv.ParseUint(c.Upper,10,64)}else{
  upper,err=client.UpperCursor(ctx,c.Delta)
  if err==nil{
   tag,e:=pg.Exec(ctx,`UPDATE incremental_runs SET cursor_upper=$4::numeric
     WHERE id=$1::uuid AND lease_owner=$2::uuid AND fence_token=$3
     AND state='building' AND cursor_upper IS NULL`,c.ID,c.Owner,c.Fence,strconv.FormatUint(upper,10))
   if e!=nil||tag.RowsAffected()!=1{return}
  }
 }
 if err!=nil{
  _=finishIncremental(ctx,pg,c,"failed","cannot read CDC watermark", "",safetable.DeltaStats{})
  return
 }
 if upper<=lower{
  _=finishIncremental(ctx,pg,c,"no_changes","", "",safetable.DeltaStats{})
  return
 }
 plan:=safetable.DeltaPlan{Target:c.Target,Delta:c.Delta,Key:c.Key,Stage:c.Stage,
  Lower:lower,Upper:upper,BaselineUUID:targetUUID}
 stats,err:=client.BuildDeltaStage(ctx,plan)
 if err!=nil{
  _=finishIncremental(ctx,pg,c,"failed",err.Error(),"",safetable.DeltaStats{})
  return
 }
 uuid,err:=client.UUID(ctx,c.Stage)
 if err!=nil||uuid==""{
  _=finishIncremental(ctx,pg,c,"failed","staging UUID missing","",safetable.DeltaStats{})
  return
 }
 if err=finishIncremental(ctx,pg,c,"ready","",uuid,stats);err!=nil{
  log.Printf("incremental staging %s completed physically but metadata failed: %v",c.ID,err)
 }
}
func guardedIncrementalSwap(ctx context.Context,pg *pgxpool.Pool,c incrementalClaim,client safetable.Client,rollback bool)error{
 tx,err:=pg.Begin(ctx);if err!=nil{return err}
 defer tx.Rollback(ctx)
 var state,owner,old,stageUUID,lower,upper string
 err=tx.QueryRow(ctx,`SELECT state,COALESCE(lease_owner::text,''),
 COALESCE(old_target_uuid,''),COALESCE(stage_uuid,''),cursor_lower::text,cursor_upper::text
 FROM incremental_runs WHERE id=$1::uuid FOR UPDATE`,c.ID).
 Scan(&state,&owner,&old,&stageUUID,&lower,&upper)
 if err!=nil{return err}
 if state!=c.State||owner!=c.Owner||old==""||stageUUID==""{
  return errors.New("incremental worker lost fencing or prepared UUID")
 }
 var checkpoint,physicalUUID,lastID string
 err=tx.QueryRow(ctx,`SELECT cursor_value::text,COALESCE(target_uuid,''),
 COALESCE(latest_run_id::text,'') FROM incremental_targets
 WHERE target_name=$1 FOR UPDATE`,c.Target).Scan(&checkpoint,&physicalUUID,&lastID)
 if err!=nil{return err}
 pub:=safetable.Publication{Target:c.Target,Stage:c.Stage,
  StageUUID:stageUUID,OldUUID:old}
 if rollback{
  if checkpoint!=upper||physicalUUID!=stageUUID||lastID!=c.ID{
   return errors.New("rollback checkpoint or published UUID changed")
  }
  if err=client.Rollback(ctx,pub);err!=nil{return err}
  var predecessor *string
  err=tx.QueryRow(ctx,`SELECT id::text FROM incremental_runs WHERE target_name=$1
    AND state='published' AND cursor_upper=$2::numeric AND id<>$3::uuid
    ORDER BY created_at DESC LIMIT 1`,c.Target,lower,c.ID).Scan(&predecessor)
  if errors.Is(err,pgx.ErrNoRows){predecessor=nil;err=nil}
  if err!=nil{return err}
  _,err=tx.Exec(ctx,`UPDATE incremental_targets SET cursor_value=$2::numeric,target_uuid=$3,
   latest_run_id=$4::uuid,updated_at=clock_timestamp()
   WHERE target_name=$1`,c.Target,lower,old,predecessor)
  if err!=nil{return err}
  _,err=tx.Exec(ctx,`UPDATE incremental_runs SET state='rolled_back',
   lease_owner=NULL,lease_until=NULL,finished_at=clock_timestamp()
   WHERE id=$1::uuid`,c.ID)
 }else{
  if checkpoint!=lower||physicalUUID!=old{
   return errors.New("publish checkpoint or baseline UUID changed")
  }
  if err=client.Publish(ctx,pub);err!=nil{return err}
  _,err=tx.Exec(ctx,`UPDATE incremental_targets SET cursor_value=$2::numeric,target_uuid=$3,
   latest_run_id=$4::uuid,updated_at=clock_timestamp()
   WHERE target_name=$1`,c.Target,upper,stageUUID,c.ID)
  if err!=nil{return err}
  _,err=tx.Exec(ctx,`UPDATE incremental_runs SET state='published',
   lease_owner=NULL,lease_until=NULL,finished_at=clock_timestamp()
   WHERE id=$1::uuid`,c.ID)
 }
 if err!=nil{return err}
 return tx.Commit(ctx)
}
func runIncrementalClaim(parent context.Context,pg *pgxpool.Pool,c incrementalClaim,client safetable.Client){
 ctx,cancel:=context.WithTimeout(parent,7*time.Minute);defer cancel()
 switch c.State{
 case "building":runIncrementalBuild(ctx,pg,c,client)
 case "publishing":
  if !c.Prepared{
   baseline,err:=client.UUID(ctx,c.Target);if err!=nil||baseline!=c.BaselineUUID{return}
   stage,err:=client.UUID(ctx,c.Stage);if err!=nil||stage!=c.StageUUID{return}
   tag,e:=pg.Exec(ctx,`UPDATE incremental_runs SET old_target_uuid=$4,publish_prepared=true
    WHERE id=$1::uuid AND lease_owner=$2::uuid AND fence_token=$3 AND state='publishing'
    AND lease_until>clock_timestamp()`,c.ID,c.Owner,c.Fence,baseline)
   if e!=nil||tag.RowsAffected()!=1{return}
  }
  if err:=guardedIncrementalSwap(ctx,pg,c,client,false);err!=nil{
   log.Printf("incremental publish %s refused or ambiguous: %v",c.ID,err)
  }
 case "rolling_back":
  if err:=guardedIncrementalSwap(ctx,pg,c,client,true);err!=nil{
   log.Printf("incremental rollback %s refused or ambiguous: %v",c.ID,err)
  }
 }
}
func dispatchIncremental(ctx context.Context,pg *pgxpool.Pool,config dbtworker.Config){
 if !incrementalWorkerEnabled(){return}
 claimCtx,cancel:=context.WithTimeout(ctx,10*time.Second)
 claim,err:=claimIncremental(claimCtx,pg)
 cancel()
 if err==nil{runIncrementalClaim(ctx,pg,claim,tableClickhouse(config));return}
 if !errors.Is(err,pgx.ErrNoRows)&&ctx.Err()==nil{log.Printf("incremental claim: %v",err)}
}
