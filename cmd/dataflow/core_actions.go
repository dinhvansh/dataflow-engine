package main

import (
 "context"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "strings"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/safetable"
)

// CORE-RUN-4 explicit, isolated scratch-only rollback. It never touches a
// SQL Server source or changes any source checkpoint. The old physical UUID
// must be the exact backup left by this run's atomic EXCHANGE.
func(s *server)coreActionAPI(w http.ResponseWriter,r *http.Request){
 if !s.validBearer(r){http.Error(w,"unauthorized",401);return}
 if !corePipelineEnabled(){http.Error(w,"isolated core actions disabled",403);return}
 if r.Method!=http.MethodPost{http.Error(w,"POST required",405);return}
 r.Body=http.MaxBytesReader(w,r.Body,4096)
 d:=json.NewDecoder(r.Body);d.DisallowUnknownFields()
 var req struct{
  RunID string `json:"run_id"`
  Action string `json:"action"`
  Confirm string `json:"confirm"`
 }
 if d.Decode(&req)!=nil||!validUUID.MatchString(req.RunID)||
   req.Action!="rollback"||req.Confirm!="ROLLBACK"{
  http.Error(w,"explicit rollback and ROLLBACK confirmation required",400);return
 }
 var extra any
 if err:=d.Decode(&extra);!errors.Is(err,io.EOF){
  http.Error(w,"extra JSON not allowed",400);return
 }
 ctx,stop:=context.WithTimeout(r.Context(),25*time.Second);defer stop()
 tx,err:=s.pg.Begin(ctx);if err!=nil{http.Error(w,"rollback state unavailable",503);return}
 defer tx.Rollback(ctx)
 var status,target,stage string
 var stageUUID,oldUUID *string
 err=tx.QueryRow(ctx,`SELECT status,target_name,stage_name,stage_uuid,old_uuid
  FROM core_pipeline_runs WHERE id=$1::uuid FOR UPDATE`,req.RunID).
  Scan(&status,&target,&stage,&stageUUID,&oldUUID)
 if err!=nil{http.Error(w,"core run not found",404);return}
 if status!="succeeded"&&status!="rolled_back"{
  http.Error(w,"only successful core publication can be rolled back",409);return
 }
 if !strings.HasPrefix(target,"dft_core_")||stageUUID==nil||
   oldUUID==nil||*oldUUID==""||*stageUUID==""||
   safetable.SafeNames(target,stage)!=nil{
  http.Error(w,"previous immutable physical target is unavailable",409);return
 }
 writer:=safetable.Client{Endpoint:s.chURL,User:s.chUser,Password:s.chPass}
 pub:=safetable.Publication{
  Target:target,Stage:stage,StageUUID:*stageUUID,OldUUID:*oldUUID,
 }
 if err=writer.Rollback(ctx,pub);err!=nil{
  // A newer publication, missing backup or ambiguous ClickHouse response
  // must never be silently treated as success.
  http.Error(w,"rollback blocked: target or backup identity changed",409);return
 }
 if _,err=tx.Exec(ctx,`UPDATE core_pipeline_runs SET status='rolled_back',
   rolled_back_at=COALESCE(rolled_back_at,clock_timestamp())
   WHERE id=$1::uuid AND status IN('succeeded','rolled_back')`,
   req.RunID);err!=nil{http.Error(w,"rollback state not persisted",503);return}
 if err=tx.Commit(ctx);err!=nil{http.Error(w,"rollback commit uncertain; verify status before retry",503);return}
 jsonResponse(w,200,map[string]any{
  "run_id":req.RunID,"status":"rolled_back",
  "target":"scratch."+target,"checkpoint_changed":false,
 })
}
