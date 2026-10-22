package sqlserver

import(
 "context"
 "database/sql"
 "errors"
 "fmt"
 "regexp"
 "strings"
 "time"

 _ "github.com/microsoft/go-mssqldb"
 "github.com/dinhvansh/dataflow-engine/internal/engine"
 "github.com/dinhvansh/dataflow-engine/internal/cursor"
)
var identifier=regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
func quote(s string)(string,error){if !identifier.MatchString(s){return "",fmt.Errorf("invalid identifier %q",s)};return "["+s+"]",nil}

func normalizeValue(v any, databaseType string) any {
 typ:=strings.ToLower(databaseType)
 if b,ok:=v.([]byte);ok{
  switch typ{
  case "uniqueidentifier":
   if len(b)==16{return fmt.Sprintf("%02X%02X%02X%02X-%02X%02X-%02X%02X-%02X%02X-%02X%02X%02X%02X%02X%02X",b[3],b[2],b[1],b[0],b[5],b[4],b[7],b[6],b[8],b[9],b[10],b[11],b[12],b[13],b[14],b[15])}
  }
  if strings.Contains(typ,"binary")||typ=="timestamp"||typ=="rowversion" {return fmt.Sprintf("0x%X",b)}
  return string(b)
 }
 if ts,ok:=v.(time.Time);ok{
  if typ=="date"{return ts.Format("2006-01-02")}
  return ts.Format("2006-01-02 15:04:05.9999999")
 }
 return v
}

// Reader supports FULL REFRESH only. No OFFSET, cursor or source writes.
// SQL Server result is streamed into bounded batches; source query runs with caller context.
type Reader struct{
 DB *sql.DB
 Schema,Table string
 KeyColumn,CursorColumn string // mapped source identity/version; empty cursor means snapshot-only
 CaptureBoundary bool // bootstrap: safe next RowVersion lower bound from same SNAPSHOT transaction
 Boundary cursor.RowVersion // valid only after committed Read
 BatchSize int
 Columns []string // nil = legacy SELECT *; selected projections validated from catalog
 Snapshot bool
}
func(r *Reader)Read(ctx context.Context,emit func(engine.Batch)error)error{
 if r.DB==nil{return errors.New("SQL Server DB is nil")}
 if r.BatchSize<1 || r.BatchSize>100000{return errors.New("invalid batch size")}
 schema,err:=quote(r.Schema);if err!=nil{return err}
 table,err:=quote(r.Table);if err!=nil{return err}
 var tx *sql.Tx
 if r.Snapshot{
  // go-mssqldb rejects TxOptions.ReadOnly even for a SELECT-only login.
  // Source immutability is enforced by SQL Server grants; SNAPSHOT provides
  // consistency without asking the driver for its unsupported read-only flag.
  tx,err=r.DB.BeginTx(ctx,&sql.TxOptions{Isolation:sql.LevelSnapshot})
  if err!=nil{return fmt.Errorf("consistent full refresh requires SQL Server SNAPSHOT isolation: %w",err)}
  defer tx.Rollback()
 }
 projection:="*"
 if r.CaptureBoundary{
  if tx==nil||r.CursorColumn==""{return errors.New("bootstrap boundary requires SNAPSHOT and a rowversion cursor")}
  var raw []byte
  if err=tx.QueryRowContext(ctx,"SELECT CONVERT(binary(8),MIN_ACTIVE_ROWVERSION())").Scan(&raw);err!=nil{return fmt.Errorf("cannot capture consistent snapshot boundary: %w",err)}
  r.Boundary,err=cursor.FromBytes(raw);if err!=nil{return err}
 }
 if len(r.Columns)>0{
  catalog,e:=DiscoverColumns(ctx,r.DB,r.Schema,r.Table)
  if e!=nil{return fmt.Errorf("full source metadata unavailable: %w",e)}
  if r.KeyColumn==""&&r.CursorColumn==""{
   projection,e=BuildRowVersionProjection(catalog,r.Columns)
  }else{
   projection,e=BuildMappedProjection(catalog,r.Columns,r.KeyColumn,r.CursorColumn)
  }
  if e!=nil{return fmt.Errorf("full selected fields invalid: %w",e)}
 }
 var rows *sql.Rows
 query:="SELECT "+projection+" FROM "+schema+"."+table
 if tx!=nil{rows,err=tx.QueryContext(ctx,query)} else {rows,err=r.DB.QueryContext(ctx,query)}
 if err!=nil{return err};defer rows.Close()
 cols,err:=rows.Columns();if err!=nil{return err}
 types,err:=rows.ColumnTypes();if err!=nil{return err}
 var seq uint64
 batch:=engine.Batch{}
 for rows.Next(){
  vals:=make([]any,len(cols));ptrs:=make([]any,len(cols))
  for i:=range vals{ptrs[i]=&vals[i]}
  if err:=rows.Scan(ptrs...);err!=nil{return err}
  obj:=make(map[string]any,len(cols))
  for i,c:=range cols{
   v:=vals[i]
   obj[c]=normalizeValue(v,types[i].DatabaseTypeName())
  }
  batch.Rows=append(batch.Rows,obj)
  if len(batch.Rows)>=r.BatchSize{batch.Sequence=seq;seq++;if err:=emit(batch);err!=nil{return err};batch=engine.Batch{}}
 }
 if err:=rows.Err();err!=nil{return err}
 if len(batch.Rows)>0{batch.Sequence=seq;if err:=emit(batch);err!=nil{return err}}
 if err:=rows.Close();err!=nil{return err}
 if tx!=nil{return tx.Commit()}
 return nil
}
