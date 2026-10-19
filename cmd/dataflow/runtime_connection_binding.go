package main

import (
 "context"
 "database/sql"
 "errors"
 "net"
 "net/url"
 "strconv"
 "time"

 "github.com/jackc/pgx/v5"
)

// Resolve immutable stream-scoped binding at the worker, not by mutating shared
// server connection globals. Unbound legacy streams keep their existing path.
// The ClickHouse host MUST be identical to the isolated DBT worker's endpoint,
// because publication is performed through that independently provisioned worker.
func(s *server)runtimeForManagedStream(ctx context.Context,streamID string)(*sql.DB,string,string,string,func(),error){
 keep:=func(){}
 if !runtimeLabEnabled(){return s.sql,s.chURL,s.chUser,s.chPass,keep,nil}
 var sourceID,targetID string;var sourceRev,targetRev int64
 e:=s.pg.QueryRow(ctx,`SELECT source_profile_id::text,target_profile_id::text,
 source_revision,target_revision FROM connection_runtime_bindings WHERE stream_id=$1::uuid`,streamID).
 Scan(&sourceID,&targetID,&sourceRev,&targetRev)
 if errors.Is(e,pgx.ErrNoRows){return s.sql,s.chURL,s.chUser,s.chPass,keep,nil}
 if e!=nil||!runtimeLabEnabled(){return nil,"","","",keep,errUnsafeLabConnection}
 source,sourcePass,e:=s.readRuntimeProfile(ctx,sourceID)
 if e!=nil{return nil,"","","",keep,errUnsafeLabConnection}
 target,targetPass,e:=s.readRuntimeProfile(ctx,targetID)
 if e!=nil{return nil,"","","",keep,errUnsafeLabConnection}
 if source.Revision!=sourceRev||target.Revision!=targetRev||
 source.Status!="verified"||target.Status!="verified"||
 source.CheckedAt==nil||target.CheckedAt==nil||
 time.Since(*source.CheckedAt)>30*time.Minute||
 time.Since(*target.CheckedAt)>30*time.Minute{
  return nil,"","","",keep,errUnsafeLabConnection
 }
 backend,e:=url.Parse(s.chURL)
 if e!=nil||backend.Hostname()!=target.Host{return nil,"","","",keep,errUnsafeLabConnection}
 port,e:=strconv.Atoi(backend.Port())
 if e!=nil||port!=target.Port||net.JoinHostPort(target.Host,strconv.Itoa(target.Port))!=backend.Host{
  return nil,"","","",keep,errUnsafeLabConnection
 }
 db,e:=openLabSQLServer(ctx,source,sourcePass)
 if e!=nil{return nil,"","","",keep,errUnsafeLabConnection}
 if !checkLabSQLSource(ctx,db){db.Close();return nil,"","","",keep,errUnsafeLabConnection}
 return db,s.chURL,target.Username,targetPass,func(){_ = db.Close()},nil
}
