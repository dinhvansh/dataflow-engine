package lineage
import ("encoding/json";"reflect";"strings";"testing")
func TestLineageEmptyCollectionsMarshalAsArrays(t *testing.T){
 g,err:=Build(nil);if err!=nil{t.Fatal(err)}
 raw,err:=json.Marshal(g);if err!=nil{t.Fatal(err)}
 body:=string(raw)
 for _,field:=range []string{`"nodes":[]`,`"edges":[]`,`"model_order":[]`,`"problems":[]`}{
  if !strings.Contains(body,field){t.Fatalf("missing array %s in %s",field,body)}
 }
}
func TestLineageChainAndImpact(t *testing.T){
 g,err:=Build([]Model{
  {Name:"report",Database:"scratch",Table:"dfv_enriched",Mode:"visual"},
  {Name:"enriched",Database:"scratch",Table:"dfv_staging",Mode:"visual"},
  {Name:"staging",Database:"scratch",Table:"invoices",Mode:"visual"},
  {Name:"other",Database:"scratch",Table:"invoices",Mode:"visual"},
 })
 if err!=nil{t.Fatal(err)}
 if !reflect.DeepEqual(g.Order,[]string{"staging","enriched","other","report"}){t.Fatalf("order %v",g.Order)}
 if !reflect.DeepEqual(Impact(g,"staging"),[]string{"enriched","report"}){t.Fatalf("impact %v",Impact(g,"staging"))}
 if len(g.Nodes)!=5||len(g.Edges)!=4{t.Fatalf("graph %+v",g)}
 if e:=CheckCycle(g);e!=nil{t.Fatal(e)}
}
func TestMissingAndCycle(t *testing.T){
 g,_:=Build([]Model{{Name:"a",Mode:"visual",Database:"scratch",Table:"dfv_unknown"}})
 if g.Nodes[0].Status!="missing_dependency"{t.Fatal(g)}
 c,_:=Build([]Model{
  {Name:"a",Mode:"visual",Database:"scratch",Table:"dfv_b"},
  {Name:"b",Mode:"visual",Database:"scratch",Table:"dfv_a"},
 })
 if CheckCycle(c)==nil||len(c.Order)!=0{t.Fatalf("cycle accepted %+v",c)}
 self,_:=Build([]Model{{Name:"a",Mode:"visual",Database:"scratch",Table:"dfv_a"}})
 if CheckCycle(self)==nil{t.Fatal("self dependency accepted")}
}
func TestSQLUntracked(t *testing.T){
 g,_:=Build([]Model{{Name:"raw",Mode:"sql",Database:"scratch",Table:"dfv_other"}})
 if g.Nodes[0].Status!="untracked_sql"||len(g.Edges)!=0{t.Fatal(g)}
}
