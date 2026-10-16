import {useEffect,useState} from "react";
import {GitCompareArrows,RefreshCw,Play,ShieldAlert} from "lucide-react";
import {Button} from "./ui/button";
import {reconciliationApi,type ManagedStream,type ReconciliationRun} from "../lib/api";

export function ReconciliationPanel({token,streams}:{token:string;streams:ManagedStream[]}){
 const[chosen,setChosen]=useState("");
 const[run,setRun]=useState<ReconciliationRun|null>(null);
 const[recent,setRecent]=useState<ReconciliationRun[]>([]);
 const[error,setError]=useState("");
 const[busy,setBusy]=useState(false);
 const load=async()=>{setBusy(true);setError("");try{setRecent((await reconciliationApi.list(token)).runs)}catch(e){setError(String(e))}finally{setBusy(false)}};
 const start=async()=>{setBusy(true);setError("");try{setRun(await reconciliationApi.run(chosen,token))}catch(e){setError(String(e))}finally{setBusy(false)}};
 useEffect(()=>{
  if(!token||!run||!["queued","running"].includes(run.status))return;
  let active=true;
  const tick=setInterval(()=>{void reconciliationApi.status(run.run_id,token).then(v=>{if(active)setRun(v)}).catch(()=>{})},4000);
  return()=>{active=false;clearInterval(tick)};
 },[token,run?.run_id,run?.status]);
 return <section className="section mt-5 space-y-4">
  <div className="flex items-center gap-2 font-semibold"><GitCompareArrows size={20}/>Snapshot Reconciliation</div>
  <div className="flex items-start gap-2 rounded-lg border border-amber-700/40 bg-amber-500/10 p-3 text-sm text-amber-200">
   <ShieldAlert size={18} className="shrink-0"/>
   <span>Detection only: compares two verified full source-PK snapshots. First run builds the baseline; second run reports added, missing (potential DELETE), and version-changed keys. No tombstones or ERP writes.</span>
  </div>
  <div className="flex flex-wrap gap-2">
   <select aria-label="Stream for reconciliation" className="field max-w-lg" value={chosen} onChange={e=>setChosen(e.target.value)}>
    <option value="">Select registered stream</option>
    {streams.map(s=><option value={s.id} key={s.id}>{s.source_schema}.{s.source_table} · {s.id.slice(0,8)}</option>)}
   </select>
   <Button disabled={!chosen||!token||busy} onClick={start}><Play size={15}/>Capture snapshot</Button>
   <Button variant="outline" disabled={!token||busy} onClick={load}><RefreshCw size={15}/>Recent comparisons</Button>
  </div>
  {error&&<p role="alert" className="text-sm text-red-400">{error}</p>}
  {run&&<div className="rounded-lg border border-slate-700 bg-slate-800 p-3">
   <div className="text-sm font-semibold">Run {run.run_id.slice(0,8)} · {run.status}</div>
   <div className="mt-2 grid grid-cols-2 gap-2 text-sm lg:grid-cols-5">
    <div>Scanned: <b>{run.scanned_rows??0}</b></div>
    <div>Verified: <b>{run.verified_rows??0}</b></div>
    <div>Added: <b>{run.added_keys??"Baseline"}</b></div>
    <div>Missing: <b>{run.deleted_keys??"Baseline"}</b></div>
    <div>Changed: <b>{run.changed_versions??"Baseline"}</b></div>
   </div>{run.error&&<p className="mt-2 text-sm text-red-400">{run.error}</p>}
  </div>}
  {recent.length>0&&<div className="overflow-x-auto"><table className="w-full min-w-[660px] text-left text-xs"><thead><tr className="border-b border-slate-700 text-slate-400"><th className="py-2">Run ID</th><th>Status</th><th>Rows</th><th>Added</th><th>Missing</th><th>Changed</th></tr></thead><tbody>{recent.map(r=><tr key={r.run_id} className="border-b border-slate-800"><td className="py-2 font-mono">{r.run_id.slice(0,8)}</td><td>{r.status}</td><td>{r.verified_rows}</td><td>{r.added_keys??"—"}</td><td>{r.deleted_keys??"—"}</td><td>{r.changed_versions??"—"}</td></tr>)}</tbody></table></div>}
 </section>;
}
