package clickhouse

import(
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "net/http"
 "net/url"
 "regexp"
 "strings"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/engine"
)
var identifier=regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
// WriteHTTPError exposes only a response status, never a ClickHouse response
// body which might contain sensitive query/connection information.
// The managed queue uses this typed error to retry transient HTTP failures.
type WriteHTTPError struct{ StatusCode int }
func(e *WriteHTTPError)Error()string{return fmt.Sprintf("ClickHouse HTTP %d",e.StatusCode)}

type Writer struct {
 Endpoint string
 Database,Table string
 Username,Password string
 Client *http.Client
}
func (w *Writer) Write(ctx context.Context,b engine.Batch)error{
 if !identifier.MatchString(w.Database)||!identifier.MatchString(w.Table){return errors.New("invalid ClickHouse identifier")}
 if len(b.Rows)==0{return nil}
 var body bytes.Buffer
 for _,row:=range b.Rows{data,err:=json.Marshal(row);if err!=nil{return err};body.Write(data);body.WriteByte('\n')}
 endpoint:=strings.TrimRight(w.Endpoint,"/")
 u,err:=url.Parse(endpoint);if err!=nil{return err}
 q:=u.Query()
 q.Set("query",fmt.Sprintf("INSERT INTO `%s`.`%s` FORMAT JSONEachRow",w.Database,w.Table))
 u.RawQuery=q.Encode()
 req,err:=http.NewRequestWithContext(ctx,http.MethodPost,u.String(),&body);if err!=nil{return err}
 if w.Username!=""{req.SetBasicAuth(w.Username,w.Password)}
 req.Header.Set("Content-Type","application/x-ndjson")
 client:=w.Client;if client==nil{client=&http.Client{Timeout:5*time.Minute}}
 res,err:=client.Do(req);if err!=nil{return err};defer res.Body.Close()
 if res.StatusCode<200||res.StatusCode>=300{return &WriteHTTPError{StatusCode:res.StatusCode}}
 return nil
}
// This initial writer requires a PRECREATED ClickHouse table with compatible column
// names and types. Full-refresh reruns append duplicates; safe generation publication
// is not implemented yet. Do not use it against production tables.
