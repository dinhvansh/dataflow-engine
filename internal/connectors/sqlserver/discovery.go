package sqlserver

import (
 "context"
 "database/sql"
 "fmt"
 "strings"

 "github.com/dinhvansh/dataflow-engine/internal/cursor"
)

type TableInfo struct{
 Schema string `json:"schema"`
 Name string `json:"name"`
}
type ColumnInfo struct{
 Name string `json:"name"`
 SQLType string `json:"sql_type"`
 MaxLength int `json:"max_length"`
 Precision int `json:"precision"`
 Scale int `json:"scale"`
 Nullable bool `json:"nullable"`
 Identity bool `json:"identity"`
 PrimaryKey bool `json:"primary_key"`
 CursorCandidate bool `json:"cursor_candidate"`
}
func DiscoverTables(ctx context.Context,db *sql.DB,schema string)([]TableInfo,error){
 if db==nil{return nil,fmt.Errorf("nil SQL connection")}
 if _,err:=quote(schema);err!=nil{return nil,err}
 const query=`SELECT TOP (500) s.name,t.name FROM sys.tables t JOIN sys.schemas s ON t.schema_id=s.schema_id WHERE s.name=@p1 ORDER BY t.name`
 rows,err:=db.QueryContext(ctx,query,schema);if err!=nil{return nil,err};defer rows.Close()
 out:=make([]TableInfo,0)
 for rows.Next(){var x TableInfo;if err=rows.Scan(&x.Schema,&x.Name);err!=nil{return nil,err};out=append(out,x)}
 return out,rows.Err()
}
func DiscoverColumns(ctx context.Context,db *sql.DB,schema,table string)([]ColumnInfo,error){
 if db==nil{return nil,fmt.Errorf("nil SQL connection")}
 if _,err:=quote(schema);err!=nil{return nil,err}
 if _,err:=quote(table);err!=nil{return nil,err}
 const query=`SELECT c.name, ty.name, CONVERT(int,c.max_length), CONVERT(int,c.precision), CONVERT(int,c.scale),
 c.is_nullable,c.is_identity,
 CONVERT(bit,CASE WHEN pk.column_id IS NULL THEN 0 ELSE 1 END)
 FROM sys.columns c
 JOIN sys.tables t ON t.object_id=c.object_id
 JOIN sys.schemas s ON s.schema_id=t.schema_id
 JOIN sys.types ty ON ty.user_type_id=c.user_type_id
 LEFT JOIN (
  SELECT ic.object_id,ic.column_id
  FROM sys.indexes i JOIN sys.index_columns ic ON i.object_id=ic.object_id AND i.index_id=ic.index_id
  WHERE i.is_primary_key=1
 ) pk ON pk.object_id=c.object_id AND pk.column_id=c.column_id
 WHERE s.name=@p1 AND t.name=@p2
 ORDER BY c.column_id`
 rows,err:=db.QueryContext(ctx,query,schema,table);if err!=nil{return nil,err};defer rows.Close()
 out:=make([]ColumnInfo,0)
 for rows.Next(){
  var c ColumnInfo
  if err=rows.Scan(&c.Name,&c.SQLType,&c.MaxLength,&c.Precision,&c.Scale,&c.Nullable,&c.Identity,&c.PrimaryKey);err!=nil{return nil,err}
  kind:=strings.ToLower(c.SQLType);c.CursorCandidate=kind=="timestamp"||kind=="rowversion"
  out=append(out,c)
 }
 return out,rows.Err()
}
func SafeRowVersionUpper(ctx context.Context,db *sql.DB)(cursor.RowVersion,error){
 if db==nil{return cursor.RowVersion{},fmt.Errorf("nil SQL connection")}
 var raw []byte
 err:=db.QueryRowContext(ctx,"SELECT CONVERT(binary(8), MIN_ACTIVE_ROWVERSION())").Scan(&raw)
 if err!=nil{return cursor.RowVersion{},err}
 return cursor.FromBytes(raw)
}
