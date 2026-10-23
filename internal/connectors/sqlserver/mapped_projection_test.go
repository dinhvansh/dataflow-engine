package sqlserver

import (
 "strings"
 "testing"
)

func mappedCatalog()[]ColumnInfo{
 return []ColumnInfo{
  {Name:"OrderID",SQLType:"bigint",PrimaryKey:true},
  {Name:"RowChanged",SQLType:"timestamp",CursorCandidate:true},
  {Name:"Amount",SQLType:"decimal"},
 }
}
func TestMappedProjectionFullWithoutSysColumns(t *testing.T){
 cols:=[]ColumnInfo{{Name:"OrderID",SQLType:"bigint"},{Name:"Amount",SQLType:"decimal"}}
 got,err:=BuildMappedProjection(cols,[]string{"OrderID","Amount"},"OrderID","")
 if err!=nil{t.Fatal(err)}
 for _,required:=range []string{"[OrderID]","AS [SysRowID]","'0x0000000000000000' AS [SysRevID]"}{
  if !strings.Contains(got,required){t.Fatalf("missing %s in %q",required,got)}
 }
}
func TestMappedProjectionCustomRowVersionAndPagination(t *testing.T){
 got,err:=BuildMappedProjection(mappedCatalog(),[]string{"OrderID","RowChanged","Amount"},"OrderID","RowChanged")
 if err!=nil{t.Fatal(err)}
 for _,required:=range []string{"AS [SysRowID]","AS [SysRevID]"}{
  if !strings.Contains(got,required){t.Fatalf("missing %s: %q",required,got)}
 }
 query,err:=BuildMappedPageQuery("Erp","Orders",got,"OrderID","RowChanged",true)
 if err!=nil{t.Fatal(err)}
 for _,required:=range []string{"[RowChanged] >= @p2","[RowChanged] < @p3","[OrderID] > @p6","ORDER BY [Orders].[RowChanged],[Orders].[OrderID]"}{
  if !strings.Contains(query,required){t.Fatalf("missing %s: %q",required,query)}
 }
}
func TestMappedProjectionFailsClosed(t *testing.T){
 cases:=[]struct{name,key,rv string;selected []string;cols []ColumnInfo}{
  {"missing key","Other","RowChanged",[]string{"OrderID","RowChanged"},mappedCatalog()},
  {"unselected key","OrderID","RowChanged",[]string{"RowChanged","Amount"},mappedCatalog()},
  {"bad cursor","OrderID","Amount",[]string{"OrderID","Amount"},mappedCatalog()},
  {"injection","OrderID];DROP TABLE","RowChanged",[]string{"OrderID","RowChanged"},mappedCatalog()},
  {"nullable key","OrderID","",[]string{"OrderID"},[]ColumnInfo{{Name:"OrderID",SQLType:"int",Nullable:true}}},
  {"alias collision","OrderID","RowChanged",[]string{"OrderID","RowChanged","SysRowID"},append(mappedCatalog(),ColumnInfo{Name:"SysRowID",SQLType:"uniqueidentifier"})},
  {"duplicate column","OrderID","RowChanged",[]string{"OrderID","OrderID","RowChanged"},mappedCatalog()},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  if v,e:=BuildMappedProjection(tc.cols,tc.selected,tc.key,tc.rv);e==nil{t.Fatalf("unsafe projection %q",v)}
 })}
 if _,e:=BuildMappedPageQuery("Erp","Orders;DROP","*", "OrderID","RowChanged",false);e==nil{
  t.Fatal("unsafe table identifier accepted")
 }
}
