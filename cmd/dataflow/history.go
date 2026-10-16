package main

import (
 "context"
 "database/sql"
 "encoding/json"
 "net/http"
 "strconv"
 "time"
)
type historyItem struct{
 ID string `json:"run_id"`
 Status string `json:"status"`
 Source string `json:"source"`
 Target string `json:"target"`
 RowsWritten int64 `json:"rows_written"`
 BatchesWritten int64 `json:"batches_written"`
 Error string `json:"error,omitempty"`
 CreatedAt time.Time `json:"created_at"`
 FinishedAt *time.Time `json:"finished_at,omitempty"`
}
func(s *server)history(w http.ResponseWriter,r *http.Request){
 if r.Method!=http.MethodGet{http.Error(w,"GET required",405);return}
 if !s.validBearer(r){http.Error(w,"unauthorized",401);return}
 limit:=20
 if raw:=r.URL.Query().Get("limit");raw!=""{
  n,err:=strconv.Atoi(raw);if err!=nil||n<1||n>100{http.Error(w,"limit must be 1..100",400);return};limit=n
 }
 ctx,cancel:=context.WithTimeout(r.Context(),10*time.Second);defer cancel()
 rows,err:=s.pg.Query(ctx,`SELECT id::text,status,source_table,target_table,rows_written,batches_written,COALESCE(error_message,''),created_at,finished_at
 FROM bootstrap_runs ORDER BY created_at DESC LIMIT $1`,limit)
 if err!=nil{http.Error(w,"cannot load runs",500);return}
 defer rows.Close()
 items:=make([]historyItem,0)
 for rows.Next(){
  var item historyItem;var finished sql.NullTime
  if err=rows.Scan(&item.ID,&item.Status,&item.Source,&item.Target,&item.RowsWritten,&item.BatchesWritten,&item.Error,&item.CreatedAt,&finished);err!=nil{http.Error(w,"cannot scan runs",500);return}
  if finished.Valid{item.FinishedAt=&finished.Time}
  items=append(items,item)
 }
 if err=rows.Err();err!=nil{http.Error(w,"cannot read runs",500);return}
 w.Header().Set("Content-Type","application/json")
 _=json.NewEncoder(w).Encode(map[string]any{"runs":items,"count":len(items)})
}
