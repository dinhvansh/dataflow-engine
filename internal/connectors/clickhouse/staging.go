package clickhouse

import(
 "context"
 "errors"
 "fmt"
 "io"
 "net/http"
 "net/url"
 "strings"
 "time"
)

type Staging struct{
 Writer *Writer
 StageTable string
 Published bool
}
// exec executes target-only DDL; never forward SQL from user-controlled text.
func(w *Writer) exec(ctx context.Context,query string)error{
 u,err:=url.Parse(strings.TrimRight(w.Endpoint,"/"));if err!=nil{return err}
 q:=u.Query();q.Set("query",query);u.RawQuery=q.Encode()
 req,err:=http.NewRequestWithContext(ctx,http.MethodPost,u.String(),nil);if err!=nil{return err}
 if w.Username!=""{req.SetBasicAuth(w.Username,w.Password)}
 client:=w.Client;if client==nil{client=&http.Client{Timeout:5*time.Minute}}
 resp,err:=client.Do(req);if err!=nil{return err};defer resp.Body.Close()
 if resp.StatusCode<200||resp.StatusCode>=300{
  b,_:=io.ReadAll(io.LimitReader(resp.Body,2048))
  return fmt.Errorf("ClickHouse DDL %d: %s",resp.StatusCode,string(b))
 }
 return nil
}
func stageName(target,runID string)(string,error){
 if !identifier.MatchString(target){return "",errors.New("invalid target table")}
 token:=strings.ReplaceAll(runID,"-","")
 if len(token)!=32{return "",errors.New("invalid run ID for stage")}
 for _,c:=range token{if !strings.ContainsRune("0123456789abcdefABCDEF",c){return "",errors.New("invalid run ID for stage")}}
 name:=target+"_df_"+strings.ToLower(token[:16])
 if !identifier.MatchString(name){return "",errors.New("stage table name invalid")}
 return name,nil
}
// PrepareStage clones the existing table definition in an Atomic ClickHouse
// database. Requires CREATE and later EXCHANGE / DROP target permissions.
// It is intended for disposable test targets until production DDL is audited.
func(w *Writer)PrepareStage(ctx context.Context,runID string)(*Staging,error){
 if !identifier.MatchString(w.Database)||!identifier.MatchString(w.Table){return nil,errors.New("invalid CH identifiers")}
 stage,err:=stageName(w.Table,runID);if err!=nil{return nil,err}
 db:="`"+w.Database+"`"
 if err=w.exec(ctx,"CREATE TABLE "+db+".`"+stage+"` AS "+db+".`"+w.Table+"`");err!=nil{return nil,err}
 copyWriter:=*w;copyWriter.Table=stage
 return &Staging{Writer:&copyWriter,StageTable:stage},nil
}
// Publish atomically exchanges destination and staged table names. The old
// target remains in StageTable until Cleanup removes it. If Publish returns
// an ambiguous network error, inspect ClickHouse metadata before retrying.
func(w *Writer)PublishStage(ctx context.Context,stage *Staging)error{
 if stage==nil||stage.Published{return errors.New("invalid/unpublished stage")}
 if !identifier.MatchString(w.Database)||!identifier.MatchString(w.Table)||!identifier.MatchString(stage.StageTable){return errors.New("invalid identifiers")}
 db:="`"+w.Database+"`"
 if err:=w.exec(ctx,"EXCHANGE TABLES "+db+".`"+w.Table+"` AND "+db+".`"+stage.StageTable+"`");err!=nil{return err}
 stage.Published=true
 return nil
}
func(w *Writer)DropStage(ctx context.Context,stage *Staging)error{
 if stage==nil||!identifier.MatchString(stage.StageTable)||!identifier.MatchString(w.Database){return errors.New("invalid stage")}
 return w.exec(ctx,"DROP TABLE IF EXISTS `"+w.Database+"`.`"+stage.StageTable+"`")
}
