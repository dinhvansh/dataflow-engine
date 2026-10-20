// Package safetable publishes VERSIONED dbt staging tables in a ClickHouse
// Atomic scratch database. It is intentionally separate from raw ingestion.
package safetable

import(
 "context"
 "encoding/base64"
 "errors"
 "fmt"
 "io"
 "net/http"
 "net/url"
 "regexp"
 "strconv"
 "strings"
 "time"
)
var identifier=regexp.MustCompile("^[a-z][a-z0-9_]{0,85}$")
var stageID=regexp.MustCompile("^dft_stage_[a-f0-9]{32}$")
type Client struct{Endpoint,User,Password string}
func (c Client)Query(ctx context.Context,sql string)(string,error){
 if len(sql)>3000{return "",errors.New("query too large")}
 u,err:=url.Parse(c.Endpoint);if err!=nil{return "",err}
 args:=u.Query();args.Set("query",sql);u.RawQuery=args.Encode()
 req,err:=http.NewRequestWithContext(ctx,http.MethodPost,u.String(),nil);if err!=nil{return "",err}
 if c.User!=""{req.Header.Set("Authorization","Basic "+base64.StdEncoding.EncodeToString([]byte(c.User+":"+c.Password)))}
 cli:=&http.Client{Timeout:35*time.Second}
 response,err:=cli.Do(req);if err!=nil{return "",err}
 defer response.Body.Close()
 body,err:=io.ReadAll(io.LimitReader(response.Body,4096))
 if err!=nil{return "",err}
 if response.StatusCode/100!=2{return "",fmt.Errorf("clickhouse HTTP %d (query rejected)",response.StatusCode)}
 return strings.TrimSpace(string(body)),nil
}
func SafeNames(name,stage string)error{
 if !identifier.MatchString(name)||!stageID.MatchString(stage){return errors.New("unsafe table naming")}
 return nil
}
func(c Client)RequireAtomic(ctx context.Context)error{
 v,err:=c.Query(ctx,"SELECT engine FROM system.databases WHERE name='scratch' FORMAT TabSeparatedRaw")
 if err!=nil{return err}
 if v!="Atomic"{return fmt.Errorf("scratch database must use Atomic engine, found %q",v)}
 return nil
}
func(c Client)UUID(ctx context.Context,name string)(string,error){
 if !identifier.MatchString(name)&&!stageID.MatchString(name){return "",errors.New("invalid table name")}
 v,err:=c.Query(ctx,"SELECT toString(uuid) FROM system.tables WHERE database='scratch' AND name='"+name+"' FORMAT TabSeparatedRaw")
 if err!=nil{return "",err}
 return v,nil
}
type Counts struct{Rows uint64;Unique uint64}
func(c Client)ValidateStage(ctx context.Context,stage,key string)(Counts,error){
 var stats Counts
 if !stageID.MatchString(stage){return stats,errors.New("invalid staged table name")}
 if key!=""&&!identifier.MatchString(key){return stats,errors.New("invalid unique key")}
 q:="SELECT count()"
 if key!=""{q+=", uniqExact("+key+")"}
 q+=" FROM scratch."+stage+" FORMAT TabSeparatedRaw"
 v,err:=c.Query(ctx,q);if err!=nil{return stats,err}
 chunks:=strings.Split(v,"\t")
 if len(chunks)<1{return stats,errors.New("invalid DQ result")}
 stats.Rows,err=strconv.ParseUint(chunks[0],10,64);if err!=nil{return stats,err}
 if stats.Rows==0{return stats,errors.New("empty output rejected by DQ gate")}
 if stats.Rows>TestRowLimit(){return stats,errors.New("TEST output exceeds approved row safety cap")}
 if key!=""{
  if len(chunks)!=2{return stats,errors.New("missing key cardinality")}
  stats.Unique,err=strconv.ParseUint(chunks[1],10,64);if err!=nil{return stats,err}
  if stats.Unique!=stats.Rows{return stats,errors.New("duplicate unique key rejected by DQ gate")}
 }
 return stats,nil
}
type Publication struct{
 Target string
 Stage string
 StageUUID string
 OldUUID string // empty means initial publication
}
func(p Publication)validate()error{
 if !stageID.MatchString(p.Stage)||!strings.HasPrefix(p.Target,"dft_")||!identifier.MatchString(p.Target)||p.Stage==p.Target{return errors.New("invalid versioned table relation")}
 if p.StageUUID==""{return errors.New("missing immutable stage UUID")}
 return nil
}
// Publish can be REPLAYED after an uncertain HTTP/PG commit. When UUID
// shows the target already holds the stage, it DOES NOT EXCHANGE again.
// A pre-persisted old UUID identifies the previous physical table.
func(c Client)Publish(ctx context.Context,p Publication)error{
 if err:=p.validate();err!=nil{return err}
 if err:=c.RequireAtomic(ctx);err!=nil{return err}
 current,err:=c.UUID(ctx,p.Target);if err!=nil{return err}
 stage,err:=c.UUID(ctx,p.Stage);if err!=nil{return err}
 if current==p.StageUUID{return nil} // previous attempt already published
 if p.OldUUID==""{
  if current!=""||stage!=p.StageUUID{return errors.New("initial publish UUID precondition failed")}
  _,err=c.Query(ctx,"RENAME TABLE scratch."+p.Stage+" TO scratch."+p.Target)
  return err
 }
 if current!=p.OldUUID||stage!=p.StageUUID{return errors.New("publish UUID mismatch; refuse unsafe exchange")}
 _,err=c.Query(ctx,"EXCHANGE TABLES scratch."+p.Target+" AND scratch."+p.Stage)
 return err
}
// Rollback swaps the untouched prior physical table back. It intentionally
// rejects the first publication (there is no prior table to restore).
func(c Client)Rollback(ctx context.Context,p Publication)error{
 if err:=p.validate();err!=nil{return err}
 if p.OldUUID==""{return errors.New("no previous table to roll back")}
 if err:=c.RequireAtomic(ctx);err!=nil{return err}
 current,err:=c.UUID(ctx,p.Target);if err!=nil{return err}
 backup,err:=c.UUID(ctx,p.Stage);if err!=nil{return err}
 if current==p.OldUUID&&backup==p.StageUUID{return nil} // replay after rollback
 if current!=p.StageUUID||backup!=p.OldUUID{return errors.New("rollback UUID mismatch; newer publication or backup changed")}
 _,err=c.Query(ctx,"EXCHANGE TABLES scratch."+p.Target+" AND scratch."+p.Stage)
 return err
}
func StageName(runID string)string{
 return "dft_stage_"+strings.ReplaceAll(runID,"-","")
}
func TargetName(model string)string{return "dft_"+model}
