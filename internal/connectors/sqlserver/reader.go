package sqlserver

import(
 "context"
 "database/sql"
 "errors"
 "fmt"
 "regexp"
 "strings"

 _ "github.com/microsoft/go-mssqldb"
 "github.com/dinhvansh/dataflow-engine/internal/engine"
)
var identifier=regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
func quote(s string)(string,error){if !identifier.MatchString(s){return "",fmt.Errorf("invalid identifier %q",s)};return "["+s+"]",nil}

// Reader supports FULL REFRESH only. No OFFSET, cursor or source writes.
// SQL Server result is streamed into bounded batches; source query runs with caller context.
type Reader struct{
 DB *sql.DB
 Schema,Table string
 BatchSize int
}
func(r *Reader)Read(ctx context.Context,emit func(engine.Batch)error)error{
 if r.DB==nil{return errors.New("SQL Server DB is nil")}
 if r.BatchSize<1 || r.BatchSize>100000{return errors.New("invalid batch size")}
 schema,err:=quote(r.Schema);if err!=nil{return err}
 table,err:=quote(r.Table);if err!=nil{return err}
 rows,err:=r.DB.QueryContext(ctx,"SELECT * FROM "+schema+"."+table)
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
   if b,ok:=v.([]byte);ok{
    // Preserve binary values; JSON encoding can be mapped in destination.
    cp:=append([]byte(nil),b...)
    typ:=strings.ToLower(types[i].DatabaseTypeName())
    if strings.Contains(typ,"binary")||typ=="timestamp"||typ=="rowversion" {obj[c]=fmt.Sprintf("0x%X",cp)} else {obj[c]=string(cp)}
   }else{obj[c]=v}
  }
  batch.Rows=append(batch.Rows,obj)
  if len(batch.Rows)>=r.BatchSize{batch.Sequence=seq;seq++;if err:=emit(batch);err!=nil{return err};batch=engine.Batch{}}
 }
 if err:=rows.Err();err!=nil{return err}
 if len(batch.Rows)>0{batch.Sequence=seq;return emit(batch)}
 return nil
}
