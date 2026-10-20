package main

import (
 "context"
 "database/sql/driver"
 "errors"
 "fmt"
 "io"
 "net"
 "testing"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/connectors/clickhouse"
)

func TestCoreRun2TransientRetryClassification(t *testing.T){
 cases:=[]struct{
  err error
  retry bool
 }{
  {nil,false},
  {&clickhouse.WriteHTTPError{StatusCode:429},true},
  {fmt.Errorf("wrapped: %w",&clickhouse.WriteHTTPError{StatusCode:503}),true},
  {&clickhouse.WriteHTTPError{StatusCode:504},true},
  {&clickhouse.WriteHTTPError{StatusCode:401},false},
  {&clickhouse.WriteHTTPError{StatusCode:400},false},
  {&clickhouse.WriteHTTPError{StatusCode:409},false},
  {errors.New("invalid source schema"),false},
  {driver.ErrBadConn,true},
  {io.EOF,true},
  {io.ErrUnexpectedEOF,true},
  {context.DeadlineExceeded,true},
  {&net.DNSError{IsTimeout:true},true},
  {&net.DNSError{IsTemporary:true},true},
  {&net.DNSError{IsNotFound:true},true},
  {fmt.Errorf("wrapped service discovery: %w",&net.DNSError{IsNotFound:true}),true},
  {&net.DNSError{Err:"invalid DNS response"},false},
 }
 for i,tt:=range cases{
  if got:=retryableManagedFailure(tt.err);got!=tt.retry{
   t.Fatalf("case %d err=%v got retry=%t want=%t",i,tt.err,got,tt.retry)
  }
 }
 if managedMaxAttempts>3||managedMaxAttempts<2{
  t.Fatalf("bounded retry budget changed: %d",managedMaxAttempts)
 }
 if managedLeaseDuration<time.Minute{
  t.Fatal("unsafe lease duration")
 }
}
