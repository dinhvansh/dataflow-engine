package safetable

import(
 "context"
 "errors"
 "fmt"
 "regexp"
 "strconv"
 "strings"
)
var deltaTableName=regexp.MustCompile("^[a-z][a-z0-9_]{0,85}$")
var deltaColumnName=regexp.MustCompile("^[A-Za-z_][A-Za-z0-9_]*$")
type DeltaPlan struct{
 Target,Delta,Stage,Key string
 Lower,Upper uint64
 BaselineUUID string
}
type DeltaStats struct{Events,Rows uint64}
type column struct{Name,Type string}
func (c Client)columns(ctx context.Context,table string)([]column,error){
 if !identifier.MatchString(table)&&!stageID.MatchString(table){return nil,errors.New("unsafe table name")}
 raw,err:=c.Query(ctx,"SELECT name,type FROM system.columns WHERE database='scratch' AND table='"+table+"' ORDER BY position FORMAT TabSeparatedRaw")
 if err!=nil{return nil,err}
 var result []column
 for _,line:=range strings.Split(raw,"\n"){
  if strings.TrimSpace(line)==""{continue}
  fields:=strings.Split(line,"\t")
  if len(fields)!=2||!deltaColumnName.MatchString(fields[0]){return nil,errors.New("unsafe schema column")}
  result=append(result,column{fields[0],fields[1]})
 }
 if len(result)==0||len(result)>30{return nil,errors.New("schema must have 1-30 columns")}
 return result,nil
}
func(c Client)ValidateDeltaSchema(ctx context.Context,target,delta,key string)([]string,error){
 if !strings.HasPrefix(target,"dft_")||!identifier.MatchString(target)||!deltaTableName.MatchString(delta)||!deltaColumnName.MatchString(key){
  return nil,errors.New("unsafe target, delta, or key")
 }
 if err:=c.RequireAtomic(ctx);err!=nil{return nil,err}
 targetCols,err:=c.columns(ctx,target);if err!=nil{return nil,err}
 changes,err:=c.columns(ctx,delta);if err!=nil{return nil,err}
 if len(changes)!=len(targetCols)+2{return nil,errors.New("delta schema must equal target columns plus _df_version and _df_deleted")}
 names:=make([]string,0,len(targetCols));foundKey:=false
 for i,col:=range targetCols{
  if col!=changes[i]{return nil,fmt.Errorf("schema drift at %s: delta payload must match target ordering and types",col.Name)}
  if col.Name==key{
   foundKey=true
   if strings.Contains(col.Type,"Nullable("){return nil,errors.New("incremental key must be NOT NULL")}
  }
  names=append(names,col.Name)
 }
 if !foundKey{return nil,errors.New("incremental key not found in target")}
 if changes[len(changes)-2]!=(column{"_df_version","UInt64"})||changes[len(changes)-1]!=(column{"_df_deleted","UInt8"}){
  return nil,errors.New("delta must end with UInt64 _df_version and UInt8 _df_deleted")
 }
 return names,nil
}
func(c Client)UpperCursor(ctx context.Context,delta string)(uint64,error){
 if !deltaTableName.MatchString(delta){return 0,errors.New("unsafe delta table")}
 raw,err:=c.Query(ctx,"SELECT ifNull(max(_df_version),0) FROM scratch."+delta+" FORMAT TabSeparatedRaw")
 if err!=nil{return 0,err}
 return strconv.ParseUint(strings.TrimSpace(raw),10,64)
}
func(c Client)DeltaStats(ctx context.Context,p DeltaPlan)(uint64,error){
 if _,err:=c.ValidateDeltaSchema(ctx,p.Target,p.Delta,p.Key);err!=nil{return 0,err}
 if p.Upper<=p.Lower{return 0,errors.New("delta window has no new version")}
 q:=fmt.Sprintf("SELECT count(),uniqExact(tuple(%s,_df_version)),countIf(_df_deleted NOT IN (0,1)) FROM scratch.%s WHERE _df_version>%d AND _df_version<=%d FORMAT TabSeparatedRaw",
 p.Key,p.Delta,p.Lower,p.Upper)
 raw,err:=c.Query(ctx,q);if err!=nil{return 0,err}
 fields:=strings.Split(raw,"\t");if len(fields)!=3{return 0,errors.New("invalid delta stats")}
 total,e:=strconv.ParseUint(fields[0],10,64);if e!=nil{return 0,e}
 unique,e:=strconv.ParseUint(fields[1],10,64);if e!=nil{return 0,e}
 bad,e:=strconv.ParseUint(fields[2],10,64);if e!=nil{return 0,e}
 if total==0{return 0,errors.New("delta window empty")}
 if total>50000{return 0,errors.New("TEST delta cap exceeded (50k events)")}
 if unique!=total{return 0,errors.New("two events share same key and version")}
 if bad!=0{return 0,errors.New("invalid deletion marker")}
 return total,nil
}
// BuildDeltaStage uses a full copy-on-write physical shadow, but applies only
// the latest change per key in its bounded cursor window. This deliberately
// prioritizes update/delete correctness and atomic publish over write speed.
func(c Client)BuildDeltaStage(ctx context.Context,p DeltaPlan)(DeltaStats,error){
 var result DeltaStats
 if !stageID.MatchString(p.Stage)||p.Stage==p.Target||p.BaselineUUID==""{return result,errors.New("unsafe staging contract")}
 cols,err:=c.ValidateDeltaSchema(ctx,p.Target,p.Delta,p.Key);if err!=nil{return result,err}
 existing,err:=c.UUID(ctx,p.Target);if err!=nil{return result,err}
 if existing!=p.BaselineUUID{return result,errors.New("target UUID changed since checkpoint")}
 result.Events,err=c.DeltaStats(ctx,p);if err!=nil{return result,err}
 raw,err:=c.Query(ctx,fmt.Sprintf("SELECT count(),uniqExact(%s) FROM scratch.%s FORMAT TabSeparatedRaw",p.Key,p.Target))
 if err!=nil{return result,err}
 pair:=strings.Split(raw,"\t")
 if len(pair)!=2{return result,errors.New("invalid target uniqueness stats")}
 total,e:=strconv.ParseUint(pair[0],10,64);if e!=nil{return result,e}
 unique,e:=strconv.ParseUint(pair[1],10,64);if e!=nil{return result,e}
 if total>1000000||total!=unique{return result,errors.New("target must contain <=1m unique key rows")}
 // Rebuild an existing private stage after a crash rather than appending twice.
 if _,err=c.Query(ctx,"CREATE TABLE IF NOT EXISTS scratch."+p.Stage+" AS scratch."+p.Target);err!=nil{return result,err}
 if _,err=c.Query(ctx,"TRUNCATE TABLE scratch."+p.Stage);err!=nil{return result,err}
 touched:=fmt.Sprintf("SELECT %s FROM scratch.%s WHERE _df_version>%d AND _df_version<=%d",p.Key,p.Delta,p.Lower,p.Upper)
 keep:=fmt.Sprintf("INSERT INTO scratch.%s (%s) SELECT %s FROM scratch.%s WHERE %s NOT IN (%s)",p.Stage,strings.Join(cols,","),strings.Join(cols,","),p.Target,p.Key,touched)
 if _,err=c.Query(ctx,keep);err!=nil{return result,err}
 columns:=strings.Join(cols,",")
 latest:=fmt.Sprintf("SELECT %s,_df_version,_df_deleted FROM scratch.%s WHERE _df_version>%d AND _df_version<=%d ORDER BY %s ASC,_df_version DESC LIMIT 1 BY %s",
 columns,p.Delta,p.Lower,p.Upper,p.Key,p.Key)
 replace:=fmt.Sprintf("INSERT INTO scratch.%s (%s) SELECT %s FROM (%s) WHERE _df_deleted=0",p.Stage,columns,columns,latest)
 if _,err=c.Query(ctx,replace);err!=nil{return result,err}
 after,err:=c.Query(ctx,fmt.Sprintf("SELECT count(),uniqExact(%s) FROM scratch.%s FORMAT TabSeparatedRaw",p.Key,p.Stage))
 if err!=nil{return result,err}
 fields:=strings.Split(after,"\t")
 if len(fields)!=2{return result,errors.New("invalid output stats")}
 result.Rows,e=strconv.ParseUint(fields[0],10,64);if e!=nil{return result,e}
 distinct,e:=strconv.ParseUint(fields[1],10,64);if e!=nil{return result,e}
 if result.Rows>1000000||result.Rows!=distinct{return result,errors.New("staging output has duplicate keys or exceeds 1m TEST cap")}
 // Empty result is valid if the whole dataset was deleted.
 return result,nil
}
