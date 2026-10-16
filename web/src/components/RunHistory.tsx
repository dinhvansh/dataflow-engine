import {useState} from "react";
import {History,RefreshCw} from "lucide-react";
import {Button} from "./ui/button";
import {getRunHistory,type HistoryRun} from "../lib/api";

export function RunHistory({token,onSelect}:{token:string;onSelect:(runID:string)=>void}){
 const[runs,setRuns]=useState<HistoryRun[]>([]);
 const[error,setError]=useState("");
 const[loading,setLoading]=useState(false);
 const[loaded,setLoaded]=useState(false);
 const refresh=async()=>{setLoading(true);setError("");try{const r=await getRunHistory(token,50);setRuns(r.runs);setLoaded(true)}catch(e){setError(String(e))}finally{setLoading(false)}};
 return <section className="section mb-5">
  <div className="mb-3 flex items-center justify-between gap-3"><div className="flex items-center gap-2 font-semibold"><History size={19}/>Recent Runs</div><Button variant="outline" size="sm" onClick={refresh} disabled={!token||loading}><RefreshCw size={15}/>{loading?"Loading":"Refresh"}</Button></div>
  {!token&&<p className="muted text-sm">Enter your API token in the Run Inspector below to load history.</p>}
  {loaded&&runs.length===0&&<p className="muted text-sm">No recorded runs yet.</p>}
  {error&&<p className="text-sm text-red-300">{error}</p>}
  {runs.length>0&&<div className="overflow-x-auto"><table className="w-full min-w-[700px] text-left text-sm"><thead><tr className="border-b border-slate-700 text-xs text-slate-400"><th className="py-2">Created</th><th>Source → Target</th><th>Status</th><th>Rows</th><th>Run</th></tr></thead><tbody>{runs.map(r=><tr key={r.run_id} className="border-b border-slate-800"><td className="py-3 text-xs">{new Date(r.created_at).toLocaleString()}</td><td className="text-xs">{r.source} → {r.target}</td><td className={r.status==="succeeded"?"text-emerald-400":r.status==="failed"?"text-red-400":"text-amber-400"}>{r.status}</td><td>{r.rows_written.toLocaleString()}</td><td><button className="font-mono text-xs text-sky-300 hover:underline" onClick={()=>onSelect(r.run_id)}>Inspect</button></td></tr>)}</tbody></table></div>}
 </section>;
}
