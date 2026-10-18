package main

import (
 "testing"
 "time"
)
func TestManagedWriteHTTPTimeoutQAOnly(t *testing.T){
 t.Setenv("DATAFLOW_QA_MANAGED_WRITE_TIMEOUT_SECONDS","")
 got,err:=managedWriteHTTPClient()
 if err!=nil||got!=nil{t.Fatalf("default must retain Writer timeout: %v %#v",err,got)}
 t.Setenv("DATAFLOW_QA_MANAGED_WRITE_TIMEOUT_SECONDS","15")
 t.Setenv("DATAFLOW_ALLOW_TEST_LOAD","false")
 t.Setenv("DATAFLOW_TEST_TARGET_DATABASE","scratch")
 if _,err=managedWriteHTTPClient();err==nil{t.Fatal("non-QA override must refuse")}
 t.Setenv("DATAFLOW_ALLOW_TEST_LOAD","true")
 t.Setenv("DATAFLOW_TEST_TARGET_DATABASE","elsewhere")
 if _,err=managedWriteHTTPClient();err==nil{t.Fatal("non-scratch override must refuse")}
 t.Setenv("DATAFLOW_TEST_TARGET_DATABASE","scratch")
 got,err=managedWriteHTTPClient()
 if err!=nil||got==nil||got.Timeout!=15*time.Second{t.Fatalf("QA 15-second client: %v %#v",err,got)}
 for _,bad:=range []string{"0","4","301","abc","-1"}{
  t.Setenv("DATAFLOW_QA_MANAGED_WRITE_TIMEOUT_SECONDS",bad)
  if _,e:=managedWriteHTTPClient();e==nil{t.Fatalf("invalid timeout %q accepted",bad)}
 }
}
