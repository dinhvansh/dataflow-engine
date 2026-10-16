package executor

import(
 "context"
 "errors"
 "sync"
 "sync/atomic"
 "testing"
 "time"
 "github.com/dinhvansh/dataflow-engine/internal/jobs/dag"
)
func graph()dag.Plan{return dag.Plan{Nodes:[]dag.Node{
 {ID:"extract_a",JobID:"a"},
 {ID:"extract_b",JobID:"b"},
 {ID:"transform",JobID:"c",Requires:[]string{"extract_a","extract_b"}},
 {ID:"quality",JobID:"d",Requires:[]string{"transform"}},
}}}
func settings()Options{return Options{Concurrency:2,Timeout:time.Second,Retries:0}}
func TestForkJoinAndExecute(t *testing.T){
 var mu sync.Mutex;complete:=map[string]bool{}
 result,err:=Run(context.Background(),graph(),func(_ context.Context,job string)error{
  mu.Lock();defer mu.Unlock()
  if job=="c"&&(!complete["a"]||!complete["b"]){return errors.New("join launched early")}
  if job=="d"&&!complete["c"]{return errors.New("quality launched early")}
  complete[job]=true;return nil
 },settings())
 if err!=nil{t.Fatal(err)}
 if len(complete)!=4||result.State["quality"]!=dag.Succeeded{t.Fatalf("%+v",result)}
}
func TestRetries(t *testing.T){
 var count atomic.Int32
 plan:=dag.Plan{Nodes:[]dag.Node{{ID:"one",JobID:"retry"}}}
 opts:=settings();opts.Retries=2
 res,err:=Run(context.Background(),plan,func(ctx context.Context,job string)error{
  if count.Add(1)<3{return errors.New("temporary error")}
  return nil
 },opts)
 if err!=nil||res.Attempts["one"]!=3{t.Fatalf("res=%+v err=%v",res,err)}
}
func TestFailuresSkipDependents(t *testing.T){
 invoked:=map[string]bool{};var mu sync.Mutex
 res,err:=Run(context.Background(),graph(),func(_ context.Context,job string)error{
  mu.Lock();invoked[job]=true;mu.Unlock()
  if job=="a"{return errors.New("failure")}
  return nil
 },settings())
 if err==nil{t.Fatal("expected pipeline error")}
 if res.State["transform"]!=dag.Skipped||res.State["quality"]!=dag.Skipped{t.Fatalf("%+v",res.State)}
 if invoked["c"]||invoked["d"]{t.Fatal("skipped tasks executed")}
}
func TestCancelled(t *testing.T){
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 res,err:=Run(ctx,graph(),func(context.Context,string)error{return nil},settings())
 if !errors.Is(err,context.Canceled){t.Fatalf("got %v",err)}
 if res.State["transform"]!=dag.Cancelled{t.Fatal(res.State)}
}
func TestBadOptions(t *testing.T){
 if _,err:=Run(context.Background(),graph(),func(context.Context,string)error{return nil},Options{Concurrency:0});err==nil{t.Fatal("invalid options accepted")}
}
