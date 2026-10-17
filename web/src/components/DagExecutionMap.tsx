import {AlertTriangle,ArrowRight,CheckCircle2,CircleDashed,Clock3,GitBranch,PlayCircle,XCircle} from "lucide-react";
import type {PipelineNode} from "../lib/api";
import {Badge} from "./ui/badge";

type Props={
 plan?:PipelineNode[];
 state?:Record<string,string>;
 attempts?:Record<string,number>;
 errors?:Record<string,string>;
};
type VisualNode={id:string;requires:string[];level:number;status?:string;attempts?:number;error?:string};

function buildLevels(plan:PipelineNode[]|undefined,state:Record<string,string>={},attempts:Record<string,number>={},errors:Record<string,string>={}):VisualNode[][]{
 const source=plan?.length?plan.map(n=>({id:n.id,requires:n.requires??[]})):Object.keys(state).sort().map(id=>({id,requires:[] as string[]}));
 const known=new Set(source.map(n=>n.id));
 // Server validates persisted DAGs; this bounded layout still fails safe if
 // a historical definition is incomplete or references a removed step.
 const level=new Map(source.map(n=>[n.id,0]));
 for(let i=0;i<source.length;i++){
  let changed=false;
  for(const n of source){
   const upstream=n.requires.filter(dep=>known.has(dep));
   const next=upstream.length?Math.max(...upstream.map(dep=>level.get(dep)??0))+1:0;
   if(next>(level.get(n.id)??0)){level.set(n.id,next);changed=true}
  }
  if(!changed)break;
 }
 const result=new Map<number,VisualNode[]>();
 for(const n of source.slice(0,80)){
  // A cyclic/malformed stored DAG must not create unlimited screen columns.
  const depth=Math.min(level.get(n.id)??0,Math.min(source.length,12));
  result.set(depth,[...(result.get(depth)??[]),{id:n.id,requires:n.requires,level:depth,status:state[n.id],attempts:attempts[n.id],error:errors[n.id]}]);
 }
 return [...result].sort(([a],[b])=>a-b).map(([,nodes])=>nodes);
}
function variant(status:string|undefined):"success"|"danger"|"warning"|"neutral"{
 if(status==="succeeded")return "success";
 if(status==="failed"||status==="cancelled")return "danger";
 if(status==="queued"||status==="running")return "warning";
 return "neutral";
}
function StateIcon({status}:{status?:string}){
 if(status==="succeeded")return <CheckCircle2 className="text-emerald-400" size={17}/>;
 if(status==="failed"||status==="cancelled")return <XCircle className="text-red-400" size={17}/>;
 if(status==="running")return <PlayCircle className="text-amber-400" size={17}/>;
 if(status==="queued")return <Clock3 className="text-amber-400" size={17}/>;
 return <CircleDashed className="muted" size={17}/>;
}
export function DagExecutionMap({plan,state,attempts,errors}:Props){
 const columns=buildLevels(plan,state,attempts,errors);
 const visibleCount=columns.reduce((sum,column)=>sum+column.length,0);
 const evidenceCount=Object.keys(state??{}).length;
 if(visibleCount===0)return <div className="muted rounded-xl border border-border px-4 py-8 text-center text-sm">No DAG definition or node execution evidence is available for this run.</div>;
 return <section className="space-y-3" aria-label="DAG execution map">
  <div className="flex flex-wrap items-center justify-between gap-3">
   <div><h4 className="flex items-center gap-2 text-sm font-semibold"><GitBranch size={16}/>Execution graph</h4><p className="muted mt-1 text-xs">Each node shows its actual stored state and recorded dependencies.</p></div>
   <div className="flex flex-wrap gap-2"><Badge variant="neutral">{visibleCount} shown</Badge><Badge variant="default">{evidenceCount} node results</Badge></div>
  </div>
  <div className="enterprise-dag-canvas overflow-x-auto rounded-xl border border-border p-4">
   <div className="flex min-w-max items-stretch gap-3">
    {columns.map((column,i)=><div key={i} className="flex items-center gap-3">
     {i>0&&<ArrowRight size={18} className="shrink-0 text-primary" aria-hidden="true"/>}
     <div className="flex w-[250px] flex-col gap-3">
      <div className="muted text-[11px] font-semibold uppercase tracking-wider">Stage {i+1}</div>
      {column.map(node=><div className="enterprise-dag-node rounded-xl border p-3" key={node.id}>
       <div className="flex items-start justify-between gap-2"><div className="flex min-w-0 items-center gap-2"><StateIcon status={node.status}/><span className="break-all text-xs font-bold">{node.id}</span></div><Badge variant={variant(node.status)}>{node.status??"No result yet"}</Badge></div>
       <div className="mt-3 space-y-1.5 text-[11px]">
        <p className="muted">Attempts: {node.attempts===undefined?"Not reported":node.attempts}</p>
        {node.requires.length>0?<div className="muted flex flex-wrap items-start gap-1"><span>Needs:</span>{node.requires.map(dep=><span key={dep} className="enterprise-dependency rounded px-1.5 py-0.5 font-mono">{dep}</span>)}</div>:<p className="muted">Entry node</p>}
       </div>
       {node.error&&<div className="enterprise-node-error mt-3 flex items-start gap-2 rounded-lg p-2 text-[11px]"><AlertTriangle size={13} className="mt-0.5 shrink-0"/><span className="min-w-0 break-words">{node.error}</span></div>}
      </div>)}
     </div>
    </div>)}
   </div>
  </div>
  {plan&&plan.length>80&&<p className="muted text-xs">Showing the first 80 nodes out of {plan.length}. Open the full DAG definition to inspect the rest.</p>}
  <p className="muted text-xs">Stage columns are a layout of the saved dependencies, not a timestamped execution trace. A missing state means the API did not report an outcome.</p>
 </section>;
}
