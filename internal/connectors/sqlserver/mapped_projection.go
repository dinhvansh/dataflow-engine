package sqlserver

import (
 "context"
 "database/sql"
 "errors"
 "fmt"
 "strings"
)

// BuildMappedProjection keeps the versioned-event contract intact while
// admitting SQL tables whose UNIQUE key and rowversion have other names.
// A full snapshot may omit the cursor: synthetic version zero is used only
// for a complete, atomic replacement, never as an incremental checkpoint.
func BuildMappedProjection(cols []ColumnInfo, selected []string, key, version string)(string,error){
 if len(cols)==0{return "",errors.New("source metadata missing")}
 if key==""{return "",errors.New("source key is required")}
 if key=="SysRowID"&&version=="SysRevID"{return BuildRowVersionProjection(cols,selected)}
 available:=make(map[string]ColumnInfo,len(cols))
 for _,c:=range cols{available[c.Name]=c}
 keyInfo,ok:=available[key];if !ok{return "",errors.New("source key not found")}
 if keyInfo.Nullable{return "",errors.New("source key must be non-null")}
 if !safeMappedKeyType(keyInfo){return "",fmt.Errorf("unsupported mapped key type %s",keyInfo.SQLType)}
 if version!=""{
  c,ok:=available[version];if !ok||!c.CursorCandidate{return "",errors.New("cursor must be a rowversion/timestamp column")}
 }
 if len(selected)==0||len(selected)>256{return "",errors.New("mapped projection requires 1..256 explicit source columns")}
 chosen:=make(map[string]bool,len(selected))
 for _,name:=range selected{
  if _,ok:=available[name];!ok||chosen[name]{return "",fmt.Errorf("invalid or duplicate selected column %q",name)}
  chosen[name]=true
 }
 if !chosen[key]||(version!=""&&!chosen[version]){return "",errors.New("source key and cursor must be selected")}
 if key!="SysRowID"&&chosen["SysRowID"]{return "",errors.New("SysRowID alias collision")}
 if version!="SysRevID"&&chosen["SysRevID"]{return "",errors.New("SysRevID alias collision")}
 expr:=make([]string,0,len(selected)+2)
 for _,c:=range cols{
  if !chosen[c.Name]{continue}
  q,e:=quote(c.Name);if e!=nil{return "",e}
  expr=append(expr,q)
 }
 // Append canonical event aliases. SQL result key is UTF-8 text even
 // for numeric/GUID keys; the original source column is also retained.
 if key!="SysRowID"{
  q,_:=quote(key)
  expr=append(expr,"CONVERT(nvarchar(512),"+q+") AS [SysRowID]")
 }
 if version==""{
  expr=append(expr,"'0x0000000000000000' AS [SysRevID]")
 }else if version!="SysRevID"{
  q,_:=quote(version)
  expr=append(expr,"CONVERT(varchar(18),CONVERT(varbinary(8),"+q+"),1) AS [SysRevID]")
 }
 return strings.Join(expr,","),nil
}
func safeMappedKeyType(c ColumnInfo)bool{
 switch strings.ToLower(c.SQLType){
 case "uniqueidentifier","bigint","int","smallint","tinyint":return true
 case "varchar":return c.MaxLength>0&&c.MaxLength<=512
 case "nvarchar":return c.MaxLength>0&&c.MaxLength<=1024
 }
 return false
}

// HasUniqueSourceKey requires a real single-key, unfiltered, enabled unique
// index. Nullable keys and composite keys are not accepted.
func HasUniqueSourceKey(ctx context.Context,db *sql.DB,schema,table,key string)(bool,error){
 if db==nil{return false,errors.New("nil SQL connection")}
 for _,v:=range []string{schema,table,key}{if _,e:=quote(v);e!=nil{return false,e}}
 const q=`SELECT CASE WHEN EXISTS (
  SELECT 1 FROM sys.indexes i
  JOIN sys.tables t ON t.object_id=i.object_id
  JOIN sys.schemas s ON s.schema_id=t.schema_id
  JOIN sys.index_columns ic ON ic.object_id=i.object_id AND ic.index_id=i.index_id AND ic.key_ordinal=1
  JOIN sys.columns c ON c.object_id=ic.object_id AND c.column_id=ic.column_id
  WHERE s.name=@p1 AND t.name=@p2 AND c.name=@p3
    AND i.is_unique=1 AND i.is_disabled=0 AND i.is_hypothetical=0 AND i.has_filter=0
    AND NOT EXISTS (
     SELECT 1 FROM sys.index_columns second_key
     WHERE second_key.object_id=i.object_id AND second_key.index_id=i.index_id
       AND second_key.key_ordinal>1
    )
 ) THEN 1 ELSE 0 END`
 var n int
 err:=db.QueryRowContext(ctx,q,schema,table,key).Scan(&n)
 return n==1,err
}

// BuildMappedPageQuery uses real source columns for filtering/order rather
// than the display aliases, preserving index seek performance.
func BuildMappedPageQuery(schema,table,projection,key,version string,hasAfter bool)(string,error){
 if projection==""||key==""||version==""{return "",errors.New("mapped incremental query needs key, cursor and projection")}
 sch,e:=quote(schema);if e!=nil{return "",e}
 tbl,e:=quote(table);if e!=nil{return "",e}
 pk,e:=quote(key);if e!=nil{return "",e}
 rv,e:=quote(version);if e!=nil{return "",e}
 query:="SELECT TOP (@p1) "+projection+" FROM "+sch+"."+tbl+" WHERE "+rv+" >= @p2 AND "+rv+" < @p3"
 if hasAfter{
  query+=" AND ("+rv+" > @p4 OR ("+rv+" = @p5 AND "+pk+" > @p6))"
 }
 query+=" ORDER BY "+tbl+"."+rv+","+tbl+"."+pk+" OPTION (MAXDOP 1)"
 return query,nil
}
