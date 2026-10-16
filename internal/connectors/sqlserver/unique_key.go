package sqlserver

import(
 "context"
 "database/sql"
 "errors"
)

// HasUniqueSysRowID verifies the source table has a trustworthy, unfiltered,
// single-key unique index on SysRowID, including a single-column PK.
// Included (non-key) index columns are allowed. This query is SELECT-only.
func HasUniqueSysRowID(ctx context.Context,db *sql.DB,schema,table string)(bool,error){
 if db==nil{return false,errors.New("nil SQL connection")}
 if _,err:=quote(schema);err!=nil{return false,err}
 if _,err:=quote(table);err!=nil{return false,err}
 const q=`SELECT CASE WHEN EXISTS (
  SELECT 1 FROM sys.indexes i
  JOIN sys.tables t ON t.object_id=i.object_id
  JOIN sys.schemas s ON s.schema_id=t.schema_id
  JOIN sys.index_columns ic ON ic.object_id=i.object_id AND ic.index_id=i.index_id AND ic.key_ordinal=1
  JOIN sys.columns c ON c.object_id=ic.object_id AND c.column_id=ic.column_id
  WHERE s.name=@p1 AND t.name=@p2 AND c.name='SysRowID'
    AND i.is_unique=1 AND i.is_disabled=0 AND i.is_hypothetical=0 AND i.has_filter=0
    AND NOT EXISTS (
      SELECT 1 FROM sys.index_columns second_key
      WHERE second_key.object_id=i.object_id AND second_key.index_id=i.index_id
        AND second_key.key_ordinal>1
    )
 ) THEN 1 ELSE 0 END`
 var good int
 if err:=db.QueryRowContext(ctx,q,schema,table).Scan(&good);err!=nil{return false,err}
 return good==1,nil
}
