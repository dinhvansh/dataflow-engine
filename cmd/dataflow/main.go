package main

import (
 "context"
 "database/sql"
 "encoding/json"
 "errors"
 "fmt"
 "log"
 "net/http"
 "os"
 "strconv"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/connectors/clickhouse"
 "github.com/dinhvansh/dataflow-engine/internal/connectors/sqlserver"
 "github.com/dinhvansh/dataflow-engine/internal/engine"
 "github.com/jackc/pgx/v5/pgxpool"
)
type RunRequest struct {
 SourceSchema string `json:"source_schema"`
 SourceTable string `json:"source_table"`
 TargetDatabase string `json:"target_database"`
 TargetTable string `json:"target_table"`
 BatchSize int `json:"batch_size"`
 Workers int `json:"workers"`
 QueueCapacity int `json:"queue_capacity"`
}
type server struct {pg *pgxpool.Pool; sql *sql.DB; chURL,chUser,chPass,token string}
func (s *server) run(w http.ResponseWriter,r *http.Request){
 if r.Method!=http.MethodPost{http.Error(w,"POST required",405);return}
 if !s.validBearer(r){http.Error(w,"unauthorized",401);return}
 r.Body=http.MaxBytesReader(w,r.Body,16384)
 var req RunRequest
 if err:=json.NewDecoder(r.Body).Decode(&req);err!=nil{http.Error(w,err.Error(),400);return}
 if req.BatchSize<100||req.BatchSize>100000||req.Workers<1||req.Workers>32||req.QueueCapacity<1||req.QueueCapacity>100{
  http.Error(w,"invalid runtime settings",400);return
 }
 // This bootstrap only supports intentionally isolated TEST targets.
 if os.Getenv("DATAFLOW_ALLOW_TEST_LOAD")!="true"{http.Error(w,"test load disabled",403);return}
 ctx:=r.Context()
 var id string
 err:=s.pg.QueryRow(ctx,`INSERT INTO bootstrap_runs(status,source_table,target_table)
 VALUES ('running',$1,$2) RETURNING id`,req.SourceSchema+"."+req.SourceTable,req.TargetDatabase+"."+req.TargetTable).Scan(&id)
 if err!=nil{http.Error(w,"cannot create run",500);return}
 started:=time.Now()
 rd:=&sqlserver.Reader{DB:s.sql,Schema:req.SourceSchema,Table:req.SourceTable,BatchSize:req.BatchSize}
 wr:=&clickhouse.Writer{Endpoint:s.chURL,Database:req.TargetDatabase,Table:req.TargetTable,Username:s.chUser,Password:s.chPass}
 stats,runErr:=engine.Run(ctx,engine.Config{Workers:req.Workers,QueueCapacity:req.QueueCapacity},rd,wr)
 status:="succeeded";message:=""
 if runErr!=nil{status="failed";message=runErr.Error()}
 persistCtx,cancel:=context.WithTimeout(context.Background(),5*time.Second);defer cancel()
 if _,err=s.pg.Exec(persistCtx,`UPDATE bootstrap_runs SET status=$2,rows_written=$3,batches_written=$4,error_message=$5,finished_at=now() WHERE id=$1`,id,status,stats.Rows,stats.Batches,message);err!=nil{log.Printf("run=%s persist: %v",id,err)}
 w.Header().Set("Content-Type","application/json")
 if runErr!=nil{w.WriteHeader(502)}
 _=json.NewEncoder(w).Encode(map[string]any{"run_id":id,"status":status,"rows_written":stats.Rows,"batches_written":stats.Batches,"elapsed_seconds":time.Since(started).Seconds(),"error":message})
}
func(s *server) status(w http.ResponseWriter,r *http.Request){
 if !s.validBearer(r){http.Error(w,"unauthorized",401);return}
 id:=r.URL.Query().Get("id")
 var status,source,target string;var rows,batches int64;var msg *string
 err:=s.pg.QueryRow(r.Context(),`SELECT status,source_table,target_table,rows_written,batches_written,error_message FROM bootstrap_runs WHERE id::text=$1`,id).Scan(&status,&source,&target,&rows,&batches,&msg)
 if err!=nil{http.Error(w,"run not found",404);return}
 w.Header().Set("Content-Type","application/json")
 _=json.NewEncoder(w).Encode(map[string]any{"run_id":id,"status":status,"source":source,"target":target,"rows_written":rows,"batches_written":batches,"error":msg})
}
func main(){
 required:=[]string{"DATAFLOW_POSTGRES_DSN","DATAFLOW_SQLSERVER_DSN","DATAFLOW_CH_URL","DATAFLOW_API_TOKEN"}
 for _,k:=range required{if os.Getenv(k)==""{log.Fatal(errors.New("missing "+k))}}
 ctx,cancel:=context.WithTimeout(context.Background(),15*time.Second);defer cancel()
 pg,err:=pgxpool.New(ctx,os.Getenv("DATAFLOW_POSTGRES_DSN"));if err!=nil{log.Fatal(err)};defer pg.Close()
 if err=pg.Ping(ctx);err!=nil{log.Fatal(err)}
 db,err:=sql.Open("sqlserver",os.Getenv("DATAFLOW_SQLSERVER_DSN"));if err!=nil{log.Fatal(err)};defer db.Close()
 db.SetMaxOpenConns(4);db.SetMaxIdleConns(2)
 if err=db.PingContext(ctx);err!=nil{log.Fatal(err)}
 svc:=&server{pg:pg,sql:db,chURL:os.Getenv("DATAFLOW_CH_URL"),chUser:os.Getenv("DATAFLOW_CH_USER"),chPass:os.Getenv("DATAFLOW_CH_PASSWORD"),token:os.Getenv("DATAFLOW_API_TOKEN")}
 mux:=http.NewServeMux()
 mux.HandleFunc("/healthz",func(w http.ResponseWriter,r *http.Request){_,_=w.Write([]byte("ok"))})
 mux.HandleFunc("/api/v1/bootstrap/runs",svc.run)
 mux.HandleFunc("/api/v1/bootstrap/status",svc.status)
 mux.HandleFunc("/api/v1/source/tables",svc.sourceAPI)
 mux.HandleFunc("/api/v1/source/columns",svc.sourceAPI)
 mux.HandleFunc("/api/v1/source/rowversion-upper",svc.sourceAPI)
 port:=8080;if p:=os.Getenv("PORT");p!=""{if v,e:=strconv.Atoi(p);e==nil&&v>0&&v<65536{port=v}}
 srv:=&http.Server{Addr:fmt.Sprintf(":%d",port),Handler:mux,ReadHeaderTimeout:5*time.Second}
 log.Printf("DataFlow bootstrap listening on :%d",port)
 log.Fatal(srv.ListenAndServe())
}
