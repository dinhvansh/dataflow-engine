package main

// Transactional multi-stream QA setup: verified profile pair, SELECT-only SQL
// catalog verification and generated isolated ClickHouse raw table names.
// Never adopt or overwrite user-named ClickHouse tables or reset checkpoints.
import (
 "context"
 "database/sql"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net"
 "net/http"
 "net/url"
 "regexp"
 "strconv"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/connectors/clickhouse"
 "github.com/dinhvansh/dataflow-engine/internal/connectors/sqlserver"
 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
)

type qaStreamBatchInput struct{
 SourceConnectionID string `json:"source_connection_id"`
 TargetConnectionID string `json:"target_connection_id"`
 Streams []labStreamInput `json:"streams"`
}
var qaAlias = regexp.MustCompile(`^([a-z][a-z0-9_]{0,29})$`)

func safeQADestination(alias,id string)(string,error){
 if !qaAlias.MatchString(alias){return "",errors.New("QA destination alias must be 1..30 lowercase letters, numbers or underscores")}
 if uuid.Validate(id)!=nil{return "",errors.New("invalid stream identity")}
 plain:=""
 for _,r:=range id{if r!='-'{plain+=string(r)}}
 name:="dfqa_"+alias+"_"+plain[:12]
 if !clickhouse.ValidQARawTarget("scratch",name){return "",errors.New("unsafe QA table name")}
 return name,nil
}
func validBatchStream(in labStreamInput)bool{
 return (in.Mode=="incremental_rowversion"||in.Mode=="full_refresh")&&
  in.TargetDatabase=="scratch"&&qaAlias.MatchString(in.TargetTable)&&
  len(in.SourceSchema)>0&&len(in.SourceTable)>0&&
  in.BatchSize>=100&&in.BatchSize<=100000&&in.Workers>=1&&in.Workers<=32&&
  in.QueueCapacity>=1&&in.QueueCapacity<=100&&len(in.SelectedColumns)>=2&&len(in.SelectedColumns)<=256
}
func verifyQASourceTable(ctx context.Context,db *sql.DB,item labStreamInput)error{
 // Identifier syntax must be checked before passing through SQL Server
 // catalog validation; no arbitrary or qualified SQL is accepted.
 cols,e:=sqlserver.DiscoverColumns(ctx,db,item.SourceSchema,item.SourceTable)
 if e!=nil||len(cols)==0{return errors.New("table metadata unavailable")}
 if _,e=sqlserver.BuildRowVersionProjection(cols,item.SelectedColumns);e!=nil{return errors.New("invalid key/cursor or selected columns")}
 ok,e:=sqlserver.HasUniqueSysRowID(ctx,db,item.SourceSchema,item.SourceTable)
 if e!=nil||!ok{return errors.New("SysRowID requires a single-column unique index")}
 var read,insert,update,del,alter sql.NullInt64
 object:=item.SourceSchema+"."+item.SourceTable
 e=db.QueryRowContext(ctx,`SELECT
 HAS_PERMS_BY_NAME(@p1,'OBJECT','SELECT'),
 HAS_PERMS_BY_NAME(@p2,'OBJECT','INSERT'),
 HAS_PERMS_BY_NAME(@p3,'OBJECT','UPDATE'),
 HAS_PERMS_BY_NAME(@p4,'OBJECT','DELETE'),
 HAS_PERMS_BY_NAME(@p5,'OBJECT','ALTER')`,
 object,object,object,object,object).Scan(&read,&insert,&update,&del,&alter)
 if e!=nil||!read.Valid||read.Int64!=1||!insert.Valid||insert.Int64!=0||
  !update.Valid||update.Int64!=0||!del.Valid||del.Int64!=0||
  !alter.Valid||alter.Int64!=0{return errors.New("source table must have SELECT-only grants")}
 return nil
}
func(s *server)qaBatchSetupAPI(w http.ResponseWriter,r *http.Request){
 w.Header().Set("Cache-Control","no-store")
 if r.Method!=http.MethodPost{http.Error(w,"POST required",405);return}
 if !runtimeLabEnabled()||!runtimeConnectionAuthorized(s,r){http.Error(w,"not permitted",403);return}
 if s.pg==nil{http.Error(w,"metadata unavailable",503);return}
 r.Body=http.MaxBytesReader(w,r.Body,128*1024)
 d:=json.NewDecoder(r.Body);d.DisallowUnknownFields()
 var input qaStreamBatchInput
 if d.Decode(&input)!=nil{http.Error(w,"invalid configuration",400);return}
 var extra any
 if d.Decode(&extra)!=io.EOF{http.Error(w,"extra JSON not accepted",400);return}
 if len(input.Streams)==0||len(input.Streams)>10||
  uuid.Validate(input.SourceConnectionID)!=nil||
  uuid.Validate(input.TargetConnectionID)!=nil||
  input.SourceConnectionID==input.TargetConnectionID{
  http.Error(w,"1..10 QA streams and two distinct verified connection IDs required",422);return
 }
 ctx,cancel:=context.WithTimeout(r.Context(),45*time.Second);defer cancel()
 tx,e:=s.pg.Begin(ctx)
 if e!=nil{http.Error(w,"metadata unavailable",503);return}
 defer tx.Rollback(ctx)
 var source,target runtimeConnection
 for _,x:=range []struct{id string;p *runtimeConnection;driver string}{
  {input.SourceConnectionID,&source,"sqlserver"},
  {input.TargetConnectionID,&target,"clickhouse"},
 }{
  e=tx.QueryRow(ctx,`SELECT id::text,driver,kind,host,port,database_name,status,revision,checked_at
   FROM connection_runtime_profiles WHERE workspace='lab' AND id=$1::uuid FOR SHARE`,x.id).
  Scan(&x.p.ID,&x.p.Driver,&x.p.Kind,&x.p.Host,&x.p.Port,&x.p.Database,&x.p.Status,&x.p.Revision,&x.p.CheckedAt)
  if e!=nil||!verifiedLabProfile(*x.p,x.driver){
   http.Error(w,"connection not recently verified",409);return
  }
 }
 if source.Kind!="source"||source.Database!="DataFlowFixtureQA"||
    target.Kind!="destination"||target.Database!="scratch"{
  http.Error(w,"QA profiles only",403);return
 }
 configured,e:=url.Parse(s.chURL)
 if e!=nil||configured.Host!=net.JoinHostPort(target.Host,strconv.Itoa(target.Port)){
  http.Error(w,"target differs from verified QA worker",409);return
 }
 sourceDB,e:=s.labSourceConnection(ctx,source.ID)
 if e!=nil{http.Error(w,"source unavailable",409);return}
 defer sourceDB.Close()
 var isolation string
 if e=sourceDB.QueryRowContext(ctx,
  "SELECT snapshot_isolation_state_desc FROM sys.databases WHERE name=DB_NAME()").Scan(&isolation);e!=nil||isolation!="ON"{
  http.Error(w,"SNAPSHOT isolation required",422);return
 }
 seen:=make(map[string]bool)
 type result struct{
  StreamID string `json:"stream_id"`
  Source string `json:"source"`
  Mode string `json:"mode"`
  Destination string `json:"destination"`
 }
 created:=make([]result,0,len(input.Streams))
 for _,item:=range input.Streams{
  if !validBatchStream(item){http.Error(w,"invalid sync mode, target alias, columns or performance bounds",422);return}
  key:=item.SourceSchema+"."+item.SourceTable
  if seen[key]{http.Error(w,"duplicate table in batch",422);return}
  seen[key]=true
  if e=verifyQASourceTable(ctx,sourceDB,item);e!=nil{
   http.Error(w,"source table rejected: "+e.Error(),422);return
  }
  streamID:=uuid.NewString()
  dest,e:=safeQADestination(item.TargetTable,streamID)
  if e!=nil{http.Error(w,"invalid scratch target",422);return}
  // Entire batch is one PostgreSQL transaction. A single bad item rolls it
  // all back. Physical destination tables are created by the worker ON RUN.
  _,e=tx.Exec(ctx,`INSERT INTO sync_streams(id,source_schema,source_table,target_database,
  target_table,batch_size,workers,queue_capacity,selected_columns,sync_mode)
  VALUES($1::uuid,$2,$3,'scratch',$4,$5,$6,$7,$8,$9)`,
  streamID,item.SourceSchema,item.SourceTable,dest,item.BatchSize,item.Workers,
  item.QueueCapacity,item.SelectedColumns,item.Mode)
  if e!=nil{http.Error(w,"stream already exists or config rejected",409);return}
  if _,e=tx.Exec(ctx,`INSERT INTO sync_checkpoints(stream_id) VALUES($1::uuid)`,streamID);e!=nil{
   http.Error(w,"checkpoint unavailable",503);return
  }
  if _,e=tx.Exec(ctx,`INSERT INTO connection_runtime_bindings
  (stream_id,source_profile_id,target_profile_id,source_revision,target_revision)
  VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5)`,
   streamID,source.ID,target.ID,source.Revision,target.Revision);e!=nil{
   http.Error(w,"connection binding unavailable",503);return
  }
  created=append(created,result{StreamID:streamID,Source:key,Mode:item.Mode,Destination:"scratch."+dest})
 }
 if e=tx.Commit(ctx);e!=nil{http.Error(w,"batch commit failed",503);return}
 jsonResponse(w,201,map[string]any{"streams":created,"count":len(created)})
}

func qaBatchPrintSafe(in qaStreamBatchInput)string{
 return fmt.Sprintf("streams=%d",len(in.Streams)) // never log secrets
}
