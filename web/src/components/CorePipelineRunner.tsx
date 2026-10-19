import {useEffect,useState} from "react";
import {Database,Play,RefreshCw,RotateCcw,ShieldAlert} from "lucide-react";
import {Button} from "./ui/button";
import {ConfirmActionDialog} from "./ui/confirm-action-dialog";
import {corePipelineApi,managedApi,type CorePipelineRun,type ManagedStream,type SavedCoreGraph} from "../lib/api";
import {transformApi,type SavedTransform} from "../lib/transformApi";
import {CoreRuntimeCanvas} from "./CoreRuntimeCanvas";
import {CoreMixedGraphEditor} from "./CoreMixedGraphEditor";

// Minimal operator surface for the *real* server-owned CORE-RUN pipeline.
// This is intentionally separate from the older in-process SQL DAG Canvas
// executor. The UI NEVER manufactures a successful status or triggers
// constituent workers directly.
const terminal=new Set(["succeeded","failed","rolled_back"]);
type Confirm="run"|"graph_run"|"rollback"|null;
type Props={token:string;onTokenChange:(next:string)=>void};
export function CorePipelineRunner({token,onTokenChange}:Props){
 const[streams,setStreams]=useState<ManagedStream[]>([]);
 const[models,setModels]=useState<SavedTransform[]>([]);
 const[streamId,setStreamId]=useState("");
 const[modelId,setModelId]=useState("");
 const[uniqueColumn,setUniqueColumn]=useState("stream");
 const[runID,setRunID]=useState("");
 const[run,setRun]=useState<CorePipelineRun|null>(null);
 const[busy,setBusy]=useState(false);
 const[refreshing,setRefreshing]=useState(false);
 const[error,setError]=useState("");
 const[pollError,setPollError]=useState("");
 const[confirm,setConfirm]=useState<Confirm>(null);
 const[pendingGraph,setPendingGraph]=useState<SavedCoreGraph|null>(null);
 const[lastPlan,setLastPlan]=useState<string[]>([]);

 const eligibleModels=models.filter(m=>m.model.mode==="visual"&&
  m.model.source.database==="scratch");
 const eligibleStreams=streams.filter(s=>s.target_database==="scratch"&&
  s.target_table==="dataflow_raw_events");
 const selectedStream=eligibleStreams.find(s=>s.id===streamId);
 const selectedModel=eligibleModels.find(m=>m.id===modelId);
 const actionable=Boolean(token&&selectedStream&&selectedModel&&
  /^[A-Za-z_][A-Za-z0-9_]{0,84}$/.test(uniqueColumn));
 const load=async()=>{
  if(!token)return;
  setBusy(true);setError("");
  try{
   const [available,registry]=await Promise.all([managedApi.list(token),transformApi.models(token)]);
   const nextStreams=available.streams.filter(s=>s.target_database==="scratch"&&s.target_table==="dataflow_raw_events");
   const nextModels=registry.models.filter(m=>m.model.mode==="visual"&&m.model.source.database==="scratch");
   setStreams(nextStreams);setModels(nextModels);
   setStreamId(p=>nextStreams.some(s=>s.id===p)?p:(nextStreams[0]?.id??""));
   setModelId(p=>nextModels.some(m=>m.id===p)?p:(nextModels[0]?.id??""));
  }catch(e){setStreams([]);setModels([]);setError("Cannot load available server-defined streams/models: "+String(e))}
  finally{setBusy(false)}
 };
 const inspect=async(id=runID)=>{
  if(!token||!id.trim())return;
  setRefreshing(true);setError("");setPollError("");
  try{
   const observed=await corePipelineApi.status(id.trim(),token);
   setRun(observed);setRunID(observed.run_id);
  }catch(e){setError("Could not inspect core run: "+String(e))}
  finally{setRefreshing(false)}
 };
 const start=async(graph?:SavedCoreGraph)=>{
  if(graph&&!token)return;
  if(!graph&&(!actionable||!selectedModel||!selectedStream))return;
  setBusy(true);setError("");setPollError("");setRun(null);setLastPlan([]);
  try{
   const response=await corePipelineApi.start(graph?.stream_id??selectedStream!.id,
     graph?.model_id??selectedModel!.id,graph?.unique_column??uniqueColumn,token,graph);
   setRunID(response.run_id);setLastPlan(response.dependency_order??[]);
   // GET is the authoritative state including linked child run status.
   const observed=await corePipelineApi.status(response.run_id,token);
   setRun(observed);
  }catch(e){setError("Core Run not started or status unavailable: "+String(e))}
  finally{setBusy(false)}
 };
 const rollback=async()=>{
  if(!run||run.status!=="succeeded"||!run.has_backup)return;
  const current=run.run_id;setBusy(true);setError("");setPollError("");
  try{
   await corePipelineApi.rollback(current,token);
   // Always trust server GET over optimistic 'rolled_back' UI state.
   setRun(await corePipelineApi.status(current,token));
  }catch(e){setError("Rollback could not be confirmed. Inspect the server state before retry: "+String(e))}
  finally{setBusy(false)}
 };
 useEffect(()=>{
  // Drop previously observed run on token replacement; do not show a status
  // fetched under another operator's credentials.
  setRun(null);setRunID("");setLastPlan([]);setPollError("");setConfirm(null);setPendingGraph(null);
 },[token]);
 useEffect(()=>{
  if(!token||!run||terminal.has(run.status))return;
  let cancelled=false;let fetching=false;
  const handle=setInterval(()=>{
   if(cancelled||fetching)return;
   fetching=true;
   void corePipelineApi.status(run.run_id,token).then(next=>{
    if(cancelled)return;
    setRun(previous=>previous?.run_id===next.run_id?next:previous);
    setPollError("");
   }).catch(e=>{
    if(!cancelled)setPollError("Live status temporarily unavailable: "+String(e));
   }).finally(()=>{fetching=false});
  },3000);
  return()=>{cancelled=true;clearInterval(handle)}
 },[run?.run_id,run?.status,token]);

 return <section className="df-studio-panel section space-y-4" aria-label="Durable core pipeline operator">
  <div className="flex flex-wrap items-start justify-between gap-3">
   <div><h3 className="flex items-center gap-2 font-semibold"><Database size={17}/>Real Core Pipeline / QA</h3>
    <p className="muted mt-1 max-w-3xl text-sm">Managed SQL Server read-only → ClickHouse raw → saved dbt dependencies → Data Quality → versioned publish. The backend persists every linked run in PostgreSQL.</p></div>
   <span className="rounded-md df-bg-warning-soft px-2 py-1 text-xs df-text-warning">ISOLATED SCRATCH ONLY</span>
  </div>
  <p className="flex items-start gap-2 rounded border df-border-warning df-bg-warning-soft p-3 text-xs df-text-warning">
   <ShieldAlert size={17} className="shrink-0"/>
   This Core runtime graph observes the real durable pipeline; the editable SQL DAG above still uses a separate legacy executor. Backend CORE QA flags and migrations 028/029/030 are required. The mixed Canvas below persists strictly linear typed flows and launches immutable graph revisions; fork/join and production cutover are not yet supported. No production cutover, live ERP writes or verified 7M throughput.
  </p>
  <label className="block max-w-xl space-y-1"><span className="muted text-xs">API bearer token (memory only)</span>
   <input aria-label="Core API bearer token" className="field" type="password" autoComplete="off"
    value={token} onChange={e=>onTokenChange(e.target.value)} placeholder="Enter TEST API token"/>
  </label>
  <Button type="button" size="sm" variant="outline" disabled={!token||busy} onClick={()=>{void load()}}>
   <RefreshCw size={15}/>{busy?"Loading…":"Load saved streams and models"}
  </Button>
  <div className="grid gap-3 md:grid-cols-3">
   <label className="space-y-1 text-xs"><span className="muted">SELECT-only managed source stream</span>
    <select aria-label="Core source stream" className="field" value={streamId} onChange={e=>setStreamId(e.target.value)}>
     <option value="">Choose scratch raw stream…</option>
     {eligibleStreams.map(s=><option value={s.id} key={s.id}>{s.source_schema}.{s.source_table} → {s.target_database}.{s.target_table}</option>)}
    </select>
   </label>
   <label className="space-y-1 text-xs"><span className="muted">Saved final Visual model</span>
    <select aria-label="Core final model" className="field" value={modelId} onChange={e=>setModelId(e.target.value)}>
     <option value="">Choose saved model…</option>{eligibleModels.map(m=><option key={m.id} value={m.id}>{m.name}</option>)}
    </select>
   </label>
   <label className="space-y-1 text-xs"><span className="muted">Final output unique key (DQ gate)</span>
    <input aria-label="Core final output unique key" className="field" maxLength={85}
     value={uniqueColumn} onChange={e=>setUniqueColumn(e.target.value)} placeholder="stream"/>
   </label>
  </div>
  <CoreMixedGraphEditor token={token} streams={eligibleStreams} models={eligibleModels}
   active={busy||!!run&&!terminal.has(run.status)}
   onRun={graph=>{setPendingGraph(graph);setConfirm("graph_run")}}/>
  <CoreRuntimeCanvas run={run}
   sourceLabel={run?
    (eligibleStreams.find(s=>s.id===run.stream_id)
      ? (()=>{const s=eligibleStreams.find(s=>s.id===run.stream_id)!;return s.source_schema+"."+s.source_table})()
      : "Saved stream "+(run.stream_id??"unavailable")):
     (selectedStream?selectedStream.source_schema+"."+selectedStream.source_table:"")}
   modelLabel={selectedModel?.name??""}/>
  <div className="flex flex-wrap items-center gap-3">
   <Button type="button" disabled={!actionable||busy||!!run&&!terminal.has(run.status)}
    onClick={()=>setConfirm("run")}><Play size={15}/>Run complete core pipeline</Button>
   <span className="muted text-xs">Only backend-validated, saved dependency chains are accepted. No unsaved Canvas draft is executed.</span>
  </div>
  <div className="rounded border df-border-neutral df-surface-soft p-3 space-y-3">
   <label className="block space-y-1"><span className="muted text-xs">Inspect existing persisted Core Run UUID</span>
    <input aria-label="Core run UUID" className="field font-mono text-xs" value={runID}
     onChange={e=>{setRunID(e.target.value);setRun(null);setLastPlan([])}} placeholder="Paste /api/v1/core/runs run_id"/>
   </label>
   <Button type="button" size="sm" variant="outline" disabled={!token||!runID||refreshing}
    onClick={()=>{void inspect()}}><RefreshCw size={14}/>{refreshing?"Inspecting…":"Inspect backend state"}</Button>
   {run&&<div className="space-y-3" aria-label="Observed real core pipeline run">
    <div className="flex flex-wrap items-center justify-between gap-2">
     <div><span className={"font-semibold "+(run.status==="succeeded"?"df-text-success":run.status==="failed"?"df-text-error":"df-text-warning")} role="status">{run.status}</span>
      <p className="muted mt-1 break-all font-mono text-xs">{run.run_id}</p></div>
     <div className="text-right text-sm">Published rows: <strong>{(run.rows_verified??0).toLocaleString()}</strong><p className="muted text-xs">{run.target??"Pending target"}</p></div>
    </div>
    <div className="grid gap-2 sm:grid-cols-3 text-xs">
     <div>Source ingestion: <strong>{run.managed_status||"waiting"}</strong></div>
     <div>dbt + quality: <strong>{run.model_flow_status||"waiting"}</strong></div>
     <div>Publication: <strong>{run.status}</strong></div>
    </div>
    {(run.dependency_order?.length||lastPlan.length)>0&&<p className="muted text-xs">Server dependency order: {(run.dependency_order?.length?run.dependency_order:lastPlan).join(" → ")}</p>}
    {run.error&&<p className="df-text-error text-xs" role="alert">{run.error}</p>}
    {run.status==="succeeded"&&run.has_backup&&<Button type="button" size="sm" variant="destructive" disabled={busy}
     onClick={()=>setConfirm("rollback")}><RotateCcw size={14}/>Rollback to previous physical version</Button>}
    {run.status==="succeeded"&&!run.has_backup&&<p className="muted text-xs">Initial publication has no previous version to restore.</p>}
    {run.status==="rolled_back"&&<p className="df-text-success text-xs">Rollback acknowledged by PostgreSQL. Checkpoint unchanged.</p>}
   </div>}
   {pollError&&<p className="df-text-warning text-xs" role="alert">{pollError}</p>}
  </div>
  {error&&<p className="df-text-error text-sm" role="alert">{error}</p>}
  {confirm==="run"&&selectedModel&&selectedStream&&<ConfirmActionDialog
   title="Start real core execution in disposable QA?"
   target={selectedStream.source_schema+"."+selectedStream.source_table+" → "+selectedModel.name}
   targetLabel="Persisted managed stream → saved final model"
   message="This creates a durable PostgreSQL run, reads only the approved SQL Server source and publishes to ClickHouse scratch after dbt and DQ succeed. Requires the isolated QA environment."
   confirmLabel="Start CORE Run" onCancel={()=>setConfirm(null)}
   onConfirm={()=>{setConfirm(null);void start()}}/>}
  {confirm==="graph_run"&&pendingGraph&&<ConfirmActionDialog
   title="Run saved immutable Core graph in disposable QA?"
   target={pendingGraph.name+" · revision "+pendingGraph.version}
   targetLabel="Saved PostgreSQL graph · exact server revision"
   message="Creates a durable Core Run bound to this exact saved graph, its dbt dependency digest and immutable snapshot. Refuses stale revisions or changed saved models. Reads ERP-like QA source SELECT-only and publishes to ClickHouse scratch only after quality checks."
   confirmLabel="Start saved Core graph" onCancel={()=>{setConfirm(null);setPendingGraph(null)}}
   onConfirm={()=>{const graph=pendingGraph;setConfirm(null);setPendingGraph(null);void start(graph)}}/>}
  {confirm==="rollback"&&run&&<ConfirmActionDialog
   title="Restore previous scratch table version?"
   target={run.target??run.run_id}
   targetLabel="Physical ClickHouse versioned target"
   message="This explicitly restores the exact previous immutable physical table if its UUID still matches. It does not rewind SQL Server or the source checkpoint. Newer versions are protected."
   confirmLabel="Confirm scratch rollback" destructive onCancel={()=>setConfirm(null)}
   onConfirm={()=>{setConfirm(null);void rollback()}}/>}
 </section>;
}
