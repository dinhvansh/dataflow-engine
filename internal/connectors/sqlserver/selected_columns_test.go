package sqlserver

import (
 "strings"
 "testing"
)

func TestBuildRowVersionProjectionSelected(t *testing.T){
 cols:=[]ColumnInfo{
  {Name:"PartNum",SQLType:"nvarchar"},
  {Name:"SysRowID",SQLType:"uniqueidentifier"},
  {Name:"SysRevID",SQLType:"timestamp",CursorCandidate:true},
  {Name:"TranQty",SQLType:"decimal"},
 }
 sql,err:=BuildRowVersionProjection(cols,[]string{"SysRowID","SysRevID","PartNum"})
 if err!=nil{t.Fatal(err)}
 if !strings.Contains(sql,"[PartNum]")||!strings.Contains(sql,"AS [SysRevID]")||!strings.Contains(sql,"AS [SysRowID]"){
  t.Fatalf("missing selected/key projection: %s",sql)
 }
 if strings.Contains(sql,"[TranQty]"){t.Fatalf("excluded column leaked: %s",sql)}
}

func TestBuildRowVersionProjectionLegacyAll(t *testing.T){
 cols:=[]ColumnInfo{{Name:"SysRowID",SQLType:"uniqueidentifier"},{Name:"SysRevID",SQLType:"rowversion",CursorCandidate:true},{Name:"TranQty",SQLType:"decimal"}}
 sql,err:=BuildRowVersionProjection(cols,nil)
 if err!=nil{t.Fatal(err)}
 if !strings.Contains(sql,"[TranQty]"){t.Fatalf("legacy all-column projection changed: %s",sql)}
}

func TestBuildRowVersionProjectionRejectsUnsafeSelections(t *testing.T){
 cols:=[]ColumnInfo{{Name:"SysRowID",SQLType:"uniqueidentifier"},{Name:"SysRevID",SQLType:"timestamp",CursorCandidate:true},{Name:"PartNum",SQLType:"nvarchar"}}
 cases:=[][]string{
  {"SysRevID"}, // required key
  {"SysRowID"}, // required cursor
  {"SysRowID","SysRevID","Unknown"},
  {"SysRowID","SysRevID","PartNum; DROP TABLE Erp.PartTran"},
  {"SysRowID","SysRevID","PartNum","PartNum"},
 }
 for _,selected:=range cases{
  if sql,err:=BuildRowVersionProjection(cols,selected);err==nil{t.Errorf("accepted invalid %v: %s",selected,sql)}
 }
}

func TestBuildRowVersionProjectionRejectsBadCatalog(t *testing.T){
 cols:=[]ColumnInfo{{Name:"SysRowID",SQLType:"uniqueidentifier"},{Name:"SysRevID",SQLType:"int"},{Name:"PartNum",SQLType:"nvarchar"}}
 if _,err:=BuildRowVersionProjection(cols,nil);err==nil{t.Fatal("non-rowversion cursor accepted")}
}
