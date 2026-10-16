package engine

import (
 "context"
 "errors"
 "fmt"
 "sync"
 "sync/atomic"
)

type Batch struct {
 Sequence uint64
 Rows []map[string]any
}
type Reader interface { Read(ctx context.Context, emit func(Batch) error) error }
type Writer interface { Write(ctx context.Context, batch Batch) error }
type Config struct { Workers, QueueCapacity int }
type Stats struct { Batches, Rows uint64 }

// Run provides bounded in-flight batches and context-aware cancellation.
// It guarantees all submitted batches have been acknowledged before success.
// It does not itself implement cross-database exactly-once/checkpointing.
func Run(parent context.Context, cfg Config, reader Reader, writer Writer) (Stats,error) {
 if cfg.Workers<1 || cfg.Workers>32 || cfg.QueueCapacity<1 || cfg.QueueCapacity>100 { return Stats{}, errors.New("workers must be 1..32 and queue capacity 1..100") }
 ctx,cancel:=context.WithCancel(parent); defer cancel()
 batches:=make(chan Batch,cfg.QueueCapacity)
 var wg sync.WaitGroup
 var errOnce sync.Once
 var firstErr error
 fail:=func(err error){if err!=nil {errOnce.Do(func(){firstErr=err;cancel()})}}
 var rows,done atomic.Uint64
 for i:=0;i<cfg.Workers;i++ {
  wg.Add(1)
  go func(){defer wg.Done();for {select{
   case <-ctx.Done(): return
   case b,ok:= <-batches:
    if !ok{return}
    if err:=writer.Write(ctx,b);err!=nil {fail(fmt.Errorf("batch %d: %w",b.Sequence,err));return}
    done.Add(1);rows.Add(uint64(len(b.Rows)))
  }}}()
 }
 readErr:=reader.Read(ctx,func(b Batch)error{
  if len(b.Rows)==0{return nil}
  select {case <-ctx.Done():return ctx.Err();case batches<-b:return nil}
 })
 if readErr!=nil {fail(readErr)}
 close(batches);wg.Wait()
 if firstErr!=nil{return Stats{Batches:done.Load(),Rows:rows.Load()},firstErr}
 if err:=parent.Err();err!=nil{return Stats{Batches:done.Load(),Rows:rows.Load()},err}
 return Stats{Batches:done.Load(),Rows:rows.Load()},nil
}
