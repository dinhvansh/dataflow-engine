// DataFlow MCP stdio adapter. This is a starter, not the full management MCP.
package main

import (
 "bufio"
 "bytes"
 "context"
 "encoding/json"
 "fmt"
 "io"
 "net/http"
 "net/url"
 "os"
 "strings"
 "time"
)

type request struct { JSONRPC string `json:"jsonrpc"`; ID json.RawMessage `json:"id"`; Method string `json:"method"`; Params json.RawMessage `json:"params"` }
type server struct { client *http.Client; endpoint,token string; allowWrites bool }
func response(id json.RawMessage, result any) any { return map[string]any{"jsonrpc":"2.0","id":id,"result":result} }
func failure(id json.RawMessage,code int,msg string)any{return map[string]any{"jsonrpc":"2.0","id":id,"error":map[string]any{"code":code,"message":msg}}}
func tool(name,desc string, schema any)any{return map[string]any{"name":name,"description":desc,"inputSchema":schema}}
func shape(required []string,properties map[string]any)any{return map[string]any{"type":"object","required":required,"properties":properties,"additionalProperties":false}}
func textResult(v string,err bool)any{return map[string]any{"content":[]any{map[string]any{"type":"text","text":v}},"isError":err}}
func(s *server) callAPI(ctx context.Context,method,path string,payload any)(string,error){
 var body io.Reader
 if payload!=nil{b,e:=json.Marshal(payload);if e!=nil{return "",e};body=bytes.NewReader(b)}
 req,err:=http.NewRequestWithContext(ctx,method,s.endpoint+path,body);if err!=nil{return "",err}
 if s.token!=""{req.Header.Set("Authorization","Bearer "+s.token)}
 if payload!=nil{req.Header.Set("Content-Type","application/json")}
 res,err:=s.client.Do(req);if err!=nil{return "",err};defer res.Body.Close()
 b,err:=io.ReadAll(io.LimitReader(res.Body,32768));if err!=nil{return "",err}
 if res.StatusCode<200||res.StatusCode>=300{return "",fmt.Errorf("DataFlow API HTTP %d: %s",res.StatusCode,string(b))}
 return string(b),nil
}
func(s *server) handle(r request)any{
 switch r.Method{
 case "initialize":
  return response(r.ID,map[string]any{"protocolVersion":"2024-11-05","capabilities":map[string]any{"tools":map[string]any{}},"serverInfo":map[string]any{"name":"dataflow-mcp","version":"0.1.0"}})
 case "notifications/initialized":return nil
 case "ping":return response(r.ID,map[string]any{})
 case "tools/list":
  t:=[]any{
   tool("dataflow_health","Check the DataFlow backend health",shape(nil,map[string]any{})),
   tool("dataflow_list_recent_runs","List latest PostgreSQL-backed sync runs (read only)",shape(nil,map[string]any{})),
   tool("dataflow_list_managed_streams","List registered streams and committed native rowversion checkpoints",shape(nil,map[string]any{})),
   tool("dataflow_list_sql_jobs","Read registered SQL Job definitions (may contain internal SQL; only connect trusted agents)",shape(nil,map[string]any{})),
   tool("dataflow_list_executable_dags","Read saved executable DAG definitions",shape(nil,map[string]any{})),
   tool("dataflow_list_reconciliations","View last 50 PK snapshot comparisons (read-only)",shape(nil,map[string]any{})),
   tool("dataflow_managed_run_status","Inspect an asynchronous managed incremental run by UUID",shape([]string{"run_id"},map[string]any{"run_id":map[string]any{"type":"string"}})),
   tool("dataflow_get_run","Read status of one known run UUID",shape([]string{"run_id"},map[string]any{"run_id":map[string]any{"type":"string"}})),
   tool("dataflow_list_tables","List SQL Server tables in one schema using read-only metadata",shape([]string{"schema"},map[string]any{"schema":map[string]any{"type":"string"}})),
   tool("dataflow_inspect_columns","Read SQL Server column names/types/primary keys and rowversion hints",shape([]string{"schema","table"},map[string]any{"schema":map[string]any{"type":"string"},"table":map[string]any{"type":"string"}})),
   tool("dataflow_rowversion_upper","Read current SQL Server MIN_ACTIVE_ROWVERSION boundary; NOT a checkpoint",shape(nil,map[string]any{})),
   tool("dataflow_validate_pipeline","Validate a pipeline DAG for missing dependencies/cycles; does not execute jobs",shape([]string{"nodes"},map[string]any{"nodes":map[string]any{"type":"array","items":map[string]any{"type":"object","required":[]string{"id","job_id","requires"},"properties":map[string]any{"id":map[string]any{"type":"string"},"job_id":map[string]any{"type":"string"},"requires":map[string]any{"type":"array","items":map[string]any{"type":"string"}}}}}})),
  }
  if s.allowWrites{t=append(t,tool("dataflow_run_test_load","Starts an experimental append-only full load into a disposable ClickHouse target. REQUIRES user approval; never use against production.",shape([]string{"source_schema","source_table","target_database","target_table","batch_size","workers","queue_capacity"},map[string]any{
   "source_schema":map[string]any{"type":"string"},"source_table":map[string]any{"type":"string"},"target_database":map[string]any{"type":"string"},"target_table":map[string]any{"type":"string"},"batch_size":map[string]any{"type":"integer","minimum":100,"maximum":100000},"workers":map[string]any{"type":"integer","minimum":1,"maximum":32},"queue_capacity":map[string]any{"type":"integer","minimum":1,"maximum":100},
  })))}
  if s.allowWrites {
   t=append(t,tool("dataflow_run_managed_stream","Start an opt-in scratch-only managed incremental stream; requires explicit user approval.",shape([]string{"stream_id"},map[string]any{"stream_id":map[string]any{"type":"string"}})))
   t=append(t,tool("dataflow_run_executable_dag","Execute enabled SELECT-only jobs for a registered DAG; requires explicit human approval.",shape([]string{"dag_id"},map[string]any{"dag_id":map[string]any{"type":"string"}})))
   t=append(t,tool("dataflow_capture_reconciliation","Capture a full PK snapshot for a scratch stream. Heavy ERP SELECT read; requires user approval and server flag.",shape([]string{"stream_id"},map[string]any{"stream_id":map[string]any{"type":"string"}})))
  }
  return response(r.ID,map[string]any{"tools":t})
 case "tools/call":
  var p struct{Name string `json:"name"`;Arguments json.RawMessage `json:"arguments"`}
  if err:=json.Unmarshal(r.Params,&p);err!=nil{return failure(r.ID,-32602,"invalid parameters")}
  var out string;var err error
  ctx,cancel:=context.WithTimeout(context.Background(),10*time.Minute);defer cancel()
  switch p.Name{
  case "dataflow_health":
   out,err=s.callAPI(ctx,"GET","/healthz",nil)
  case "dataflow_list_recent_runs":
   out,err=s.callAPI(ctx,"GET","/api/v1/bootstrap/history?limit=20",nil)
  case "dataflow_list_managed_streams":
   out,err=s.callAPI(ctx,"GET","/api/v1/managed/streams",nil)
  case "dataflow_list_sql_jobs":
   out,err=s.callAPI(ctx,"GET","/api/v1/sql/jobs",nil)
  case "dataflow_list_executable_dags":
   out,err=s.callAPI(ctx,"GET","/api/v1/sql/dags",nil)
  case "dataflow_run_executable_dag":
   if !s.allowWrites{return response(r.ID,textResult("write tools disabled",true))}
   var a struct{DAGID string `json:"dag_id"`}
   if json.Unmarshal(p.Arguments,&a)!=nil||len(a.DAGID)!=36||strings.ContainsAny(a.DAGID,"/?&#"){return response(r.ID,textResult("invalid DAG ID",true))}
   out,err=s.callAPI(ctx,"POST","/api/v1/sql/dag-runs",a)
  case "dataflow_list_reconciliations":
   out,err=s.callAPI(ctx,"GET","/api/v1/reconcile/runs",nil)
  case "dataflow_capture_reconciliation":
   if !s.allowWrites{return response(r.ID,textResult("write tool disabled",true))}
   var a struct{StreamID string `json:"stream_id"`}
   if json.Unmarshal(p.Arguments,&a)!=nil||len(a.StreamID)!=36||strings.ContainsAny(a.StreamID,"/?&#"){return response(r.ID,textResult("invalid stream id",true))}
   out,err=s.callAPI(ctx,"POST","/api/v1/reconcile/runs",a)
  case "dataflow_managed_run_status":
   var a struct{RunID string `json:"run_id"`}
   if json.Unmarshal(p.Arguments,&a)!=nil||len(a.RunID)!=36||strings.ContainsAny(a.RunID,"/?&#"){return response(r.ID,textResult("invalid run id",true))}
   out,err=s.callAPI(ctx,"GET","/api/v1/managed/runs?id="+url.QueryEscape(a.RunID),nil)
  case "dataflow_run_managed_stream":
   if !s.allowWrites{return response(r.ID,textResult("writes are disabled",true))}
   var a struct{StreamID string `json:"stream_id"`}
   if json.Unmarshal(p.Arguments,&a)!=nil||len(a.StreamID)!=36||strings.ContainsAny(a.StreamID,"/?&#"){return response(r.ID,textResult("invalid stream id",true))}
   out,err=s.callAPI(ctx,"POST","/api/v1/managed/runs",a)
  case "dataflow_list_tables":
   var a struct{Schema string `json:"schema"`}
   if json.Unmarshal(p.Arguments,&a)!=nil||a.Schema==""{return response(r.ID,textResult("schema required",true))}
   out,err=s.callAPI(ctx,"GET","/api/v1/source/tables?schema="+url.QueryEscape(a.Schema),nil)
  case "dataflow_inspect_columns":
   var a struct{Schema string `json:"schema"`;Table string `json:"table"`}
   if json.Unmarshal(p.Arguments,&a)!=nil||a.Schema==""||a.Table==""{return response(r.ID,textResult("schema and table required",true))}
   out,err=s.callAPI(ctx,"GET","/api/v1/source/columns?schema="+url.QueryEscape(a.Schema)+"&table="+url.QueryEscape(a.Table),nil)
  case "dataflow_rowversion_upper":
   out,err=s.callAPI(ctx,"GET","/api/v1/source/rowversion-upper",nil)
  case "dataflow_validate_pipeline":
   var plan struct{Nodes []struct{ID string `json:"id"`;JobID string `json:"job_id"`;Requires []string `json:"requires"`} `json:"nodes"`}
   if json.Unmarshal(p.Arguments,&plan)!=nil||len(plan.Nodes)==0{return response(r.ID,textResult("invalid pipeline nodes",true))}
   out,err=s.callAPI(ctx,"POST","/api/v1/pipelines/validate",plan)
  case "dataflow_get_run":
   var a struct{RunID string `json:"run_id"`}
   if json.Unmarshal(p.Arguments,&a)!=nil||len(a.RunID)!=36||strings.ContainsAny(a.RunID,"/?&#"){return response(r.ID,textResult("Invalid run UUID",true))}
   out,err=s.callAPI(ctx,"GET","/api/v1/bootstrap/status?id="+a.RunID,nil)
  case "dataflow_run_test_load":
   if !s.allowWrites{return response(r.ID,textResult("Write operations are disabled",true))}
   var a struct{SourceSchema string `json:"source_schema"`;SourceTable string `json:"source_table"`;TargetDatabase string `json:"target_database"`;TargetTable string `json:"target_table"`;BatchSize int `json:"batch_size"`;Workers int `json:"workers"`;QueueCapacity int `json:"queue_capacity"`}
   if json.Unmarshal(p.Arguments,&a)!=nil||a.BatchSize<100||a.BatchSize>100000||a.Workers<1||a.Workers>32||a.QueueCapacity<1||a.QueueCapacity>100||a.SourceSchema==""||a.SourceTable==""||a.TargetDatabase==""||a.TargetTable==""{return response(r.ID,textResult("Invalid sync configuration",true))}
   out,err=s.callAPI(ctx,"POST","/api/v1/bootstrap/runs",a)
  default:return failure(r.ID,-32602,"unknown tool")
  }
  if err!=nil{return response(r.ID,textResult(err.Error(),true))}
  return response(r.ID,textResult(out,false))
 default:
  if len(r.ID)==0{return nil}
  return failure(r.ID,-32601,"method not found")
 }
}
func main(){
 s:=server{client:&http.Client{Timeout:10*time.Minute},endpoint:strings.TrimRight(os.Getenv("DATAFLOW_API_URL"),"/"),token:os.Getenv("DATAFLOW_API_TOKEN"),allowWrites:os.Getenv("DATAFLOW_MCP_ALLOW_WRITES")=="true"}
 if s.endpoint==""{s.endpoint="http://127.0.0.1:8080"}
 in:=bufio.NewScanner(os.Stdin);in.Buffer(make([]byte,65536),4*1024*1024)
 writer:=bufio.NewWriter(os.Stdout);defer writer.Flush()
 for in.Scan(){
  var r request
  if err:=json.Unmarshal(in.Bytes(),&r);err!=nil{b,_:=json.Marshal(failure(nil,-32700,"invalid JSON"));fmt.Fprintln(writer,string(b));writer.Flush();continue}
  result:=s.handle(r)
  if result==nil{continue}
  b,err:=json.Marshal(result);if err!=nil{continue}
  fmt.Fprintln(writer,string(b));writer.Flush()
 }
 if err:=in.Err();err!=nil{fmt.Fprintln(os.Stderr,err)}
}
