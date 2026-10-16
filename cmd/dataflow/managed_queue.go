package main

import (
 "context"
 "errors"
 "fmt"
 "log"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/connectors/clickhouse"
 "github.com/dinhvansh/dataflow-engine/internal/connectors/sqlserver"
 "github.com/dinhvansh/dataflow-engine/internal/cursor"
 "github.com/dinhvansh/dataflow-engine/internal/engine"
 pgstate "github.com/dinhvansh/dataflow-engine/internal/state/postgres"
 "github.com/jackc/pgx/v5"
)

const managedMaxAttempts=3
const managedLeaseDuration=5*time.Minute

type managedClaim struct{
 RunID string
 StreamID string
 OwnerID string // unique per ATTEMPT: ClickHouse _run_id, not logical managed run ID
 Fence int64
 Generation int64
 Lower cursor.RowVersion
 Attempts int
}

// claimManagedRun uses a single PostgreSQL transaction to lease BOTH the
// logical run and its stream checkpoint. No pending run is lost if a Go worker
// exits between enqueue and execution; the polling dispatcher claims it.
func(s *server)claimManagedRun(ctx context.Context,onlyID string)(managedClaim,error){
 var c managedClaim
 if onlyID!=""&&!validUUID.MatchString(onlyID){return c,errors.New("invalid managed run ID")}
 tx,err:=s.pg.Begin(ctx);if err!=nil{return c,err}
 defer tx.Rollback(ctx)
 // Only exhausted, expired runs are terminally failed. Never move cursor.
 _,err=tx.Exec(ctx,`UPDATE managed_runs SET status='failed',
    error_message='Maximum restart attempts exceeded; checkpoint unchanged',
    finished_at=clock_timestamp(),lease_owner=NULL,lease_expires_at=NULL
    WHERE status='running' AND lease_expires_at<clock_timestamp()
    AND dispatch_attempts >= $1`,managedMaxAttempts)
 if err!=nil{return c,err}
 var oldFence int64
 var raw []byte
 err=tx.QueryRow(ctx,`
 SELECT r.id::text,r.stream_id::text,c.lower_cursor,c.generation,c.fence_token,r.dispatch_attempts
 FROM managed_runs r JOIN sync_checkpoints c ON c.stream_id=r.stream_id
 WHERE r.dispatch_attempts<$1
   AND ($2::text='' OR r.id::text=$2)
   AND (r.status='queued' OR
       (r.status='running' AND r.lease_expires_at<clock_timestamp()))
   AND (c.lease_owner IS NULL OR c.lease_expires_at<clock_timestamp())
 ORDER BY r.requested_at,r.id
 LIMIT 1 FOR UPDATE OF r,c SKIP LOCKED
 `,managedMaxAttempts,onlyID).Scan(&c.RunID,&c.StreamID,&raw,&c.Generation,&oldFence,&c.Attempts)
 if err!=nil{return c,err}
 c.Lower,err=cursor.FromBytes(raw);if err!=nil{return c,err}
 c.Attempts++
 // New UUID per attempt separates partially inserted events after a crash.
 err=tx.QueryRow(ctx,`
 UPDATE sync_checkpoints SET lease_owner=gen_random_uuid(),fence_token=fence_token+1,
    lease_expires_at=clock_timestamp()+interval '5 minutes',updated_at=now()
 WHERE stream_id=$1::uuid AND fence_token=$2 AND generation=$3
   AND (lease_owner IS NULL OR lease_expires_at<clock_timestamp())
 RETURNING lease_owner::text,fence_token
 `,c.StreamID,oldFence,c.Generation).Scan(&c.OwnerID,&c.Fence)
 if err!=nil{return c,err}
 tag,err:=tx.Exec(ctx,`UPDATE managed_runs SET status='running',
    dispatch_attempts=$4,lease_owner=$2::uuid,fence_token=$3,
    lease_expires_at=clock_timestamp()+interval '5 minutes',
    started_at=COALESCE(started_at,clock_timestamp()),
    error_message=NULL
 WHERE id=$1::uuid AND status IN ('queued','running')`,c.RunID,c.OwnerID,c.Fence,c.Attempts)
 if err!=nil{return c,err}
 if tag.RowsAffected()!=1{return c,errors.New("managed run claim lost")}
 if err=tx.Commit(ctx);err!=nil{return c,err}
 return c,nil
}

func(s *server)renewManagedRun(ctx context.Context,c managedClaim)error{
 tx,err:=s.pg.Begin(ctx);if err!=nil{return err};defer tx.Rollback(ctx)
 tag,err:=tx.Exec(ctx,`UPDATE sync_checkpoints
  SET lease_expires_at=clock_timestamp()+interval '5 minutes'
  WHERE stream_id=$1::uuid AND lease_owner=$2::uuid AND fence_token=$3
    AND lease_expires_at>clock_timestamp()`,c.StreamID,c.OwnerID,c.Fence)
 if err!=nil{return err}
 if tag.RowsAffected()!=1{return pgstate.ErrLeaseLost}
 tag,err=tx.Exec(ctx,`UPDATE managed_runs
   SET lease_expires_at=clock_timestamp()+interval '5 minutes'
   WHERE id=$1::uuid AND lease_owner=$2::uuid AND fence_token=$3
      AND status='running' AND lease_expires_at>clock_timestamp()`,
 c.RunID,c.OwnerID,c.Fence)
 if err!=nil{return err}
 if tag.RowsAffected()!=1{return pgstate.ErrLeaseLost}
 return tx.Commit(ctx)
}

// finishManagedSuccess atomically updates checkpoint and terminal managed run.
// A partial ClickHouse write never reaches this method. A stale worker cannot
// move cursor or mark success. The target rows have been count-verified first.
func(s *server)finishManagedSuccess(ctx context.Context,c managedClaim,next cursor.RowVersion,stats engine.Stats)error{
 if next.Compare(c.Lower)<0{return pgstate.ErrRegression}
 tx,err:=s.pg.Begin(ctx);if err!=nil{return err};defer tx.Rollback(ctx)
 tag,err:=tx.Exec(ctx,`UPDATE sync_checkpoints SET lower_cursor=$5,generation=generation+1,
    lease_owner=NULL,lease_expires_at=NULL,updated_at=now()
  WHERE stream_id=$1::uuid AND lease_owner=$2::uuid AND fence_token=$3
    AND generation=$4 AND lease_expires_at>clock_timestamp()
    AND lower_cursor <= $5`,c.StreamID,c.OwnerID,c.Fence,c.Generation,next.Bytes())
 if err!=nil{return err}
 if tag.RowsAffected()!=1{return pgstate.ErrLeaseLost}
 tag,err=tx.Exec(ctx,`UPDATE managed_runs SET status='succeeded',lower_cursor=$5,
    upper_cursor=$6,rows_written=$7,batches_written=$8,finished_at=now(),
    lease_owner=NULL,lease_expires_at=NULL,error_message=NULL
  WHERE id=$1::uuid AND stream_id=$2::uuid AND lease_owner=$3::uuid
    AND fence_token=$4 AND status='running'`,
 c.RunID,c.StreamID,c.OwnerID,c.Fence,c.Lower.Bytes(),next.Bytes(),stats.Rows,stats.Batches)
 if err!=nil{return err}
 if tag.RowsAffected()!=1{return pgstate.ErrLeaseLost}
 return tx.Commit(ctx)
}

// finishManagedFailure is terminal for a known IO/data error. The checkpoint
// stays unchanged. Crash/cancellation is NOT finalized: the lease expires
// and the dispatcher reclaims that run (up to managedMaxAttempts).
func(s *server)finishManagedFailure(ctx context.Context,c managedClaim,reason error,stats engine.Stats)error{
 tx,err:=s.pg.Begin(ctx);if err!=nil{return err};defer tx.Rollback(ctx)
 tag,err:=tx.Exec(ctx,`UPDATE sync_checkpoints SET lease_owner=NULL,
    lease_expires_at=NULL WHERE stream_id=$1::uuid AND lease_owner=$2::uuid
    AND fence_token=$3 AND lease_expires_at>clock_timestamp()`,
 c.StreamID,c.OwnerID,c.Fence)
 if err!=nil{return err}
 if tag.RowsAffected()!=1{return pgstate.ErrLeaseLost}
 tag,err=tx.Exec(ctx,`UPDATE managed_runs SET status='failed',
    lower_cursor=$5,rows_written=$6,batches_written=$7,
    error_message=$8,finished_at=now(),lease_owner=NULL,lease_expires_at=NULL
    WHERE id=$1::uuid AND stream_id=$2::uuid AND lease_owner=$3::uuid
    AND fence_token=$4 AND status='running'`,
 c.RunID,c.StreamID,c.OwnerID,c.Fence,c.Lower.Bytes(),stats.Rows,stats.Batches,reason.Error())
 if err!=nil{return err}
 if tag.RowsAffected()!=1{return pgstate.ErrLeaseLost}
 return tx.Commit(ctx)
}
func(s *server)checkManagedSchema(ctx context.Context)error{
 if !managedEnabled(){return nil}
 rows,err:=s.pg.Query(ctx,`SELECT dispatch_attempts,lease_owner,lease_expires_at,
   fence_token FROM managed_runs LIMIT 0`)
 if err!=nil{return fmt.Errorf("managed runner requires PostgreSQL migration 009: %w",err)}
 defer rows.Close()
 if err:=rows.Err();err!=nil{return err}
 var valid bool
 err=s.pg.QueryRow(ctx,`SELECT EXISTS(
  SELECT 1 FROM pg_class idx
  JOIN pg_index ix ON ix.indexrelid=idx.oid
  WHERE idx.relname='managed_runs_one_active_stream'
    AND ix.indisunique AND ix.indisvalid
 )`).Scan(&valid)
 if err!=nil{return fmt.Errorf("managed active-stream index inspection: %w",err)}
 if !valid{return errors.New("managed runner requires valid single-active-stream index from migration 009")}
 return nil
}
func(s *server)runClaimedManaged(parent context.Context,c managedClaim){
 ctx,cancel:=context.WithTimeout(parent,2*time.Hour);defer cancel()
 done:=make(chan struct{})
 go func(){
  defer close(done)
  t:=time.NewTicker(30*time.Second);defer t.Stop()
  for {select{
   case <-ctx.Done():return
   case <-t.C:
    heartCtx,stop:=context.WithTimeout(ctx,10*time.Second)
    err:=s.renewManagedRun(heartCtx,c);stop()
    if err!=nil{log.Printf("managed %s lease lost: %v",c.RunID,err);cancel();return}
  }}
 }()
 defer func(){cancel();<-done}()
 stats:=engine.Stats{}
 var cfg managedStream
 err:=s.pg.QueryRow(ctx,`SELECT source_schema,source_table,target_database,target_table,
    batch_size,workers,queue_capacity FROM sync_streams WHERE id=$1::uuid`,
 c.StreamID).Scan(&cfg.SourceSchema,&cfg.SourceTable,&cfg.TargetDatabase,&cfg.TargetTable,
 &cfg.BatchSize,&cfg.Workers,&cfg.QueueCapacity)
 if err==nil&&(cfg.TargetDatabase!=scratchDatabase()||cfg.TargetTable!="dataflow_raw_events"){
  err=errors.New("managed destination must be isolated scratch raw events")
 }
 if err==nil{
  reader:=&sqlserver.RowVersionReader{DB:s.sql,Schema:cfg.SourceSchema,Table:cfg.SourceTable,
   BatchSize:cfg.BatchSize,Lower:c.Lower}
  dest:=&clickhouse.Writer{Endpoint:s.chURL,Database:cfg.TargetDatabase,Table:cfg.TargetTable,
   Username:s.chUser,Password:s.chPass}
  writer:=&clickhouse.EventWriter{Destination:dest,StreamID:c.StreamID,RunID:c.OwnerID}
  stats,err=engine.Run(ctx,engine.Config{Workers:cfg.Workers,QueueCapacity:cfg.QueueCapacity},reader,writer)
  if err==nil{err=ctx.Err()}
  if err==nil{
   // _run_id is the unique attempt owner, not the logical run UUID.
   // Old partial attempts cannot inflate this attempt's row count.
   err=verifyManagedAttempt(ctx,dest,c.OwnerID,stats.Rows)
   if err==nil{
    finishCtx,stop:=context.WithTimeout(ctx,20*time.Second)
    err=s.finishManagedSuccess(finishCtx,c,reader.Upper,stats)
    stop()
    if err==nil{return}
    // An ambiguous PG commit may have succeeded; don't mark it failed.
    log.Printf("managed %s checkpoint/terminal commit uncertain: %v",c.RunID,err)
    return
   }
  }
 }
 if err==nil{return}
 if ctx.Err()!=nil{return} // shutdown or lost lease: reclaim from last checkpoint
 finalCtx,stop:=context.WithTimeout(context.Background(),10*time.Second)
 defer stop()
 if failErr:=s.finishManagedFailure(finalCtx,c,err,stats);failErr!=nil{
  log.Printf("managed %s terminal failure write rejected: %v",c.RunID,failErr)
 }
}
// verifyManagedAttempt rejects a mismatch before any checkpoint mutation.
// Each attempt has a distinct _run_id, isolating partial previous attempts.
func verifyManagedAttempt(ctx context.Context,dest *clickhouse.Writer,attemptID string,expected uint64)error{
 n,err:=dest.CountRunRows(ctx,attemptID)
 if err!=nil{return fmt.Errorf("destination verification failed: %w",err)}
 if n!=expected{return fmt.Errorf("destination mismatch for attempt: expected %d rows got %d",expected,n)}
 return nil
}
func(s *server)startManagedDispatcher(ctx context.Context){
 if !managedEnabled(){log.Print("managed durable dispatcher disabled");return}
 if s.managedSlots==nil{s.managedSlots=make(chan struct{},2)}
 go func(){
  ticker:=time.NewTicker(2*time.Second);defer ticker.Stop()
  s.dispatchManaged(ctx)
  for {select{
   case <-ctx.Done():return
   case <-ticker.C:s.dispatchManaged(ctx)
  }}
 }()
}
func(s *server)dispatchManaged(ctx context.Context){
 for n:=0;n<cap(s.managedSlots);n++{
  select{case s.managedSlots<-struct{}{}:default:return}
  claimCtx,stop:=context.WithTimeout(ctx,10*time.Second)
  claim,err:=s.claimManagedRun(claimCtx,"")
  stop()
  if err!=nil{
   <-s.managedSlots
   if !errors.Is(err,pgx.ErrNoRows)&&ctx.Err()==nil{log.Printf("managed claim failed: %v",err)}
   return
  }
  go func(c managedClaim){defer func(){<-s.managedSlots}();s.runClaimedManaged(ctx,c)}(claim)
 }
}
