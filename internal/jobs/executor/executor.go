// Package executor runs validated DAGs as independent dependency waves.
// It does not persist a queue. A production scheduler must durably store
// attempts, lease workers and replay unfinished steps on restart.
package executor

import (
 "context"
 "errors"
 "fmt"
 "sync"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/jobs/dag"
)
type Task func(context.Context,string) error
type Options struct {
 Concurrency int
 Timeout time.Duration
 Retries int
}
type Result struct {
 State map[string]dag.Status `json:"state"`
 Attempts map[string]int `json:"attempts"`
 Errors map[string]string `json:"errors,omitempty"`
 StartedAt time.Time `json:"started_at"`
 FinishedAt time.Time `json:"finished_at"`
}
type outcome struct{ id string; attempts int; err error }
func Run(ctx context.Context,plan dag.Plan,task Task,opts Options)(Result,error){
 result:=Result{State:map[string]dag.Status{},Attempts:map[string]int{},Errors:map[string]string{},StartedAt:time.Now().UTC()}
 if err:=plan.Validate();err!=nil{return result,err}
 if task==nil{return result,errors.New("nil task callback")}
 if opts.Concurrency<1||opts.Concurrency>32{return result,errors.New("concurrency must be 1..32")}
 if opts.Retries<0||opts.Retries>5{return result,errors.New("retries must be 0..5")}
 if opts.Timeout<=0||opts.Timeout>time.Hour{return result,errors.New("timeout must be greater than 0 and at most 1 hour")}
 jobs:=make(map[string]string,len(plan.Nodes))
 for _,n:=range plan.Nodes{result.State[n.ID]=dag.Pending;jobs[n.ID]=n.JobID}
 for {
  if err:=ctx.Err();err!=nil{
   for id,status:=range result.State{if status==dag.Pending{result.State[id]=dag.Cancelled}}
   result.FinishedAt=time.Now().UTC()
   return result,err
  }
  blocked,err:=plan.Blocked(result.State);if err!=nil{return result,err}
  for _,id:=range blocked{result.State[id]=dag.Skipped;result.Errors[id]="upstream dependency failed/skipped/cancelled"}
  ready,err:=plan.Ready(result.State);if err!=nil{return result,err}
  if len(ready)==0{break}
  concurrency:=opts.Concurrency
  if concurrency>len(ready){concurrency=len(ready)}
  sem:=make(chan struct{},concurrency)
  completions:=make(chan outcome,len(ready))
  var wg sync.WaitGroup
  for _,id:=range ready{
   result.State[id]=dag.Running
   sem<-struct{}{}
   wg.Add(1)
   go func(nodeID,jobID string){
    defer wg.Done();defer func(){<-sem}()
    attempts,err:=runWithRetries(ctx,jobID,task,opts)
    completions<-outcome{id:nodeID,attempts:attempts,err:err}
   }(id,jobs[id])
  }
  wg.Wait();close(completions)
  for item:=range completions{
   result.Attempts[item.id]=item.attempts
   if item.err!=nil{
    result.State[item.id]=dag.Failed
    result.Errors[item.id]=item.err.Error()
   }else{result.State[item.id]=dag.Succeeded}
  }
 }
 for _,s:=range result.State{
  if s==dag.Pending||s==dag.Running{return result,errors.New("pipeline stalled")}
 }
 result.FinishedAt=time.Now().UTC()
 for _,s:=range result.State{if s==dag.Failed||s==dag.Skipped{return result,errors.New("pipeline contains failed or skipped tasks")}}
 return result,nil
}
func runWithRetries(ctx context.Context,jobID string,task Task,opts Options)(int,error){
 var last error
 for attempt:=1;attempt<=opts.Retries+1;attempt++{
  if err:=ctx.Err();err!=nil{return attempt-1,err}
  stepCtx,cancel:=context.WithTimeout(ctx,opts.Timeout)
  last=task(stepCtx,jobID)
  if last==nil&&stepCtx.Err()!=nil{last=stepCtx.Err()}
  cancel()
  if last==nil{return attempt,nil}
  if ctx.Err()!=nil{return attempt,ctx.Err()}
  if attempt<=opts.Retries{
   delay:=time.Duration(attempt)*100*time.Millisecond
   select{case <-ctx.Done():return attempt,ctx.Err();case <-time.After(delay):}
  }
 }
 return opts.Retries+1,fmt.Errorf("job %q failed after %d attempts: %w",jobID,opts.Retries+1,last)
}
