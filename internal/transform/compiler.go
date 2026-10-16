// Package transform compiles a restricted, auditable visual step pipeline
// into ClickHouse SELECT. No SQL mutations, DDL, dbt worker, or implicit writes.
package transform

import (
 "errors"
 "fmt"
 "regexp"
 "strconv"
 "strings"
)

var identifier=regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var numeric=regexp.MustCompile(`^-?(?:[0-9]+(?:\.[0-9]+)?|\.[0-9]+)$`)
var allowedOps=map[string]string{"equals":"=","not_equals":"!=","greater":">","less":"<","greater_or_equal":">=","less_or_equal":"<="}

type Source struct{
 Database string `json:"database"`
 Table string `json:"table"`
}
type Step struct{
 ID string `json:"id"`
 Type string `json:"type"`
 Enabled *bool `json:"enabled,omitempty"`
 Column string `json:"column,omitempty"`
 Columns []string `json:"columns,omitempty"`
 Operator string `json:"operator,omitempty"`
 Value string `json:"value,omitempty"`
 ValueType string `json:"value_type,omitempty"`
 Alias string `json:"alias,omitempty"`
 FormulaOperator string `json:"formula_operator,omitempty"`
 GroupBy []string `json:"group_by,omitempty"`
 Function string `json:"function,omitempty"`
 Direction string `json:"direction,omitempty"`
 Limit int `json:"limit,omitempty"`
}
type Model struct{
 Name string `json:"name"`
 Mode string `json:"mode"`
 Source Source `json:"source"`
 Steps []Step `json:"steps"`
 SQL string `json:"sql,omitempty"`
}
type Compiled struct{
 SQL string `json:"sql"`
 Steps []string `json:"steps_sql"`
}
// ValidateModelName restricts stored names so later dbt-file exports are safe.
func ValidateModelName(name string)(string,error){
 if !identifier.MatchString(name)||len(name)>120{return "",errors.New("model name must be a safe SQL identifier (letters, digits, underscore)")}
 return name,nil
}
func quoted(name string)(string,error){
 if !identifier.MatchString(name){return "",fmt.Errorf("invalid column or table identifier %q",name)}
 return "`"+name+"`",nil
}
func columns(names []string)(string,error){
 if len(names)==0||len(names)>80{return "",errors.New("choose 1..80 columns")}
 out:=make([]string,0,len(names));seen:=map[string]bool{}
 for _,n:=range names{
  q,err:=quoted(n);if err!=nil{return "",err}
  if seen[strings.ToLower(n)]{return "",errors.New("duplicate selected column")}
  seen[strings.ToLower(n)]=true;out=append(out,q)
 }
 return strings.Join(out,", "),nil
}
func literal(value,kind string)(string,error){
 if len(value)>512||strings.ContainsRune(value,0){return "",errors.New("filter value is too long or invalid")}
 switch kind{
 case "number":
  if !numeric.MatchString(value){return "",errors.New("invalid numeric filter value")}
  return value,nil
 case "text","":
  // ClickHouse SQL standard escaping: every single quote is doubled.
  return "'"+strings.ReplaceAll(value,"'","''")+"'",nil
 default:return "",errors.New("filter value_type must be text or number")
 }
}
func validateSource(source Source,scratch string)(string,error){
 if source.Database!=scratch||!identifier.MatchString(scratch){
  return "",errors.New("visual transformation restricted to configured scratch database")
 }
 db,err:=quoted(source.Database);if err!=nil{return "",err}
 table,err:=quoted(source.Table);if err!=nil{return "",err}
 return db+"."+table,nil
}
func Compile(m Model,scratch string)(Compiled,error){
 var out Compiled
 if len(m.Steps)>30{return out,errors.New("maximum 30 transformation steps")}
 if m.Mode=="sql"{
  if len(m.SQL)==0||len(m.SQL)>16384{return out,errors.New("custom SQL must be 1..16384 characters")}
  // The actual execution guard is the server-side SELECT-only database user.
  // Raw SQL has no writable grants; visual mode is more tightly constrained.
  query:=strings.TrimSpace(m.SQL)
  if !strings.HasPrefix(strings.ToUpper(query),"SELECT "){return out,errors.New("custom SQL must start with SELECT")}
  if strings.Contains(query,";")||strings.Contains(query,"--")||strings.Contains(query,"/*"){
   return out,errors.New("multiple statements and SQL comments are forbidden")
  }
  out.SQL=query;out.Steps=[]string{query};return out,nil
 }
 if m.Mode!=""&&m.Mode!="visual"{return out,errors.New("mode must be visual or sql")}
 src,err:=validateSource(m.Source,scratch);if err!=nil{return out,err}
 sql:="SELECT * FROM "+src
 out.Steps=append(out.Steps,sql)
 used:=map[string]bool{}
 for _,s:=range m.Steps{
  if !identifier.MatchString(s.ID){return Compiled{},errors.New("every step needs a safe unique ID")}
  if used[s.ID]{return Compiled{},errors.New("duplicate step ID")}
  used[s.ID]=true
  if s.Enabled!=nil&&!*s.Enabled{continue}
  parent:="("+sql+") AS _df_step"
  switch s.Type{
  case "filter":
   q,err:=quoted(s.Column);if err!=nil{return Compiled{},err}
   switch s.Operator{
   case "contains":
    if s.ValueType=="number"{return Compiled{},errors.New("contains works with text")}
    v,err:=literal(s.Value,"text");if err!=nil{return Compiled{},err}
    sql="SELECT * FROM "+parent+" WHERE positionUTF8(toString("+q+"), "+v+") > 0"
   case "is_null":
    sql="SELECT * FROM "+parent+" WHERE "+q+" IS NULL"
   case "is_not_null":
    sql="SELECT * FROM "+parent+" WHERE "+q+" IS NOT NULL"
   default:
    op,ok:=allowedOps[s.Operator];if !ok{return Compiled{},errors.New("unsupported filter operator")}
    v,err:=literal(s.Value,s.ValueType);if err!=nil{return Compiled{},err}
    sql="SELECT * FROM "+parent+" WHERE "+q+" "+op+" "+v
   }
  case "select":
   cols,err:=columns(s.Columns);if err!=nil{return Compiled{},err}
   sql="SELECT "+cols+" FROM "+parent
  case "rename":
   column,err:=quoted(s.Column);if err!=nil{return Compiled{},err}
   alias,err:=quoted(s.Alias);if err!=nil{return Compiled{},err}
   if s.Column==s.Alias{return Compiled{},errors.New("renamed column must have different name")}
   sql="SELECT * REPLACE("+column+" AS "+alias+") FROM "+parent
   // ClickHouse REPLACE requires an existing column name. Use EXCEPT
   // to remove the old field before adding a renamed expression instead.
   sql="SELECT * EXCEPT("+column+"), "+column+" AS "+alias+" FROM "+parent
  case "calculate":
   q,err:=quoted(s.Column);if err!=nil{return Compiled{},err}
   alias,err:=quoted(s.Alias);if err!=nil{return Compiled{},err}
   op:=map[string]string{"add":"+","subtract":"-","multiply":"*","divide":"/"}[s.FormulaOperator]
   if op==""{return Compiled{},errors.New("calculate supports add/subtract/multiply/divide")}
   value,err:=literal(s.Value,"number");if err!=nil{return Compiled{},err}
   if s.FormulaOperator=="divide"{
    number,_:=strconv.ParseFloat(value,64);if number==0{return Compiled{},errors.New("division by zero")}
   }
   sql="SELECT *, ("+q+" "+op+" "+value+") AS "+alias+" FROM "+parent
  case "aggregate":
   if len(s.GroupBy)>0{
    gb,err:=columns(s.GroupBy);if err!=nil{return Compiled{},err}
    alias,err:=quoted(s.Alias);if err!=nil{return Compiled{},err}
    var expr string
    if s.Function=="count"{expr="count()"}else{
     fn:=map[string]string{"sum":"sum","avg":"avg","min":"min","max":"max"}[s.Function]
     if fn==""{return Compiled{},errors.New("unsupported aggregate function")}
     col,err:=quoted(s.Column);if err!=nil{return Compiled{},err}
     expr=fn+"("+col+")"
    }
    sql="SELECT "+gb+", "+expr+" AS "+alias+" FROM "+parent+" GROUP BY "+gb
   }else{
    alias,err:=quoted(s.Alias);if err!=nil{return Compiled{},err}
    var expr string
    if s.Function=="count"{expr="count()"}else{
     fn:=map[string]string{"sum":"sum","avg":"avg","min":"min","max":"max"}[s.Function]
     if fn==""{return Compiled{},errors.New("unsupported aggregate function")}
     col,err:=quoted(s.Column);if err!=nil{return Compiled{},err}
     expr=fn+"("+col+")"
    }
    sql="SELECT "+expr+" AS "+alias+" FROM "+parent
   }
  case "sort":
   col,err:=quoted(s.Column);if err!=nil{return Compiled{},err}
   d:=strings.ToUpper(s.Direction)
   if d!="ASC"&&d!="DESC"{return Compiled{},errors.New("sort direction must be ASC or DESC")}
   sql="SELECT * FROM "+parent+" ORDER BY "+col+" "+d
  case "limit":
   if s.Limit<1||s.Limit>10000{return Compiled{},errors.New("limit must be 1..10000")}
   sql=fmt.Sprintf("SELECT * FROM %s LIMIT %d",parent,s.Limit)
  default:return Compiled{},fmt.Errorf("unsupported step type %q",s.Type)
  }
  if len(sql)>16000{return Compiled{},errors.New("SQL exceeds safe complexity limit")}
  out.Steps=append(out.Steps,sql)
 }
 out.SQL=sql
 return out,nil
}
func PreviewSQL(sql string,limit int)(string,error){
 if limit<1||limit>200{return "",errors.New("preview limit must be 1..200")}
 if len(sql)>16384{return "",errors.New("SQL too large")}
 return fmt.Sprintf("SELECT * FROM (%s) AS _df_preview LIMIT %d",sql,limit),nil
}
