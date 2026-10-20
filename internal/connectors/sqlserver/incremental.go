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

// RowVersionReader reads a consistent CURRENT-STATE window with a SQL Server SNAPSHOT
// transaction. This reader does not write checkpoints; its caller must commit only
// after all destination batches succeed. Physical deletes are not detected.
// It deliberately fails if snapshot isolation is unavailable.
type RowVersionReader struct {
 DB *sql.DB
 Schema,Table string
 BatchSize int
 Lower cursor.RowVersion
 Upper cursor.RowVersion // populated after Read, valid only on successful return
}

func (r *RowVersionReader) projection(ctx context.Context)(string,error){
 cols,err:=DiscoverColumns(ctx,r.DB,r.Schema,r.Table)
 if err!=nil{return "",err}
 if len(cols)==0{return "",errors.New("source table missing or metadata not visible")}
 foundRev,foundID:=false,false
 expr:=make([]string,0,len(cols))
 for _,c:=range cols{
  quoted,err:=quote(c.Name);if err!=nil{return "",err}
  switch strings.ToLower(c.Name){
  case "sysrevid":
   if !c.CursorCandidate{return "",fmt.Errorf("SysRevID must be rowversion, found %s",c.SQLType)}
   foundRev=true
   // SQL Server ignores binary style 1 when converting rowversion directly;
   // normalize through varbinary so the driver receives stable ASCII hex.
   expr=append(expr,"CONVERT(varchar(18),CONVERT(varbinary(8),"+quoted+"),1) AS "+quoted)
  case "sysrowid":
   if !strings.EqualFold(c.SQLType,"uniqueidentifier"){return "",fmt.Errorf("SysRowID must be uniqueidentifier, found %s",c.SQLType)}
   foundID=true
   expr=append(expr,"CONVERT(varchar(36),"+quoted+") AS "+quoted)
  default:expr=append(expr,quoted)
  }
 }
 if !foundRev||!foundID{return "",errors.New("source requires SysRevID rowversion and SysRowID uniqueidentifier")}
 unique,err:=HasUniqueSysRowID(ctx,r.DB,r.Schema,r.Table)
 if err!=nil{return "",fmt.Errorf("source unique index inspection: %w",err)}
 if !unique{return "",errors.New("SysRowID requires a declared unique single-key index for deterministic incremental pagination")}
 return strings.Join(expr,","),nil
}

func BuildRowVersionPageQuery(schema,table,projection string,hasAfter bool)(string,error){
 sch,err:=quote(schema);if err!=nil{return "",err}
 tbl,err:=quote(table);if err!=nil{return "",err}
 if projection==""{return "",errors.New("column projection required")}
 // projection comes from trusted catalog metadata through projection().
 query:="SELECT TOP (@p1) "+projection+" FROM "+sch+"."+tbl+" WHERE [SysRevID] >= @p2 AND [SysRevID] < @p3"
 if hasAfter{
  query+=" AND ([SysRevID] > @p4 OR ([SysRevID] = @p5 AND [SysRowID] > CONVERT(uniqueidentifier,@p6)))"
 }
 query+=" ORDER BY [SysRevID],[SysRowID]"
 return query,nil
}

func(r *RowVersionReader)Read(ctx context.Context,emit func(engine.Batch)error)error{
 if r.DB==nil{return errors.New("nil source DB")}
 if r.BatchSize<1||r.BatchSize>100000{return errors.New("batch size out of bounds")}
 projection,err:=r.projection(ctx);if err!=nil{return err}
 tx,err:=r.DB.BeginTx(ctx,&sql.TxOptions{Isolation:sql.LevelSnapshot})
 if err!=nil{return fmt.Errorf("SNAPSHOT transaction unavailable; enable via DBA approval: %w",err)}
 defer tx.Rollback()
 var upperBytes []byte
 if err=tx.QueryRowContext(ctx,"SELECT CONVERT(binary(8),MIN_ACTIVE_ROWVERSION())").Scan(&upperBytes);err!=nil{
  return fmt.Errorf("cannot capture stable rowversion boundary: %w",err)
 }
 upper,err:=cursor.FromBytes(upperBytes);if err!=nil{return err}
 window:=cursor.Window{LowerInclusive:r.Lower,UpperExclusive:upper}
 if err=window.Validate();err!=nil{return err}
 var seq uint64
 var afterRev cursor.RowVersion
 var afterID string
 hasAfter:=false
 for {
  query,err:=BuildRowVersionPageQuery(r.Schema,r.Table,projection,hasAfter);if err!=nil{return err}
  pageLower:=r.Lower
  if hasAfter{pageLower=afterRev}
  args:=[]any{r.BatchSize,pageLower.Bytes(),upper.Bytes()}
  if hasAfter{args=append(args,afterRev.Bytes(),afterRev.Bytes(),afterID)}
  rows,err:=tx.QueryContext(ctx,query,args...);if err!=nil{return err}
  names,err:=rows.Columns();if err!=nil{rows.Close();return err}
  types,err:=rows.ColumnTypes();if err!=nil{rows.Close();return err}
  batch:=engine.Batch{Sequence:seq,Rows:make([]map[string]any,0,r.BatchSize)}
  for rows.Next(){
   vals:=make([]any,len(names));ptrs:=make([]any,len(names))
   for i:=range vals{ptrs[i]=&vals[i]}
   if err=rows.Scan(ptrs...);err!=nil{break}
   record:=make(map[string]any,len(names))
   for i,name:=range names{
    if raw,ok:=vals[i].([]byte);ok&&strings.EqualFold(name,"SysRevID")&&len(raw)==8{
     record[name]=fmt.Sprintf("0x%X",raw)
    }else if raw,ok:=vals[i].(string);ok&&strings.EqualFold(name,"SysRevID")&&len(raw)==8{
     record[name]=fmt.Sprintf("0x%X",[]byte(raw))
    }else if raw,ok:=vals[i].([]byte);ok&&strings.EqualFold(name,"SysRowID")&&len(raw)==16{
     record[name]=normalizeValue(raw,"uniqueidentifier")
    }else if raw,ok:=vals[i].(string);ok&&strings.EqualFold(name,"SysRowID")&&len(raw)==16{
     record[name]=normalizeValue([]byte(raw),"uniqueidentifier")
    }else{
     record[name]=normalizeValue(vals[i],types[i].DatabaseTypeName())
    }
   }
   rev,parseErr:=cursor.ParseRowVersion(fmt.Sprint(record["SysRevID"]))
   if parseErr!=nil{err=fmt.Errorf("invalid SysRevID %q (%T): %w",record["SysRevID"],record["SysRevID"],parseErr);break}
   id,ok:=record["SysRowID"].(string)
   if !ok||id==""{err=errors.New("missing SysRowID");break}
   afterRev,afterID=rev,id
   batch.Rows=append(batch.Rows,record)
  }
  if err==nil{err=rows.Err()}
  closeErr:=rows.Close()
  if err!=nil{return err}
  if closeErr!=nil{return closeErr}
  count:=len(batch.Rows)
  if count>0{
   if err=emit(batch);err!=nil{return err}
   seq++;hasAfter=true
  }
  if count<r.BatchSize{break}
 }
 if err=tx.Commit();err!=nil{return err}
 r.Upper=upper
 return nil
}
