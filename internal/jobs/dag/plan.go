// Package dag validates versioned pipeline dependency graphs and identifies
// nodes whose upstream prerequisites have all completed successfully.
package dag

import (
 "errors"
 "fmt"
 "sort"
)

type Status string
const(
 Pending Status="pending"
 Running Status="running"
 Succeeded Status="succeeded"
 Failed Status="failed"
 Skipped Status="skipped"
 Cancelled Status="cancelled"
)
type Node struct{
 ID string `json:"id"`
 JobID string `json:"job_id"`
 Requires []string `json:"requires"`
}
type Plan struct{
 Nodes []Node `json:"nodes"`
}
func(p Plan)Validate()error{
 if len(p.Nodes)==0{return errors.New("pipeline needs at least one node")}
 if len(p.Nodes)>500{return errors.New("pipeline exceeds 500 nodes")}
 nodes:=make(map[string]Node,len(p.Nodes))
 for _,n:=range p.Nodes{
  if n.ID==""||n.JobID==""{return errors.New("node id and job_id required")}
  if _,ok:=nodes[n.ID];ok{return fmt.Errorf("duplicate node %q",n.ID)}
  nodes[n.ID]=n
 }
 color:=make(map[string]uint8)
 var visit func(string)error
 visit=func(id string)error{
  if color[id]==1{return fmt.Errorf("dependency cycle at %q",id)}
  if color[id]==2{return nil}
  color[id]=1
  seen:=make(map[string]bool)
  for _,dep:=range nodes[id].Requires{
   if dep==id{return fmt.Errorf("self dependency at %q",id)}
   if seen[dep]{return fmt.Errorf("duplicate dependency %q at %q",dep,id)}
   seen[dep]=true
   if _,ok:=nodes[dep];!ok{return fmt.Errorf("unknown dependency %q at %q",dep,id)}
   if err:=visit(dep);err!=nil{return err}
  }
  color[id]=2
  return nil
 }
 for id:=range nodes{if err:=visit(id);err!=nil{return err}}
 return nil
}
// Ready returns deterministic node IDs ready for scheduling. A node is
// ready only when ALL required predecessors have succeeded.
func(p Plan)Ready(state map[string]Status)([]string,error){
 if err:=p.Validate();err!=nil{return nil,err}
 ready:=make([]string,0)
 for _,n:=range p.Nodes{
  if state[n.ID]!=""&&state[n.ID]!=Pending{continue}
  allowed:=true
  for _,dep:=range n.Requires{if state[dep]!=Succeeded{allowed=false;break}}
  if allowed{ready=append(ready,n.ID)}
 }
 sort.Strings(ready)
 return ready,nil
}
// Blocked lists pending nodes with failed, skipped or cancelled predecessors.
// The scheduler decides whether to mark such nodes skipped or fail the run.
func(p Plan)Blocked(state map[string]Status)([]string,error){
 if err:=p.Validate();err!=nil{return nil,err}
 out:=make([]string,0)
 for _,n:=range p.Nodes{
  if state[n.ID]!=""&&state[n.ID]!=Pending{continue}
  for _,dep:=range n.Requires{
   if state[dep]==Failed||state[dep]==Skipped||state[dep]==Cancelled{out=append(out,n.ID);break}
  }
 }
 sort.Strings(out)
 return out,nil
}
