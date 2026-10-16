package main

import (
 "context"
 "encoding/json"
 "log"
 "errors"
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
func(s *server)startManagedScheduler(parent context.Context){
 if !managedEnabled()||os.Getenv("DATAFLOW_ENABLE_MANAGED_SCHEDULER")!="true"{
  log.Print("managed interval scheduler disabled");return
 }
 go func(){
  ticker:=time.NewTicker(15*time.Second);defer ticker.Stop()
  log.Print("managed scheduler enqueues only; durable worker owns execution")
  for {select{
   case <-parent.Done():return
   case <-ticker.C:
    for i:=0;i<2;i++{
     ctx,stop:=context.WithTimeout(parent,10*time.Second)
     err:=s.claimScheduled(ctx)
     stop()
     if errors.Is(err,pgx.ErrNoRows){break}
     if err!=nil{log.Printf("scheduler enqueue failed: %v",err);break}
    }
  }}
 }()
}
// claimScheduled records a durable queued run; it NEVER starts a worker
// goroutine. Workers claim it using sync_checkpoints fencing in PostgreSQL.
func(s *server)claimScheduled(ctx context.Context)error{
 tx,err:=s.pg.Begin(ctx);if err!=nil{return err}
 defer tx.Rollback(ctx)
 var scheduleID,streamID string
 var interval int
 err=tx.QueryRow(ctx,`SELECT ms.id::text,ms.stream_id::text,ms.interval_seconds
  FROM managed_schedules ms
  WHERE ms.enabled=true AND ms.next_run_at<=clock_timestamp()
   AND NOT EXISTS (
    SELECT 1 FROM managed_runs r
    WHERE r.stream_id=ms.stream_id AND r.status IN ('queued','running')
   )
  ORDER BY ms.next_run_at LIMIT 1
  FOR UPDATE OF ms SKIP LOCKED`).Scan(&scheduleID,&streamID,&interval)
 if err!=nil{return err}
 if _,err=tx.Exec(ctx,`INSERT INTO managed_runs(stream_id,status) VALUES($1::uuid,'queued')`,streamID);err!=nil{return err}
 if _,err=tx.Exec(ctx,`UPDATE managed_schedules SET next_run_at=clock_timestamp()
  +($2::integer*interval '1 second'),updated_at=now() WHERE id=$1::uuid`,
 scheduleID,interval);err!=nil{return err}
 return tx.Commit(ctx)
}
