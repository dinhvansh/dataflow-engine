import {useMemo,useState} from "react";
import {AlertTriangle,Code2,GitFork,ShieldCheck} from "lucide-react";
import {Button} from "./ui/button";
import {VisualPipelineCanvas} from "./VisualPipelineCanvas";
import {sqlDAGApi,validatePipeline,type PipelineNode,type PipelinePlan,type PipelineValidation,type SQLJob} from "../lib/api";

type Props={token:string;onTokenChange:(value:string)=>void};
function readPlan(raw:string):{plan:PipelinePlan|null;issue:string}{
 try{
  const value:unknown=JSON.parse(raw);
  if(!value||typeof value!=="object"||!Array.isArray((value as {nodes?:unknown}).nodes)){
   return {plan:null,issue:"Definition must be an object with a nodes array."};
  }
  const nodes=(value as {nodes:unknown[]}).nodes;
  const valid=(item:unknown):item is PipelineNode=>{
   if(!item||typeof item!=="object")return false;
   const n=item as Partial<PipelineNode>;
   return typeof n.id==="string"&&n.id.trim().length>0&&
    typeof n.job_id==="string"&&n.job_id.trim().length>0&&
    Array.isArray(n.requires)&&n.requires.every(dep=>typeof dep==="string");
  };
  if(!nodes.every(valid))return {plan:null,issue:"Each node needs a nonempty id, job_id and string[] requires."};
  const typed=nodes as PipelineNode[];
  if(new Set(typed.map(n=>n.id)).size!==typed.length)return {plan:null,issue:"Node IDs must be unique."};
  return {plan:{nodes:typed.map(n=>({id:n.id,job_id:n.job_id,requires:[...n.requires]}))},issue:""};
 }catch{
  return {plan:null,issue:"Invalid JSON syntax. Fix the definition before editing the canvas."};
 }
}
export function PipelineEditor({token,onTokenChange}:Props){
 const[definition,setDefinition]=useState('{\n  "nodes": []\n}');
 const[jobs,setJobs]=useState<SQLJob[]>([]);
 const[busyJobs,setBusyJobs]=useState(false);
 const[busyValidation,setBusyValidation]=useState(false);
 const[jobError,setJobError]=useState("");
 const[error,setError]=useState("");
 const[result,setResult]=useState<PipelineValidation|null>(null);
 const parsed=useMemo(()=>readPlan(definition),[definition]);
 const updatePlan=(plan:PipelinePlan)=>{
  setDefinition(JSON.stringify(plan,null,2));
  setResult(null);setError("");
 };
 const updateJSON=(value:string)=>{
  setDefinition(value);setResult(null);setError("");
 };
 const loadJobs=async()=>{
  if(!token)return;
  setBusyJobs(true);setJobError("");
  try{
   const response=await sqlDAGApi.jobs(token);
   setJobs(response.jobs);
  }catch(e){setJobs([]);setJobError(String(e));}finally{setBusyJobs(false);}
 };
 const validate=async()=>{
  if(!parsed.plan||!token)return;
  setBusyValidation(true);setError("");setResult(null);
  try{setResult(await validatePipeline(parsed.plan,token));}
  catch(e){setError(String(e));}
  finally{setBusyValidation(false);}
 };
 return <section className="df-studio-workspace df-pipeline-workspace space-y-5">
  <div className="df-studio-panel section space-y-4">
   <div className="flex flex-wrap items-start justify-between gap-3">
    <div><h2 className="flex items-center gap-2 text-lg font-semibold"><GitFork size={19}/>Visual Pipeline Builder <span className="signal-inline-code">/ V2.1</span></h2>
     <p className="muted mt-1 max-w-3xl text-sm">Canvas and JSON describe the same DAG draft. This stage can load SQL Jobs and validate a plan; it cannot save, schedule or execute a pipeline. Existing executable SQL DAGs remain managed below.</p></div>
    <span className="df-pipeline-stat">DESIGN / VALIDATE ONLY</span>
   </div>
   <label className="block max-w-lg space-y-1"><span className="muted text-xs">API token (memory only)</span>
    <input aria-label="Pipeline API token" className="field" type="password" autoComplete="off" value={token} onChange={event=>{onTokenChange(event.target.value);setJobs([]);setJobError("");}}/>
   </label>
   {jobError&&<p role="alert" className="df-text-error text-xs">Could not load SQL Jobs: {jobError}</p>}
   {parsed.plan?
    <VisualPipelineCanvas plan={parsed.plan} jobs={jobs} onChange={updatePlan} onReloadJobs={()=>{void loadJobs();}} loadingJobs={busyJobs} canLoadJobs={Boolean(token)}/>:
    <div className="rounded border df-border-warning df-bg-warning-soft p-4 text-sm df-text-warning" role="alert"><AlertTriangle size={16} className="mr-2 inline"/>Canvas paused: {parsed.issue}</div>
   }
  </div>
  <div className="df-studio-panel section space-y-4">
   <div className="flex flex-wrap items-center justify-between gap-2">
    <h3 className="flex items-center gap-2 font-semibold"><Code2 size={17}/>Pipeline Definition (JSON)</h3>
    <span className="muted text-xs">Advanced edit · changes update the graph immediately when valid</span>
   </div>
   <textarea aria-label="Pipeline definition JSON" spellCheck={false} className="field min-h-[170px] font-mono text-xs leading-5" value={definition} onChange={event=>updateJSON(event.target.value)}/>
   {parsed.issue&&<p className="df-text-error text-xs" role="alert">{parsed.issue}</p>}
   <div className="flex flex-wrap items-center gap-3">
    <Button type="button" disabled={busyValidation||!token||!parsed.plan} onClick={()=>{void validate();}}><ShieldCheck size={16}/>{busyValidation?"Validating…":"Validate DAG"}</Button>
    <p className="muted text-xs">Validation calls the existing Go endpoint. No SQL Job is executed.</p>
   </div>
   {error&&<p role="alert" className="rounded border df-border-error df-bg-error-soft p-3 text-sm df-text-error">{error}</p>}
   {result&&<div role="status" className={result.valid?"rounded border df-border-success df-bg-success-soft p-3":"rounded border df-border-warning df-bg-warning-soft p-3"}>
    <div className="flex items-center gap-2 text-sm font-semibold">{result.valid?<ShieldCheck size={16}/>:<AlertTriangle size={16}/>} {result.valid?"DAG valid":"DAG invalid"}</div>
    <p className="muted mt-1 text-xs">{result.valid?"Node count: "+result.nodes+". Initially ready: "+(result.ready_on_start?.join(", ")||"none"):result.error}</p>
    <p className="muted mt-2 text-xs">Validation only — no jobs were started.</p>
   </div>}
  </div>
 </section>;
}
