// Package mixedpipeline implements a disconnected, synthetic-only orchestration
// proof for heterogeneous ETL stages. No database/ERP/API network I/O exists.
package mixedpipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"

	"github.com/dinhvansh/dataflow-engine/internal/jobs/dag"
)

var ErrInvalid = errors.New("invalid mixed pipeline")
var ErrIncomplete = errors.New("mixed pipeline run incomplete")

type Kind string

const (
	Source Kind = "SOURCE"
	Sync Kind = "SYNC"
	Transform Kind = "TRANSFORM"
	Quality Kind = "QUALITY"
	Publish Kind = "PUBLISH"
)

type Node struct {
	ID string `json:"id"`
	Kind Kind `json:"kind"`
	// Ref is an opaque *stored-job identifier*, NOT a SQL statement, URL,
	// credential, hostname, connection string or arbitrary executable payload.
	Ref string `json:"ref"`
	Requires []string `json:"requires"`
}

type Plan struct {
	Nodes []Node `json:"nodes"`
}

var idPattern=regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
var refPattern=regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func validEdge(prev, next Kind)bool{
	switch next {
	case Source:
		return false
	case Sync:
		return prev==Source
	case Transform:
		return prev==Sync || prev==Transform
	case Quality:
		return prev==Transform
	case Publish:
		return prev==Quality
	}
	return false
}

func (p Plan) Validate()error {
	if len(p.Nodes)<5||len(p.Nodes)>100{return ErrInvalid}
	index:=make(map[string]Node,len(p.Nodes))
	publish:=0
	for _,n:=range p.Nodes{
		if !idPattern.MatchString(n.ID)||!refPattern.MatchString(n.Ref)||len(n.Requires)>32{return ErrInvalid}
		if _,exists:=index[n.ID];exists{return ErrInvalid}
		switch n.Kind{case Source,Sync,Transform,Quality,Publish:default:return ErrInvalid}
		if n.Kind==Source&&len(n.Requires)!=0{return ErrInvalid}
		if n.Kind!=Source&&len(n.Requires)==0{return ErrInvalid}
		if n.Kind==Publish{publish++}
		index[n.ID]=n
	}
	if publish!=1{return ErrInvalid}
	graph:=dag.Plan{Nodes:make([]dag.Node,0,len(p.Nodes))}
	for _,n:=range p.Nodes{
		for _,dep:=range n.Requires {
			prev,ok:=index[dep]
			if !ok||!validEdge(prev.Kind,n.Kind){return ErrInvalid}
		}
		graph.Nodes=append(graph.Nodes,dag.Node{ID:n.ID,JobID:n.Ref,Requires:n.Requires})
	}
	if graph.Validate()!=nil{return ErrInvalid}
	// Each declared step must actually lead to the final Publish node.
	// Reject dead-end nodes that could silently run outside the release gate.
	downstream:=map[string]bool{}
	var visit func(string)
	visit=func(id string){
		if downstream[id]{return}
		downstream[id]=true
		for _,dep:=range index[id].Requires{visit(dep)}
	}
	for _,n:=range p.Nodes{if n.Kind==Publish{visit(n.ID)}}
	if len(downstream)!=len(p.Nodes){return ErrInvalid}
	return nil
}

func (p Plan) freeze()(Plan,dag.Plan,string,error){
	if err:=p.Validate();err!=nil{return Plan{},dag.Plan{},"",err}
	out:=Plan{Nodes:make([]Node,len(p.Nodes))}
	graph:=dag.Plan{Nodes:make([]dag.Node,len(p.Nodes))}
	for i,n:=range p.Nodes{
		copyNode:=Node{ID:n.ID,Kind:n.Kind,Ref:n.Ref,Requires:append([]string(nil),n.Requires...)}
		out.Nodes[i]=copyNode
		graph.Nodes[i]=dag.Node{ID:n.ID,JobID:n.Ref,Requires:append([]string(nil),n.Requires...)}
	}
	encoded,_:=json.Marshal(out)
	hash:=sha256.Sum256(encoded)
	return out,graph,hex.EncodeToString(hash[:]),nil
}

type Handler func(ctx context.Context,node Node)error

type Report struct {
	SnapshotSHA256 string `json:"snapshot_sha256"`
	Status map[string]dag.Status `json:"status"`
	// Only sanitized failure markers: no raw database errors or secret strings.
	Failure map[string]string `json:"failure,omitempty"`
}

// Execute executes a *single-threaded synthetic-injected* plan. It is not the
// persistent production scheduler; it reuses existing DAG readiness/blocked
// semantics and leaves the real connectors completely unwired.
func Execute(ctx context.Context,p Plan,handlers map[Kind]Handler)(Report,error){
	frozen,graph,digest,err:=p.freeze()
	if err!=nil{return Report{},err}
	if ctx==nil{return Report{},ErrInvalid}
	states:=make(map[string]dag.Status,len(frozen.Nodes))
	failure:=map[string]string{}
	nodes:=make(map[string]Node,len(frozen.Nodes))
	for _,n:=range frozen.Nodes{states[n.ID]=dag.Pending;nodes[n.ID]=n}
	report:=Report{SnapshotSHA256:digest,Status:states,Failure:failure}
	for completed:=0;completed<len(frozen.Nodes);{
		if ctx.Err()!=nil{
			for id,st:=range states{if st==dag.Pending{states[id]=dag.Cancelled}}
			return report,ErrIncomplete
		}
		blocked,e:=graph.Blocked(states)
		if e!=nil{return report,ErrInvalid}
		for _,id:=range blocked{
			if states[id]==dag.Pending{states[id]=dag.Skipped;completed++}
		}
		ready,e:=graph.Ready(states)
		if e!=nil{return report,ErrInvalid}
		if len(ready)==0{
			if completed==len(frozen.Nodes){break}
			return report,ErrIncomplete
		}
		sort.Strings(ready)
		for _,id:=range ready{
			if ctx.Err()!=nil{break}
			n:=nodes[id]
			h:=handlers[n.Kind]
			states[id]=dag.Running
			// Treat missing adapter as failure, never success/no-op.
			if h==nil{
				states[id]=dag.Failed
				failure[id]="adapter unavailable"
			}else{
				// The handler receives an isolated clone, never a slice alias of
				// the run's frozen dependency snapshot.
				hNode:=n
				hNode.Requires=append([]string(nil),n.Requires...)
				if h(ctx,hNode)!=nil{
					states[id]=dag.Failed
					failure[id]="step execution failed"
				}else{
					states[id]=dag.Succeeded
				}
			}
			completed++
		}
	}
	for _,st:=range states{if st!=dag.Succeeded{return report,ErrIncomplete}}
	return report,nil
}
