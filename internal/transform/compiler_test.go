package transform

import (
 "strings"
 "testing"
)
func TestVisualStepsCompile(t *testing.T){
 m:=Model{Source:Source{Database:"scratch",Table:"invoices"},Steps:[]Step{
  {ID:"s1",Type:"filter",Column:"CustomerName",Operator:"contains",Value:"O'Brien"},
  {ID:"s2",Type:"calculate",Column:"Revenue",Alias:"Gross",FormulaOperator:"multiply",Value:"1.1"},
  {ID:"s3",Type:"select",Columns:[]string{"CustomerName","Gross"}},
  {ID:"s4",Type:"aggregate",GroupBy:[]string{"CustomerName"},Function:"sum",Column:"Gross",Alias:"RevenueSum"},
  {ID:"s5",Type:"sort",Column:"RevenueSum",Direction:"DESC"},
  {ID:"s6",Type:"limit",Limit:20},
 }}
 c,e:=Compile(m,"scratch");if e!=nil{t.Fatal(e)}
 if len(c.Steps)!=7{t.Fatalf("steps=%d",len(c.Steps))}
 for _,want:=range []string{"O''Brien","GROUP BY `CustomerName`","ORDER BY `RevenueSum` DESC","LIMIT 20"}{
  if !strings.Contains(c.SQL,want){t.Fatalf("missing %q in %s",want,c.SQL)}
 }
 q,e:=PreviewSQL(c.Steps[2],20);if e!=nil||!strings.Contains(q,"_df_preview LIMIT 20"){t.Fatal(q,e)}
}
func TestBlockedInputs(t *testing.T){
 list:=[]Model{
  {Source:Source{Database:"production",Table:"invoices"}},
  {Source:Source{Database:"scratch",Table:"x;DROP"}},
  {Source:Source{Database:"scratch",Table:"t"},Steps:[]Step{{ID:"a",Type:"select",Columns:[]string{"a","a"}}}},
  {Source:Source{Database:"scratch",Table:"t"},Steps:[]Step{{ID:"a",Type:"filter",Column:"x",Operator:"equals",Value:"0;DROP",ValueType:"number"}}},
  {Source:Source{Database:"scratch",Table:"t"},Steps:[]Step{{ID:"a",Type:"calculate",Column:"x",Alias:"y",FormulaOperator:"divide",Value:"0"}}},
  {Source:Source{Database:"scratch",Table:"t"},Steps:[]Step{{ID:"a",Type:"limit",Limit:100000}}},
  {Mode:"sql",SQL:"DROP TABLE scratch.x"},
  {Mode:"sql",SQL:"SELECT 1; DELETE FROM x"},
  {Mode:"sql",SQL:"SELECT 1 -- remove safeguards"},
 }
 for i,m:=range list{if _,err:=Compile(m,"scratch");err==nil{t.Fatalf("unsafe model %d accepted: %+v",i,m)}}
}
func TestStepDisableAndRename(t *testing.T){
 no:=false
 c,e:=Compile(Model{Source:Source{Database:"scratch",Table:"raw"},Steps:[]Step{
  {ID:"s1",Type:"filter",Column:"x",Operator:"equals",Value:"yes",Enabled:&no},
  {ID:"s2",Type:"rename",Column:"Old",Alias:"New"},
 }},"scratch")
 if e!=nil{t.Fatal(e)}
 if len(c.Steps)!=2||strings.Contains(c.SQL,"WHERE"){t.Fatal(c)}
 if !strings.Contains(c.SQL,"EXCEPT(`Old`)"){t.Fatal(c.SQL)}
}
func TestSQLModeAndPreviewLimit(t *testing.T){
 c,e:=Compile(Model{Mode:"sql",SQL:"SELECT 1 AS value"},"scratch")
 if e!=nil||c.SQL!="SELECT 1 AS value"{t.Fatal(c,e)}
 if _,err:=PreviewSQL(c.SQL,201);err==nil{t.Fatal("preview cap ignored")}
}
