package lineage
import("testing";"reflect")
func TestAutoBuildChainPlan(t *testing.T){
 models:=[]Model{
  {Name:"c",Mode:"visual",Database:"scratch",Table:"dfv_b"},
  {Name:"a",Mode:"visual",Database:"scratch",Table:"raw_invoices"},
  {Name:"b",Mode:"visual",Database:"scratch",Table:"dfv_a"},
  {Name:"unrelated",Mode:"visual",Database:"scratch",Table:"raw_invoices"},
 }
 plan,e:=PlanChain(models,"c");if e!=nil{t.Fatal(e)}
 var actual []string;for _,m:=range plan{actual=append(actual,m.Name)}
 if !reflect.DeepEqual(actual,[]string{"a","b","c"}){t.Fatal(actual)}
}
func TestAutoBuildRejectsBrokenDAG(t *testing.T){
 for _,cases:=range [][]Model{
  {{Name:"c",Mode:"visual",Database:"scratch",Table:"dfv_missing"}},
  {{Name:"c",Mode:"sql",Database:"scratch",Table:"dfv_a"}},
  {{Name:"a",Mode:"visual",Database:"scratch",Table:"dfv_b"},{Name:"b",Mode:"visual",Database:"scratch",Table:"dfv_a"}},
  {{Name:"c",Mode:"visual",Database:"production",Table:"orders"}},
 }{
  if _,err:=PlanChain(cases,cases[0].Name);err==nil{t.Fatalf("accepted unsafe chain: %+v",cases)}
 }
}
