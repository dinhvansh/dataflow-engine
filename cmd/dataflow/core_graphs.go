package main

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "regexp"
 "time"

 "github.com/jackc/pgx/v5"
)

type coreGraphNode struct {
 ID string `json:"id"`
 Type string `json:"type"`
 Requires []string `json:"requires"`
 StreamID string `json:"stream_id,omitempty"`
 ModelID string `json:"model_id,omitempty"`
 UniqueColumn string `json:"unique_column,omitempty"`
 X int `json:"x,omitempty"`
 Y int `json:"y,omitempty"`
}
type coreGraphDefinition struct{Nodes []coreGraphNode `json:"nodes"`}
type coreGraphBinding struct {
 StreamID string
 ModelID string
 UniqueColumn string
 PlanDigest string
 Digest string
 Bytes []byte
}
type storedCoreGraph struct {
 ID string `json:"id"`
 Name string `json:"name"`
 Version int `json:"version"`
 Graph coreGraphDefinition `json:"graph"`
 GraphDigest string `json:"graph_digest"`
 PlanDigest string `json:"plan_digest"`
 StreamID string `json:"stream_id"`
 ModelID string `json:"model_id"`
 UniqueColumn string `json:"unique_column"`
}
var coreGraphNodeID=regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`)
var coreGraphName=regexp.MustCompile(`^[a-z][a-z0-9_-]{0,99}$`)

// No speculative fork/join. A graph is executable only when its edges are
// one canonical SOURCE→SYNC→TRANSFORM*→QUALITY→PUBLISH chain and each saved
// transform matches the compiler-verified dbt dependency chain exactly.
func(s *server)validateCoreGraph(ctx context.Context,tx pgx.Tx,g coreGraphDefinition)(coreGraphBinding,error){
 var out coreGraphBinding
 if len(g.Nodes)<5||len(g.Nodes)>34{return out,errors.New("graph requires SOURCE, SYNC, 1..30 TRANSFORM, QUALITY and PUBLISH")}
 byID:=make(map[string]coreGraphNode,len(g.Nodes))
 children:=make(map[string][]string,len(g.Nodes))
 var root string
 for _,n:=range g.Nodes{
  if !coreGraphNodeID.MatchString(n.ID)||n.X < -10000||n.X > 10000||n.Y < -10000||n.Y > 10000{
   return out,errors.New("invalid graph node ID or canvas position")
  }
  if _,ok:=byID[n.ID];ok{return out,errors.New("duplicate node ID")}
  if len(n.Requires)>1{return out,errors.New("fork/join or multiple predecessors not supported by durable Core")}
  if len(n.Requires)==0{
   if root!=""{return out,errors.New("exactly one SOURCE root required")}
   root=n.ID
  }
  byID[n.ID]=n
 }
 for _,n:=range g.Nodes{
  for _,parent:=range n.Requires{
   if _,exists:=byID[parent];!exists||parent==n.ID{return out,errors.New("unknown or self dependency")}
   children[parent]=append(children[parent],n.ID)
   if len(children[parent])>1{return out,errors.New("parallel execution not yet supported")}
  }
 }
 ordered:=make([]coreGraphNode,0,len(g.Nodes));seen:=map[string]bool{}
 at:=root
 for at!=""{
  if seen[at]{return out,errors.New("dependency cycle detected")}
  seen[at]=true;ordered=append(ordered,byID[at])
  if len(children[at])==1{at=children[at][0]}else{at=""}
 }
 if len(ordered)!=len(g.Nodes){return out,errors.New("disconnected graph or dependency cycle")}
 for i,n:=range ordered{
  want:="TRANSFORM"
  switch i{case 0:want="SOURCE";case 1:want="SYNC";case len(ordered)-2:want="QUALITY";case len(ordered)-1:want="PUBLISH"}
  if n.Type!=want{return out,fmt.Errorf("invalid step %d: expected %s",i,want)}
  if i>0&&(len(n.Requires)!=1||n.Requires[0]!=ordered[i-1].ID){return out,errors.New("broken linear dependency order")}
  // Reject hidden executable parameters on nodes of the wrong type.
  if n.Type!="SOURCE"&&n.StreamID!=""{return out,errors.New("unexpected stream binding")}
  if n.Type!="TRANSFORM"&&n.ModelID!=""{return out,errors.New("unexpected model binding")}
  if n.Type!="QUALITY"&&n.UniqueColumn!=""{return out,errors.New("unexpected quality binding")}
 }
 out.StreamID=ordered[0].StreamID
 out.ModelID=ordered[len(ordered)-3].ModelID
 out.UniqueColumn=ordered[len(ordered)-2].UniqueColumn
 if !validUUID.MatchString(out.StreamID)||!validUUID.MatchString(out.ModelID)||
   len(out.UniqueColumn)>85||!safeDQColumn.MatchString(out.UniqueColumn){
  return out,errors.New("source/model/quality bindings required and must be valid")
 }
 var db,table string
 if err:=tx.QueryRow(ctx,`SELECT target_database,target_table FROM sync_streams WHERE id=$1::uuid`,out.StreamID).Scan(&db,&table);err!=nil{
  return out,errors.New("source stream not found")
 }
 if db!="scratch"||table!="dataflow_raw_events"{return out,errors.New("source must target scratch.dataflow_raw_events")}
 plan,err:=s.corePlan(ctx,tx,out.ModelID)
 if err!=nil{return out,fmt.Errorf("saved model dependency chain invalid: %w",err)}
 if len(plan.Steps)!=len(ordered)-4{return out,errors.New("graph TRANSFORM count differs from validated saved model chain")}
 for i,step:=range plan.Steps{
  if ordered[i+2].ModelID!=step.ID{return out,fmt.Errorf("transform %d does not match immutable dbt dependency order",i)}
 }
 // Canonical order is derived from verified edges, never untrusted JSON input order.
 raw,e:=json.Marshal(coreGraphDefinition{Nodes:ordered});if e!=nil{return out,e}
 digest:=sha256.Sum256(raw)
 out.Bytes=raw;out.Digest=hex.EncodeToString(digest[:]);out.PlanDigest=plan.Digest
 return out,nil
}

func(s *server)coreGraphsAPI(w http.ResponseWriter,r *http.Request){
 if !s.validBearer(r){http.Error(w,"unauthorized",401);return}
 if !corePipelineEnabled(){http.Error(w,"isolated Core graph API disabled",403);return}
 ctx,cancel:=context.WithTimeout(r.Context(),15*time.Second);defer cancel()
 switch r.Method{
 case http.MethodGet:
  rows,err:=s.pg.Query(ctx,`SELECT id::text,name,version,graph,graph_digest,plan_digest,
      stream_id::text,target_model_id::text,unique_column
      FROM core_pipeline_graphs ORDER BY updated_at DESC,id LIMIT 50`)
  if err!=nil{http.Error(w,"Core graph registry unavailable: apply migration 030",503);return}
  graphs:=make([]storedCoreGraph,0)
  for rows.Next(){
   var g storedCoreGraph;var raw []byte
   if err=rows.Scan(&g.ID,&g.Name,&g.Version,&raw,&g.GraphDigest,&g.PlanDigest,&g.StreamID,&g.ModelID,&g.UniqueColumn);err!=nil{break}
   if err=json.Unmarshal(raw,&g.Graph);err!=nil{break}
   graphs=append(graphs,g)
  }
  if rows.Err()!=nil&&err==nil{err=rows.Err()}
  rows.Close()
  if err!=nil{http.Error(w,"Core graph registry unreadable",503);return}
  jsonResponse(w,200,map[string]any{"graphs":graphs})
 case http.MethodPost:
  r.Body=http.MaxBytesReader(w,r.Body,32768)
  dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields()
  var request struct{
   Action string `json:"action"`
   Name string `json:"name"`
   ExpectedVersion int `json:"expected_version"`
   Graph coreGraphDefinition `json:"graph"`
  }
  if dec.Decode(&request)!=nil||(request.Action!="validate"&&request.Action!="save")||
     !coreGraphName.MatchString(request.Name)||request.ExpectedVersion<0{
   http.Error(w,"valid graph action/name/version required",400);return
  }
  var extra any
  if e:=dec.Decode(&extra);!errors.Is(e,io.EOF){http.Error(w,"extra JSON not accepted",400);return}
  tx,err:=s.pg.Begin(ctx);if err!=nil{http.Error(w,"graph validation unavailable",503);return}
  defer tx.Rollback(ctx)
  binding,err:=s.validateCoreGraph(ctx,tx,request.Graph)
  if err!=nil{http.Error(w,err.Error(),422);return}
  if request.Action=="validate"{
   jsonResponse(w,200,map[string]any{"valid":true,"digest":binding.Digest,
      "model_plan_digest":binding.PlanDigest,"stream_id":binding.StreamID,
      "model_id":binding.ModelID,"unique_column":binding.UniqueColumn})
   return
  }
  var id string;version:=request.ExpectedVersion+1
  if request.ExpectedVersion==0{
   err=tx.QueryRow(ctx,`INSERT INTO core_pipeline_graphs
     (name,graph,graph_digest,plan_digest,stream_id,target_model_id,unique_column)
     VALUES($1,$2::jsonb,$3,$4,$5::uuid,$6::uuid,$7) RETURNING id::text`,
     request.Name,string(binding.Bytes),binding.Digest,binding.PlanDigest,
     binding.StreamID,binding.ModelID,binding.UniqueColumn).Scan(&id)
  }else{
   err=tx.QueryRow(ctx,`UPDATE core_pipeline_graphs SET version=version+1,
      graph=$3::jsonb,graph_digest=$4,plan_digest=$5,
      stream_id=$6::uuid,target_model_id=$7::uuid,unique_column=$8,
      updated_at=clock_timestamp()
      WHERE name=$1 AND version=$2 RETURNING id::text`,
      request.Name,request.ExpectedVersion,string(binding.Bytes),binding.Digest,
      binding.PlanDigest,binding.StreamID,binding.ModelID,binding.UniqueColumn).Scan(&id)
  }
  if err!=nil{http.Error(w,"graph already exists or saved revision changed; reload before retry",409);return}
  if err=tx.Commit(ctx);err!=nil{http.Error(w,"graph save commit uncertain; reload registry",503);return}
  var canonical coreGraphDefinition
  if e:=json.Unmarshal(binding.Bytes,&canonical);e!=nil{http.Error(w,"internal graph encoding error",500);return}
  jsonResponse(w,200,storedCoreGraph{ID:id,Name:request.Name,Version:version,Graph:canonical,
     GraphDigest:binding.Digest,PlanDigest:binding.PlanDigest,
     StreamID:binding.StreamID,ModelID:binding.ModelID,UniqueColumn:binding.UniqueColumn})
 default:
  http.Error(w,"GET or POST required",405)
 }
}

// Bound graph launches are fail-closed: a changed graph version or a changed
// saved compiled model chain cannot accidentally dispatch new work.
func(s *server)boundCoreGraph(ctx context.Context,tx pgx.Tx,graphID string,version int,
 streamID,modelID,unique string,modelPlan corePlan)(storedCoreGraph,[]byte,error){
 var g storedCoreGraph;var raw []byte
 if !validUUID.MatchString(graphID)||version<1{return g,nil,errors.New("graph UUID and version required")}
 err:=tx.QueryRow(ctx,`SELECT id::text,name,version,graph,graph_digest,plan_digest,
    stream_id::text,target_model_id::text,unique_column FROM core_pipeline_graphs
    WHERE id=$1::uuid FOR SHARE`,graphID).Scan(
    &g.ID,&g.Name,&g.Version,&raw,&g.GraphDigest,&g.PlanDigest,
    &g.StreamID,&g.ModelID,&g.UniqueColumn)
 if err!=nil{return g,nil,errors.New("saved graph not found")}
 if g.Version!=version||g.StreamID!=streamID||g.ModelID!=modelID||
    g.UniqueColumn!=unique||g.PlanDigest!=modelPlan.Digest{
  return g,nil,errors.New("saved graph version or dependency plan changed")
 }
 if e:=json.Unmarshal(raw,&g.Graph);e!=nil{return g,nil,e}
 validation,e:=s.validateCoreGraph(ctx,tx,g.Graph)
 if e!=nil||validation.Digest!=g.GraphDigest||validation.PlanDigest!=g.PlanDigest{
  return g,nil,errors.New("saved graph snapshot invalid or changed")
 }
 return g,validation.Bytes,nil
}
