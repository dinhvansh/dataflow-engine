// dataflowctl controls a local DataFlow bootstrap API.
package main

import (
 "bytes"
 "encoding/json"
 "errors"
 "flag"
 "fmt"
 "io"
 "net/http"
 "net/url"
 "os"
 "strings"
 "time"
)
type config struct{ endpoint,token string;client *http.Client }
func (c config) request(method,path string,body any)error{
 var rd io.Reader
 if body!=nil{b,e:=json.Marshal(body);if e!=nil{return e};rd=bytes.NewReader(b)}
 req,err:=http.NewRequest(method,c.endpoint+path,rd);if err!=nil{return err}
 if c.token!=""{req.Header.Set("Authorization","Bearer "+c.token)}
 if body!=nil{req.Header.Set("Content-Type","application/json")}
 res,err:=c.client.Do(req);if err!=nil{return err};defer res.Body.Close()
 b,err:=io.ReadAll(io.LimitReader(res.Body,65536));if err!=nil{return err}
 fmt.Println(string(b))
 if res.StatusCode<200||res.StatusCode>=300{return fmt.Errorf("HTTP %d",res.StatusCode)}
 return nil
}
func main(){
 if err:=run(os.Args[1:]);err!=nil{fmt.Fprintln(os.Stderr,"dataflowctl:",err);os.Exit(1)}
}
func run(args []string)error{
 if len(args)==0{return errors.New("usage: dataflowctl health | tables SCHEMA | columns SCHEMA TABLE | rowversion-upper | status RUN_UUID | test-load --schema Erp --table Part --database scratch --target part_test --confirm-disposable")}
 endpoint:=strings.TrimRight(os.Getenv("DATAFLOW_API_URL"),"/")
 if endpoint==""{endpoint="http://127.0.0.1:8080"}
 c:=config{endpoint:endpoint,token:os.Getenv("DATAFLOW_API_TOKEN"),client:&http.Client{Timeout:60*time.Minute}}
 switch args[0]{
 case "health":
  if len(args)!=1{return errors.New("health accepts no arguments")}
  return c.request("GET","/healthz",nil)
 case "tables":
  if len(args)!=2{return errors.New("tables requires source schema")}
  return c.request("GET","/api/v1/source/tables?schema="+url.QueryEscape(args[1]),nil)
 case "columns":
  if len(args)!=3{return errors.New("columns requires schema and table")}
  return c.request("GET","/api/v1/source/columns?schema="+url.QueryEscape(args[1])+"&table="+url.QueryEscape(args[2]),nil)
 case "rowversion-upper":
  if len(args)!=1{return errors.New("rowversion-upper accepts no args")}
  return c.request("GET","/api/v1/source/rowversion-upper",nil)
 case "validate-pipeline":
  if len(args)!=2{return errors.New("validate-pipeline requires a JSON file")}
  b,err:=os.ReadFile(args[1]);if err!=nil{return err}
  var definition any
  if err=json.Unmarshal(b,&definition);err!=nil{return err}
  return c.request("POST","/api/v1/pipelines/validate",definition)
 case "status":
  if len(args)!=2{return errors.New("status requires run UUID")}
  if len(args[1])!=36||strings.ContainsAny(args[1],"&?/#"){return errors.New("invalid run UUID")}
  return c.request("GET","/api/v1/bootstrap/status?id="+url.QueryEscape(args[1]),nil)
 case "test-load":
  f:=flag.NewFlagSet("test-load",flag.ContinueOnError)
  schema:=f.String("schema","Erp","source schema")
  table:=f.String("table","","source table")
  database:=f.String("database","scratch","ClickHouse database")
  target:=f.String("target","","ClickHouse disposable target")
  batch:=f.Int("batch",1000,"rows per batch")
  workers:=f.Int("workers",2,"worker count")
  queue:=f.Int("queue",4,"bounded queue")
  atomicReplace:=f.Bool("atomic-replace",true,"stage and atomically EXCHANGE the scratch target (requires CH Atomic database and SQL Server SNAPSHOT)")
  confirm:=f.Bool("confirm-disposable",false,"acknowledge APPEND duplicates and disposable target")
  if err:=f.Parse(args[1:]);err!=nil{return err}
  if !*confirm{return errors.New("test-load requires --confirm-disposable")}
  if *table==""||*target==""{return errors.New("--table and --target required")}
  if c.token==""{return errors.New("DATAFLOW_API_TOKEN required")}
  payload:=map[string]any{"source_schema":*schema,"source_table":*table,"target_database":*database,"target_table":*target,"batch_size":*batch,"workers":*workers,"queue_capacity":*queue,"mode":"full","atomic_replace":*atomicReplace}
  return c.request("POST","/api/v1/bootstrap/runs",payload)
 default:return fmt.Errorf("unknown command %q",args[0])
 }
}
