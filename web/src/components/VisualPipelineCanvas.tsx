import {memo,useMemo,useRef,useState} from "react";
import {Background,BackgroundVariant,Controls,Handle,MarkerType,MiniMap,Position,ReactFlow,type Connection,type Edge,type Node,type NodeChange,type NodeProps,type ReactFlowInstance} from "@xyflow/react";
import {GitBranch,Play,Plus,RefreshCw,Trash2,Unplug} from "lucide-react";
import {Button} from "./ui/button";
import type {PipelinePlan,SQLJob} from "../lib/api";

type JobNodeData={label:string;jobId:string;enabled:boolean|undefined;requires:number} & Record<string,unknown>;
type JobGraphNode=Node<JobNodeData,"pipelineJob">;
type PositionXY={x:number;y:number};
type Props={plan:PipelinePlan;jobs:SQLJob[];onChange:(next:PipelinePlan)=>void;onReloadJobs:()=>void;loadingJobs:boolean;canLoadJobs:boolean;onRunCanvas:()=>void;canRunCanvas:boolean;runBusy:boolean;runHint:string};

const PipelineJobNode=memo(function PipelineJobNode({data,selected}:NodeProps<JobGraphNode>){
 return <div className={"df-pipeline-graph-node "+(selected?"is-selected":"")}>
  <Handle type="target" position={Position.Left} className="df-pipeline-handle" aria-label="Dependency input"/>
  <div className="flex items-center justify-between gap-2">
   <span className="df-pipeline-kicker">SQL JOB</span>
   <span className={data.enabled===false?"df-text-warning":"df-text-neutral"}>{data.enabled===false?"Disabled":data.enabled===undefined?"Not in registry":"Registered"}</span>
  </div>
  <div className="mt-2 break-words text-sm font-semibold">{data.label}</div>
  <div className="muted mt-1 break-all font-mono text-[10px]">{data.jobId}</div>
  <div className="muted mt-3 text-xs">{data.requires} prerequisites</div>
  <Handle type="source" position={Position.Right} className="df-pipeline-handle" aria-label="Dependency output"/>
 </div>;
});
const nodeTypes={pipelineJob:PipelineJobNode};
const maxNodes=80;

function generatesCycle(plan:PipelinePlan,source:string,target:string):boolean{
 if(source===target)return true;
 const downstream=new Map<string,string[]>();
 for(const item of plan.nodes)for(const dep of item.requires){
  downstream.set(dep,[...(downstream.get(dep)??[]),item.id]);
 }
 const queue=[target],seen=new Set<string>();
 while(queue.length){
  const item=queue.shift()!;
  if(item===source)return true;
  if(seen.has(item))continue;
  seen.add(item);
  queue.push(...(downstream.get(item)??[]));
 }
 return false;
}
function nextNodeId(plan:PipelinePlan,job:SQLJob){
 const stem=(job.name.toLowerCase().replace(/[^a-z0-9_]+/g,"_").replace(/^_+|_+$/g,"").slice(0,36)||"sql_job");
 const taken=new Set(plan.nodes.map(n=>n.id));
 if(!taken.has(stem))return stem;
 let suffix=2;
 while(taken.has(stem+"_"+suffix))suffix++;
 return stem+"_"+suffix;
}
export function VisualPipelineCanvas({plan,jobs,onChange,onReloadJobs,loadingJobs,canLoadJobs,onRunCanvas,canRunCanvas,runBusy,runHint}:Props){
 const[selectedId,setSelectedId]=useState<string|null>(null);
 const[selectedJob,setSelectedJob]=useState("");
 const[dependencySource,setDependencySource]=useState("");
 const[selectedEdge,setSelectedEdge]=useState<{source:string;target:string}|null>(null);
 const[notice,setNotice]=useState("");
 const[positions,setPositions]=useState<Record<string,PositionXY>>({});
 const instance=useRef<ReactFlowInstance<JobGraphNode,Edge>|null>(null);
 const byId=useMemo(()=>new Map(jobs.map(j=>[j.id,j])),[jobs]);
 const selected=plan.nodes.find(n=>n.id===selectedId)??null;
 const selectedJobInfo=jobs.find(j=>j.id===selectedJob);
 const known=new Set(plan.nodes.map(n=>n.id));
 const missingDependencies=plan.nodes.flatMap(n=>n.requires.filter(req=>!known.has(req)).map(req=>n.id+" → "+req));
 const nodes:JobGraphNode[]=useMemo(()=>plan.nodes.map((n,i)=>({
  id:n.id,type:"pipelineJob",position:positions[n.id]??{x:65+(i%3)*290,y:60+Math.floor(i/3)*166},
  selected:n.id===selectedId,data:{label:byId.get(n.job_id)?.name??n.id,jobId:n.job_id,enabled:byId.get(n.job_id)?.enabled,requires:n.requires.length},
 })),[plan,positions,selectedId,byId]);
 const edges:Edge[]=useMemo(()=>plan.nodes.flatMap(n=>n.requires.filter(dep=>known.has(dep)&&dep!==n.id).map(dep=>({
  id:"dep:"+dep+":"+n.id,source:dep,target:n.id,type:"smoothstep",
  markerEnd:{type:MarkerType.ArrowClosed,color:"var(--console-accent)"},
  style:{stroke:"var(--console-accent)",strokeWidth:1.7},
 }))),[plan]);
 const addJob=(jobId:string,point?:PositionXY)=>{
  const job=byId.get(jobId);
  if(!job){setNotice("Choose a loaded SQL Job before adding a node.");return;}
  if(plan.nodes.length>=maxNodes){setNotice("The visual editor supports at most 80 nodes.");return;}
  const id=nextNodeId(plan,job);
  const index=plan.nodes.length;
  setPositions(prev=>({...prev,[id]:point??{x:65+(index%3)*290,y:60+Math.floor(index/3)*166}}));
  onChange({nodes:[...plan.nodes,{id,job_id:job.id,requires:[]}]});
  setSelectedId(id);setSelectedEdge(null);setNotice("");
 };
 const connect=(source:string,target:string)=>{
  if(!known.has(source)||!known.has(target)){setNotice("Both ends must reference nodes on the canvas.");return;}
  if(generatesCycle(plan,source,target)){setNotice("Connection rejected: it would introduce a dependency cycle.");return;}
  const node=plan.nodes.find(n=>n.id===target);
  if(!node||node.requires.includes(source)){setNotice("This dependency already exists.");return;}
  onChange({nodes:plan.nodes.map(n=>n.id===target?{...n,requires:[...n.requires,source]}:n)});
  setSelectedId(target);setSelectedEdge(null);setNotice("");
 };
 const unlink=(source:string,target:string)=>{
  onChange({nodes:plan.nodes.map(n=>n.id===target?{...n,requires:n.requires.filter(dep=>dep!==source)}:n)});
  setSelectedEdge(null);setNotice("");
 };
 const removeSelected=()=>{
  if(!selected)return;
  onChange({nodes:plan.nodes.filter(n=>n.id!==selected.id).map(n=>({...n,requires:n.requires.filter(dep=>dep!==selected.id)}))});
  setSelectedId(null);setSelectedEdge(null);setNotice("");
 };
 const moveNodes=(changes:NodeChange<JobGraphNode>[])=>{
  const moved=changes.filter(c=>c.type==="position"&&c.position);
  if(moved.length)setPositions(prev=>{
   const updated={...prev};
   for(const move of moved)if(move.type==="position"&&move.position)updated[move.id]=move.position;
   return updated;
  });
 };
 return <div className="df-studio-workspace df-pipeline-builder space-y-4" aria-label="Visual pipeline builder">
  <div className="flex flex-wrap items-start justify-between gap-3">
   <div><h3 className="flex items-center gap-2 text-lg font-semibold"><GitBranch size={19}/>Pipeline canvas <span className="signal-inline-code">/ XYFLOW</span></h3>
    <p className="muted mt-1 text-xs">Add or drag registered SQL Jobs; connect prerequisite edges. Run this exact canvas after Validate, Save with TEST enabled, and explicit confirmation.</p></div>
   <div className="flex flex-wrap items-center gap-2 text-xs">
    <span className="df-pipeline-stat">{plan.nodes.length} nodes</span>
    <span className="df-pipeline-stat">{edges.length} links</span>
    <Button type="button" size="sm" disabled={!canRunCanvas||runBusy} title={runHint} aria-label="Run canvas TEST" onClick={onRunCanvas}><Play size={15}/>{runBusy?"Checking…":"Run canvas"}</Button>
   </div>
  </div>
  {!canRunCanvas&&<p className="muted text-xs" role="status">{runHint}</p>}
  <div className="grid gap-4 xl:grid-cols-[235px_minmax(0,1fr)]">
   <aside className="df-pipeline-palette rounded border df-border-neutral p-3" aria-label="SQL Job palette">
    <div className="flex items-center justify-between gap-2"><h4 className="text-sm font-semibold">Registered SQL Jobs</h4>
     <Button size="sm" variant="ghost" type="button" disabled={!canLoadJobs||loadingJobs} onClick={onReloadJobs} aria-label="Load registered SQL jobs"><RefreshCw size={15}/></Button>
    </div>
    <p className="muted mb-3 mt-1 text-xs">Load jobs from the existing read-only registry. Drag a job into the canvas or use Add (mobile/keyboard).</p>
    {!canLoadJobs&&<p className="df-text-warning text-xs">Enter an API token to load SQL Jobs.</p>}
    {jobs.length===0&&<p className="muted rounded border df-border-neutral p-3 text-xs">No jobs loaded. Select the refresh icon above.</p>}
    {jobs.length>0&&<div className="space-y-2">
     <select aria-label="Registered SQL Job" className="field text-xs" value={selectedJob} onChange={event=>setSelectedJob(event.target.value)}>
      <option value="">Select SQL Job…</option>
      {jobs.map(j=><option key={j.id} value={j.id}>{j.name}{j.enabled?"":" (disabled)"}</option>)}
     </select>
     <Button size="sm" type="button" disabled={!selectedJobInfo||plan.nodes.length>=maxNodes} onClick={()=>addJob(selectedJob)}><Plus size={15}/>Add SQL job</Button>
     <div className="max-h-52 space-y-1 overflow-y-auto">
      {jobs.map(job=><div key={job.id} draggable aria-label={"Drag SQL job "+job.name}
        onDragStart={event=>{event.dataTransfer.setData("application/x-dataflow-sql-job",job.id);event.dataTransfer.effectAllowed="copy";}}
        className="df-pipeline-palette-job rounded border p-2 text-xs" title="Drag into pipeline canvas">
       <span className="font-semibold">{job.name}</span><span className="muted mt-1 block">{job.enabled?"Enabled":"Disabled"} · {job.database}</span>
      </div>)}
     </div>
    </div>}
   </aside>
   <div className="min-w-0">
    <div className="df-pipeline-edit-canvas" aria-label="Editable dependency canvas"
     onDragOver={event=>{if(event.dataTransfer.types.includes("application/x-dataflow-sql-job")){event.preventDefault();event.dataTransfer.dropEffect="copy";}}}
     onDrop={event=>{const id=event.dataTransfer.getData("application/x-dataflow-sql-job");if(!id)return;event.preventDefault();const position=instance.current?.screenToFlowPosition({x:event.clientX,y:event.clientY});addJob(id,position);}}>
     <ReactFlow<JobGraphNode,Edge> nodes={nodes} edges={edges} nodeTypes={nodeTypes}
      fitView fitViewOptions={{padding:0.25,maxZoom:1.1}} minZoom={0.25} maxZoom={1.7}
      onInit={flow=>{instance.current=flow;}}
      onNodesChange={moveNodes} onNodeClick={(_,n)=>{setSelectedId(n.id);setSelectedEdge(null);}}
      onPaneClick={()=>{setSelectedEdge(null);}}
      onEdgeClick={(_,e)=>{setSelectedEdge({source:e.source,target:e.target});setSelectedId(e.target);}}
      onConnect={(connection:Connection)=>{if(connection.source&&connection.target)connect(connection.source,connection.target);}}
      isValidConnection={connection=>!!connection.source&&!!connection.target&&known.has(connection.source)&&known.has(connection.target)&&!generatesCycle(plan,connection.source,connection.target)&&!plan.nodes.find(n=>n.id===connection.target)?.requires.includes(connection.source)}
      edgesReconnectable={false} deleteKeyCode={null} multiSelectionKeyCode={null}
      nodesDraggable nodesConnectable elementsSelectable panOnDrag zoomOnScroll zoomOnPinch zoomOnDoubleClick={false}>
      <Background variant={BackgroundVariant.Lines} color="var(--signal-grid)" gap={24} size={0.7}/>
      <Controls showInteractive={false} position="bottom-left"/>
      <MiniMap pannable zoomable position="bottom-right" nodeColor={n=>(n.data as JobNodeData).enabled===false?"var(--signal-status-warning)":"var(--console-accent)"}/>
     </ReactFlow>
     {plan.nodes.length===0&&<p className="df-pipeline-canvas-empty pointer-events-none absolute left-1/2 top-1/2 w-[80%] max-w-sm -translate-x-1/2 -translate-y-1/2 text-center text-sm">Add a registered SQL Job to begin your pipeline. You can also drag it here.</p>}
    </div>
    <p className="muted mt-2 text-xs">Drag cards to rearrange. Draw edges: upstream output → downstream input. Edits stay in this browser draft until saved separately.</p>
   </div>
  </div>
  <div className="grid gap-3 lg:grid-cols-2">
   <section className="rounded border df-border-neutral p-3" aria-label="Pipeline node inspector">
    <h4 className="mb-2 text-sm font-semibold">Node inspector</h4>
    <label className="block space-y-1"><span className="muted text-xs">Inspect node</span>
     <select aria-label="Inspect pipeline node" className="field" value={selected?.id??""} onChange={e=>{setSelectedId(e.target.value||null);setSelectedEdge(null);}}>
      <option value="">Select a node…</option>{plan.nodes.map(n=><option key={n.id} value={n.id}>{n.id}</option>)}
     </select>
    </label>
    {selected&&<>
     <p className="muted my-2 break-all text-xs">Registered job ID: {selected.job_id}</p>
     {selected.requires.length>0&&<div className="mb-3 space-y-1">{selected.requires.map(dep=><div key={dep} className="flex items-center justify-between gap-2 text-xs"><span className="min-w-0 break-all">{dep} → {selected.id}</span><Button type="button" size="sm" variant="ghost" aria-label={"Remove dependency "+dep+" to "+selected.id} onClick={()=>unlink(dep,selected.id)}><Unplug size={14}/></Button></div>)}</div>}
     <div className="flex flex-wrap items-end gap-2">
      <label className="min-w-[180px] flex-1 space-y-1"><span className="muted text-xs">Upstream dependency</span>
       <select aria-label="Dependency source" className="field" value={dependencySource} onChange={e=>setDependencySource(e.target.value)}>
        <option value="">Select upstream node…</option>
        {plan.nodes.filter(n=>n.id!==selected.id).map(n=><option key={n.id} value={n.id}>{n.id}</option>)}
       </select>
      </label>
      <Button type="button" variant="outline" size="sm" disabled={!dependencySource} onClick={()=>connect(dependencySource,selected.id)}><GitBranch size={14}/>Connect dependency</Button>
      <Button type="button" size="sm" variant="destructive" onClick={removeSelected}><Trash2 size={14}/>Remove node</Button>
     </div>
    </>}
    {!selected&&<p className="muted mt-2 text-xs">Click a node or select it above. You can connect prerequisites without dragging handles.</p>}
   </section>
   <section className="rounded border df-border-neutral p-3" aria-label="Pipeline connection inspector">
    <h4 className="mb-2 text-sm font-semibold">Connection inspector</h4>
    {selectedEdge?<div className="space-y-2 text-xs"><p className="break-words">{selectedEdge.source} → {selectedEdge.target}</p>
      <Button variant="outline" type="button" size="sm" onClick={()=>unlink(selectedEdge.source,selectedEdge.target)}><Unplug size={14}/>Remove selected connection</Button>
     </div>:<p className="muted text-xs">Click an edge in the canvas to inspect or unlink it.</p>}
    {missingDependencies.length>0&&<p role="alert" className="df-text-warning mt-2 break-words text-xs">Unresolved dependencies in JSON: {missingDependencies.join(", ")}</p>}
    {notice&&<p role="status" className="df-text-warning mt-3 text-xs">{notice}</p>}
   </section>
  </div>
 </div>;
}
