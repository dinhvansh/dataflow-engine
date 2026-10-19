import {useCallback,useEffect,useMemo,useRef,useState,memo} from "react";
import {
 ReactFlow,Background,BackgroundVariant,Controls,Handle,MarkerType,Position,
 type Connection,type Edge,type Node,type NodeChange,type NodeProps,type ReactFlowInstance
} from "@xyflow/react";
import {GitBranch,Play,Plus,RefreshCw,Save,ShieldCheck,Trash2} from "lucide-react";
import {Button} from "./ui/button";
import {
 coreGraphApi,type CoreGraphDefinition,type CoreGraphNode,
 type SavedCoreGraph,type ValidatedCoreGraph,type ManagedStream
} from "../lib/api";
import type {SavedTransform} from "../lib/transformApi";

type GraphNodeData={kind:CoreGraphNode["type"];title:string;summary:string} & Record<string,unknown>;
type TypedNode=Node<GraphNodeData,"coreEditable">;
function GraphNodeView({data,selected}:NodeProps<TypedNode>){
 return <div className={"w-[195px] rounded border-2 p-3 df-surface "+(selected?"df-border-accent":"df-border-neutral")}>
  <Handle type="target" position={Position.Left} isConnectable={data.kind!=="SOURCE"}/>
  <span className="text-[10px] uppercase font-semibold df-text-accent">{data.kind}</span>
  <p className="mt-1 text-sm font-semibold break-words">{data.title}</p>
  <p className="muted mt-1 text-[10px] break-words">{data.summary}</p>
  <Handle type="source" position={Position.Right} isConnectable={data.kind!=="PUBLISH"}/>
 </div>;
}
const nodeTypes={coreEditable:memo(GraphNodeView)};
const basicGraph=():CoreGraphDefinition=>({nodes:[
 {id:"source",type:"SOURCE",requires:[],x:40,y:80},
 {id:"sync",type:"SYNC",requires:["source"],x:290,y:80},
 {id:"quality",type:"QUALITY",requires:[],unique_column:"stream",x:790,y:80},
 {id:"publish",type:"PUBLISH",requires:["quality"],x:1040,y:80}
]});
type Props={
 token:string;streams:ManagedStream[];models:SavedTransform[];
 onRun:(graph:SavedCoreGraph)=>void;active:boolean;
};
const labels:Record<CoreGraphNode["type"],string>={
 SOURCE:"ERP read-only source",SYNC:"Managed incremental sync",
 TRANSFORM:"Saved dbt Visual model",QUALITY:"DQ uniqueness gate",
 PUBLISH:"Atomic publish"
};
export function CoreMixedGraphEditor({token,streams,models,onRun,active}:Props){
 const [graph,setGraph]=useState<CoreGraphDefinition>(basicGraph);
 const [name,setName]=useState("core_pipeline_1");
 const [saved,setSaved]=useState<SavedCoreGraph|null>(null);
 const [catalog,setCatalog]=useState<SavedCoreGraph[]>([]);
 const [selected,setSelected]=useState<string|null>(null);
 const [newModel,setNewModel]=useState("");
 const [validation,setValidation]=useState<ValidatedCoreGraph|null>(null);
 const [dirty,setDirty]=useState(true);
 const [busy,setBusy]=useState(false);
 const [error,setError]=useState("");
 const [note,setNote]=useState("");
 const instance=useRef<ReactFlowInstance<TypedNode,Edge>|null>(null);
 useEffect(()=>{
  setSaved(null);setCatalog([]);setDirty(true);setValidation(null);setError("");setNote("");
 },[token]);
 const byID=useMemo(()=>new Map(graph.nodes.map(n=>[n.id,n])),[graph]);
 const inspected=selected?byID.get(selected):null;
 const activeModels=models.filter(m=>m.model.mode==="visual"&&m.model.source.database==="scratch");
 const mutate=useCallback((fn:(old:CoreGraphDefinition)=>CoreGraphDefinition)=>{
  setGraph(fn);setDirty(true);setValidation(null);setError("");setNote("");
 },[]);
 const refresh=async()=>{
  if(!token)return;
  setBusy(true);setError("");
  try{setCatalog((await coreGraphApi.list(token)).graphs)}
  catch(e){setError("Cannot read persisted Core graphs: "+String(e))}
  finally{setBusy(false)}
 };
 const load=(id:string)=>{
  const found=catalog.find(g=>g.id===id);
  if(!found)return;
  setGraph({nodes:found.graph.nodes.map(n=>({...n,requires:[...n.requires]}))});
  setName(found.name);setSaved(found);setDirty(false);setValidation({
   valid:true,digest:found.graph_digest,model_plan_digest:found.plan_digest,
   stream_id:found.stream_id,model_id:found.model_id,unique_column:found.unique_column
  });
  setSelected(null);setError("");setNote("Loaded exact persisted revision "+found.version+". No run started.");
 };
 const validate=async()=>{
  if(!token)return;setBusy(true);setError("");setNote("");
  try{const result=await coreGraphApi.validate(name,graph,token);setValidation(result);setNote("Backend validated linear Core graph and saved dbt dependency order.")}
  catch(e){setValidation(null);setError("Core graph rejected: "+String(e))}
  finally{setBusy(false)}
 };
 const save=async()=>{
  if(!token||!validation)return;
  setBusy(true);setError("");setNote("");
  try{
   // Saving a changed existing name is an optimistic CAS update; a new name
   // must use version zero and fails if another operator already owns it.
   const expected=saved?.name===name?saved.version:0;
   const result=await coreGraphApi.save(name,graph,expected,token);
   setSaved(result);setGraph(result.graph);setDirty(false);setValidation({
    valid:true,digest:result.graph_digest,model_plan_digest:result.plan_digest,
    stream_id:result.stream_id,model_id:result.model_id,unique_column:result.unique_column
   });setNote("Saved graph "+result.name+" revision "+result.version+" in PostgreSQL. No Run started.");
   setCatalog((await coreGraphApi.list(token)).graphs);
  }catch(e){setError("Save blocked (revision conflict or invalid model): "+String(e))}
  finally{setBusy(false)}
 };
 const addTransform=(modelID:string,point?:{x:number;y:number})=>{
  if(!modelID||!activeModels.some(m=>m.id===modelID))return;
  const n=graph.nodes.filter(n=>n.type==="TRANSFORM").length;
  if(n>=30){setError("Maximum 30 dbt steps.");return}
  const id="transform_"+String(n+1)+"_"+Date.now().toString(36);
  const current=graph.nodes.find(x=>x.type==="QUALITY");
  const predecessor=current?.requires[0]||graph.nodes.find(x=>x.type==="SYNC")?.id||"sync";
  mutate(g=>({nodes:[
   ...g.nodes.map(x=>x.type==="QUALITY"?{...x,requires:[id]}:x),
   {id,type:"TRANSFORM",model_id:modelID,requires:[predecessor],x:point?.x??(520+n*250),y:point?.y??80}
  ]}));
  setSelected(id);
 };
 const remove=()=>{
  if(!inspected||inspected.type!=="TRANSFORM")return;
  mutate(g=>({nodes:g.nodes.filter(x=>x.id!==inspected.id).map(x=>({
   ...x,requires:x.requires.includes(inspected.id)?[...inspected.requires]:x.requires
  }))}));setSelected(null);
 };
 const connect=(c:Connection)=>{
  if(!c.source||!c.target||c.source===c.target)return;
  const from=byID.get(c.source),to=byID.get(c.target);
  if(!from||!to||from.type==="PUBLISH"||to.type==="SOURCE")return;
  mutate(g=>({nodes:g.nodes.map(n=>n.id===c.target?{...n,requires:[c.source!]}:n)}));
 };
 const onNodesChange=(changes:NodeChange<TypedNode>[])=>{
  const moves=changes.filter(c=>c.type==="position"&&!!c.position);
  if(!moves.length)return;
  mutate(g=>({nodes:g.nodes.map(n=>{
   const move=moves.find(c=>c.id===n.id);
   if(move&&move.type==="position"&&move.position)
    return {...n,x:Math.max(-10000,Math.min(10000,Math.round(move.position.x))),
      y:Math.max(-10000,Math.min(10000,Math.round(move.position.y)))};
   return n;
  })}));
 };
 const nodes:TypedNode[]=graph.nodes.map((n,i)=>{
  const model=models.find(m=>m.id===n.model_id);
  const stream=streams.find(s=>s.id===n.stream_id);
  return {
   id:n.id,type:"coreEditable",position:{x:n.x??(i*235+40),y:n.y??80},
   data:{kind:n.type,title:n.type==="TRANSFORM"?(model?.name??"Select saved model"):
    n.type==="SOURCE"?(stream?stream.source_schema+"."+stream.source_table:"Choose source"):
    labels[n.type],summary:n.type==="QUALITY"?"Unique: "+(n.unique_column||"missing"):
    n.type==="TRANSFORM"?(n.model_id||"No model binding"):
    n.requires.length?"Requires "+n.requires.join(", "):"Graph root"}
  };
 });
 const edges:Edge[]=graph.nodes.flatMap(n=>n.requires.filter(id=>byID.has(id)).map(id=>({
  id:id+"->"+n.id,source:id,target:n.id,type:"smoothstep",
  markerEnd:{type:MarkerType.ArrowClosed,color:"var(--console-accent)"},
  style:{stroke:"var(--console-accent)",strokeWidth:1.5}
 })));
 const allowRun=!!saved&&!dirty&&!!validation&&!busy&&!active&&saved.name===name&&validation.digest===saved.graph_digest;
 return <section aria-label="Editable mixed Core Canvas" className="rounded border df-border-neutral p-4 space-y-3">
  <header className="flex flex-wrap items-center justify-between gap-3">
   <div><h4 className="font-semibold text-sm flex items-center gap-2"><GitBranch size={17}/>Mixed Core Canvas / V8B</h4>
    <p className="muted mt-1 text-xs">Drag nodes, connect a single dependency chain, bind saved models, validate, save with revision lock, then explicitly run.</p></div>
   <div className="flex flex-wrap gap-2">
    <Button size="sm" type="button" variant="outline" disabled={!token||busy} onClick={()=>{void refresh()}}><RefreshCw size={14}/>Load graphs</Button>
    <Button size="sm" type="button" variant="outline" disabled={!token||busy} onClick={()=>{void validate()}}><ShieldCheck size={14}/>Validate</Button>
    <Button size="sm" type="button" disabled={!token||busy||!validation} onClick={()=>{void save()}}><Save size={14}/>Save Core graph</Button>
    <Button size="sm" type="button" disabled={!allowRun} onClick={()=>saved&&onRun(saved)}><Play size={14}/>Run saved graph</Button>
   </div>
  </header>
  <div className="grid gap-2 md:grid-cols-2">
   <label className="text-xs space-y-1">Graph name
    <input className="field" aria-label="Core graph name" value={name} onChange={e=>{setName(e.target.value);setDirty(true);setValidation(null)}} maxLength={100}/>
   </label>
   <label className="text-xs space-y-1">Saved revisions
    <select className="field" aria-label="Saved Core graphs" value={!dirty&&saved? saved.id:""} onChange={e=>load(e.target.value)}>
     <option value="">Select saved graph…</option>{catalog.map(g=><option key={g.id} value={g.id}>{g.name} · rev {g.version}</option>)}
    </select>
   </label>
  </div>
  <div className="grid gap-3 lg:grid-cols-[215px_minmax(0,1fr)]">
   <aside className="space-y-3 rounded border df-border-neutral p-3 text-xs">
    <p className="font-semibold">Node palette</p>
    <p className="muted">SOURCE, SYNC, QUALITY and PUBLISH are included. Drag a saved model onto the canvas or click Add; connect handles to edit the order.</p>
    <label className="block space-y-1">Saved Visual model
     <select aria-label="Palette Core transform model" className="field" value={newModel} onChange={e=>setNewModel(e.target.value)}>
      <option value="">Choose model…</option>
      {activeModels.map(m=><option key={m.id} value={m.id}>{m.name}</option>)}
     </select>
    </label>
    <div draggable={!!newModel} className="rounded border df-border-accent p-3 cursor-grab"
     onDragStart={e=>{e.dataTransfer.setData("application/x-dataflow-core-model",newModel);e.dataTransfer.effectAllowed="copy";}}>
     TRANSFORM · drag
    </div>
    <Button size="sm" type="button" variant="outline" disabled={!newModel} onClick={()=>addTransform(newModel)}><Plus size={14}/>Add transform</Button>
    {inspected&&<div className="space-y-2 border-t df-border-neutral pt-3">
     <p className="font-semibold break-all">{inspected.type} · {inspected.id}</p>
     {inspected.type==="SOURCE"&&<label className="block space-y-1">Managed stream
      <select className="field" aria-label="Core graph source stream" value={inspected.stream_id??""} onChange={e=>mutate(g=>({nodes:g.nodes.map(n=>n.id===inspected.id?{...n,stream_id:e.target.value}:n)}))}>
       <option value="">Select stream…</option>
       {streams.filter(s=>s.target_database==="scratch"&&s.target_table==="dataflow_raw_events").map(s=><option key={s.id} value={s.id}>{s.source_schema}.{s.source_table}</option>)}
      </select>
     </label>}
     {inspected.type==="TRANSFORM"&&<label className="block space-y-1">Model
      <select className="field" aria-label="Core graph transform binding" value={inspected.model_id??""} onChange={e=>mutate(g=>({nodes:g.nodes.map(n=>n.id===inspected.id?{...n,model_id:e.target.value}:n)}))}>
       <option value="">Select…</option>{activeModels.map(m=><option key={m.id} value={m.id}>{m.name}</option>)}
      </select>
     </label>}
     {inspected.type==="QUALITY"&&<label className="block space-y-1">Unique key column
      <input className="field" aria-label="Core graph DQ key" value={inspected.unique_column??""}
       onChange={e=>mutate(g=>({nodes:g.nodes.map(n=>n.id===inspected.id?{...n,unique_column:e.target.value}:n)}))}/>
     </label>}
     {inspected.type==="TRANSFORM"&&<Button type="button" variant="destructive" size="sm" onClick={remove}><Trash2 size={14}/>Remove transform</Button>}
    </div>}
   </aside>
   <div className="h-[320px] min-w-0 rounded border df-border-neutral df-surface-soft"
    onDragOver={e=>{if(e.dataTransfer.types.includes("application/x-dataflow-core-model")){e.preventDefault();e.dataTransfer.dropEffect="copy"}}}
    onDrop={e=>{const model=e.dataTransfer.getData("application/x-dataflow-core-model");if(!model)return;e.preventDefault();addTransform(model,instance.current?.screenToFlowPosition({x:e.clientX,y:e.clientY}))}}>
    <ReactFlow<TypedNode,Edge> nodes={nodes} edges={edges} nodeTypes={nodeTypes}
     onInit={v=>{instance.current=v}} onNodesChange={onNodesChange}
     onConnect={connect}
     isValidConnection={c=>!!c.source&&!!c.target&&c.source!==c.target}
     onNodeClick={(_,n)=>setSelected(n.id)} onPaneClick={()=>setSelected(null)}
     fitView fitViewOptions={{padding:0.23,maxZoom:0.9}} minZoom={0.22} maxZoom={1.5}
     nodesDraggable nodesConnectable edgesReconnectable={false} deleteKeyCode={null}>
     <Background variant={BackgroundVariant.Lines} color="var(--signal-grid)" gap={24}/>
     <Controls showInteractive={false}/>
    </ReactFlow>
   </div>
  </div>
  <p className="muted text-xs">Supports only one connected, ordered SOURCE → SYNC → 1–30 TRANSFORM → QUALITY → PUBLISH chain. Parallel branches, raw SQL and unsaved graph execution are rejected by Go.</p>
  {validation&&<p className="df-text-success text-xs" role="status">Server validated graph digest: <code>{validation.digest.slice(0,16)}</code>…</p>}
  {saved&&!dirty&&<p className="muted text-xs">Persisted graph: {saved.name} · rev {saved.version} · {saved.id}</p>}
  {note&&<p className="df-text-success text-xs" role="status">{note}</p>}
  {error&&<p className="df-text-error text-xs" role="alert">{error}</p>}
 </section>;
}
