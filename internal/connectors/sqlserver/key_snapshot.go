package sqlserver

import (
 "context"
 "database/sql"
 "errors"
 "fmt"
 "strings"

 "github.com/dinhvansh/dataflow-engine/internal/cursor"
 "github.com/dinhvansh/dataflow-engine/internal/engine"
)

// KeySnapshotReader captures a single transactionally consistent view of
// all surviving source keys. SQL Server SNAPSHOT isolation MUST already be ON.
// A PK snapshot enables evidence of DELETE between complete generations,
// not realtime CDC or historical versions.
type KeySnapshotReader struct{
 DB *sql.DB
 Schema,Table string
 BatchSize int
}
func (r *KeySnapshotReader) Read(ctx context.Context,emit func(engine.Batch)error)error{
 if r.DB==nil{return errors.New("nil SQL Server DB")}
 if r.BatchSize<100||r.BatchSize>100000{return errors.New("batch size must be 100..100000")}
 schema,err:=quote(r.Schema);if err!=nil{return err}
 table,err:=quote(r.Table);if err!=nil{return err}
 cols,err:=DiscoverColumns(ctx,r.DB,r.Schema,r.Table);if err!=nil{return err}
 rowID,rev:=false,false
 for _,c:=range cols{
  if strings.EqualFold(c.Name,"SysRowID") && strings.EqualFold(c.SQLType,"uniqueidentifier"){rowID=true}
  if strings.EqualFold(c.Name,"SysRevID") && c.CursorCandidate{rev=true}
 }
 if !rowID||!rev{return errors.New("key snapshot requires SysRowID uniqueidentifier and SysRevID rowversion")}
 unique,err:=HasUniqueSysRowID(ctx,r.DB,r.Schema,r.Table)
 if err!=nil{return fmt.Errorf("source unique index discovery: %w",err)}
 if !unique{return errors.New("source SysRowID requires a declared unfiltered single-key UNIQUE index; no source DDL will be attempted")}
 tx,err:=r.DB.BeginTx(ctx,&sql.TxOptions{Isolation:sql.LevelSnapshot})
 if err!=nil{return fmt.Errorf("SQL Server SNAPSHOT unavailable: %w",err)}
 defer tx.Rollback()
 // Binary avoids driver-dependent truncation of the 8-byte rowversion when
 // SQL Server casts it to varchar. Source access remains strictly SELECT-only.
 query:="SELECT CONVERT(varchar(36),[SysRowID]), CONVERT(binary(8),[SysRevID]) FROM "+schema+"."+table+" ORDER BY [SysRowID]"
 rows,err:=tx.QueryContext(ctx,query);if err!=nil{return err}
 defer rows.Close()
 seq:=uint64(0)
 batch:=engine.Batch{Rows:make([]map[string]any,0,r.BatchSize)}
 for rows.Next(){
  var pk string
  var rawVersion []byte
  if err=rows.Scan(&pk,&rawVersion);err!=nil{return err}
  if pk=="" {return errors.New("empty source PK")}
  version,parseErr:=snapshotRowVersion(rawVersion);if parseErr!=nil{return parseErr}
  batch.Rows=append(batch.Rows,map[string]any{"SysRowID":strings.ToLower(pk),"SysRevID":version})
  if len(batch.Rows)>=r.BatchSize{
   batch.Sequence=seq
   if err=emit(batch);err!=nil{return err}
   seq++;batch=engine.Batch{Rows:make([]map[string]any,0,r.BatchSize)}
  }
 }
 if err=rows.Err();err!=nil{return err}
 if err=rows.Close();err!=nil{return err}
 if len(batch.Rows)>0{batch.Sequence=seq;if err=emit(batch);err!=nil{return err}}
 return tx.Commit()
}

// snapshotRowVersion accepts only a genuine 8-byte SQL Server rowversion.
// Never pad or truncate a malformed source cursor: fail closed instead.
func snapshotRowVersion(raw []byte) (string,error) {
 version,err:=cursor.FromBytes(raw)
 if err!=nil{return "",err}
 return version.String(),nil
}
