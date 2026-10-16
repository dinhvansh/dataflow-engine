package main

import(
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "log"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/connectors/clickhouse"
 "github.com/dinhvansh/dataflow-engine/internal/jobs/executor"
 "github.com/jackc/pgx/v5"
)

const (
 sqlDAGLease = 2 * time.Minute
 sqlDAGMaxClaims = 3
)

type sqlDAGClaim struct{
 ID string
 Owner string
 Fence int64
 Attempts int
 Snapshot []byte
}

// claimSQLDAG atomically leases one queued or expired read-only DAG run.
// Only one worker can claim a row at a time, even across Go processes.
func(s *server)claimSQLDAG(ctx context.Context,runID string)(sqlDAGClaim,error){
 var claim sqlDAGClaim
 if runID!=""&&!validUUID.MatchString(runID){return claim,errors.New("invalid run ID")}
 // Clear exhausted, expired attempts; leave completed rows immutable.
 _,err:=s.pg.Exec(ctx,`UPDATE dag_runs SET status='failed',
   error_message='Worker lease expired after maximum safe SELECT-only replay attempts',
   lease_owner=NULL,lease_expires_at=NULL,finished_at=clock_timestamp()
   WHERE status='running' AND lease_expires_at<clock_timestamp() AND dispatch_attempts >= $1`,sqlDAGMaxClaims)
 if err!=nil{return claim,err}
 err=s.pg.QueryRow(ctx,`
 WITH candidate AS (
  SELECT id FROM dag_runs
  WHERE (status='queued' OR (status='running' AND lease_expires_at<clock_timestamp()))
    AND dispatch_attempts<$1
    AND ($2::text='' OR id::text=$2)
  ORDER BY created_at,id
  LIMIT 1
  FOR UPDATE SKIP LOCKED
 )
 UPDATE dag_runs d SET status='running',
   lease_owner=gen_random_uuid(),
   fence_token=d.fence_token+1,
   dispatch_attempts=d.dispatch_attempts+1,
   lease_expires_at=clock_timestamp()+interval '120 seconds',
   started_at=COALESCE(d.started_at,clock_timestamp()),
   error_message=NULL
 FROM candidate c
 WHERE d.id=c.id
 RETURNING d.id::text,d.lease_owner::text,d.fence_token,d.dispatch_attempts,d.snapshot
 `,sqlDAGMaxClaims,runID).Scan(&claim.ID,&claim.Owner,&claim.Fence,&claim.Attempts,&claim.Snapshot)
 return claim,err
}
func(s *server)renewSQLDAG(ctx context.Context,claim sqlDAGClaim)error{
 tag,err:=s.pg.Exec(ctx,`UPDATE dag_runs
 SET lease_expires_at=clock_timestamp()+interval '120 seconds'
 WHERE id=$1::uuid AND lease_owner=$2::uuid AND fence_token=$3
 AND status='running' AND lease_expires_at>clock_timestamp()`,
 claim.ID,claim.Owner,claim.Fence)
 if err!=nil{return err}
 if tag.RowsAffected()!=1{return errors.New("SQL DAG lease lost")}
 return nil
}
func(s *server)completeSQLDAG(ctx context.Context,claim sqlDAGClaim,status string,result any,reason string)error{
 if status!="succeeded"&&status!="failed"{return errors.New("invalid terminal DAG status")}
 body,err:=json.Marshal(result);if err!=nil{return err}
 tag,err:=s.pg.Exec(ctx,`UPDATE dag_runs
 SET status=$4,result=$5::jsonb,error_message=$6,finished_at=clock_timestamp(),
   lease_owner=NULL,lease_expires_at=NULL
 WHERE id=$1::uuid AND lease_owner=$2::uuid AND fence_token=$3
 AND status='running' AND lease_expires_at>clock_timestamp()`,
 claim.ID,claim.Owner,claim.Fence,status,string(body),reason)
 if err!=nil{return err}
 if tag.RowsAffected()!=1{return errors.New("SQL DAG completion rejected: lease lost")}
 return nil
}
func(s *server)runClaimedSQLDAG(parent context.Context,claim sqlDAGClaim){
 ctx,cancel:=context.WithTimeout(parent,30*time.Minute)
 defer cancel()
 done:=make(chan struct{})
 go func(){
  defer close(done)
  ticker:=time.NewTicker(20*time.Second);defer ticker.Stop()
  for{select{
   case <-ctx.Done():return
   case <-ticker.C:
    beatCtx,stop:=context.WithTimeout(ctx,10*time.Second)
    err:=s.renewSQLDAG(beatCtx,claim);stop()
    if err!=nil{log.Printf("SQL DAG %s lost lease: %v",claim.ID,err);cancel();return}
  }}
 }()
 defer func(){cancel();<-done}()
 var frozen frozenDAG
 if err:=json.Unmarshal(claim.Snapshot,&frozen);err!=nil{
  s.finalizeBadSQLDAG(claim,fmt.Errorf("invalid immutable snapshot: %w",err))
  return
 }
 if err:=frozen.Plan.Validate();err!=nil{s.finalizeBadSQLDAG(claim,err);return}
 opts:=executor.Options{Concurrency:2,Retries:1,Timeout:35*time.Second}
 result,runErr:=executor.Run(ctx,frozen.Plan,func(stepCtx context.Context,jobID string)error{
  job,ok:=frozen.Jobs[jobID];if !ok{return fmt.Errorf("missing frozen job %s",jobID)}
  if job.Database!=scratchDatabase(){return errors.New("frozen job targets non-scratch database")}
  if err:=clickhouse.ValidateReadQuery(job.Query);err!=nil{return err}
  reader:=clickhouse.QueryJob{Endpoint:s.chURL,Database:job.Database,
   Username:envReadonlyUser(),Password:envReadonlyPassword()}
  _,err:=reader.Execute(stepCtx,job.Query)
  return err
 },opts)
 status:="succeeded";message:=""
 if runErr!=nil{status="failed";message=runErr.Error()}
 persistCtx,stop:=context.WithTimeout(context.Background(),10*time.Second)
 defer stop()
 if err:=s.completeSQLDAG(persistCtx,claim,status,result,message);err!=nil{
  // Lost-lease workers must never overwrite recovered worker results.
  log.Printf("SQL DAG %s result persistence rejected: %v",claim.ID,err)
 }
}
func(s *server)finalizeBadSQLDAG(claim sqlDAGClaim,cause error){
 ctx,stop:=context.WithTimeout(context.Background(),10*time.Second);defer stop()
 if err:=s.completeSQLDAG(ctx,claim,"failed",map[string]any{"error":cause.Error()},cause.Error());err!=nil{
  log.Printf("SQL DAG %s corrupt snapshot finalization: %v",claim.ID,err)
 }
}
// Dispatcher polls PostgreSQL; no direct goroutine spawned by the enqueue API.
// Each scheduled run survives process restarts. Only readonly SELECT tasks
// may replay after a worker lease expires.
func(s *server)startSQLDAGDispatcher(ctx context.Context){
 if !sqlDAGEnabled(){log.Print("SQL DAG dispatcher disabled");return}
 if s.sqlDAGSlots==nil{s.sqlDAGSlots=make(chan struct{},2)}
 go func(){
  tick:=time.NewTicker(2*time.Second);defer tick.Stop()
  s.dispatchSQLDAG(ctx)
  for{select{
   case <-ctx.Done():return
   case <-tick.C:s.dispatchSQLDAG(ctx)
  }}
 }()
}
func(s *server)dispatchSQLDAG(ctx context.Context){
 for n:=0;n<cap(s.sqlDAGSlots);n++{
  select{case s.sqlDAGSlots<-struct{}{}:default:return}
  claimCtx,stop:=context.WithTimeout(ctx,10*time.Second)
  claim,err:=s.claimSQLDAG(claimCtx,"")
  stop()
  if err!=nil{
   <-s.sqlDAGSlots
   if !errors.Is(err,pgx.ErrNoRows)&&ctx.Err()==nil{log.Printf("SQL DAG claim failed: %v",err)}
   return
  }
  go func(c sqlDAGClaim){
   defer func(){<-s.sqlDAGSlots}()
   s.runClaimedSQLDAG(ctx,c)
  }(claim)
 }
}
