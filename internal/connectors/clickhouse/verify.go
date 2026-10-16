package clickhouse

import (
 "context"
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
var canonicalRunID=regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
// CountRunRows reads the number of raw events persisted for a single run.
// This validates row counts only, not source-content correctness or history.
// Requires SELECT on the raw event table.
func(w *Writer)CountRunRows(ctx context.Context,runID string)(uint64,error){
 if !identifier.MatchString(w.Database)||!identifier.MatchString(w.Table) {
  return 0,errors.New("unsafe destination identifier")
 }
 if !canonicalRunID.MatchString(runID){return 0,errors.New("invalid run ID")}
 u,err:=url.Parse(strings.TrimRight(w.Endpoint,"/"));if err!=nil{return 0,err}
 q:=u.Query();q.Set("query",fmt.Sprintf("SELECT count() FROM `%s`.`%s` WHERE _run_id = '%s' FORMAT TabSeparated",w.Database,w.Table,runID));u.RawQuery=q.Encode()
 req,err:=http.NewRequestWithContext(ctx,http.MethodPost,u.String(),nil);if err!=nil{return 0,err}
 if w.Username!=""{req.SetBasicAuth(w.Username,w.Password)}
 client:=w.Client;if client==nil{client=&http.Client{Timeout:5*time.Minute}}
 resp,err:=client.Do(req);if err!=nil{return 0,err};defer resp.Body.Close()
 raw,err:=io.ReadAll(io.LimitReader(resp.Body,4096));if err!=nil{return 0,err}
 if resp.StatusCode<200||resp.StatusCode>=300{return 0,fmt.Errorf("ClickHouse verification HTTP %d",resp.StatusCode)}
 result,err:=strconv.ParseUint(strings.TrimSpace(string(raw)),10,64)
 if err!=nil{return 0,fmt.Errorf("invalid ClickHouse count response: %w",err)}
 return result,nil
}
