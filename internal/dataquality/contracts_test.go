package dataquality

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/dinhvansh/dataflow-engine/internal/jobs/dag"
	"github.com/dinhvansh/dataflow-engine/internal/mixedpipeline"
)

func ptrI(v int64)*int64{return &v}
func ptrF(v float64)*float64{return &v}

func sampleContract()Contract{
	return Contract{Checks:[]Check{
		{ID:"min_rows",Kind:MinRows,Limit:100},
		{ID:"required_customer",Kind:RequiredColumn,Column:"CustomerID"},
		{ID:"null_customer",Kind:MaxNullRatio,Column:"CustomerID",Limit:0.01},
		{ID:"unique_keys",Kind:MaxDuplicateKeys,Limit:0},
		{ID:"freshness",Kind:MaxAgeSeconds,Limit:3600},
		{ID:"reconciliation",Kind:MaxReconciliationDelta,Limit:0.02},
	}}
}
func sampleEvidence()Evidence{
	return Evidence{
		RowCount:ptrI(1000),
		Columns:map[string]bool{"CustomerID":true},
		NullByColumn:map[string]int64{"CustomerID":0},
		DuplicateKeys:ptrI(0),AgeSeconds:ptrI(60),
		ReconciliationDelta:ptrF(0.001),
	}
}
func TestDataQualityAllContractsPass(t *testing.T){
	r,err:=Evaluate(sampleContract(),sampleEvidence())
	if err!=nil||!r.Passed||len(r.Checks)!=6{t.Fatalf("good contract failed: %+v %v",r,err)}
	for _,c:=range r.Checks{if !c.Passed||c.Reason!=""{t.Fatal("good check falsely failed",c)}}
}
func TestDataQualityFailsClosedOnBadOrMissingEvidence(t *testing.T){
	bad:=[]func(*Evidence){
		func(e *Evidence){e.RowCount=ptrI(10)},
		func(e *Evidence){e.RowCount=nil},
		func(e *Evidence){e.Columns=nil},
		func(e *Evidence){e.NullByColumn=nil},
		func(e *Evidence){e.NullByColumn["CustomerID"]=1001},
		func(e *Evidence){e.NullByColumn["CustomerID"]=11},
		func(e *Evidence){e.DuplicateKeys=ptrI(1)},
		func(e *Evidence){e.DuplicateKeys=nil},
		func(e *Evidence){e.AgeSeconds=ptrI(4000)},
		func(e *Evidence){e.AgeSeconds=nil},
		func(e *Evidence){e.ReconciliationDelta=ptrF(0.5)},
		func(e *Evidence){e.ReconciliationDelta=ptrF(math.NaN())},
		func(e *Evidence){e.ReconciliationDelta=nil},
	}
	for i,mutate:=range bad{
		e:=sampleEvidence()
		mutate(&e)
		r,err:=Evaluate(sampleContract(),e)
		if !errors.Is(err,ErrBlocked)||r.Passed{t.Fatalf("invalid evidence #%d accepted: %+v %v",i,r,err)}
		for _,c:=range r.Checks{
			if c.Reason!=""&&c.Reason!="threshold_failed"&&c.Reason!="schema_mismatch"&&
				c.Reason!="stale_data"&&c.Reason!="reconciliation_failed"&&
				c.Reason!="evidence_unavailable"{
				t.Fatalf("leaky reason: %q",c.Reason)
			}
		}
	}
}
func TestDataQualityRejectsInvalidContracts(t *testing.T){
	bad:=[]Contract{
		{}, {Checks:[]Check{{ID:"same",Kind:MinRows,Limit:10},{ID:"same",Kind:MinRows,Limit:20}}},
		{Checks:[]Check{{ID:"bad",Kind:"EXECUTE_SQL",Limit:1}}},
		{Checks:[]Check{{ID:"x",Kind:MaxNullRatio,Column:"A",Limit:1.1}}},
		{Checks:[]Check{{ID:"x",Kind:MaxNullRatio,Column:"",Limit:0}}},
		{Checks:[]Check{{ID:"x",Kind:MaxReconciliationDelta,Limit:math.NaN()}}},
		{Checks:[]Check{{ID:"x",Kind:MaxDuplicateKeys,Limit:-1}}},
		{Checks:[]Check{{ID:"x",Kind:MinRows,Limit:0.5}}},
		{Checks:[]Check{{ID:"x",Kind:RequiredColumn,Column:"A",Limit:1}}},
	}
	for i,c:=range bad{
		if _,err:=Evaluate(c,sampleEvidence());!errors.Is(err,ErrInvalid){
			t.Fatalf("invalid contract #%d accepted: %v",i,err)
		}
	}
}

func TestDataQualityBlocksSyntheticPipelinePublish(t *testing.T){
	p:=mixedpipeline.Plan{Nodes:[]mixedpipeline.Node{
		{ID:"source",Kind:mixedpipeline.Source,Ref:"source_ref"},
		{ID:"sync",Kind:mixedpipeline.Sync,Ref:"sync_ref",Requires:[]string{"source"}},
		{ID:"transform",Kind:mixedpipeline.Transform,Ref:"transform_ref",Requires:[]string{"sync"}},
		{ID:"quality",Kind:mixedpipeline.Quality,Ref:"quality_ref",Requires:[]string{"transform"}},
		{ID:"publish",Kind:mixedpipeline.Publish,Ref:"publish_ref",Requires:[]string{"quality"}},
	}}
	var publishes int
	makeHandlers:=func(e Evidence)map[mixedpipeline.Kind]mixedpipeline.Handler{
		h:=map[mixedpipeline.Kind]mixedpipeline.Handler{}
		for _,kind:=range []mixedpipeline.Kind{mixedpipeline.Source,mixedpipeline.Sync,mixedpipeline.Transform}{
			h[kind]=func(context.Context,mixedpipeline.Node)error{return nil}
		}
		h[mixedpipeline.Quality]=func(context.Context,mixedpipeline.Node)error{
			_,err:=Evaluate(sampleContract(),e)
			return err
		}
		h[mixedpipeline.Publish]=func(context.Context,mixedpipeline.Node)error{publishes++;return nil}
		return h
	}
	bad:=sampleEvidence()
	bad.DuplicateKeys=ptrI(8)
	r,err:=mixedpipeline.Execute(context.Background(),p,makeHandlers(bad))
	if !errors.Is(err,mixedpipeline.ErrIncomplete)||publishes!=0||r.Status["quality"]!=dag.Failed||r.Status["publish"]!=dag.Skipped{
		t.Fatalf("DQ failure triggered publish: %+v %v",r,err)
	}
	good,err:=mixedpipeline.Execute(context.Background(),p,makeHandlers(sampleEvidence()))
	if err!=nil||publishes!=1||good.Status["publish"]!=dag.Succeeded{
		t.Fatalf("valid DQ did not publish in synthetic runner: %+v %v",good,err)
	}
}
