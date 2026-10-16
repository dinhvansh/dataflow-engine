package engine

import (
 "context"
 "errors"
 "sync"
 "testing"
)
type testReader struct { n int }
func(r testReader)Read(ctx context.Context,emit func(Batch)error)error{
 for i:=0;i<r.n;i++ {if err:=emit(Batch{Sequence:uint64(i),Rows:[]map[string]any{{"id":i}}});err!=nil{return err}}
 return nil
}
type testWriter struct {mu sync.Mutex; seen map[uint64]bool; fail bool}
func(w *testWriter)Write(_ context.Context,b Batch)error{
 if w.fail{return errors.New("write failed")}
 w.mu.Lock();defer w.mu.Unlock()
 if w.seen[b.Sequence] {return errors.New("duplicate")}
 w.seen[b.Sequence]=true;return nil
}
func TestRun(t *testing.T){
 w:=&testWriter{seen:make(map[uint64]bool)}
 got,err:=Run(context.Background(),Config{Workers:4,QueueCapacity:2},testReader{n:1000},w)
 if err!=nil || got.Rows!=1000 || len(w.seen)!=1000 {t.Fatalf("stats=%+v err=%v seen=%d",got,err,len(w.seen))}
}
func TestWriterFailure(t *testing.T){
 w:=&testWriter{seen:make(map[uint64]bool),fail:true}
 _,err:=Run(context.Background(),Config{Workers:2,QueueCapacity:2},testReader{n:10},w)
 if err==nil{t.Fatal("expected error")}
}
func TestInvalidSettings(t *testing.T){
 _,err:=Run(context.Background(),Config{Workers:0,QueueCapacity:2},testReader{},&testWriter{})
 if err==nil{t.Fatal("expected error")}
}
