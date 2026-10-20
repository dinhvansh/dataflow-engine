package main

import(
 "testing"

 "github.com/dinhvansh/dataflow-engine/internal/connectors/clickhouse"
 "github.com/google/uuid"
)

func TestQABatchModesAndMappings(t *testing.T){
 base:=labStreamInput{
  SourceSchema:"Erp",SourceTable:"PartTran",
  TargetDatabase:"scratch",TargetTable:"parttran",
  SelectedColumns:[]string{"SysRowID","SysRevID"},
  Mode:"incremental_rowversion",BatchSize:1000,Workers:2,QueueCapacity:4,
 }
 if !validBatchStream(base){t.Fatal("valid incremental QA config refused")}
 base.Mode="full_refresh"
 if !validBatchStream(base){t.Fatal("full-refresh QA config refused")}
 id:=uuid.NewString()
 name,err:=safeQADestination(base.TargetTable,id)
 if err!=nil||!clickhouse.ValidQARawTarget("scratch",name){
  t.Fatalf("invalid physical owned destination: %q %v",name,err)
 }
 for _,mutate:=range []func(*labStreamInput){
  func(x *labStreamInput){x.Mode="truncate"},
  func(x *labStreamInput){x.TargetDatabase="production"},
  func(x *labStreamInput){x.TargetTable="parttran; DROP TABLE scratch.events"},
  func(x *labStreamInput){x.TargetTable="../outside"},
  func(x *labStreamInput){x.SelectedColumns=nil},
  func(x *labStreamInput){x.Workers=33},
  func(x *labStreamInput){x.BatchSize=0},
 }{
  value:=base;mutate(&value)
  if validBatchStream(value){t.Fatalf("unsafe config accepted: %+v",value)}
 }
 if _,err=safeQADestination("parttran","bad-id");err==nil{
  t.Fatal("invalid stream UUID accepted")
 }
}
