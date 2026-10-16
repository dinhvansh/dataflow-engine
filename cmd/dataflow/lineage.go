package main

import (
 "context"
 "encoding/json"
 "errors"
 "net/http"
 "time"
 "github.com/dinhvansh/dataflow-engine/internal/lineage"
 "github.com/dinhvansh/dataflow-engine/internal/transform"
 "github.com/jackc/pgx/v5"
)

func(s *server)readLineageModels(ctx context.Context,tx pgx.Tx)([]lineage.Model,error){
 rows,err:=tx.Query(ctx,"SELECT id::text,name,definition FROM transform_models ORDER BY name LIMIT 501")
 if err!=nil{return nil,err}
 defer rows.Close()
 models:=make([]lineage.Model,0)
 for rows.Next(){
  var m lineage.Model
  var raw []byte
  if err=rows.Scan(&m.ID,&m.Name,&raw);err!=nil{return nil,err}
  var v transform.Model
  if err=json.Unmarshal(raw,&v);err!=nil{return nil,err}
  models=append(models,lineage.Model{ID:m.ID,Name:m.Name,Mode:v.Mode,Database:v.Source.Database,Table:v.Source.Table})
 }
 if err=rows.Err();err!=nil{return nil,err}
 if len(models)>500{return nil,errors.New("model catalog exceeds 500 entries")}
 return models,nil
}
func(s *server)lineageAPI(w http.ResponseWriter,r *http.Request){
 if !s.validBearer(r){http.Error(w,"unauthorized",401);return}
 if !requireSQLDev(w){return}
 if r.Method!=http.MethodGet{http.Error(w,"method not allowed",405);return}
 ctx,cancel:=context.WithTimeout(r.Context(),10*time.Second);defer cancel()
 tx,err:=s.pg.BeginTx(ctx,pgx.TxOptions{AccessMode:pgx.ReadOnly})
 if err!=nil{http.Error(w,"lineage database unavailable",503);return}
 defer tx.Rollback(ctx)
 models,err:=s.readLineageModels(ctx,tx)
 if err!=nil{http.Error(w,"lineage registry unavailable",503);return}
 graph,err:=lineage.Build(models)
 if err!=nil{http.Error(w,"lineage graph exceeds limits",422);return}
 impactName:=r.URL.Query().Get("impact")
 if impactName!=""{
  found:=false
  for _,m:=range models{if m.Name==impactName{found=true;break}}
  if !found{http.Error(w,"model not found",404);return}
 }
 // No dbt CLI or SQL Server calls: lineage is metadata-only and fast.
 response:=map[string]any{"nodes":graph.Nodes,"edges":graph.Edges,"model_order":graph.Order,"problems":graph.Problems}
 if impactName!=""{response["impact"]=lineage.Impact(graph,impactName)}
 jsonResponse(w,200,response)
}
