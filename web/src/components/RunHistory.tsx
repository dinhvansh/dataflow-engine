import {useMemo,useState} from "react";
import {History,RefreshCw,Search} from "lucide-react";
import {Badge} from "./ui/badge";
import {Button} from "./ui/button";
import {getRunHistory,type HistoryRun} from "../lib/api";

const PAGE_SIZE=10;
function statusVariant(status:string){
 return status==="succeeded"?"success":status==="failed"?"warning":"neutral";
}
export function RunHistory({token,onSelect}:{token:string;onSelect:(runID:string)=>void}){
 const[runs,setRuns]=useState<HistoryRun[]>([]);
 const[error,setError]=useState("");
 const[loading,setLoading]=useState(false);
 const[loaded,setLoaded]=useState(false);
 const[search,setSearch]=useState("");
 const[status,setStatus]=useState("all");
 const[page,setPage]=useState(0);
 const filtered=useMemo(()=>runs.filter(r=>{
  const q=search.trim().toLowerCase();
  return(status==="all"||r.status===status)&&(!q||[r.run_id,r.source,r.target].some(v=>v.toLowerCase().includes(q)));
 }),[runs,search,status]);
 const pages=Math.max(1,Math.ceil(filtered.length/PAGE_SIZE));
 const currentPage=Math.min(page,pages-1);
 const visible=filtered.slice(currentPage*PAGE_SIZE,(currentPage+1)*PAGE_SIZE);
 const refresh=async()=>{
  setLoading(true);setError("");
  try{const response=await getRunHistory(token,50);setRuns(response.runs);setPage(0);setLoaded(true)}
  catch(e){setError(String(e))}
  finally{setLoading(false)}
 };
 return <section className="df-studio-workspace df-studio-panel section mb-5 space-y-4" aria-label="Recent runs">
  <div className="flex flex-wrap items-center justify-between gap-3">
   <div><div className="flex items-center gap-2 font-semibold"><History size={19}/>Recent Runs</div><p className="muted mt-1 text-xs">Filter the latest 50 retrieved runs (not a global history search).</p></div>
   <Button variant="outline" size="sm" onClick={refresh} disabled={!token||loading}><RefreshCw size={15} className={loading?"animate-spin":""}/>{loading?"Loading":"Refresh"}</Button>
  </div>
  {!token&&<p className="muted text-sm">Enter your API token in the Run Inspector below to load history.</p>}
  {error&&<p role="alert" className="rounded-lg border border-red-700 bg-red-950/40 p-3 text-sm text-red-300">{error}</p>}
  {loaded&&<div className="grid gap-3 sm:grid-cols-[1fr_190px]">
   <label className="relative block"><Search className="muted absolute left-3 top-[11px]" size={16}/><input className="field pl-9" aria-label="Search recent runs" value={search} onChange={e=>{setSearch(e.target.value);setPage(0)}} placeholder="Run ID, source or destination"/></label>
   <select className="field" aria-label="Filter run status" value={status} onChange={e=>{setStatus(e.target.value);setPage(0)}}><option value="all">All statuses</option><option value="succeeded">Succeeded</option><option value="failed">Failed</option><option value="running">Running</option><option value="queued">Queued</option></select>
  </div>}
  {loaded&&runs.length===0&&<p className="muted rounded-lg border border-border p-5 text-center text-sm">No recorded runs yet.</p>}
  {loaded&&runs.length>0&&filtered.length===0&&<p className="muted rounded-lg border border-border p-5 text-center text-sm">No runs match these filters.</p>}
  {visible.length>0&&<div className="overflow-x-auto rounded-lg border border-border"><table className="w-full min-w-[730px] text-left text-sm"><thead className="enterprise-table-head"><tr><th scope="col" className="px-4 py-3">Created</th><th scope="col" className="px-4 py-3">Source → Destination</th><th scope="col" className="px-4 py-3">Status</th><th scope="col" className="px-4 py-3 text-right">Rows</th><th scope="col" className="px-4 py-3 text-right">Details</th></tr></thead><tbody>{visible.map(r=><tr key={r.run_id} className="enterprise-table-row border-t border-border"><td className="whitespace-nowrap px-4 py-3 text-xs">{new Date(r.created_at).toLocaleString()}</td><td className="px-4 py-3 text-xs"><div className="font-medium">{r.source}</div><div className="muted mt-1">{r.target}</div></td><td className="px-4 py-3"><Badge variant={statusVariant(r.status)}>{r.status}</Badge></td><td className="px-4 py-3 text-right tabular-nums">{r.rows_written.toLocaleString()}</td><td className="px-4 py-3 text-right"><button type="button" className="enterprise-text-action text-xs font-semibold" onClick={()=>onSelect(r.run_id)} aria-label={`Inspect run ${r.run_id}`}>Inspect</button></td></tr>)}</tbody></table></div>}
  {loaded&&runs.length>0&&<div className="flex flex-wrap items-center justify-between gap-3"><span className="muted text-xs">{filtered.length===0?"0":currentPage*PAGE_SIZE+1}–{Math.min((currentPage+1)*PAGE_SIZE,filtered.length)} of {filtered.length} matching runs</span><div className="flex items-center gap-2"><Button size="sm" variant="outline" disabled={currentPage===0} onClick={()=>setPage(p=>Math.max(0,p-1))}>Previous</Button><span className="muted text-xs tabular-nums">{currentPage+1} / {pages}</span><Button size="sm" variant="outline" disabled={currentPage>=pages-1} onClick={()=>setPage(p=>Math.min(pages-1,p+1))}>Next</Button></div></div>}
 </section>;
}
