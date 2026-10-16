package main

import (
 "encoding/json"
 "net/http"
 "github.com/dinhvansh/dataflow-engine/internal/jobs/dag"
)
func(s *server)pipelineAPI(w http.ResponseWriter,r *http.Request){
 if r.Method!=http.MethodPost{http.Error(w,"POST required",http.StatusMethodNotAllowed);return}
 if !s.validBearer(r){http.Error(w,"unauthorized",http.StatusUnauthorized);return}
 r.Body=http.MaxBytesReader(w,r.Body,65536)
 dec:=json.NewDecoder(r.Body)
 dec.DisallowUnknownFields()
 var plan dag.Plan
 if err:=dec.Decode(&plan);err!=nil{http.Error(w,err.Error(),http.StatusBadRequest);return}
 roots,err:=plan.Ready(nil)
 if err!=nil{w.Header().Set("Content-Type","application/json");w.WriteHeader(http.StatusUnprocessableEntity)
  _=json.NewEncoder(w).Encode(map[string]any{"valid":false,"error":err.Error()});return
 }
 w.Header().Set("Content-Type","application/json")
 _=json.NewEncoder(w).Encode(map[string]any{"valid":true,"nodes":len(plan.Nodes),"ready_on_start":roots,"status":"definition_only"})
}
