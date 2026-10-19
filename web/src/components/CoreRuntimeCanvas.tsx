import {memo,useMemo} from "react";
import {
 Background,BackgroundVariant,Controls,Handle,MarkerType,Position,ReactFlow,
 type Edge,type Node,type NodeProps
} from "@xyflow/react";
import type {CorePipelineRun} from "../lib/api";

type RuntimeData={
 title:string;kind:string;status:string;description:string;
} & Record<string,unknown>;
type RuntimeNode=Node<RuntimeData,"coreRuntime">;
type Props={run:CorePipelineRun|null;sourceLabel:string;modelLabel:string};
const colors:Record<string,string>={
 succeeded:"df-border-success df-text-success",
 published:"df-border-success df-text-success",
 failed:"df-border-error df-text-error",
 rolled_back:"df-border-warning df-text-warning",
 running:"df-border-accent df-text-accent",
 publishing:"df-border-accent df-text-accent",
 waiting_sync:"df-border-warning df-text-warning",
 waiting_flow:"df-border-warning df-text-warning",
 queued:"df-border-warning df-text-warning",
};
const RuntimeNodeView=memo(function RuntimeNodeView({data}:NodeProps<RuntimeNode>){
 const color=colors[data.status]??"df-border-neutral df-text-neutral";
 return <div className={"w-[205px] rounded-md border-2 df-surface p-3 shadow-sm "+color}
  data-testid={"core-runtime-node-"+String(data.kind)}>
  <Handle type="target" position={Position.Left} isConnectable={false}/>
  <span className="block text-[10px] font-semibold uppercase tracking-wide">{data.kind}</span>
  <span className="mt-1 block break-words text-sm font-semibold">{data.title}</span>
  <span className="muted mt-1 block break-words text-[11px]">{data.description}</span>
  <span className="mt-2 block break-words text-xs font-semibold" data-testid="core-runtime-status">{data.status}</span>
  <Handle type="source" position={Position.Right} isConnectable={false}/>
 </div>;
});
const nodeTypes={coreRuntime:RuntimeNodeView};
function observedStatus(status?:string){return status||"not started";}

export function CoreRuntimeCanvas({run,sourceLabel,modelLabel}:Props){
 // Immutable order/status comes exclusively from GET /api/v1/core/runs.
 // Local stream/model selection is only a preview before a persisted Run.
 const graph=useMemo(()=>{
  const specs:{id:string;kind:string;title:string;status:string;description:string}[]=[
   {id:"source",kind:"SOURCE",title:sourceLabel||"Choose a scratch stream",
    status:run?"read-only":"not started",description:"Source identity selected on server; no ERP writes"},
   {id:"sync",kind:"SYNC",title:"Managed incremental ingestion",
    status:observedStatus(run?.managed_status),description:"SQL Server → ClickHouse raw / PG checkpoint"},
  ];
  if(run){
   if(run.steps?.length){
    run.steps.forEach((s,i)=>specs.push({
     id:"step-"+s.index,kind:"TRANSFORM",title:s.model_name,
     status:observedStatus(s.status),
     description:"dbt step "+(i+1)+": "+s.rows_verified.toLocaleString()+" verified rows"+(s.error?" · "+s.error:"")
    }));
   }else{
    specs.push({id:"awaiting",kind:"TRANSFORM",title:run.target_model_name||"Saved model",
     status:"snapshot pending",description:"Awaiting immutable model flow steps in PostgreSQL"});
   }
  }else{
   specs.push({id:"preview",kind:"TRANSFORM",title:modelLabel||"Choose final saved model",
    status:"not started",description:"Preview only; server validates the immutable dependency chain at enqueue"});
  }
  specs.push({id:"dq",kind:"QUALITY",title:"Data Quality gate",
   status:observedStatus(run?.model_flow_status),
   description:"Only backend-confirmed dbt and DQ results permit publication"});
  specs.push({id:"publish",kind:"PUBLISH",title:"Atomic ClickHouse publish",
   status:observedStatus(run?.status),
   description:run?.target||"Versioned target / explicit rollback"});
  const nodes:RuntimeNode[]=specs.map((s,i)=>({
   id:s.id,type:"coreRuntime",position:{x:40+i*245,y:70},
   data:{title:s.title,kind:s.kind,status:s.status,description:s.description},
   draggable:false,connectable:false,selectable:false,
  }));
  const edges:Edge[]=specs.slice(1).map((s,i)=>({
   id:"edge-"+i,source:specs[i].id,target:s.id,type:"smoothstep",
   markerEnd:{type:MarkerType.ArrowClosed,color:"var(--console-accent)"},
   style:{stroke:"var(--console-accent)",strokeWidth:1.7},
   animated:false
  }));
  return {nodes,edges};
 },[run,sourceLabel,modelLabel]);
 return <section className="space-y-2" aria-label="Durable Core runtime canvas">
  <div className="flex flex-wrap items-center justify-between gap-2">
   <h4 className="text-sm font-semibold">Durable Core Canvas <span className="signal-inline-code">/ XYFLOW</span></h4>
   <span className="muted text-xs">{run?"Observed from PostgreSQL · "+run.run_id:"Draft preview · no execution"}</span>
  </div>
  <div className="h-[270px] overflow-hidden rounded border df-border-neutral df-surface-soft" data-testid="core-runtime-graph">
   <ReactFlow<RuntimeNode,Edge> nodes={graph.nodes} edges={graph.edges} nodeTypes={nodeTypes}
    fitView fitViewOptions={{padding:0.18,maxZoom:1}} minZoom={0.28} maxZoom={1.4}
    nodesDraggable={false} nodesConnectable={false} elementsSelectable={false}
    edgesReconnectable={false} deleteKeyCode={null} zoomOnDoubleClick={false}
    panOnDrag zoomOnScroll zoomOnPinch>
    <Background variant={BackgroundVariant.Lines} color="var(--signal-grid)" gap={24} size={0.7}/>
    <Controls showInteractive={false} position="bottom-left"/>
   </ReactFlow>
  </div>
  <p className="muted text-xs">Read-only runtime view. Nodes and progress come from the durable Core API, not the legacy SQL DAG executor. Use the explicit confirmation below to start or roll back.</p>
 </section>;
}
