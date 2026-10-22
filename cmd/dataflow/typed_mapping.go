package main

import (
 "errors"
 "fmt"
 "os"
 "strings"

 "github.com/dinhvansh/dataflow-engine/internal/connectors/clickhouse"
 "github.com/dinhvansh/dataflow-engine/internal/connectors/sqlserver"
)

// Every typed target database must be provisioned/approved independently by
// the operator. The feature cannot create databases or escape a local QA
// allowlist: default is scratch and real ERP connections stay prohibited.
func approvedTypedDatabase(name string)bool{
 if name==""||len(name)>64{return false}
 for i,c:=range name{
  if !((c>='a'&&c<='z')||(c>='A'&&c<='Z')||c=='_'||
       (i>0&&c>='0'&&c<='9')){return false}
 }
 allowed:=os.Getenv("DATAFLOW_QA_CLICKHOUSE_DATABASE_ALLOWLIST")
 if allowed==""{allowed="scratch"}
 for _,v:=range strings.Split(allowed,","){if strings.TrimSpace(v)==name{return true}}
 return false
}
type typedMappingRequest struct{
 Source string `json:"source"`
 Target string `json:"target"`
}
func sqlCatalogCHType(c sqlserver.ColumnInfo)(string,error){
 sqlType:=strings.ToLower(c.SQLType)
 var chType string
 switch sqlType{
 case "bigint":chType="Int64"
 case "int":chType="Int32"
 case "smallint":chType="Int16"
 case "tinyint","bit":chType="UInt8"
 case "float":chType="Float64"
 case "real":chType="Float32"
 case "decimal","numeric":
  if c.Precision<1||c.Precision>38||c.Scale<0||c.Scale>c.Precision{
   return "",errors.New("unsupported decimal precision/scale")
  }
  chType=fmt.Sprintf("Decimal(%d,%d)",c.Precision,c.Scale)
 case "date":chType="Date"
 // UTC conversions are business-context-dependent: preserve SQL source
 // representation in String instead of inventing time-zone assumptions.
 case "datetime","datetime2","smalldatetime","datetimeoffset","time",
   "char","nchar","varchar","nvarchar","text","ntext","uniqueidentifier",
   "binary","varbinary","timestamp","rowversion":chType="String"
 default:return "",fmt.Errorf("unsupported SQL Server type %s",c.SQLType)
 }
 if c.Nullable{chType="Nullable("+chType+")"}
 if !clickhouse.SafeCHType(chType){return "",errors.New("ClickHouse type not in safe whitelist")}
 return chType,nil
}
func resolveTypedMappings(catalog []sqlserver.ColumnInfo,selected []string,requested []typedMappingRequest,sourceKey string)([]clickhouse.TypedColumn,error){
 if len(selected)==0||len(selected)>256{return nil,errors.New("select between 1 and 256 columns")}
 existing:=make(map[string]sqlserver.ColumnInfo)
 for _,col:=range catalog{existing[col.Name]=col}
 pairs:=requested
 if len(pairs)==0{
  pairs=make([]typedMappingRequest,0,len(selected))
  for _,s:=range selected{pairs=append(pairs,typedMappingRequest{Source:s,Target:s})}
 }
 if len(pairs)!=len(selected){return nil,errors.New("typed mapping must include every selected column exactly once")}
 selectedSet:=make(map[string]bool)
 for _,s:=range selected{
  if selectedSet[s]{return nil,errors.New("duplicate selected source column")}
  selectedSet[s]=true
 }
 seen,targets:=map[string]bool{},map[string]bool{}
 out:=make([]clickhouse.TypedColumn,0,len(pairs))
 for _,pair:=range pairs{
  col,ok:=existing[pair.Source]
  if !ok||!selectedSet[pair.Source]||seen[pair.Source]{
   return nil,errors.New("mapped source not in selected catalog columns")
  }
  seen[pair.Source]=true
  lower:=strings.ToLower(pair.Target)
  if !validTypedColumnName(pair.Target)||targets[lower]{
   return nil,errors.New("unsafe duplicate or reserved target column")
  }
  targets[lower]=true
  typ,err:=sqlCatalogCHType(col);if err!=nil{return nil,err}
  out=append(out,clickhouse.TypedColumn{Source:pair.Source,Target:pair.Target,CHType:typ})
 }
 if !seen[sourceKey]{return nil,errors.New("primary key must be mapped")}
 return out,nil
}
func validTypedColumnName(name string)bool{
 if name==""||len(name)>64||strings.HasPrefix(strings.ToLower(name),"_df_")||
  strings.EqualFold(name,"_run_id")||strings.EqualFold(name,"_batch_seq"){return false}
 for i,c:=range name{
  if !((c>='A'&&c<='Z')||(c>='a'&&c<='z')||c=='_'||
   (i>0&&c>='0'&&c<='9')){return false}
 }
 return true
}
