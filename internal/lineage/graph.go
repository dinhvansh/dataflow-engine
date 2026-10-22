// Package lineage provides deterministic, read-only data lineage from saved
// visual model sources. No arbitrary SQL parsing and no implicit execution.
package lineage

import (
 "errors"
 "fmt"
 "sort"
 "strings"
)

type Model struct {
 ID string `json:"id"`
 Name string `json:"name"`
 Mode string `json:"mode"`
 Database string `json:"database"`
 Table string `json:"table"`
}
type Node struct {
 ID string `json:"id"`
 Label string `json:"label"`
 Kind string `json:"kind"` // source, model, or missing
 Status string `json:"status"`
 Target string `json:"target,omitempty"`
}
type Edge struct {
 From string `json:"from"`
 To string `json:"to"`
}
type Problem struct {
 Model string `json:"model"`
 Code string `json:"code"`
 Detail string `json:"detail"`
}
type Graph struct {
 Nodes []Node `json:"nodes"`
 Edges []Edge `json:"edges"`
 Order []string `json:"model_order"`
 Problems []Problem `json:"problems"`
}
func DependsOn(m Model)string{
 if m.Mode=="sql"||m.Database!="scratch"{return ""}
 if strings.HasPrefix(m.Table,"dfv_")&&len(m.Table)>4{return strings.TrimPrefix(m.Table,"dfv_")}
 return ""
}
func Build(models []Model)(Graph,error){
 // Keep the JSON contract stable for empty collections. The React lineage
 // screen iterates every field and `null` is not a valid graph collection.
 g:=Graph{Nodes:[]Node{},Edges:[]Edge{},Order:[]string{},Problems:[]Problem{}}
 if len(models)>500{return g,errors.New("too many models for lineage (max 500)")}
 byName:=make(map[string]Model,len(models))
 for _,m:=range models{
  if m.Name==""{return g,errors.New("model with empty name")}
  if _,ok:=byName[m.Name];ok{return g,fmt.Errorf("duplicate model name %s",m.Name)}
  byName[m.Name]=m
 }
 names:=make([]string,0,len(byName))
 for name:=range byName{names=append(names,name)}
 sort.Strings(names)
 dependencies:=map[string]string{}
 uniqueSources:=map[string]Node{}
 for _,name:=range names{
  m:=byName[name]
  node:=Node{ID:"model:"+name,Label:name,Kind:"model",Target:"scratch.dfv_"+name,Status:"ready"}
  if m.Mode=="sql"{
   node.Status="untracked_sql"
   g.Problems=append(g.Problems,Problem{Model:name,Code:"untracked_sql",Detail:"SQL editor dependencies cannot be inferred safely"})
  }else if parent:=DependsOn(m);parent!=""{
   dependencies[name]=parent
   if _,ok:=byName[parent];!ok{
    node.Status="missing_dependency"
    g.Problems=append(g.Problems,Problem{Model:name,Code:"missing_dependency",Detail:"Referenced model "+parent+" is not saved"})
    id:="missing:"+parent
    uniqueSources[id]=Node{ID:id,Label:parent,Kind:"missing",Status:"missing"}
    g.Edges=append(g.Edges,Edge{From:id,To:node.ID})
   }else{
    g.Edges=append(g.Edges,Edge{From:"model:"+parent,To:node.ID})
   }
  }else{
   if m.Database==""||m.Table==""{
    node.Status="missing_source"
    g.Problems=append(g.Problems,Problem{Model:name,Code:"missing_source",Detail:"No source configured"})
   }else{
    id:="source:"+m.Database+"."+m.Table
    uniqueSources[id]=Node{ID:id,Label:m.Database+"."+m.Table,Kind:"source",Status:"ready"}
    g.Edges=append(g.Edges,Edge{From:id,To:node.ID})
   }
  }
  g.Nodes=append(g.Nodes,node)
 }
 sourceIDs:=make([]string,0,len(uniqueSources))
 for id:=range uniqueSources{sourceIDs=append(sourceIDs,id)}
 sort.Strings(sourceIDs)
 for _,id:=range sourceIDs{g.Nodes=append(g.Nodes,uniqueSources[id])}
 sort.Slice(g.Edges,func(i,j int)bool{
  if g.Edges[i].From!=g.Edges[j].From{return g.Edges[i].From<g.Edges[j].From}
  return g.Edges[i].To<g.Edges[j].To
 })
 const unseen,visiting,done=0,1,2
 state:=make(map[string]int,len(names))
 cycle:=false
 var visit func(string)
 visit=func(name string){
  if state[name]==done{return}
  if state[name]==visiting{
   cycle=true
   g.Problems=append(g.Problems,Problem{Model:name,Code:"cycle",Detail:"Cycle detected in model dependencies"})
   return
  }
  state[name]=visiting
  if parent:=dependencies[name];parent!=""{
   if _,ok:=byName[parent];ok{visit(parent)}
  }
  state[name]=done
  g.Order=append(g.Order,name)
 }
 for _,name:=range names{if state[name]==unseen{visit(name)}}
 if cycle{g.Order=nil}
 return g,nil
}
func CheckCycle(g Graph)error{
 for _,problem:=range g.Problems{
  if problem.Code=="cycle"{return errors.New("dependency cycle: cannot save a model that creates a loop")}
 }
 return nil
}
func Impact(g Graph,modelName string)[]string{
 seen:=map[string]bool{}
 queue:=[]string{"model:"+modelName}
 for len(queue)>0{
  head:=queue[0];queue=queue[1:]
  for _,edge:=range g.Edges{
   if edge.From==head&&!seen[edge.To]&&strings.HasPrefix(edge.To,"model:"){
    seen[edge.To]=true
    queue=append(queue,edge.To)
   }
  }
 }
 delete(seen,"model:"+modelName)
 items:=make([]string,0,len(seen))
 for key:=range seen{items=append(items,strings.TrimPrefix(key,"model:"))}
 sort.Strings(items);return items
}
