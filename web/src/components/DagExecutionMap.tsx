import {memo,useMemo} from "react";
import {AlertTriangle,CheckCircle2,CircleDashed,Clock3,GitBranch,PlayCircle,XCircle} from "lucide-react";
import {
 Background,BackgroundVariant,Controls,Handle,MarkerType,MiniMap,
 Position,ReactFlow,type Edge,type Node,type NodeProps,
} from "@xyflow/react";
import type {PipelineNode} from "../lib/api";
import {Badge} from "./ui/badge";

type EvidenceNodeData={
 label:string; jobId?:string; status?:string; attempts?:number;
 error?:string; requires:string[];
} & Record<string,unknown>;
type EvidenceNode=Node<EvidenceNodeData,"evidence">;
type Props={
 plan?:PipelineNode[];
 state?:Record<string,string>;
 attempts?:Record<string,number>;
 errors?:Record<string,string>;
};
const MAX_RENDERED=80;
const MAX_LEVEL=12;
function variant(status?:string):"success"|"danger"|"warning"|"neutral"{
 if(status==="succeeded")return "success";
 if(status==="failed"||status==="cancelled")return "danger";
 if(status==="queued"||status==="running")return "warning";
 return "neutral";
}
function StateIcon({status}:{status?:string}){
 if(status==="succeeded")return <CheckCircle2 className="text-emerald-400" size={16}/>;
 if(status==="failed"||status==="cancelled")return <XCircle className="text-red-400" size={16}/>;
 if(status==="running")return <PlayCircle className="text-amber-400" size={16}/>;
 if(status==="queued")return <Clock3 className="text-amber-400" size={16}/>;
 return <CircleDashed className="muted" size={16}/>;
}
const SignalNode=memo(function SignalNode({data,selected}:NodeProps<EvidenceNode>){
 return <div className={`xyflow-signal-node ${selected?"is-selected":""} ${data.status?"status-"+data.status:"status-unreported"}`}>
  <Handle type="target" position={Position.Left} isConnectable={false} className="xyflow-handle"/>
  <div className="flex items-center justify-between gap-2">
   <span className="signal-node-kicker">JOB / STEP</span>
   <StateIcon status={data.status}/>
  </div>
  <p className="signal-node-name mt-2 break-all">{data.label}</p>
  <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
   <Badge variant={variant(data.status)}>{data.status??"No result yet"}</Badge>
   <span className="signal-node-attempts">Attempts: {data.attempts??"Not reported"}</span>
  </div>
  {data.error&&<p className="signal-node-error mt-3 break-words"><AlertTriangle size={12} className="mr-1 inline"/>{data.error}</p>}
  {data.requires.length>0&&<p className="signal-node-dependencies mt-2 break-words">Needs: {data.requires.join(", ")}</p>}
  <Handle type="source" position={Position.Right} isConnectable={false} className="xyflow-handle"/>
 </div>;
});
const nodeTypes={evidence:SignalNode};

function buildGraph(plan:PipelineNode[]|undefined,state:Record<string,string>,attempts:Record<string,number>,errors:Record<string,string>){
 const source=plan?.length?plan: Object.keys(state).sort().map(id=>({id,job_id:"",requires:[] as string[]}));
 const included=source.slice(0,MAX_RENDERED);
 const names=new Set(included.map(n=>n.id));
 const ranks=new Map(included.map(n=>[n.id,0]));
 for(let i=0;i<included.length;i++){
  let changed=false;
  for(const item of included){
   const parents=(item.requires??[]).filter(k=>names.has(k));
   const rank=parents.length?Math.max(...parents.map(k=>ranks.get(k)??0))+1:0;
   const bounded=Math.min(rank,MAX_LEVEL);
   if(bounded>(ranks.get(item.id)??0)){ranks.set(item.id,bounded);changed=true}
  }
  if(!changed)break;
 }
 const columns=new Map<number,PipelineNode[]>();
 for(const item of included){
  const rank=ranks.get(item.id)??0;
  columns.set(rank,[...(columns.get(rank)??[]),item]);
 }
 const nodes:EvidenceNode[]=[];
 for(const [level,items] of [...columns].sort(([a],[b])=>a-b)){
  const height=items.length*158-18;
  items.forEach((item,index)=>{
   nodes.push({
    id:item.id,type:"evidence",
    position:{x:level*318,y:index*158-Math.floor(height/2)},
    draggable:false,selectable:true,
    data:{label:item.id,jobId:item.job_id,status:state[item.id],attempts:attempts[item.id],error:errors[item.id],requires:item.requires??[]},
   });
  });
 }
 const edges:Edge[]=[];
 for(const item of included){
  for(const prerequisite of item.requires??[]){
   if(!names.has(prerequisite)||prerequisite===item.id)continue;
   const outcome=state[item.id];
   const stroke=outcome==="failed"?"#dd6c61":outcome==="succeeded"?"#4baf7e":"var(--console-muted)";
   edges.push({
    id:`dependency:${prerequisite}:${item.id}`,source:prerequisite,target:item.id,
    type:"smoothstep",animated:outcome==="running",
    style:{stroke,strokeWidth:outcome==="running"?2.5:1.7},
    markerEnd:{type:MarkerType.ArrowClosed,color:stroke},
   });
  }
 }
 return {nodes,edges,total:source.length};
}
export function DagExecutionMap({plan,state,attempts,errors}:Props){
 const safeState=state??{},safeAttempts=attempts??{},safeErrors=errors??{};
 const graph=useMemo(()=>buildGraph(plan,safeState,safeAttempts,safeErrors),[plan,state,attempts,errors]);
 if(graph.nodes.length===0)return <div className="muted rounded border border-border px-4 py-8 text-center text-sm">No DAG definition or node execution evidence is available for this run.</div>;
 return <section className="space-y-3" aria-label="DAG execution map">
  <div className="flex flex-wrap items-center justify-between gap-3">
   <div><h4 className="flex items-center gap-2 text-sm font-semibold"><GitBranch size={16}/>Execution graph <span className="signal-inline-code">/ XYFLOW</span></h4>
    <p className="muted mt-1 text-xs">Pan, zoom and inspect real dependency edges and recorded node outcomes.</p></div>
   <div className="flex flex-wrap gap-2"><Badge variant="neutral">{graph.nodes.length} shown</Badge><Badge variant="default">{Object.keys(safeState).length} node results</Badge></div>
  </div>
  <div className="xyflow-signal-canvas" role="img" aria-label="Interactive pipeline execution diagram">
   <ReactFlow<EvidenceNode,Edge>
    nodes={graph.nodes} edges={graph.edges} nodeTypes={nodeTypes}
    fitView fitViewOptions={{padding:0.24,maxZoom:1.15}}
    minZoom={0.25} maxZoom={1.7}
    nodesDraggable={false} nodesConnectable={false}
    elementsSelectable={true} edgesReconnectable={false}
    deleteKeyCode={null} multiSelectionKeyCode={null}
    panOnDrag zoomOnScroll zoomOnPinch zoomOnDoubleClick={false}
    colorMode="dark"
    proOptions={{hideAttribution:false}}
   >
    <Background variant={BackgroundVariant.Lines} gap={24} size={0.7} color="var(--signal-grid)"/>
    <MiniMap pannable zoomable position="bottom-right" nodeColor={n=>{
     const st=(n.data as EvidenceNodeData)?.status;
     return st==="failed"?"#dd6c61":st==="succeeded"?"#4baf7e":st==="running"?"#e1be70":"#879c8b";
    }}/>
    <Controls showInteractive={false} position="bottom-left"/>
   </ReactFlow>
  </div>
  {graph.total>MAX_RENDERED&&<p className="muted text-xs">Showing first {MAX_RENDERED} of {graph.total} DAG nodes. Open the saved DAG definition to inspect additional nodes.</p>}
  <p className="muted text-xs">Edges reflect saved dependencies, not chronological execution. Missing node results and timing remain unreported; dragging cannot change the pipeline.</p>
 </section>;
}
