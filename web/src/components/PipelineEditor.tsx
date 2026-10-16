import {useState} from "react";
import {GitFork,ShieldCheck,AlertTriangle} from "lucide-react";
import {Button} from "./ui/button";
import {validatePipeline,type PipelinePlan,type PipelineValidation} from "../lib/api";

const example:PipelinePlan={nodes:[
 {id:"sync_invc_head",job_id:"invc-head-sync",requires:[]},
 {id:"sync_invc_dtl",job_id:"invc-dtl-sync",requires:[]},
 {id:"transform_revenue",job_id:"revenue-sql-job",requires:["sync_invc_head","sync_invc_dtl"]},
 {id:"quality_gate",job_id:"revenue-quality",requires:["transform_revenue"]}
]};
type Props={token:string;onTokenChange:(value:string)=>void};
export function PipelineEditor({token,onTokenChange}:Props){
 const[definition,setDefinition]=useState(JSON.stringify(example,null,2));
 const[result,setResult]=useState<PipelineValidation|null>(null);
 const[busy,setBusy]=useState(false);
 const[error,setError]=useState("");
 const validate=async()=>{
  setBusy(true);setError("");setResult(null);
  try{
   const raw=JSON.parse(definition) as PipelinePlan;
   if(!raw||!Array.isArray(raw.nodes)){throw new Error("JSON must have a nodes array")}
   setResult(await validatePipeline(raw,token));
  }catch(e){setError(String(e))}finally{setBusy(false)}
 };
 return <div className="grid gap-5 xl:grid-cols-2">
  <section className="section space-y-4">
   <div className="flex items-center gap-2 font-semibold"><GitFork size={19}/>Pipeline Definition (JSON)</div>
   <p className="muted text-sm">Edit node dependencies and validate against the Go DAG engine. Execution/scheduling are not enabled yet.</p>
   <label className="block text-xs text-slate-400">API Token</label><input className="field" type="password" value={token} onChange={e=>onTokenChange(e.target.value)} autoComplete="off"/>
   <textarea className="field min-h-[340px] font-mono text-xs leading-5" spellCheck={false} value={definition} onChange={e=>setDefinition(e.target.value)} aria-label="Pipeline definition JSON"/>
   <Button disabled={busy||!token} onClick={validate}><ShieldCheck size={16}/>{busy?"Validating…":"Validate DAG"}</Button>
   {error&&<p role="alert" className="rounded-md bg-red-950 p-3 text-sm text-red-300">{error}</p>}
  </section>
  <section className="section space-y-4">
   <div className="font-semibold">Dependency Preview</div>
   {(()=>{
    try{
     const p=JSON.parse(definition) as PipelinePlan;
     if(!Array.isArray(p.nodes))return <p className="muted text-sm">Expected nodes array</p>;
     return <div className="space-y-2">{p.nodes.map((n,i)=><div key={n.id+"-"+i} className="rounded-lg border border-slate-700 bg-slate-800/50 p-3"><div className="font-mono text-sm font-semibold text-sky-200">{n.id}</div><div className="muted text-xs">Job: {n.job_id}</div><div className="mt-1 text-xs text-slate-300">{n.requires?.length?"Depends on: "+n.requires.join(", "):"Entry node · no dependencies"}</div></div>)}</div>
    }catch{return <p className="muted text-sm">Fix JSON syntax to preview nodes</p>}
   })()}
   {result&&<div className={result.valid?"rounded-lg border border-emerald-800 bg-emerald-950/20 p-3":"rounded-lg border border-amber-800 bg-amber-950/20 p-3"}>
    <div className="flex items-center gap-2 text-sm font-semibold">{result.valid?<ShieldCheck size={16}/>:<AlertTriangle size={16}/>} {result.valid?"DAG valid":"DAG invalid"}</div>
    <p className="mt-1 text-sm">{result.valid?`Node count: ${result.nodes}. Initially ready: ${result.ready_on_start?.join(", ")||"none"}`:result.error}</p>
    <p className="muted mt-2 text-xs">Validation only — no jobs were started.</p>
   </div>}
  </section>
 </div>;
}
