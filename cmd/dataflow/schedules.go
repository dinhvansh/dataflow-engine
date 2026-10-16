package main

import (
 "context"
 "encoding/json"
 "errors"
 "log"
 "net/http"
 "os"
 "time"

 "github.com/jackc/pgx/v5"
)

type scheduleInput struct{
 StreamID string `json:"stream_id"`
 IntervalSeconds int `json:"interval_seconds"`
 Enabled bool `json:"enabled"`
}
type scheduleRecord struct{
 ScheduleID string `json:"id"`
 StreamID string `json:"stream_id"`
 IntervalSeconds int `json:"interval_seconds"`
 Enabled bool `json:"enabled"`
 NextRunAt time.Time `json:"next_run_at"`
}
func(s *server)scheduleAPI(w http.ResponseWriter,r *http.Request){
 if !s.validBearer(r){http.Error(w,"unauthorized",401);return}
 switch r.Method{
 case http.MethodGet:
  ctx,cancel:=context.WithTimeout(r.Context(),10*time.Second);defer cancel()
  rows,err:=s.pg.Query(ctx,`SELECT id::text,stream_id::text,interval_seconds,enabled,next_run_at
   FROM managed_schedules ORDER BY next_run_at LIMIT 100`)
  if err!=nil{http.Error(w,"cannot list schedules",500);return}
  defer rows.Close()
  result:=make([]scheduleRecord,0)
  for rows.Next(){var item scheduleRecord
   if err=rows.Scan(&item.ScheduleID,&item.StreamID,&item.IntervalSeconds,&item.Enabled,&item.NextRunAt);err!=nil{http.Error(w,"cannot read schedule",500);return}
   result=append(result,item)
  }
  if err=rows.Err();err!=nil{http.Error(w,"cannot read schedules",500);return}
  jsonResponse(w,200,map[string]any{"schedules":result})
 case http.MethodPost:
  if !managedEnabled(){http.Error(w,"managed test is disabled",403);return}
  r.Body=http.MaxBytesReader(w,r.Body,4096)
  dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields()
  var payload scheduleInput
  if err:=dec.Decode(&payload);err!=nil||!validUUID.MatchString(payload.StreamID)||payload.IntervalSeconds<60||payload.IntervalSeconds>86400{
   http.Error(w,"valid stream_id and interval_seconds 60..86400 required",400);return
  }
  ctx,cancel:=context.WithTimeout(r.Context(),10*time.Second);defer cancel()
  var record scheduleRecord
  err:=s.pg.QueryRow(ctx,`INSERT INTO managed_schedules(stream_id,interval_seconds,enabled,next_run_at)
   VALUES($1::uuid,$2,$3,clock_timestamp()+($2::integer*interval '1 second'))
   ON CONFLICT(stream_id) DO UPDATE SET interval_seconds=EXCLUDED.interval_seconds,
    enabled=EXCLUDED.enabled,next_run_at=EXCLUDED.next_run_at,updated_at=now()
   RETURNING id::text,stream_id::text,interval_seconds,enabled,next_run_at`,
   payload.StreamID,payload.IntervalSeconds,payload.Enabled).Scan(&record.ScheduleID,&record.StreamID,&record.IntervalSeconds,&record.Enabled,&record.NextRunAt)
  if err!=nil{http.Error(w,"cannot save schedule",500);return}
  jsonResponse(w,201,record)
 default:http.Error(w,"method not allowed",405)
 }
}
func(s *server)startManagedScheduler(){
 if !managedEnabled()||os.Getenv("DATAFLOW_ENABLE_MANAGED_SCHEDULER")!="true"{
  log.Print("managed interval scheduler disabled")
  return
 }
 go func(){
  tick:=time.NewTicker(15*time.Second);defer tick.Stop()
  log.Print("managed interval scheduler enabled: 15-second due scan")
  for range tick.C{
   // Fail orphaned runs only after a grace period and only when no
   // matching PostgreSQL lease remains active. Never auto-reset cursor.
   recoveryCtx,recoveryCancel:=context.WithTimeout(context.Background(),10*time.Second)
   if err:=s.failOrphanedRuns(recoveryCtx);err!=nil{log.Printf("scheduler orphan scan: %v",err)}
   recoveryCancel()
   // A bounded number of schedules per tick avoids monopolizing API capacity.
   for attempt:=0;attempt<2;attempt++{
    slotAcquired:=false
    select{case s.managedSlots<-struct{}{}:slotAcquired=true;default:}
    if !slotAcquired{break}
    ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second)
    runID,streamID,err:=s.claimScheduled(ctx)
    cancel()
    if err!=nil{<-s.managedSlots
     if !errors.Is(err,pgx.ErrNoRows){log.Printf("scheduler: %v",err)}
     break
    }
    go func(run,stream string){defer func(){<-s.managedSlots}();s.executeManaged(run,stream)}(runID,streamID)
   }
  }
 }()
}
// claimScheduled atomically claims one due schedule and inserts its run.
// PostgreSQL row locking prevents two scheduler replicas claiming the same tick.
// It does NOT provide durable queue worker recovery after process crashes.
func(s *server)claimScheduled(ctx context.Context)(string,string,error){
 tx,err:=s.pg.Begin(ctx);if err!=nil{return "","",err}
 defer tx.Rollback(ctx)
 var scheduleID,streamID string;var interval int
 err=tx.QueryRow(ctx,`SELECT ms.id::text,ms.stream_id::text,ms.interval_seconds
  FROM managed_schedules ms
  WHERE ms.enabled=true AND ms.next_run_at<=clock_timestamp()
  AND NOT EXISTS (
   SELECT 1 FROM managed_runs r
   WHERE r.stream_id=ms.stream_id AND r.status IN ('queued','running')
   AND r.requested_at>clock_timestamp()-interval '3 hours'
  )
  ORDER BY ms.next_run_at LIMIT 1 FOR UPDATE OF ms SKIP LOCKED`).Scan(&scheduleID,&streamID,&interval)
 if err!=nil{return "","",err}
 var runID string
 err=tx.QueryRow(ctx,`INSERT INTO managed_runs(stream_id,status) VALUES($1::uuid,'queued') RETURNING id::text`,streamID).Scan(&runID)
 if err!=nil{return "","",err}
 _,err=tx.Exec(ctx,`UPDATE managed_schedules SET
   next_run_at=clock_timestamp()+($2::integer*interval '1 second'),updated_at=now()
   WHERE id=$1::uuid`,scheduleID,interval)
 if err!=nil{return "","",err}
 if err=tx.Commit(ctx);err!=nil{return "","",err}
 return runID,streamID,nil
}

func(s *server)failOrphanedRuns(ctx context.Context)error{
 _,err:=s.pg.Exec(ctx,`UPDATE managed_runs r
  SET status='failed',
      error_message='Interrupted run with no active lease; check committed checkpoint before retry',
      finished_at=clock_timestamp()
  WHERE r.status IN ('queued','running')
    AND r.requested_at<clock_timestamp()-interval '10 minutes'
    AND NOT EXISTS (
      SELECT 1 FROM sync_checkpoints c
      WHERE c.stream_id=r.stream_id AND c.lease_owner=r.id
        AND c.lease_expires_at>clock_timestamp()
    )`)
 return err
}
