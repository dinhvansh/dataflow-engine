package clickhouse

import (
 "context"
 "crypto/sha256"
 "encoding/binary"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "net/url"
 "strings"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/cursor"
 "github.com/dinhvansh/dataflow-engine/internal/engine"
)

// TypedColumn is an immutable mapping resolved from the SQL Server catalog,
// never a free-form SQL/ClickHouse expression supplied by the browser.
type TypedColumn struct {
 Source string `json:"source"`
 Target string `json:"target"`
 CHType string `json:"ch_type"`
}
type TypedDestination struct{
 Writer *Writer
 StreamID string
 RunID string
 KeyColumn string
 CursorColumn string
 Columns []TypedColumn
}
func (d *TypedDestination) Validate()error{
 if d==nil||d.Writer==nil||!canonicalRunID.MatchString(d.StreamID)||!canonicalRunID.MatchString(d.RunID){
  return errors.New("invalid typed destination identity")
 }
 if !identifier.MatchString(d.Writer.Database)||!identifier.MatchString(d.Writer.Table){
  return errors.New("unsafe typed destination name")
 }
 if !identifier.MatchString(d.KeyColumn)||len(d.Columns)==0||len(d.Columns)>256{
  return errors.New("invalid typed mapping")
 }
 seenSources,seenTargets:=map[string]bool{},map[string]bool{}
 haveKey:=false
 for _,m:=range d.Columns{
  if !identifier.MatchString(m.Source)||!identifier.MatchString(m.Target)||
   strings.HasPrefix(strings.ToLower(m.Target),"_df_")||
   strings.EqualFold(m.Target,"_run_id")||strings.EqualFold(m.Target,"_batch_seq")||
   seenSources[strings.ToLower(m.Source)]||seenTargets[strings.ToLower(m.Target)]||
   !SafeCHType(m.CHType){return errors.New("unsafe or duplicate typed column mapping")}
  seenSources[strings.ToLower(m.Source)]=true
  seenTargets[strings.ToLower(m.Target)]=true
  if m.Source==d.KeyColumn{haveKey=true}
 }
 if !haveKey{return errors.New("primary key missing from typed projection")}
 return nil
}
func SafeCHType(t string)bool{
 if strings.HasPrefix(t,"Nullable(")&&strings.HasSuffix(t,")"){
  return SafeCHType(strings.TrimSuffix(strings.TrimPrefix(t,"Nullable("),")"))
 }
 switch t {
 case "String","Int8","Int16","Int32","Int64","UInt8","UInt16","UInt32","UInt64",
      "Float32","Float64","Date","Date32","DateTime64(3, 'UTC')":return true
 }
 var p,s int
 if n,_:=fmt.Sscanf(t,"Decimal(%d,%d)",&p,&s);n==2&&p>=1&&p<=38&&s>=0&&s<=p{
  return fmt.Sprintf("Decimal(%d,%d)",p,s)==t
 }
 return false
}
func (d *TypedDestination) ownerComment()string{
 data,_:=json.Marshal(d.Columns)
 h:=sha256.Sum256(append([]byte(d.KeyColumn+"|"+d.CursorColumn+"|"),data...))
 return "dataflow-owned:"+d.StreamID+":"+hex.EncodeToString(h[:12])
}
func (w *Writer) typedScalar(ctx context.Context,query string)(string,error){
 u,err:=url.Parse(strings.TrimRight(w.Endpoint,"/"));if err!=nil{return "",err}
 q:=u.Query();q.Set("query",query);u.RawQuery=q.Encode()
 req,err:=http.NewRequestWithContext(ctx,http.MethodPost,u.String(),nil);if err!=nil{return "",err}
 if w.Username!=""{req.SetBasicAuth(w.Username,w.Password)}
 client:=w.Client;if client==nil{client=&http.Client{Timeout:5*time.Minute}}
 resp,err:=client.Do(req);if err!=nil{return "",err}
 defer resp.Body.Close()
 raw,e:=io.ReadAll(io.LimitReader(resp.Body,8192));if e!=nil{return "",e}
 if resp.StatusCode<200||resp.StatusCode>=300{return "",&WriteHTTPError{StatusCode:resp.StatusCode}}
 return strings.TrimSpace(string(raw)),nil
}
func (d *TypedDestination) EnsureOwned(ctx context.Context)error{
 if err:=d.Validate();err!=nil{return err}
 w:=d.Writer
 // Ownership and exact schema fingerprint are checked BEFORE and AFTER
 // CREATE IF NOT EXISTS. Unknown existing tables are never adopted.
 comment:=d.ownerComment()
 lookup:=fmt.Sprintf("SELECT comment FROM system.tables WHERE database='%s' AND name='%s' FORMAT TabSeparated",w.Database,w.Table)
 existing,err:=w.typedScalar(ctx,lookup);if err!=nil{return err}
 if existing!=""&&existing!=comment{return errors.New("destination exists but is not owned by this immutable mapping")}
 defs:=make([]string,0,len(d.Columns)+5)
 for _,m:=range d.Columns{defs=append(defs,"`"+m.Target+"` "+m.CHType)}
 defs=append(defs,"`_df_key` String","`_df_version` UInt64","`_run_id` String",
  "`_batch_seq` UInt64","`_df_ingested_at` DateTime64(3, 'UTC') DEFAULT now64(3)")
 stmt:="CREATE TABLE IF NOT EXISTS `"+w.Database+"`.`"+w.Table+"` ("+
  strings.Join(defs,",")+") ENGINE=ReplacingMergeTree(_df_version) ORDER BY (_df_key) COMMENT '"+comment+"'"
 if err=w.exec(ctx,stmt);err!=nil{return err}
 actual,err:=w.typedScalar(ctx,lookup)
 if err!=nil||actual!=comment{return errors.New("ClickHouse destination ownership/schema fingerprint rejected")}
 return nil
}
// StampStage ensures a stage carries the exact ownership fingerprint after
// EXCHANGE TABLES. CREATE TABLE AS does not guarantee table comments are copied
// across ClickHouse versions. Only the stage generated for this run may be stamped.
func (d *TypedDestination) StampStage(ctx context.Context,stage *Staging)error{
 if err:=d.Validate();err!=nil{return err}
 expected,err:=stageName(d.Writer.Table,d.RunID)
 if err!=nil||stage==nil||stage.StageTable!=expected||stage.Writer==nil||
  stage.Writer.Database!=d.Writer.Database||stage.Writer.Table!=expected{
  return errors.New("cannot stamp an unrelated ClickHouse stage")
 }
 stmt:="ALTER TABLE `"+d.Writer.Database+"`.`"+expected+"` MODIFY COMMENT '"+d.ownerComment()+"'"
 if err=d.Writer.exec(ctx,stmt);err!=nil{return err}
 got,err:=d.Writer.typedScalar(ctx,fmt.Sprintf("SELECT comment FROM system.tables WHERE database='%s' AND name='%s' FORMAT TabSeparated",d.Writer.Database,expected))
 if err!=nil||got!=d.ownerComment(){return errors.New("stage ownership fingerprint could not be verified")}
 return nil
}
func (d *TypedDestination) Write(ctx context.Context,b engine.Batch)error{
 if err:=d.Validate();err!=nil{return err}
 out:=engine.Batch{Sequence:b.Sequence,Rows:make([]map[string]any,0,len(b.Rows))}
 for _,source:=range b.Rows{
  pk,ok:=source["SysRowID"].(string);if !ok||pk==""{return errors.New("mapped key missing")}
  version,ok:=source["SysRevID"].(string);if !ok{return errors.New("mapped rowversion missing")}
  parsed,err:=cursor.ParseRowVersion(version);if err!=nil{return err}
  row:=map[string]any{"_df_key":pk,"_df_version":binary.BigEndian.Uint64(parsed.Bytes()),
   "_run_id":d.RunID,"_batch_seq":b.Sequence}
  for _,m:=range d.Columns{
   value,ok:=source[m.Source];if !ok{return fmt.Errorf("source column %q missing",m.Source)}
   row[m.Target]=value
  }
  out.Rows=append(out.Rows,row)
 }
 return d.Writer.Write(ctx,out)
}
