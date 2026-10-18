package mixedpipeline

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/dinhvansh/dataflow-engine/internal/jobs/dag"
)

func demoPlan()Plan{
	return Plan{Nodes:[]Node{
		{ID:"src",Kind:Source,Ref:"stored_source"},
		{ID:"sync",Kind:Sync,Ref:"stored_sync",Requires:[]string{"src"}},
		{ID:"transform",Kind:Transform,Ref:"stored_transform",Requires:[]string{"sync"}},
		{ID:"quality",Kind:Quality,Ref:"stored_quality",Requires:[]string{"transform"}},
		{ID:"publish",Kind:Publish,Ref:"stored_publish",Requires:[]string{"quality"}},
	}}
}

func TestMixedPipelineSyntheticEndToEnd(t *testing.T){
	p:=demoPlan()
	order:=make([]string,0,5)
	handlers:=map[Kind]Handler{}
	for _,typ:=range []Kind{Source,Sync,Transform,Quality,Publish}{
		handlers[typ]=func(_ context.Context,n Node)error{
			order=append(order,n.ID)
			// Mutate handler input; frozen Plan must remain intact.
			if len(n.Requires)>0{n.Requires[0]="malicious-modification"}
			return nil
		}
	}
	r,err:=Execute(context.Background(),p,handlers)
	if err!=nil{t.Fatal(err)}
	if !reflect.DeepEqual(order,[]string{"src","sync","transform","quality","publish"}) {
		t.Fatalf("wrong execution order: %v",order)
	}
	for _,v:=range r.Status{if v!=dag.Succeeded{t.Fatalf("not fully completed: %v",r.Status)}}
	if r.SnapshotSHA256=="" {t.Fatal("snapshot digest missing")}
	if p.Nodes[4].Requires[0]!="quality"{t.Fatal("handler mutated source plan")}
	r2,err:=Execute(context.Background(),p,handlers)
	if err!=nil||r2.SnapshotSHA256!=r.SnapshotSHA256{t.Fatal("snapshot changed during replay")}
}

func TestMixedPipelineQualityFailureBlocksPublish(t *testing.T){
	calls:=map[Kind]int{}
	h:=map[Kind]Handler{}
	for _,typ:=range []Kind{Source,Sync,Transform,Quality,Publish}{
		h[typ]=func(_ context.Context,_ Node)error{
			calls[typ]++
			if typ==Quality{return errors.New("SYNTHETIC_SECRET_BACKEND_ERROR must not leak")}
			return nil
		}
	}
	r,err:=Execute(context.Background(),demoPlan(),h)
	if !errors.Is(err,ErrIncomplete){t.Fatalf("quality failure not reported: %v",err)}
	if calls[Publish]!=0||r.Status["publish"]!=dag.Skipped||r.Status["quality"]!=dag.Failed {
		t.Fatalf("dangerous downstream publish: %+v calls=%+v",r.Status,calls)
	}
	if r.Failure["quality"]!="step execution failed"{t.Fatalf("raw backend error exposed: %+v",r.Failure)}
}

func TestMixedPipelineMissingAdapterNeverPublishes(t *testing.T){
	h:=map[Kind]Handler{
		Source:func(context.Context,Node)error{return nil},
		Sync:func(context.Context,Node)error{return nil},
		Transform:func(context.Context,Node)error{return nil},
		Quality:func(context.Context,Node)error{return nil},
	}
	r,err:=Execute(context.Background(),demoPlan(),h)
	if !errors.Is(err,ErrIncomplete)||r.Status["publish"]!=dag.Failed||
		r.Failure["publish"]!="adapter unavailable"{t.Fatalf("missing publish adapter incorrectly succeeded: %+v %v",r,err)}
}

func TestMixedPipelineRejectsUnsafeDAGs(t *testing.T){
	bad:=[]Plan{
		{},
		{Nodes:[]Node{{ID:"a",Kind:Source,Ref:"valid"}}},
	}
	for _,mutate:=range []func(*Plan){
		func(p *Plan){p.Nodes[0].ID=""},
		func(p *Plan){p.Nodes[0].Ref="SELECT * FROM ERPDB"},
		func(p *Plan){p.Nodes[0].Ref="https://malicious.example"},
		func(p *Plan){p.Nodes[1].Kind=Publish},
		func(p *Plan){p.Nodes[4].Requires=[]string{"transform"}},
		func(p *Plan){p.Nodes[4].Requires=[]string{"unknown"}},
		func(p *Plan){p.Nodes[3].Requires=[]string{"publish"}},
		func(p *Plan){p.Nodes[4].Requires=[]string{"quality","quality"}},
		func(p *Plan){p.Nodes=append(p.Nodes,p.Nodes[0])},
		func(p *Plan){p.Nodes=append(p.Nodes,Node{ID:"orphan",Kind:Source,Ref:"orphan"})},
		func(p *Plan){p.Nodes[1].Requires=nil},
		func(p *Plan){p.Nodes[0].Requires=[]string{"publish"}},
	}{
		p:=demoPlan()
		mutate(&p)
		bad=append(bad,p)
	}
	for i,p:=range bad{
		if err:=p.Validate();err==nil{t.Fatalf("invalid pipeline accepted #%d: %+v",i,p)}
	}
	p:=demoPlan()
	p.Nodes=append(p.Nodes[:4],Node{ID:"publish",Kind:Publish,Ref:"pub",Requires:[]string{"quality"}},Node{ID:"extra",Kind:Publish,Ref:"pub2",Requires:[]string{"quality"}})
	if p.Validate()==nil{t.Fatal("multiple uncoordinated publish stages accepted")}
}

func TestMixedPipelineCancelNoPublish(t *testing.T){
	ctx,cancel:=context.WithCancel(context.Background())
	defer cancel()
	var ranPublish bool
	h:=map[Kind]Handler{
		Source:func(context.Context,Node)error{cancel();return nil},
		Publish:func(context.Context,Node)error{ranPublish=true;return nil},
	}
	r,err:=Execute(ctx,demoPlan(),h)
	if !errors.Is(err,ErrIncomplete)||ranPublish||r.Status["publish"]!=dag.Cancelled {
		t.Fatalf("canceled pipeline published: %+v err=%v",r,err)
	}
}

func TestMixedPipelineBranchJoinAndIndependentFailure(t *testing.T){
	p:=Plan{Nodes:[]Node{
		{ID:"srcA",Kind:Source,Ref:"source_a"},
		{ID:"srcB",Kind:Source,Ref:"source_b"},
		{ID:"syncA",Kind:Sync,Ref:"sync_a",Requires:[]string{"srcA"}},
		{ID:"syncB",Kind:Sync,Ref:"sync_b",Requires:[]string{"srcB"}},
		{ID:"join",Kind:Transform,Ref:"model_c",Requires:[]string{"syncA","syncB"}},
		{ID:"quality",Kind:Quality,Ref:"quality_gate",Requires:[]string{"join"}},
		{ID:"publish",Kind:Publish,Ref:"controlled_publish",Requires:[]string{"quality"}},
	}}
	for _,fail:=range []string{"","syncB"}{
		var observed []string
		h:=map[Kind]Handler{}
		for _,typ:=range []Kind{Source,Sync,Transform,Quality,Publish}{
			h[typ]=func(_ context.Context,n Node)error{
				observed=append(observed,n.ID)
				if n.ID==fail{return errors.New("synthetic failure")}
				return nil
			}
		}
		r,err:=Execute(context.Background(),p,h)
		if fail==""{
			if err!=nil||r.Status["publish"]!=dag.Succeeded{t.Fatalf("healthy join failed: %v %+v",err,r)}
		}else{
			if !errors.Is(err,ErrIncomplete)||r.Status["join"]!=dag.Skipped||
				r.Status["quality"]!=dag.Skipped||r.Status["publish"]!=dag.Skipped{
				t.Fatalf("failed sync reached publish: %v %+v",err,r)
			}
			for _,id:=range observed{if id=="join"||id=="quality"||id=="publish"{t.Fatalf("downstream executed: %v",observed)}}
		}
	}
}
