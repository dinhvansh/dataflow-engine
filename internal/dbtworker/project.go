// Package dbtworker writes ephemeral dbt projects and runs a fixed dbt command.
// Keep this binary in a separate low-privilege Docker container. Never give it
// an ERP SQL Server connection or the DataFlow API writer credentials.
package dbtworker

import(
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "regexp"
 "strings"
 "time"
)
var safe=regexp.MustCompile("^[a-z][a-z0-9_]{0,85}$")
type Config struct{
 Binary string
 Host string
 Port int
 Username string
 Password string
 Secure bool
 Database string
}
type Plan struct{
 Name string
 SQL string
 Target string
}
type Outcome struct{
 Success bool \`json:"success"\`
 Log string \`json:"log"\`
 Results json.RawMessage \`json:"results"\`
 ElapsedSeconds float64 \`json:"elapsed_seconds"\`
}
func ValidatePlan(p Plan,c Config)error{
 if !safe.MatchString(p.Name)||p.Target!="dfv_"+p.Name{return errors.New("unsafe dbt model or target name")}
 if c.Database!="scratch"{return errors.New("dbt worker requires scratch database")}
 if c.Host==""||c.Port<1||c.Port>65535||c.Username==""{return errors.New("dbt connection incomplete")}
 sql:=strings.TrimSpace(p.SQL)
 if len(sql)<8||len(sql)>16000||!strings.HasPrefix(strings.ToUpper(sql),"SELECT "){return errors.New("only bounded SELECT models")}
 for _,bad:=range []string{";","--","/*","*/","{{","{%","{#"}{
  if strings.Contains(sql,bad){return errors.New("SQL contains unsupported dbt template or SQL delimiter")}
 }
 return nil
}
func jsonString(s string)string{
 x,_:=json.Marshal(s);return string(x)
}
func createProject(root string,p Plan,c Config)error{
 if err:=ValidatePlan(p,c);err!=nil{return err}
 if err:=os.MkdirAll(filepath.Join(root,"models"),0700);err!=nil{return err}
 project:="name: dataflow_workbench\nversion: '1.0.0'\nconfig-version: 2\nprofile: dataflow_scratch\nmodel-paths: ['models']\nmodels:\n  dataflow_workbench:\n    +materialized: view\n"
 profile:=fmt.Sprintf("dataflow_scratch:\n  target: dev\n  outputs:\n    dev:\n      type: clickhouse\n      host: %s\n      port: %d\n      schema: scratch\n      user: %s\n      password: %s\n      secure: %t\n      threads: 1\n",jsonString(c.Host),c.Port,jsonString(c.Username),jsonString(c.Password),c.Secure)
 if err:=os.WriteFile(filepath.Join(root,"dbt_project.yml"),[]byte(project),0600);err!=nil{return err}
 if err:=os.WriteFile(filepath.Join(root,"profiles.yml"),[]byte(profile),0600);err!=nil{return err}
 // No untrusted Jinja: dbt evaluates templates inside SQL models.
 model:="{{ config(materialized='view') }}\n"+strings.TrimSpace(p.SQL)+"\n"
 return os.WriteFile(filepath.Join(root,"models",p.Target+".sql"),[]byte(model),0600)
}
type boundedBuffer struct{bytes.Buffer;max int}
func(b *boundedBuffer)Write(p []byte)(int,error){
 n:=len(p)
 if n>b.max{p=p[n-b.max:]}
 if b.Len()+len(p)>b.max{
  old:=append([]byte(nil),b.Bytes()...)
  b.Reset()
  keep:=b.max-len(p)
  if keep>0&&len(old)>keep{old=old[len(old)-keep:]}
  if keep>0{_,_=b.Buffer.Write(old)}
 }
 _,_=b.Buffer.Write(p);return n,nil
}
func redact(in,secret string)string{
 if secret!=""{in=strings.ReplaceAll(in,secret,"[REDACTED]")}
 return in
}
// Execute returns structured dbt run_results.json if available and an
// independently bounded error log. It never returns the project directory.
func Execute(ctx context.Context,p Plan,c Config)(Outcome,error){
 out:=Outcome{}
 if err:=ValidatePlan(p,c);err!=nil{return out,err}
 root,err:=os.MkdirTemp("","dataflow-dbt-");if err!=nil{return out,err}
 defer os.RemoveAll(root)
 if err=createProject(root,p,c);err!=nil{return out,err}
 binary:=c.Binary;if binary==""{binary="dbt"}
 args:=[]string{"run","--select",p.Target,"--project-dir",root,"--profiles-dir",root,"--target","dev","--threads","1","--no-use-colors"}
 command:=exec.CommandContext(ctx,binary,args...)
 command.Dir=root
 command.Env=append(os.Environ(),"DBT_SEND_ANONYMOUS_USAGE_STATS=false","DO_NOT_TRACK=1",
  "DBT_LOG_PATH="+filepath.Join(root,"logs"),"DBT_TARGET_PATH="+filepath.Join(root,"target"))
 buff:=&boundedBuffer{max:12000};command.Stdout=buff;command.Stderr=buff
 start:=time.Now()
 runErr:=command.Run()
 out.ElapsedSeconds=time.Since(start).Seconds()
 out.Log=redact(buff.String(),c.Password)
 artifact,readErr:=os.ReadFile(filepath.Join(root,"target","run_results.json"))
 if readErr==nil&&len(artifact)<512*1024&&json.Valid(artifact){
  var summary struct{
   Results []struct{Status string \`json:"status"\`} \`json:"results"\`
  }
  if json.Unmarshal(artifact,&summary)==nil{
   out.Results=json.RawMessage(artifact)
   out.Success=runErr==nil&&len(summary.Results)==1&&summary.Results[0].Status=="success"
  }
 }
 if err:=ctx.Err();err!=nil{return out,err}
 if !out.Success{
  if runErr!=nil{return out,fmt.Errorf("dbt process failed: %w",runErr)}
  return out,errors.New("dbt model did not report exactly one successful materialization")
 }
 return out,nil
}
