package lineage

import("errors";"fmt")

// PlanChain returns a deterministic upstream-first View-only execution plan.
// V1.2 intentionally supports one visual source per model; raw SQL dependencies
// and fan-in must not be guessed from arbitrary SQL.
func PlanChain(models []Model,target string)([]Model,error){
 if len(models)>500{return nil,errors.New("model catalog too large")}
 byName:=map[string]Model{}
 for _,m:=range models{
  if _,ok:=byName[m.Name];ok{return nil,fmt.Errorf("duplicate model %q",m.Name)}
  byName[m.Name]=m
 }
 graph,err:=Build(models)
 if err!=nil{return nil,err}
 if err:=CheckCycle(graph);err!=nil{return nil,err}
 var result []Model
 visited:=map[string]bool{}
 var visit func(string)error
 visit=func(name string)error{
  if visited[name]{return fmt.Errorf("dependency cycle detected at %q",name)}
  m,exists:=byName[name]
  if !exists{return fmt.Errorf("missing upstream model %q",name)}
  if m.Mode!="visual"{return fmt.Errorf("model %q must use Visual mode",name)}
  if m.Database!="scratch"||m.Table==""{return fmt.Errorf("model %q must read a scratch source",name)}
  if len(result)>=30{return errors.New("auto build exceeds 30 steps")}
  visited[name]=true
  if parent:=DependsOn(m);parent!=""{
   if parent==name{return errors.New("model cannot depend on itself")}
   if err:=visit(parent);err!=nil{return err}
  }
  visited[name]=false
  result=append(result,m)
  return nil
 }
 if err:=visit(target);err!=nil{return nil,err}
 return result,nil
}
