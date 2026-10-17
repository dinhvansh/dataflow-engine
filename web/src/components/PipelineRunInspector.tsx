import {useEffect,useState} from "react";
import {Activity,Play,RefreshCw,ShieldAlert} from "lucide-react";
import {Button} from "./ui/button";
import {ConfirmActionDialog} from "./ui/confirm-action-dialog";
import {DagExecutionMap} from "./DagExecutionMap";
import {sqlDAGApi,type DAGRun,type ExecutableDAG,type PipelineNode} from "../lib/api";

export type CanvasRunRequest={dag:ExecutableDAG;revision:number};
type PendingRun={dag:ExecutableDAG;revision:number|null};
type Props={token:string;dags:ExecutableDAG[];selectedId:string;theme:"light"|"dark";onOpenOperations:()=>void;canvasRunRequest:CanvasRunRequest|null;currentRevision:number};
type RunSnapshot={dagName:string;plan:PipelineNode[];run:DAGRun};
const activeStatuses=new Set(["queued","running"]);

export function PipelineRunInspector({token,dags,selectedId,theme,onOpenOperations,canvasRunRequest,currentRevision}:Props){
 const selected=dags.find(d=>d.id===selectedId)??null;
 const[pending,setPending]=useState<PendingRun|null>(null);
 const[busy,setBusy]=useState(false);
 const[refreshing,setRefreshing]=useState(false);
 const[error,setError]=useState("");
 const[pollError,setPollError]=useState("");
 const[snapshot,setSnapshot]=useState<RunSnapshot|null>(null);
 const run=snapshot?.run;
 const active=!!run&&activeStatuses.has(run.status);
 useEffect(()=>{
  if(!canvasRunRequest)return;
  if(active||busy){setError("A run is already active. Wait for it to finish before starting another.");return;}
  setPending({dag:canvasRunRequest.dag,revision:canvasRunRequest.revision});
 },[canvasRunRequest]);
 const startRun=async(dag:ExecutableDAG)=>{
  setBusy(true);setError("");setPollError("");
  try{
   // A saved DAG can be overwritten elsewhere between canvas verification and confirmation.
   // Re-read and fail closed if the ID, enablement or topology changed.
   const registry=await sqlDAGApi.dags(token);
   const latest=registry.pipelines.find(d=>d.id===dag.id);
   if(!latest||!latest.enabled||latest.name!==dag.name||JSON.stringify(latest.nodes)!==JSON.stringify(dag.nodes)){
    throw new Error("Saved DAG changed or was disabled before Run. Refresh and validate again.");
   }
   const result=await sqlDAGApi.start(dag.id,token);
   // Freeze only the locally viewed topology; authoritative execution status
   // always comes from the backend run API, never the editor or mock data.
   setSnapshot({dagName:dag.name,plan:dag.nodes.map(n=>({...n,requires:[...n.requires]})),run:result});
  }catch(e){setError(String(e));}
  finally{setBusy(false);}
 };
 const refresh=async()=>{
  if(!token||!run)return;
  setRefreshing(true);setPollError("");
  try{
   const updated=await sqlDAGApi.run(run.run_id,token);
   setSnapshot(prev=>prev&&prev.run.run_id===updated.run_id?{...prev,run:updated}:prev);
  }catch(e){setPollError("Could not refresh run status: "+String(e));}
  finally{setRefreshing(false);}
 };
 useEffect(()=>{
  if(!token||!run||!active)return;
  let cancelled=false;
  let requesting=false;
  const timer=setInterval(()=>{
   if(requesting||cancelled)return;
   requesting=true;
   void sqlDAGApi.run(run.run_id,token).then(updated=>{
    if(cancelled)return;
    setSnapshot(prev=>prev&&prev.run.run_id===updated.run_id?{...prev,run:updated}:prev);
    setPollError("");
   }).catch(e=>{if(!cancelled)setPollError("Status polling failed: "+String(e));})
    .finally(()=>{requesting=false;});
  },3000);
  return()=>{cancelled=true;clearInterval(timer);};
 },[token,run?.run_id,run?.status,active]);
 const status=run?.status??"not started";
 const statusClass=status==="succeeded"?"df-text-success":status==="failed"||status==="cancelled"?"df-text-error":status==="queued"||status==="running"?"df-text-warning":"df-text-neutral";
 return <section className="rounded border df-border-neutral p-3 sm:p-4 space-y-4" aria-label="Pipeline run inspector">
  <div className="flex flex-wrap items-start justify-between gap-2">
   <div><h4 className="flex items-center gap-2 font-semibold"><Activity size={17}/>Run & Inspector <span className="signal-inline-code">/ TEST</span></h4>
    <p className="muted mt-1 text-xs">Run an enabled saved SQL DAG only after confirmation. Backend permissions, scratch guards and durable execution remain authoritative.</p></div>
   <Button type="button" size="sm" variant="outline" onClick={onOpenOperations}>Operations Center</Button>
  </div>
  {!selected&&<p className="muted text-sm">Select an existing saved DAG from the registry above to inspect its execution eligibility.</p>}
  {selected&&<div className="flex flex-wrap items-center justify-between gap-3 rounded border df-border-neutral df-surface-soft p-3">
   <div><p className="text-sm font-semibold">{selected.name}</p>
    <p className="muted mt-1 text-xs">{selected.nodes.length} nodes · {selected.enabled?"Enabled for TEST execution":"Disabled — save with explicit TEST enablement first"}</p></div>
   <Button type="button" disabled={!token||!selected.enabled||busy||active} onClick={()=>setPending({dag:selected,revision:null})}><Play size={15}/>{busy?"Starting…":"Run selected DAG"}</Button>
  </div>}
  {selected&&!selected.enabled&&<p className="df-text-warning flex items-start gap-2 text-xs"><ShieldAlert className="shrink-0" size={15}/>This saved DAG is disabled. Loading it into the editor does not enable it. Authorize TEST execution in Save, then select it here.</p>}
  {error&&<p role="alert" className="rounded border df-border-error df-bg-error-soft p-3 text-sm df-text-error">{error}</p>}
  {snapshot&&<div className="space-y-4" aria-label="Observed pipeline run">
   <div className="flex flex-wrap items-center justify-between gap-3">
    <div><p className="muted text-[10px] font-semibold uppercase tracking-wide">Backend run · {snapshot.dagName}</p>
     <p className={"mt-1 font-semibold "+statusClass} role="status">Status: {status}</p>
     <p className="muted mt-1 break-all font-mono text-xs">{run?.run_id}</p></div>
    <Button type="button" size="sm" variant="outline" disabled={!token||refreshing} onClick={()=>{void refresh();}}><RefreshCw size={14}/>{refreshing?"Refreshing…":"Refresh run"}</Button>
   </div>
   {pollError&&<p className="df-text-warning text-xs" role="alert">{pollError}</p>}
   {run?.error&&<p className="rounded border df-border-error df-bg-error-soft p-3 text-sm df-text-error" role="alert">{run.error}</p>}
   {Object.keys(run?.result?.state??{}).length>0&&<div className="overflow-x-auto rounded border df-border-neutral"><table className="min-w-[430px] w-full text-left text-xs">
    <thead className="enterprise-table-head"><tr><th className="p-2">Node</th><th className="p-2">Observed status</th><th className="p-2">Attempts</th><th className="p-2">Error</th></tr></thead>
    <tbody>{Object.entries(run?.result?.state??{}).map(([id,nodeStatus])=><tr key={id} className="border-t df-border-neutral">
     <td className="break-all p-2 font-mono">{id}</td><td className="p-2">{nodeStatus}</td>
     <td className="p-2">{run?.result?.attempts?.[id]??"Unreported"}</td>
     <td className="p-2 break-words">{run?.result?.errors?.[id]??"—"}</td>
    </tr>)}</tbody>
   </table></div>}
   <DagExecutionMap theme={theme} plan={snapshot.plan} state={run?.result?.state} attempts={run?.result?.attempts} errors={run?.result?.errors}/>
   <p className="muted text-xs">Node labels/edges are the saved DAG definition observed at click time; status, attempts and errors are returned by the backend. Do not treat this as an immutable server-side topology snapshot.</p>
  </div>}
  {pending&&<ConfirmActionDialog title="Run saved SQL DAG in TEST?" target={pending.dag.name}
   targetLabel="Executable SQL DAG · confirm selected TEST target"
   message="This starts enabled read-only SQL Jobs against the configured TEST ClickHouse account. Execution is controlled by server-side authorization and scratch restrictions. Confirm only in an approved isolated QA environment."
   confirmLabel="Confirm TEST run" onCancel={()=>setPending(null)}
   onConfirm={()=>{
    const request=pending;setPending(null);
    if(request.revision!==null&&request.revision!==currentRevision){setError("Canvas changed after Run was requested. Validate and save again.");return;}
    void startRun(request.dag);
   }}/>}
 </section>;
}
