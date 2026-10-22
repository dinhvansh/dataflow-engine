package main

// The QA Sync Setup API binds a freshly configured stream to two previously
// verified runtime profiles, in one PostgreSQL transaction. It is NOT a
// general-purpose connector or production credential enrollment endpoint.
import (
 "context"
 "database/sql"
 "encoding/json"
 "errors"
 "net"
 "net/http"
 "net/url"
 "strconv"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/connectors/sqlserver"
 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
)

type labStreamInput struct {
 SourceConnectionID string `json:"source_connection_id"`
 TargetConnectionID string `json:"target_connection_id"`
 SourceSchema string `json:"source_schema"`
 SourceTable string `json:"source_table"`
 SelectedColumns []string `json:"selected_columns"`
 SourceKeyColumn string `json:"source_key_column,omitempty"`
 SourceCursorColumn string `json:"source_cursor_column,omitempty"`
 DestinationLayout string `json:"destination_layout,omitempty"`
 ColumnMappings []typedMappingRequest `json:"column_mappings,omitempty"`
 Mode string `json:"mode"`
 TargetDatabase string `json:"target_database"`
 TargetTable string `json:"target_table"`
 BatchSize int `json:"batch_size"`
 Workers int `json:"workers"`
 QueueCapacity int `json:"queue_capacity"`
}

func validLabStreamInput(in labStreamInput)bool{
 return in.Mode=="incremental_rowversion" &&
 in.TargetDatabase=="scratch" && in.TargetTable=="dataflow_raw_events" &&
 in.BatchSize>=100&&in.BatchSize<=100000 &&
 in.Workers>=1&&in.Workers<=32 &&
 in.QueueCapacity>=1&&in.QueueCapacity<=100 &&
 in.SourceConnectionID!=in.TargetConnectionID &&
 uuid.Validate(in.SourceConnectionID)==nil && uuid.Validate(in.TargetConnectionID)==nil &&
 len(in.SourceSchema)>0&&len(in.SourceTable)>0
}

func verifiedLabProfile(p runtimeConnection,driver string)bool{
 return p.Driver==driver&&p.Status=="verified"&&p.CheckedAt!=nil&&
 time.Since(*p.CheckedAt)<=30*time.Minute&&time.Since(*p.CheckedAt)>=-time.Minute&&
 approvedRuntimeEndpoint(p.Driver,p.Host,p.Port)
}
func (s *server) labSourceConnection(ctx context.Context,id string)(*sql.DB,error){
 p,secret,err:=s.readRuntimeProfile(ctx,id)
 if err!=nil||!verifiedLabProfile(p,"sqlserver"){return nil,errUnsafeLabConnection}
 db,err:=openLabSQLServer(ctx,p,secret)
 if err!=nil{return nil,errUnsafeLabConnection}
 if !checkLabSQLSource(ctx,db){db.Close();return nil,errUnsafeLabConnection}
 return db,nil
}
// This discovery is scoped to the saved and recently tested QA SQL Server
// profile, not the process-wide DATAFLOW_SQLSERVER_DSN.
func(s *server)runtimeDiscoverAPI(w http.ResponseWriter,r *http.Request){
 w.Header().Set("Cache-Control","no-store")
 if r.Method!=http.MethodGet{http.Error(w,"GET required",405);return}
 if !runtimeLabEnabled()||!runtimeConnectionAuthorized(s,r){http.Error(w,"not permitted",403);return}
 id:=r.URL.Query().Get("source_connection_id")
 if uuid.Validate(id)!=nil{http.Error(w,"invalid source connection",400);return}
 ctx,cancel:=context.WithTimeout(r.Context(),15*time.Second);defer cancel()
 db,err:=s.labSourceConnection(ctx,id)
 if err!=nil{http.Error(w,"source profile not currently verified",409);return}
 defer db.Close()
 schema:=r.URL.Query().Get("schema");if schema==""{schema="Erp"}
 table:=r.URL.Query().Get("table")
 w.Header().Set("Content-Type","application/json")
 if table==""{
  result,e:=sqlserver.DiscoverTables(ctx,db,schema)
  if e!=nil{http.Error(w,"source metadata discovery rejected",422);return}
  _=json.NewEncoder(w).Encode(map[string]any{"tables":result});return
 }
 result,e:=sqlserver.DiscoverColumns(ctx,db,schema,table)
 if e!=nil||len(result)==0{http.Error(w,"source column discovery rejected",422);return}
 _=json.NewEncoder(w).Encode(map[string]any{"columns":result})
}

// Only a NEW scratch stream is allowed: never attach another credential pair
// to a checkpointed stream or mutate its identity/column projection.
func(s *server)runtimeStreamSetupAPI(w http.ResponseWriter,r *http.Request){
 w.Header().Set("Cache-Control","no-store")
 if r.Method!=http.MethodPost{http.Error(w,"POST required",405);return}
 if !runtimeLabEnabled()||!runtimeConnectionAuthorized(s,r){http.Error(w,"not permitted",403);return}
 if s.pg==nil{http.Error(w,"metadata unavailable",503);return}
 var in labStreamInput
 if decodeRuntimeJSON(w,r,&in)!=nil||!validLabStreamInput(in){http.Error(w,"invalid or unsupported QA stream configuration",422);return}
 ctx,cancel:=context.WithTimeout(r.Context(),30*time.Second);defer cancel()

 tx,err:=s.pg.Begin(ctx);if err!=nil{http.Error(w,"metadata unavailable",503);return}
 defer tx.Rollback(ctx)
 var source,target runtimeConnection
 for _,item:=range []struct{id string;profile *runtimeConnection;driver string}{
  {in.SourceConnectionID,&source,"sqlserver"},{in.TargetConnectionID,&target,"clickhouse"},
 }{
  err=tx.QueryRow(ctx,`SELECT id::text,driver,kind,host,port,database_name,status,revision,checked_at
   FROM connection_runtime_profiles WHERE workspace='lab' AND id=$1::uuid FOR SHARE`,item.id).
   Scan(&item.profile.ID,&item.profile.Driver,&item.profile.Kind,&item.profile.Host,&item.profile.Port,
        &item.profile.Database,&item.profile.Status,&item.profile.Revision,&item.profile.CheckedAt)
  if err!=nil||!verifiedLabProfile(*item.profile,item.driver){
   http.Error(w,"connection not recently verified",409);return
  }
 }
 if source.Kind!="source"||source.Database!="DataFlowFixtureQA"||
 target.Kind!="destination"||target.Database!="scratch"{
  http.Error(w,"only isolated QA source and scratch destination permitted",403);return
 }
 // Worker and dbt publisher must target the same isolated CH instance.
 configured,e:=url.Parse(s.chURL)
 targetHost:=net.JoinHostPort(target.Host,strconv.Itoa(target.Port))
 if e!=nil||configured.Host!=targetHost{
  http.Error(w,"target does not match isolated worker ClickHouse",409);return
 }
 db,e:=s.labSourceConnection(ctx,source.ID)
 if e!=nil{http.Error(w,"read-only source unavailable",409);return}
 defer db.Close()
 // Check that the exact selected table (not just the fixture's Part table)
 // has SELECT permission and no DML/DDL grants.
 fullName:=in.SourceSchema+"."+in.SourceTable
 var read,insert,update,del,alter sql.NullInt64
 e=db.QueryRowContext(ctx,`SELECT
  HAS_PERMS_BY_NAME(@p1,'OBJECT','SELECT'),
  HAS_PERMS_BY_NAME(@p2,'OBJECT','INSERT'),
  HAS_PERMS_BY_NAME(@p3,'OBJECT','UPDATE'),
  HAS_PERMS_BY_NAME(@p4,'OBJECT','DELETE'),
  HAS_PERMS_BY_NAME(@p5,'OBJECT','ALTER')`,
  fullName,fullName,fullName,fullName,fullName).Scan(&read,&insert,&update,&del,&alter)
 if e!=nil||!read.Valid||read.Int64!=1||
  !insert.Valid||insert.Int64!=0||!update.Valid||update.Int64!=0||
  !del.Valid||del.Int64!=0||!alter.Valid||alter.Int64!=0{
  http.Error(w,"source table must be SELECT-only",403);return
 }
 cols,e:=sqlserver.DiscoverColumns(ctx,db,in.SourceSchema,in.SourceTable)
 if e!=nil||len(cols)==0{http.Error(w,"invalid source metadata",422);return}
 if _,e=sqlserver.BuildRowVersionProjection(cols,in.SelectedColumns);e!=nil{
  http.Error(w,"invalid column projection / cursor",422);return
 }
 unique,e:=sqlserver.HasUniqueSysRowID(ctx,db,in.SourceSchema,in.SourceTable)
 if e!=nil||!unique{http.Error(w,"source requires unique SysRowID index",422);return}
 var isolation string
 if e=db.QueryRowContext(ctx,`SELECT snapshot_isolation_state_desc FROM sys.databases WHERE name=DB_NAME()`).Scan(&isolation);e!=nil||isolation!="ON"{
  http.Error(w,"SNAPSHOT isolation not enabled on QA source",422);return
 }
 // Fresh stream/config/checkpoint/binding are all-or-nothing. Enforced
 // unique schema+table+target disallows stealing an existing stream.
 var streamID string
 err=tx.QueryRow(ctx,`INSERT INTO sync_streams(source_schema,source_table,target_database,target_table,
 batch_size,workers,queue_capacity,selected_columns)
 VALUES($1,$2,'scratch','dataflow_raw_events',$3,$4,$5,$6)
 RETURNING id::text`,in.SourceSchema,in.SourceTable,in.BatchSize,in.Workers,in.QueueCapacity,in.SelectedColumns).Scan(&streamID)
 if errors.Is(err,pgx.ErrNoRows){http.Error(w,"stream exists",409);return}
 if err!=nil{http.Error(w,"stream already configured or metadata unavailable",409);return}
 if _,err=tx.Exec(ctx,`INSERT INTO sync_checkpoints(stream_id) VALUES($1::uuid)`,streamID);err!=nil{
  http.Error(w,"checkpoint unavailable",503);return
 }
 if _,err=tx.Exec(ctx,`INSERT INTO connection_runtime_bindings
 (stream_id,source_profile_id,target_profile_id,source_revision,target_revision)
 VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5)`,
 streamID,source.ID,target.ID,source.Revision,target.Revision);err!=nil{
  http.Error(w,"binding unavailable",503);return
 }
 if err=tx.Commit(ctx);err!=nil{http.Error(w,"stream transaction unavailable",503);return}
 w.Header().Set("Content-Type","application/json");w.WriteHeader(http.StatusCreated)
 _=json.NewEncoder(w).Encode(map[string]any{"stream_id":streamID,"bound":true,
 "mode":in.Mode,"source_connection_id":source.ID,"target_connection_id":target.ID})
}
