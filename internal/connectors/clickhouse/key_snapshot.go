package clickhouse

import(
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "net/http"
 "net/url"
 "io"
 "strconv"
 "strings"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/cursor"
 "github.com/dinhvansh/dataflow-engine/internal/engine"
)
type KeySnapshotWriter struct{
 Destination *Writer
 StreamID,SnapshotID string
}
func(w *KeySnapshotWriter)Write(ctx context.Context,b engine.Batch)error{
 if w.Destination==nil||!canonicalRunID.MatchString(w.StreamID)||!canonicalRunID.MatchString(w.SnapshotID){
  return errors.New("invalid snapshot destination or IDs")
 }
 next:=engine.Batch{Sequence:b.Sequence,Rows:make([]map[string]any,0,len(b.Rows))}
 for _,source:=range b.Rows{
  key,ok:=source["SysRowID"].(string)
  if !ok||!canonicalRunID.MatchString(key){return errors.New("source SysRowID is not canonical UUID")}
  rv,ok:=source["SysRevID"].(string)
  if !ok{return errors.New("source SysRevID is not a hex string")}
  version,err:=cursor.ParseRowVersion(rv);if err!=nil{return err}
  next.Rows=append(next.Rows,map[string]any{
   "_stream_id":w.StreamID,"_snapshot_id":w.SnapshotID,
   "_pk":strings.ToLower(key),"_sysrevid":version.String(),
  })
 }
 return w.Destination.Write(ctx,next)
}
type KeySnapshotStats struct{
 Rows uint64 `json:"rows"`
 DistinctKeys uint64 `json:"distinct_keys"`
}
type KeySnapshotDiff struct{
 Added uint64 `json:"added"`
 Deleted uint64 `json:"deleted"`
 Changed uint64 `json:"changed"`
}
func (w *Writer) keySnapshotContext(streamID,snapshotID string)(string,string,error){
 if !identifier.MatchString(w.Database)||!identifier.MatchString(w.Table){return "","",errors.New("invalid target identifier")}
 if !canonicalRunID.MatchString(streamID)||!canonicalRunID.MatchString(snapshotID){return "","",errors.New("invalid IDs")}
 table:="`"+w.Database+"`.`"+w.Table+"`"
 filter:=fmt.Sprintf("_stream_id='%s' AND _snapshot_id='%s'",streamID,snapshotID)
 return table,filter,nil
}
func(w *Writer)scalar(ctx context.Context,query string)(uint64,error){
 endpoint,err:=url.Parse(strings.TrimRight(w.Endpoint,"/"));if err!=nil{return 0,err}
 q:=endpoint.Query();q.Set("query",query+" FORMAT TabSeparated");endpoint.RawQuery=q.Encode()
 req,err:=http.NewRequestWithContext(ctx,http.MethodPost,endpoint.String(),nil);if err!=nil{return 0,err}
 if w.Username!=""{req.SetBasicAuth(w.Username,w.Password)}
 client:=w.Client;if client==nil{client=&http.Client{Timeout:5*time.Minute}}
 res,err:=client.Do(req);if err!=nil{return 0,err};defer res.Body.Close()
 data,err:=io.ReadAll(io.LimitReader(res.Body,4096));if err!=nil{return 0,err}
 if res.StatusCode<200||res.StatusCode>=300{return 0,fmt.Errorf("ClickHouse query status %d: %s",res.StatusCode,string(data))}
 n,err:=strconv.ParseUint(strings.TrimSpace(string(data)),10,64)
 if err!=nil{return 0,fmt.Errorf("unexpected scalar: %w",err)}
 return n,nil
}
func(w *Writer)SnapshotStats(ctx context.Context,streamID,snapshotID string)(KeySnapshotStats,error){
 var result KeySnapshotStats
 table,filter,err:=w.keySnapshotContext(streamID,snapshotID);if err!=nil{return result,err}
 result.Rows,err=w.scalar(ctx,"SELECT count() FROM "+table+" WHERE "+filter)
 if err!=nil{return result,err}
 result.DistinctKeys,err=w.scalar(ctx,"SELECT uniqExact(_pk) FROM "+table+" WHERE "+filter)
 return result,err
}
// SnapshotDiff compares two *fully verified* source-key snapshots.
// Purely diagnostic. No tombstones or deletion mutations are written.
func(w *Writer)SnapshotDiff(ctx context.Context,streamID,oldID,newID string)(KeySnapshotDiff,error){
 var result KeySnapshotDiff
 table,oldFilter,err:=w.keySnapshotContext(streamID,oldID);if err!=nil{return result,err}
 _,newFilter,err:=w.keySnapshotContext(streamID,newID);if err!=nil{return result,err}
 oldQuery:="SELECT _pk,_sysrevid FROM "+table+" WHERE "+oldFilter
 newQuery:="SELECT _pk,_sysrevid FROM "+table+" WHERE "+newFilter
 result.Added,err=w.scalar(ctx,"SELECT count() FROM ("+newQuery+") n LEFT ANTI JOIN ("+oldQuery+") p USING (_pk)")
 if err!=nil{return result,err}
 result.Deleted,err=w.scalar(ctx,"SELECT count() FROM ("+oldQuery+") p LEFT ANTI JOIN ("+newQuery+") n USING (_pk)")
 if err!=nil{return result,err}
 result.Changed,err=w.scalar(ctx,"SELECT count() FROM ("+oldQuery+") p INNER JOIN ("+newQuery+") n USING (_pk) WHERE p._sysrevid != n._sysrevid")
 return result,err
}
// Satisfy Go compile-time import usage: report structure intentionally does
// not serialize rows, only scalar statistics and aggregate differences.
var _ = json.Valid
