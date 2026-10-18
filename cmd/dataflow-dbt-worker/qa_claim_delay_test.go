package main

import ("testing";"time")
func TestQAClaimDelayGuards(t *testing.T){
 t.Setenv("DATAFLOW_QA_DBT_CLAIM_HOLD_SECONDS","")
 d,e:=qaClaimDelay();if e!=nil||d!=0{t.Fatalf("default %v %v",d,e)}
 t.Setenv("DATAFLOW_QA_DBT_CLAIM_HOLD_SECONDS","12")
 t.Setenv("DATAFLOW_ALLOW_TEST_LOAD","false")
 t.Setenv("DATAFLOW_ENABLE_DBT_TEST","true")
 t.Setenv("DATAFLOW_TEST_TARGET_DATABASE","scratch")
 if _,e=qaClaimDelay();e==nil{t.Fatal("non-qa hold allowed")}
 t.Setenv("DATAFLOW_ALLOW_TEST_LOAD","true")
 t.Setenv("DATAFLOW_TEST_TARGET_DATABASE","other")
 if _,e=qaClaimDelay();e==nil{t.Fatal("non-scratch hold allowed")}
 t.Setenv("DATAFLOW_TEST_TARGET_DATABASE","scratch")
 d,e=qaClaimDelay();if e!=nil||d!=12*time.Second{t.Fatalf("hold %v %v",d,e)}
 for _,v:=range []string{"-1","0","31","abc"}{
  t.Setenv("DATAFLOW_QA_DBT_CLAIM_HOLD_SECONDS",v)
  if _,e=qaClaimDelay();e==nil{t.Fatalf("accepted invalid delay %s",v)}
 }
}
