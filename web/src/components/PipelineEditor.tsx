import {useMemo,useRef,useState} from "react";
import {AlertTriangle,Code2,GitFork,RefreshCw,Save,ShieldCheck} from "lucide-react";
import {Button} from "./ui/button";
import {ConfirmActionDialog} from "./ui/confirm-action-dialog";
import {VisualPipelineCanvas} from "./VisualPipelineCanvas";
import {PipelineRunInspector,type CanvasRunRequest} from "./PipelineRunInspector";
import {CorePipelineRunner} from "./CorePipelineRunner";
import {sqlDAGApi,validatePipeline,type ExecutableDAG,type PipelineNode,type PipelinePlan,type PipelineValidation,type SQLJob} from "../lib/api";

type Props={token:string;onTokenChange:(value:string)=>void;theme:"light"|"dark";onOpenOperations:()=>void};
type SavePayload=Omit<ExecutableDAG,"id">;
type PendingSave={payload:SavePayload;version:number;existing:boolean};
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
export function PipelineEditor({token,onTokenChange,theme,onOpenOperations}:Props){
 const[definition,setDefinition]=useState('{\n  "nodes": []\n}');
 const[jobs,setJobs]=useState<SQLJob[]>([]);
 const[savedDags,setSavedDags]=useState<ExecutableDAG[]>([]);
 const[selectedSaved,setSelectedSaved]=useState("");
 const[name,setName]=useState("new_pipeline");
 const[enabled,setEnabled]=useState(false);
 const[busyJobs,setBusyJobs]=useState(false);
 const[busyDags,setBusyDags]=useState(false);
 const[busyValidation,setBusyValidation]=useState(false);
 const[busySave,setBusySave]=useState(false);
 const[busyRunCanvas,setBusyRunCanvas]=useState(false);
 const[canvasRunRequest,setCanvasRunRequest]=useState<CanvasRunRequest|null>(null);
 const[jobError,setJobError]=useState("");
 const[error,setError]=useState("");
 const[notice,setNotice]=useState("");
 const[result,setResult]=useState<PipelineValidation|null>(null);
 const[pendingSave,setPendingSave]=useState<PendingSave|null>(null);
 const version=useRef(0);
 const parsed=useMemo(()=>readPlan(definition),[definition]);
 const matchingSaved=savedDags.find(d=>d.id===selectedSaved&&d.name===name.trim()&&d.enabled&&enabled&&parsed.plan&&JSON.stringify(d.nodes)===JSON.stringify(parsed.plan.nodes));
 const canRunCanvas=!!token&&!!result?.valid&&!!matchingSaved&&!busySave&&!busyValidation&&!busyRunCanvas;
 const runHint=!token?"Enter an API token to run.":!result?.valid?"Validate this canvas before running.":!matchingSaved?"Save this exact DAG with TEST execution enabled before running.":"Run the validated and saved DAG in TEST after confirmation.";
 const touched=()=>{
  version.current++;
  setResult(null);setError("");setNotice("");setPendingSave(null);
 };
 const updatePlan=(plan:PipelinePlan)=>{
  touched();setDefinition(JSON.stringify(plan,null,2));
 };
 const updateJSON=(value:string)=>{touched();setDefinition(value);};
 const updateToken=(value:string)=>{
  touched();setCanvasRunRequest(null);onTokenChange(value);setJobs([]);setSavedDags([]);setSelectedSaved("");setJobError("");
 };
 const loadJobs=async()=>{
  if(!token)return;
  setBusyJobs(true);setJobError("");
  try{setJobs((await sqlDAGApi.jobs(token)).jobs);}
  catch(e){setJobs([]);setJobError(String(e));}
  finally{setBusyJobs(false);}
 };
 const requestRunCanvas=async()=>{
  if(!canRunCanvas||!matchingSaved)return;
  const revision=version.current;
  setBusyRunCanvas(true);setError("");
  try{
   // Re-read server state: an old selected local DAG ID must not authorize a different graph.
   const response=await sqlDAGApi.dags(token);
   if(version.current!==revision)return;
   setSavedDags(response.pipelines);
   const latest=response.pipelines.find(d=>d.id===matchingSaved.id);
   if(!latest||!latest.enabled||latest.name!==matchingSaved.name||JSON.stringify(latest.nodes)!==JSON.stringify(matchingSaved.nodes)){
    setError("The saved DAG changed or is disabled on the server. Reload, validate and save before running.");return;
   }
   setCanvasRunRequest({dag:latest,revision});
  }catch(e){if(version.current===revision)setError("Could not verify saved DAG: "+String(e));}
  finally{setBusyRunCanvas(false);}
 };
 const loadSaved=async()=>{
  if(!token)return;
  setBusyDags(true);setError("");
  try{setSavedDags((await sqlDAGApi.dags(token)).pipelines);}
  catch(e){setError(String(e));}
  finally{setBusyDags(false);}
 };
 const importSaved=()=>{
  const found=savedDags.find(d=>d.id===selectedSaved);
  if(!found)return;
  touched();setDefinition(JSON.stringify({nodes:found.nodes},null,2));
  setName(found.name);setEnabled(found.enabled);
  setNotice("Loaded saved pipeline into the browser editor. Validate changes before saving; execution is not started.");
 };
 const validate=async()=>{
  if(!parsed.plan||!token)return;
  const revision=version.current;
  setBusyValidation(true);setError("");setResult(null);
  try{
   const response=await validatePipeline(parsed.plan,token);
   if(version.current===revision)setResult(response);
  }catch(e){if(version.current===revision)setError(String(e));}
  finally{setBusyValidation(false);}
 };
 const persist=async(payload:SavePayload,revision:number)=>{
  if(version.current!==revision){setError("Draft changed since the save request. Validate and try again.");return;}
  setBusySave(true);setError("");setNotice("");
  try{
   const saved=await sqlDAGApi.saveDAG(payload,token);
   if(version.current===revision){
    setNotice("Saved pipeline "+saved.name+" (ID: "+saved.id+"). No job was started.");
    setSelectedSaved(saved.id);
    setSavedDags((await sqlDAGApi.dags(token)).pipelines);
   }
  }catch(e){if(version.current===revision)setError(String(e));}
  finally{setBusySave(false);}
 };
 const prepareSave=async()=>{
  if(!parsed.plan||!result?.valid||!token)return;
  const pipelineName=name.trim();
  if(!pipelineName||pipelineName.length>120){setError("Pipeline name must be 1–120 characters.");return;}
  if(parsed.plan.nodes.length===0){setError("Add at least one SQL Job before saving.");return;}
  if(!jobs.length){setError("Load registered SQL Jobs before saving so job references can be checked.");return;}
  const registered=new Map(jobs.map(j=>[j.id,j]));
  const unregistered=parsed.plan.nodes.filter(n=>!registered.has(n.job_id));
  if(unregistered.length){setError("Unregistered SQL job_id on node(s): "+unregistered.map(n=>n.id).join(", "));return;}
  const disabled=parsed.plan.nodes.filter(n=>!registered.get(n.job_id)?.enabled);
  if(enabled&&disabled.length){setError("Disable TEST execution or enable these SQL Jobs separately: "+disabled.map(n=>n.id).join(", "));return;}
  const revision=version.current;
  const payload:SavePayload={name:pipelineName,enabled,nodes:parsed.plan.nodes};
  setBusySave(true);setError("");setNotice("");
  try{
   const response=await sqlDAGApi.dags(token);
   if(revision!==version.current)return;
   setSavedDags(response.pipelines);
   const existing=response.pipelines.some(p=>p.name===pipelineName);
   if(existing||enabled)setPendingSave({payload,version:revision,existing});
   else await persist(payload,revision);
  }catch(e){if(revision===version.current)setError(String(e));}
  finally{setBusySave(false);}
 };
 return <section className="df-studio-workspace df-pipeline-workspace space-y-5">
  <div className="df-studio-panel section space-y-4">
   <div className="flex flex-wrap items-start justify-between gap-3">
    <div><h2 className="flex items-center gap-2 text-lg font-semibold"><GitFork size={19}/>Visual Pipeline Builder <span className="signal-inline-code">/ V2.4</span></h2>
     <p className="muted mt-1 max-w-3xl text-sm">Design and validate the SQL Job DAG, save it to the PostgreSQL-backed registry, then explicitly run an enabled TEST pipeline and inspect real backend outcomes. No automatic execution or scheduling.</p></div>
    <span className="df-pipeline-stat">DESIGN / SAVE / TEST RUN</span>
   </div>
   {token!=="session"&&<label className="block max-w-lg space-y-1"><span className="muted text-xs">API token (memory only)</span>
    <input aria-label="Pipeline API token" className="field" type="password" autoComplete="off" value={token} onChange={event=>updateToken(event.target.value)}/>
   </label>}
   {jobError&&<p role="alert" className="df-text-error text-xs">Could not load SQL Jobs: {jobError}</p>}
   {parsed.plan?<VisualPipelineCanvas plan={parsed.plan} jobs={jobs} onChange={updatePlan} onReloadJobs={()=>{void loadJobs();}} loadingJobs={busyJobs} canLoadJobs={Boolean(token)} onRunCanvas={()=>{void requestRunCanvas();}} canRunCanvas={canRunCanvas} runBusy={busyRunCanvas} runHint={runHint}/>:
    <div className="rounded border df-border-warning df-bg-warning-soft p-4 text-sm df-text-warning" role="alert"><AlertTriangle size={16} className="mr-2 inline"/>Canvas paused: {parsed.issue}</div>}
  </div>
  <div className="df-studio-panel section space-y-4">
   <div className="flex flex-wrap items-center justify-between gap-2">
    <h3 className="flex items-center gap-2 font-semibold"><Code2 size={17}/>Pipeline Definition (JSON)</h3>
    <span className="muted text-xs">Advanced edit · changes update the graph when valid</span>
   </div>
   <textarea aria-label="Pipeline definition JSON" spellCheck={false} className="field min-h-[170px] font-mono text-xs leading-5" value={definition} onChange={event=>updateJSON(event.target.value)}/>
   {parsed.issue&&<p className="df-text-error text-xs" role="alert">{parsed.issue}</p>}
   <div className="flex flex-wrap items-center gap-3">
    <Button type="button" disabled={busyValidation||busySave||!token||!parsed.plan} onClick={()=>{void validate();}}><ShieldCheck size={16}/>{busyValidation?"Validating…":"Validate DAG"}</Button>
    <p className="muted text-xs">Required before Save. Validation only; no SQL Job runs.</p>
   </div>
   {result&&<div role="status" className={result.valid?"rounded border df-border-success df-bg-success-soft p-3":"rounded border df-border-warning df-bg-warning-soft p-3"}>
    <div className="flex items-center gap-2 text-sm font-semibold">{result.valid?<ShieldCheck size={16}/>:<AlertTriangle size={16}/>} {result.valid?"DAG valid":"DAG invalid"}</div>
    <p className="muted mt-1 text-xs">{result.valid?"Node count: "+result.nodes+". Initially ready: "+(result.ready_on_start?.join(", ")||"none"):result.error}</p>
   </div>}
  </div>
  <div className="df-studio-panel section space-y-4" aria-label="Pipeline save workspace">
   <div><h3 className="flex items-center gap-2 font-semibold"><Save size={17}/>Save pipeline</h3><p className="muted mt-1 text-xs">A named pipeline is upserted by the existing backend. Existing names require explicit overwrite confirmation. Execution is never triggered here.</p></div>
   <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-end">
    <label className="block space-y-1"><span className="muted text-xs">Pipeline name</span>
     <input aria-label="Pipeline name" className="field" maxLength={120} value={name} onChange={e=>{touched();setName(e.target.value);}}/>
    </label>
    <Button type="button" disabled={!result?.valid||!parsed.plan||!token||busySave||busyValidation} onClick={()=>{void prepareSave();}}><Save size={15}/>{busySave?"Saving…":"Save pipeline"}</Button>
   </div>
   <label className="flex items-center gap-2 text-sm"><input aria-label="Enable TEST execution" type="checkbox" checked={enabled} onChange={e=>{touched();setEnabled(e.target.checked);}}/>
    Enable TEST execution for this saved DAG (default OFF; enabling requires confirmation)
   </label>
   {enabled&&<p className="df-text-warning text-xs">Only enabled SELECT-only SQL Jobs in a scratch test environment can run. Saving this flag does not start a run.</p>}
   {notice&&<p role="status" className="df-text-success text-xs">{notice}</p>}
   {error&&<p role="alert" className="rounded border df-border-error df-bg-error-soft p-3 text-sm df-text-error">{error}</p>}
   <div className="border-t df-border-neutral pt-4">
    <div className="flex flex-wrap items-center justify-between gap-2"><h4 className="font-semibold text-sm">Saved SQL DAG registry</h4>
     <Button type="button" size="sm" variant="outline" disabled={!token||busyDags} onClick={()=>{void loadSaved();}}><RefreshCw size={14}/>{busyDags?"Loading…":"Load saved DAGs"}</Button>
    </div>
    <p className="muted mt-1 text-xs">Select a saved DAG to copy its name, enable flag and dependencies into the editor. Unsaved local edits will be replaced; no backend writes occur until Save.</p>
    <div className="mt-3 flex flex-wrap items-end gap-2">
     <label className="min-w-[230px] flex-1 space-y-1"><span className="muted text-xs">Saved pipeline</span>
      <select aria-label="Saved pipeline" className="field" value={selectedSaved} onChange={e=>setSelectedSaved(e.target.value)}>
       <option value="">Select saved pipeline…</option>{savedDags.map(d=><option key={d.id} value={d.id}>{d.name}{d.enabled?" (enabled)":""}</option>)}
      </select>
     </label>
     <Button type="button" size="sm" variant="outline" disabled={!selectedSaved} onClick={importSaved}>Load into editor</Button>
    </div>
   </div>
  </div>
   <PipelineRunInspector token={token} dags={savedDags} selectedId={selectedSaved} theme={theme} onOpenOperations={onOpenOperations} canvasRunRequest={canvasRunRequest} currentRevision={version.current}/>
  <CorePipelineRunner token={token} onTokenChange={updateToken}/>
  {pendingSave&&<ConfirmActionDialog title={pendingSave.existing?"Overwrite saved pipeline?":"Enable TEST pipeline?"}
   message={pendingSave.existing?"This pipeline name already exists. Saving replaces its saved DAG definition and enabled flag. Ensure no conflicting test run is active.":"Enabling allows authorized users to start this SQL DAG later under backend TEST guards. Saving does not start execution."}
   target={pendingSave.payload.name} targetLabel="Named SQL DAG · verify before saving"
   confirmLabel={pendingSave.existing?"Overwrite & save":"Enable & save"} onCancel={()=>setPendingSave(null)}
   onConfirm={()=>{const requested=pendingSave;setPendingSave(null);void persist(requested.payload,requested.version);}}/>}
 </section>;
}
