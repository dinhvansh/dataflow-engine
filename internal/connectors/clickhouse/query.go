package clickhouse

import(
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "net/url"
 "regexp"
 "strings"
 "time"
)

// QueryJob is a bounded, read-only SELECT executed only against an isolated
// ClickHouse account whose grants and network access must ALSO be read-only.
// A SELECT prefix is not a security boundary: functions can access external
// resources, and the server account must be restricted by the administrator.
type QueryJob struct {
 Endpoint string
 Database string
 Username,Password string
 Client *http.Client
}
type QueryResult struct {
 Rows uint64 `json:"rows"`
 Data []map[string]any `json:"data"`
}
var readQuery=regexp.MustCompile(`(?is)^\s*SELECT\s+.+`)
func ValidateReadQuery(query string)error{
 query=strings.TrimSpace(query)
 if len(query)==0||len(query)>16384{return errors.New("query length must be 1..16384")}
 if strings.Contains(query,";"){return errors.New("multiple statements/semicolons forbidden")}
 if !readQuery.MatchString(query){return errors.New("only a single SELECT statement is supported")}
 if strings.Contains(query,"--")||strings.Contains(query,"/*")||strings.Contains(query,"*/"){return errors.New("comments are not accepted")}
 return nil
}
func(q QueryJob)Execute(ctx context.Context,query string)(QueryResult,error){
 var result QueryResult
 if err:=ValidateReadQuery(query);err!=nil{return result,err}
 if !identifier.MatchString(q.Database){return result,errors.New("invalid database")}
 endpoint,err:=url.Parse(strings.TrimRight(q.Endpoint,"/"));if err!=nil{return result,err}
 if endpoint.Scheme!="http"&&endpoint.Scheme!="https"{return result,errors.New("unsupported ClickHouse scheme")}
 if endpoint.Host==""{return result,errors.New("ClickHouse endpoint missing hostname")}
 params:=endpoint.Query()
 params.Set("database",q.Database)
 params.Set("query",strings.TrimSpace(query)+" FORMAT JSON")
 params.Set("readonly","1")
 params.Set("max_execution_time","30")
 params.Set("max_result_rows","200")
 params.Set("max_result_bytes","524288")
 params.Set("result_overflow_mode","throw")
 endpoint.RawQuery=params.Encode()
 request,err:=http.NewRequestWithContext(ctx,http.MethodPost,endpoint.String(),nil)
 if err!=nil{return result,err}
 if q.Username!=""{request.SetBasicAuth(q.Username,q.Password)}
 client:=q.Client;if client==nil{client=&http.Client{Timeout:35*time.Second}}
 response,err:=client.Do(request);if err!=nil{return result,err}
 defer response.Body.Close()
 body,err:=io.ReadAll(io.LimitReader(response.Body,1024*1024));if err!=nil{return result,err}
 if response.StatusCode<200||response.StatusCode>=300{return result,fmt.Errorf("ClickHouse read query failed (HTTP %d): %s",response.StatusCode,string(body))}
 if err=json.Unmarshal(body,&result);err!=nil{return result,fmt.Errorf("invalid ClickHouse JSON: %w",err)}
 if result.Rows>200||len(result.Data)>200{return QueryResult{},errors.New("ClickHouse query exceeded row cap")}
 return result,nil
}
