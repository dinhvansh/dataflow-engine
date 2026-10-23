package main

import(
 "testing"
 "github.com/dinhvansh/dataflow-engine/internal/connectors/sqlserver"
)
func TestTypedMappingsCatalogAndAllowlist(t *testing.T){
 t.Setenv("DATAFLOW_QA_CLICKHOUSE_DATABASE_ALLOWLIST","scratch,analytics_qa")
 if !approvedTypedDatabase("analytics_qa")||approvedTypedDatabase("production")||
  approvedTypedDatabase("analytics_qa;DROP"){t.Fatal("target database allowlist bypass")}
 catalog:=[]sqlserver.ColumnInfo{
  {Name:"OrderID",SQLType:"bigint"},
  {Name:"Amount",SQLType:"decimal",Precision:18,Scale:2,Nullable:true},
  {Name:"Customer",SQLType:"nvarchar",MaxLength:100},
 }
 pairs:=[]typedMappingRequest{{Source:"OrderID",Target:"InvoiceID"},
   {Source:"Amount",Target:"TotalAmount"},{Source:"Customer",Target:"Customer"}}
 got,err:=resolveTypedMappings(catalog,[]string{"OrderID","Amount","Customer"},pairs,"OrderID")
 if err!=nil||len(got)!=3||got[1].CHType!="Nullable(Decimal(18,2))"||
  got[0].CHType!="Int64"||got[1].Target!="TotalAmount"{
  t.Fatalf("invalid typed metadata: %+v %v",got,err)
 }
 for _,bad:=range [][]typedMappingRequest{
  {{Source:"OrderID",Target:"_df_key"},{Source:"Amount",Target:"TotalAmount"},{Source:"Customer",Target:"Customer"}},
  {{Source:"OrderID",Target:"Amount"},{Source:"Amount",Target:"Amount"},{Source:"Customer",Target:"Customer"}},
  {{Source:"OrderID",Target:"OrderID;DELETE"},{Source:"Amount",Target:"TotalAmount"},{Source:"Customer",Target:"Customer"}},
  {{Source:"OrderID",Target:"OrderID"},{Source:"Other",Target:"TotalAmount"},{Source:"Customer",Target:"Customer"}},
 }{
  if v,e:=resolveTypedMappings(catalog,[]string{"OrderID","Amount","Customer"},bad,"OrderID");e==nil{
   t.Fatalf("unsafe typed mapping passed %+v",v)
  }
 }
 if _,err:=sqlCatalogCHType(sqlserver.ColumnInfo{Name:"x",SQLType:"xml"});err==nil{
  t.Fatal("unsupported type accepted")
 }
}
