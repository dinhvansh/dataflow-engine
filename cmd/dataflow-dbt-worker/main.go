package main
import(
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "log"
 "os"
 "os/signal"
 "strconv"
 "syscall"
 "time"
 "github.com/dinhvansh/dataflow-engine/internal/dbtworker"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)
type claim struct{ID,Owner,Name,SQL,Target string; Fence int64; Attempts int}
func configuration()(dbtworker.Config,error){
 port:=8123
 if p:=os.Getenv("DATAFLOW_DBT_CH_PORT");p!=""{v,e:=strconv.Atoi(p);if e!=nil{return dbtworker.Config{},e};port=v}
 config:=dbtworker.Config{Binary:os.Getenv("DATAFLOW_DBT_BIN"),Host:os.Getenv("DATAFLOW_DBT_CH_HOST"),Port:port,Username:os.Getenv("DATAFLOW_DBT_CH_USER"),Password:os.Getenv("DATAFLOW_DBT_CH_PASSWORD"),Database:"scratch",Secure:os.Getenv("DATAFLOW_DBT_CH_SECURE")=="true"}
 if config.Host==""||config.Username==""||config.Password==""{return config,errors.New("dedicated dbt ClickHouse credentials required")}
 return config,nil
}
func ensureSchema(ctx context.Context,pg *pgxpool.Pool)error{
 rows,err:=pg.Query(ctx,"SELECT lease_owner,lease_until,fence_token,source_definition FROM dbt_runs LIMIT 0")
 if err!=nil{return fmt.Errorf("dbt worker requires migration 011: %w",err)}
 rows.Close();return rows.Err()
}
func claimNext(ctx context.Context,pg *pgxpool.Pool)(claim,error){
 var c claim
 _,err:=pg.Exec(ctx,`UPDATE dbt_runs SET status='failed',error_message='dbt lease expired; replay limit reached',lease_owner=NULL,lease_until=NULL,finished_at=clock_timestamp() WHERE status='running' AND lease_until<clock_timestamp() AND attempt_count>=2`)
 if err!=nil{return c,err}
 err=pg.QueryRow(ctx,`WITH candidate AS (
 SELECT id FROM dbt_runs WHERE (status='queued' OR (status='running' AND lease_until<clock_timestamp())) AND attempt_count<2
 ORDER BY created_at,id LIMIT 1 FOR UPDATE SKIP LOCKED
 ) UPDATE dbt_runs r SET status='running',attempt_count=r.attempt_count+1,fence_token=r.fence_token+1,
 lease_owner=gen_random_uuid(),lease_until=clock_timestamp()+interval '10 minutes',started_at=COALESCE(r.started_at,clock_timestamp())
 FROM candidate c WHERE r.id=c.id RETURNING r.id::text,r.lease_owner::text,r.fence_token,r.model_name,r.compiled_sql,r.target_relation,r.attempt_count`).
 Scan(&c.ID,&c.Owner,&c.Fence,&c.Name,&c.SQL,&c.Target,&c.Attempts)
 return c,err
}
func finish(ctx context.Context,pg *pgxpool.Pool,c claim,result dbtworker.Outcome,runErr error)error{
 status:="succeeded";message:=""
 if runErr!=nil{status="failed";message=runErr.Error()}
 details:=result.Results;if len(details)==0{details=json.RawMessage("{}")}
 tag,err:=pg.Exec(ctx,`UPDATE dbt_runs SET status=$4,log_tail=$5,run_results=$6::jsonb,error_message=$7,
 finished_at=clock_timestamp(),lease_owner=NULL,lease_until=NULL WHERE id=$1::uuid AND lease_owner=$2::uuid
 AND fence_token=$3 AND lease_until>clock_timestamp() AND status='running'`,
 c.ID,c.Owner,c.Fence,status,result.Log,string(details),message)
 if err!=nil{return err}
 if tag.RowsAffected()!=1{return errors.New("dbt worker lost lease; result not persisted")}
 return nil
}
func executeClaim(ctx context.Context,pg *pgxpool.Pool,c claim,config dbtworker.Config){
 runCtx,cancel:=context.WithTimeout(ctx,5*time.Minute);defer cancel()
 result,err:=dbtworker.Execute(runCtx,dbtworker.Plan{Name:c.Name,Target:c.Target,SQL:c.SQL},config)
 if ctx.Err()!=nil{return}
 saveCtx,stop:=context.WithTimeout(context.Background(),10*time.Second);defer stop()
 if e:=finish(saveCtx,pg,c,result,err);e!=nil{log.Printf("dbt run %s could not finalize: %v",c.ID,e)}
}
func main(){
 if os.Getenv("DATAFLOW_ALLOW_TEST_LOAD")!="true"||os.Getenv("DATAFLOW_ENABLE_DBT_TEST")!="true"||os.Getenv("DATAFLOW_TEST_TARGET_DATABASE")!="scratch"{log.Fatal("dbt worker disabled outside isolated scratch test mode")}
 config,err:=configuration();if err!=nil{log.Fatal(err)}
 dsn:=os.Getenv("DATAFLOW_POSTGRES_DSN");if dsn==""{log.Fatal("missing PostgreSQL DSN")}
 ctx,stop:=signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM);defer stop()
 pg,err:=pgxpool.New(ctx,dsn);if err!=nil{log.Fatal(err)}
 defer pg.Close()
 check,done:=context.WithTimeout(ctx,10*time.Second)
 err=pg.Ping(check);if err==nil{err=ensureSchema(check,pg)};done()
 if err!=nil{log.Fatal(err)}
 if tableEnabled(){
   chk,done:=context.WithTimeout(ctx,10*time.Second)
   err=checkTableSchema(chk,pg);done()
   if err!=nil{log.Fatal(err)}
 }
 ticker:=time.NewTicker(3*time.Second);defer ticker.Stop()
 log.Print("dbt scratch view worker ready")
 for{
  select{case <-ctx.Done():return;default:}
  if tableEnabled(){dispatchTable(ctx,pg,config)}
  claimCtx,cancel:=context.WithTimeout(ctx,10*time.Second)
  work,e:=claimNext(claimCtx,pg);cancel()
  if e==nil{executeClaim(ctx,pg,work,config);continue}
  if !errors.Is(e,pgx.ErrNoRows)&&ctx.Err()==nil{log.Printf("dbt claim: %v",e)}
  select{case <-ctx.Done():return;case <-ticker.C:}
 }
}
