package main

import (
 "context"
 "strings"
 "testing"
)

func TestCoreGraphStructuralFailClosed(t *testing.T){
 // These cases are rejected before reaching PostgreSQL. A syntactically
 // valid but unbound graph MUST NOT reach SELECT or enqueue.
 base:=func()coreGraphDefinition{
  return coreGraphDefinition{Nodes:[]coreGraphNode{
   {ID:"source",Type:"SOURCE"},
   {ID:"sync",Type:"SYNC",Requires:[]string{"source"}},
   {ID:"transform",Type:"TRANSFORM",Requires:[]string{"sync"}},
   {ID:"quality",Type:"QUALITY",Requires:[]string{"transform"}},
   {ID:"publish",Type:"PUBLISH",Requires:[]string{"quality"}},
  }}
 }
 cases:=[]struct{
  name string
  update func(*coreGraphDefinition)
  contains string
 }{
  {"empty",func(g *coreGraphDefinition){g.Nodes=nil},"graph requires"},
  {"duplicate",func(g *coreGraphDefinition){g.Nodes[1].ID="source"},"duplicate"},
  {"two_roots",func(g *coreGraphDefinition){g.Nodes[1].Requires=nil},"one SOURCE root"},
  {"fork",func(g *coreGraphDefinition){g.Nodes[3].Requires=[]string{"sync"}},"parallel"},
  {"multiple_parents",func(g *coreGraphDefinition){g.Nodes[3].Requires=[]string{"sync","transform"}},"fork/join"},
  {"cycle",func(g *coreGraphDefinition){
   g.Nodes=append(g.Nodes,
    coreGraphNode{ID:"isolated_a",Type:"TRANSFORM",Requires:[]string{"isolated_b"}},
    coreGraphNode{ID:"isolated_b",Type:"TRANSFORM",Requires:[]string{"isolated_a"}})
  },"disconnected graph or dependency cycle"},
  {"disconnected",func(g *coreGraphDefinition){g.Nodes[3].Requires=[]string{"does_not_exist"}},"unknown"},
  {"wrong_order",func(g *coreGraphDefinition){g.Nodes[2].Type="QUALITY"},"expected TRANSFORM"},
  {"invalid_name",func(g *coreGraphDefinition){g.Nodes[0].ID="bad node"},"invalid graph node"},
  {"huge_position",func(g *coreGraphDefinition){g.Nodes[0].X=10001},"canvas position"},
  {"unexpected_sql_binding",func(g *coreGraphDefinition){g.Nodes[4].ModelID="secret"},"unexpected model"},
  {"missing_required_binding",func(g *coreGraphDefinition){},"bindings required"},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   g:=base();tc.update(&g)
   var s server
   _,e:=s.validateCoreGraph(context.Background(),nil,g)
   if e==nil||!strings.Contains(e.Error(),tc.contains){t.Fatalf("want %s, got %v",tc.contains,e)}
  })
 }
}
