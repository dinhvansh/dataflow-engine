package main

import (
 "context"
 "io"
 "os"
 "log"
 "encoding/json"
 "errors"
  "net/http"
 "strings"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/connectors/clickhouse"
 "github.com/dinhvansh/dataflow-engine/internal/transform"
)

type transformInput struct {
 Model transform.Model `json:"model"`
 Limit int `json:"limit,omitempty"`
 StepIndex *int `json:"step_index,omitempty"`
}
type transformSaved struct {
 ID string `json:"id"`
 Name string `json:"name"`
 Model transform.Model `json:"model"`
 SQL string `json:"sql"`
 UpdatedAt time.Time `json:"updated_at"`
}
func decodeTransform(w http.ResponseWriter,r *http.Request)(transformInput,error){
 r.Body=http.MaxBytesReader(w,r.Body,65536)
 d:=json.NewDecoder(r.Body);d.DisallowUnknownFields()
 var v transformInput
 if err:=d.Decode(&v);err!=nil{return v,err}
 // One request only, no accepted trailing JSON objects.
 var extra any
 if err:=d.Decode(&extra);!errors.Is(err,io.EOF){return v,errors.New("trailing JSON not accepted")}
 return v,nil
}
func(s *server)transformCompile(w http.ResponseWriter,r *http.Request){
 if !s.validBearer(r){http.Error(w,"unauthorized",401);return}
 if !requireSQLDev(w){return}
 if r.Method!=http.MethodPost{http.Error(w,"method not allowed",405);return}
 input,err:=decodeTransform(w,r);if err!=nil{http.Error(w,"invalid transformation: "+err.Error(),400);return}
 result,err:=transform.Compile(input.Model,scratchDatabase())
 if err!=nil{http.Error(w,err.Error(),422);return}
 jsonResponse(w,200,result)
}
func(s *server)transformPreview(w http.ResponseWriter,r *http.Request){
 if !s.validBearer(r){http.Error(w,"unauthorized",401);return}
 if !requireSQLDev(w){return}
 if r.Method!=http.MethodPost{http.Error(w,"method not allowed",405);return}
 input,err:=decodeTransform(w,r);if err!=nil{http.Error(w,"invalid transformation: "+err.Error(),400);return}
 compiled,err:=transform.Compile(input.Model,scratchDatabase())
 if err!=nil{http.Error(w,err.Error(),422);return}
 query:=compiled.SQL
 if input.StepIndex!=nil{
  if *input.StepIndex<0||*input.StepIndex>=len(compiled.Steps){http.Error(w,"invalid step_index",422);return}
  query=compiled.Steps[*input.StepIndex]
 }
 limit:=input.Limit;if limit==0{limit=50}
 previewSQL,err:=transform.PreviewSQL(query,limit)
 if err!=nil{http.Error(w,err.Error(),422);return}
 ctx,cancel:=context.WithTimeout(r.Context(),35*time.Second);defer cancel()
 q:=clickhouse.QueryJob{Endpoint:s.chURL,Database:scratchDatabase(),Username:os.Getenv("DATAFLOW_CH_READONLY_USER"),Password:os.Getenv("DATAFLOW_CH_READONLY_PASSWORD")}
 result,err:=q.Execute(ctx,previewSQL)
 if err!=nil{
  // Do not echo the full upstream error: may contain internal connection info.
  log.Printf("transform preview failed: %v",err)
  http.Error(w,"preview query failed; check source, column names and SQL syntax",422);return
 }
 jsonResponse(w,200,map[string]any{"sql":previewSQL,"rows":result.Rows,"data":result.Data,"meta":result.Meta})
}
func(s *server)transformColumns(w http.ResponseWriter,r *http.Request){
 if !s.validBearer(r){http.Error(w,"unauthorized",401);return}
 if !requireSQLDev(w){return}
 if r.Method!=http.MethodPost{http.Error(w,"method not allowed",405);return}
 r.Body=http.MaxBytesReader(w,r.Body,4096)
 d:=json.NewDecoder(r.Body);d.DisallowUnknownFields()
 var source transform.Source
 if err:=d.Decode(&source);err!=nil{http.Error(w,"invalid source",400);return}
 compiled,err:=transform.Compile(transform.Model{Source:source},scratchDatabase())
 if err!=nil{http.Error(w,err.Error(),422);return}
 ctx,cancel:=context.WithTimeout(r.Context(),15*time.Second);defer cancel()
 q:=clickhouse.QueryJob{Endpoint:s.chURL,Database:scratchDatabase(),Username:os.Getenv("DATAFLOW_CH_READONLY_USER"),Password:os.Getenv("DATAFLOW_CH_READONLY_PASSWORD")}
 preview,err:=q.Execute(ctx,compiled.SQL+" LIMIT 0")
 if err!=nil{http.Error(w,"source metadata unavailable (check read-only user grants)",422);return}
 jsonResponse(w,200,map[string]any{"columns":preview.Meta})
}
func(s *server)transformModels(w http.ResponseWriter,r *http.Request){
 if !s.validBearer(r){http.Error(w,"unauthorized",401);return}
 if !requireSQLDev(w){return}
 ctx,cancel:=context.WithTimeout(r.Context(),10*time.Second);defer cancel()
 switch r.Method{
 case http.MethodGet:
  rows,err:=s.pg.Query(ctx,`SELECT id::text,name,definition,compiled_sql,updated_at FROM transform_models ORDER BY updated_at DESC LIMIT 100`)
  if err!=nil{http.Error(w,"transform model registry unavailable; apply migration 010",503);return}
  defer rows.Close()
  models:=make([]transformSaved,0)
  for rows.Next(){
   var m transformSaved;var data []byte
   if err=rows.Scan(&m.ID,&m.Name,&data,&m.SQL,&m.UpdatedAt);err!=nil{http.Error(w,"could not load transform models",500);return}
   if err=json.Unmarshal(data,&m.Model);err!=nil{http.Error(w,"corrupt saved model",500);return}
   models=append(models,m)
  }
  if rows.Err()!=nil{http.Error(w,"could not list models",500);return}
  jsonResponse(w,200,map[string]any{"models":models})
 case http.MethodPost:
  input,err:=decodeTransform(w,r)
  if err!=nil{http.Error(w,"invalid model JSON",400);return}
  name:=strings.TrimSpace(input.Model.Name)
  if len(name)<1||len(name)>120{http.Error(w,"name must be 1..120 chars",400);return}
  // Limit model names to safe technical IDs for future dbt export.
  if _,err=transform.ValidateModelName(name);err!=nil{http.Error(w,err.Error(),422);return}
  input.Model.Name=name
  compiled,err:=transform.Compile(input.Model,scratchDatabase())
  if err!=nil{http.Error(w,err.Error(),422);return}
  data,err:=json.Marshal(input.Model);if err!=nil{http.Error(w,"serialization failed",500);return}
  var result transformSaved
  result.Model=input.Model;result.SQL=compiled.SQL
  err=s.pg.QueryRow(ctx,`INSERT INTO transform_models(name,definition,compiled_sql) VALUES($1,$2::jsonb,$3)
   ON CONFLICT(name) DO UPDATE SET definition=EXCLUDED.definition,compiled_sql=EXCLUDED.compiled_sql,updated_at=now()
   RETURNING id::text,name,updated_at`,name,string(data),compiled.SQL).Scan(&result.ID,&result.Name,&result.UpdatedAt)
  if err!=nil{http.Error(w,"save failed (check migration 010)",500);return}
  jsonResponse(w,201,result)
 default:http.Error(w,"method not allowed",405)
 }
}
// Transformation queries are compile/preview/save only. No materialization
// or dbt execution is enabled in this first stability-gated UI release.

