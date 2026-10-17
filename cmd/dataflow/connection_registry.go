package main

import (
 "context"
 "crypto/subtle"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "os"
 "regexp"
 "strings"
 "time"

 "github.com/jackc/pgx/v5/pgconn"
)

var definitionNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9 _.-]{0,79}$`)

type connectionDefinition struct {
 ID string `json:"id"`
 Name string `json:"name"`
 Driver string `json:"driver"`
 Kind string `json:"kind"`
 Workspace string `json:"workspace"`
 Status string `json:"status"`
 Revision int `json:"revision"`
 CreatedAt time.Time `json:"created_at"`
}
type definitionInput struct{
 Name string `json:"name"`
 Driver string `json:"driver"`
 Kind string `json:"kind"`
}
func validDefinitionInput(v definitionInput) bool {
 return definitionNamePattern.MatchString(v.Name) &&
  ((v.Driver == "sqlserver" && v.Kind == "source") ||
   (v.Driver == "clickhouse" && v.Kind == "destination"))
}
func connectionRegistryEnabled() bool {
 return os.Getenv("DATAFLOW_ENABLE_CONNECTION_REGISTRY") == "true"
}
func adminConnectionAllowed(r *http.Request) bool {
 admin := os.Getenv("DATAFLOW_CONNECTION_ADMIN_TOKEN")
 provided := r.Header.Get("X-DataFlow-Connection-Admin")
 return admin != "" && len(provided)==len(admin) &&
  subtle.ConstantTimeCompare([]byte(provided), []byte(admin))==1
}

// GET: approved metadata fields only. POST: opt-in admin-only *draft* records.
// No network dialing, credential storage, active binding or secret response.
func (s *server) connectionDefinitionsAPI(w http.ResponseWriter,r *http.Request){
 w.Header().Set("Cache-Control","no-store")
 w.Header().Set("X-Content-Type-Options","nosniff")
 if r.Method!=http.MethodGet && r.Method!=http.MethodPost {
  http.Error(w,"GET or POST required",http.StatusMethodNotAllowed);return
 }
 if !s.validBearer(r) {http.Error(w,"unauthorized",http.StatusUnauthorized);return}
 if !connectionRegistryEnabled(){
  if r.Method==http.MethodPost {http.Error(w,"connection draft registry disabled",http.StatusForbidden);return}
  w.Header().Set("Content-Type","application/json")
  _=json.NewEncoder(w).Encode(map[string]any{"definitions":[]connectionDefinition{},"write_enabled":false})
  return
 }
 if r.Method==http.MethodPost && (s.token==os.Getenv("DATAFLOW_CONNECTION_ADMIN_TOKEN") || !adminConnectionAllowed(r)){http.Error(w,"independent admin token required",http.StatusForbidden);return}
 if s.pg==nil{http.Error(w,"registry unavailable",http.StatusServiceUnavailable);return}
 ctx,cancel:=context.WithTimeout(r.Context(),10*time.Second)
 defer cancel()
 switch r.Method{
 case http.MethodGet:
  rows,err:=s.pg.Query(ctx,`SELECT id::text,name,driver,kind,workspace,status,revision,created_at
   FROM connection_definitions WHERE workspace='lab' ORDER BY created_at DESC,id DESC LIMIT 100`)
  if err!=nil{http.Error(w,"registry unavailable",500);return}
  defer rows.Close()
  items:=make([]connectionDefinition,0)
  for rows.Next(){
   var d connectionDefinition
   if err=rows.Scan(&d.ID,&d.Name,&d.Driver,&d.Kind,&d.Workspace,&d.Status,&d.Revision,&d.CreatedAt);err!=nil{
    http.Error(w,"cannot read registry",500);return
   }
   items=append(items,d)
  }
  if rows.Err()!=nil{http.Error(w,"cannot read registry",500);return}
  w.Header().Set("Content-Type","application/json")
  _=json.NewEncoder(w).Encode(map[string]any{"definitions":items,"write_enabled":s.token!=os.Getenv("DATAFLOW_CONNECTION_ADMIN_TOKEN") && adminConnectionAllowed(r)})
 case http.MethodPost:
  if !adminConnectionAllowed(r){http.Error(w,"admin permission required",http.StatusForbidden);return}
  r.Body=http.MaxBytesReader(w,r.Body,2048)
  dec:=json.NewDecoder(r.Body)
  dec.DisallowUnknownFields()
  var input definitionInput
  if dec.Decode(&input)!=nil || dec.Decode(new(any))!=io.EOF{
   http.Error(w,"invalid draft definition",400);return
  }
  input.Name=strings.TrimSpace(input.Name)
  if !validDefinitionInput(input){http.Error(w,"unsupported definition: name or driver/role invalid",422);return}
  tx,err:=s.pg.Begin(ctx)
  if err!=nil{http.Error(w,"registry unavailable",500);return}
  defer tx.Rollback(ctx)
  var d connectionDefinition
  err=tx.QueryRow(ctx,`INSERT INTO connection_definitions(name,driver,kind)
   VALUES($1,$2,$3)
   RETURNING id::text,name,driver,kind,workspace,status,revision,created_at`,
   input.Name,input.Driver,input.Kind).
   Scan(&d.ID,&d.Name,&d.Driver,&d.Kind,&d.Workspace,&d.Status,&d.Revision,&d.CreatedAt)
  if err!=nil{
   var pgErr *pgconn.PgError
   if errors.As(err,&pgErr) && pgErr.Code=="23505" {http.Error(w,"definition name already exists",409);return}
   http.Error(w,"could not create definition",500);return
  }
  if _,err=tx.Exec(ctx,`INSERT INTO connection_definition_audit(definition_id,action)
     VALUES($1::uuid,'created')`,d.ID);err!=nil{
   http.Error(w,"could not audit definition",500);return
  }
  if err=tx.Commit(ctx);err!=nil{http.Error(w,"could not create definition",500);return}
  w.Header().Set("Content-Type","application/json")
  w.WriteHeader(http.StatusCreated)
  _=json.NewEncoder(w).Encode(d)
 }
}

// Called at startup only when the feature has explicitly been enabled.
func (s *server) checkConnectionRegistrySchema(ctx context.Context) error {
 if !connectionRegistryEnabled(){return nil}
 if s.pg==nil{return errors.New("connection registry requires PostgreSQL")}
 var count int
 if err:=s.pg.QueryRow(ctx,`SELECT count(*) FROM information_schema.tables
  WHERE table_schema=current_schema() AND table_name IN ('connection_definitions','connection_definition_audit')`).Scan(&count);err!=nil{return err}
 if count!=2{return errors.New("connection registry migration 017 required")}
 return nil
}
