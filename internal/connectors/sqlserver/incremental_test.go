package sqlserver

import (
 "strings"
 "testing"
)
func TestBuildPageQuery(t *testing.T){
 first,err:=BuildRowVersionPageQuery("Erp","PartTran","[SysRevID],[SysRowID]",false)
 if err!=nil{t.Fatal(err)}
 if !strings.Contains(first,"[SysRevID] >= @p2")||strings.Contains(first,"@p6")||!strings.HasSuffix(first,"OPTION (MAXDOP 1)"){t.Fatal(first)}
 follow,err:=BuildRowVersionPageQuery("Erp","PartTran","[SysRevID],[SysRowID]",true)
 if err!=nil{t.Fatal(err)}
 if !strings.Contains(follow,"CONVERT(uniqueidentifier,@p6)")||!strings.Contains(follow,"ORDER BY [SysRevID],[SysRowID] OPTION (MAXDOP 1)"){t.Fatal(follow)}
 if _,err=BuildRowVersionPageQuery("Erp;DROP","PartTran","[SysRevID]",false);err==nil{t.Fatal("unsafe identifier accepted")}
 if _,err=BuildRowVersionPageQuery("Erp","PartTran","",false);err==nil{t.Fatal("empty projection accepted")}
}
