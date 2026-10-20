package main

import (
 "testing"
 "time"
 "github.com/google/uuid"
)

func validSyncFixture()labStreamInput{
 return labStreamInput{
  SourceConnectionID:uuid.NewString(),TargetConnectionID:uuid.NewString(),
  SourceSchema:"Erp",SourceTable:"Part",Mode:"incremental_rowversion",
  SelectedColumns:[]string{"SysRowID","SysRevID"},TargetDatabase:"scratch",
  TargetTable:"dataflow_raw_events",BatchSize:1000,Workers:2,QueueCapacity:4,
 }
}
func TestLabSyncSetupInputGuard(t *testing.T){
 good:=validSyncFixture()
 if !validLabStreamInput(good){t.Fatal("valid isolated rowversion stream rejected")}
 cases:=[]func(*labStreamInput){
  func(x *labStreamInput){x.Mode="full"},
  func(x *labStreamInput){x.TargetDatabase="production"},
  func(x *labStreamInput){x.TargetTable="parttran"},
  func(x *labStreamInput){x.BatchSize=0},
  func(x *labStreamInput){x.Workers=33},
  func(x *labStreamInput){x.QueueCapacity=0},
  func(x *labStreamInput){x.SourceConnectionID=x.TargetConnectionID},
  func(x *labStreamInput){x.SourceConnectionID="not-a-uuid"},
  func(x *labStreamInput){x.SourceTable=""},
 }
 for i,mutate:=range cases{
  x:=good;mutate(&x)
  if validLabStreamInput(x){t.Fatalf("unsafe case %d accepted",i)}
 }
}
func TestLabVerifiedProfilesFailClosed(t *testing.T){
 t.Setenv("DATAFLOW_QA_SQLSERVER_ALLOWED_ENDPOINT","127.0.0.1:1433")
 now:=time.Now().UTC()
 p:=runtimeConnection{
  Driver:"sqlserver",Host:"127.0.0.1",Port:1433,
  Database:"DataFlowFixtureQA",Status:"verified",CheckedAt:&now,
 }
 if !verifiedLabProfile(p,"sqlserver"){t.Fatal("valid QA verified profile rejected")}
 old:=time.Now().Add(-31*time.Minute)
 p.CheckedAt=&old
 if verifiedLabProfile(p,"sqlserver"){t.Fatal("stale test accepted")}
 p.CheckedAt=&now;p.Status="draft"
 if verifiedLabProfile(p,"sqlserver"){t.Fatal("draft profile accepted")}
 p.Status="verified";p.Host="another-server"
 if verifiedLabProfile(p,"sqlserver"){t.Fatal("unapproved endpoint accepted")}
}
