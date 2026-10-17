import {useEffect,useMemo,useState} from "react";
import {Activity,AlertCircle,ArrowRight,CheckCircle2,Clock3,Database,Eye,RefreshCw,Search,ShieldAlert,Workflow} from "lucide-react";
import {Badge} from "./ui/badge";
import {DagExecutionMap} from "./DagExecutionMap";
import {Button} from "./ui/button";
import {Card,CardContent,CardDescription,CardHeader,CardTitle} from "./ui/card";
import {sqlDAGApi,type DAGRun,type ExecutableDAG} from "../lib/api";

type Props={token:string;onTokenChange:(next:string)=>void;onOpenPipelines:()=>void};
function variant(status:string):"success"|"danger"|"warning"|"neutral"{
 if(status==="succeeded")return "success";
 if(status==="failed"||status==="cancelled")return "danger";
 if(status==="queued"||status==="running")return "warning";
 return "neutral";
}
function nodeLabel(status:string){
 return status.replaceAll("_"," ");
}
export function OperationsCenter({token,onTokenChange,onOpenPipelines}:Props){
 const[runs,setRuns]=useState<DAGRun[]>([]);
 const[dags,setDags]=useState<ExecutableDAG[]>([]);
 const[selectedId,setSelectedId]=useState("");
 const[detail,setDetail]=useState<DAGRun|null>(null);
 const[loading,setLoading]=useState(false);
 const[detailLoading,setDetailLoading]=useState(false);
 const[loaded,setLoaded]=useState(false);
 const[error,setError]=useState("");
 const[detailError,setDetailError]=useState("");
 const[search,setSearch]=useState("");
 const[filter,setFilter]=useState("all");
 const[autoRefresh,setAutoRefresh]=useState(true);
 const[observedAt,setObservedAt]=useState("");
 const[page,setPage]=useState(0);
 const pageSize=10;

 // Never retain data from a previous token (and do not persist credentials).
 useEffect(()=>{
  setRuns([]);setDags([]);setDetail(null);setSelectedId("");setLoaded(false);
  setError("");setDetailError("");setObservedAt("");
 },[token]);

 const refresh=async()=>{
  if(!token)return;
  setLoading(true);setError("");
  try{
   const [history,definitions]=await Promise.all([sqlDAGApi.history(token),sqlDAGApi.dags(token)]);
   setRuns(history.runs??[]);setDags(definitions.pipelines??[]);
   setLoaded(true);setObservedAt(new Date().toLocaleTimeString());
  }catch(e){setError(String(e))}
  finally{setLoading(false)}
 };
 const loadDetail=async(id:string)=>{
  if(!token||!id)return;
  setDetailLoading(true);setDetailError("");
  try{const response=await sqlDAGApi.run(id,token);setDetail(response)}
  catch(e){setDetailError(String(e))}
  finally{setDetailLoading(false)}
 };
 const filtered=useMemo(()=>runs.filter(r=>{
  const name=dags.find(d=>d.id===r.dag_id)?.name??"";
  const matches=[r.run_id,r.dag_id??"",name,r.error??""].some(v=>v.toLowerCase().includes(search.trim().toLowerCase()));
  return (filter==="all"||r.status===filter)&&matches;
 }),[runs,dags,filter,search]);
 const totalPages=Math.max(1,Math.ceil(filtered.length/pageSize));
 const actualPage=Math.min(page,totalPages-1);
 const displayed=filtered.slice(actualPage*pageSize,(actualPage+1)*pageSize);
 const currentDefinition=dags.find(d=>d.id===detail?.dag_id);
 const states=detail?.result?.state??{};
 const details=useMemo(()=>{
  const order=currentDefinition?.nodes.map(n=>n.id)??[];
  return Object.entries(states).sort(([a],[b])=>{
   const ia=order.indexOf(a),ib=order.indexOf(b);
   return (ia<0?Number.MAX_SAFE_INTEGER:ia)-(ib<0?Number.MAX_SAFE_INTEGER:ib)||a.localeCompare(b);
  });
 },[states,currentDefinition]);
 const summary={
  loaded:runs.length,
  active:runs.filter(r=>["queued","running"].includes(r.status)).length,
  failed:runs.filter(r=>r.status==="failed").length,
  success:runs.filter(r=>r.status==="succeeded").length
 };

 // Refresh the selected active run only. The overview is a manual snapshot,
 // never a misleading claim of server-sent realtime streaming.
 useEffect(()=>{
  if(!token||!selectedId||!autoRefresh||!detail||!["queued","running"].includes(detail.status))return;
  let alive=true;
  const interval=setInterval(()=>{
   void sqlDAGApi.run(selectedId,token).then(next=>{if(alive)setDetail(next)}).catch(e=>{if(alive)setDetailError(String(e))});
  },5000);
  return()=>{alive=false;clearInterval(interval)};
 },[token,selectedId,autoRefresh,detail?.status]);

 return <div className="space-y-5">
  <section className="enterprise-hero rounded-2xl p-5 sm:p-6">
   <div className="flex flex-wrap items-start justify-between gap-4">
    <div className="max-w-2xl"><div className="mb-2 flex items-center gap-2"><Activity size={20} className="text-primary"/><Badge variant="neutral">SQL DAG execution</Badge></div>
     <h2 className="text-2xl font-bold tracking-tight">Pipeline Operations Center</h2>
     <p className="muted mt-2 text-sm leading-relaxed">Inspect real SQL DAG run records and per-node execution evidence. List endpoint provides the most recent 50 runs; detailed attempts and errors load on demand.</p></div>
    <Button type="button" variant="outline" onClick={onOpenPipelines}><Workflow size={16}/>Pipeline builder <ArrowRight size={14}/></Button>
   </div>
  </section>
  <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
   {([{label:"Loaded runs",value:summary.loaded,icon:Database},{label:"Queued / Running",value:summary.active,icon:Clock3},{label:"Succeeded",value:summary.success,icon:CheckCircle2},{label:"Failed",value:summary.failed,icon:AlertCircle}] as const).map(s=>{const Icon=s.icon;return <Card key={s.label}><CardHeader className="pb-2"><CardDescription className="flex items-center gap-2"><Icon size={15}/>{s.label}</CardDescription><CardTitle className="text-2xl tabular-nums">{loaded?s.value:"—"}</CardTitle></CardHeader><CardContent className="muted text-xs">Among retrieved SQL DAG runs only</CardContent></Card>})}
  </div>
  <section className="section space-y-4" aria-label="SQL DAG run history">
   <div className="flex flex-wrap items-end justify-between gap-3"><div><h3 className="font-semibold">Execution history</h3><p className="muted mt-1 text-xs">Read-only monitoring · no automatic retries or pipeline mutations.</p></div>
    <Button type="button" onClick={()=>void refresh()} disabled={!token||loading}><RefreshCw className={loading?"animate-spin":""} size={16}/>{loading?"Loading…":"Refresh runs"}</Button></div>
   <label className="block space-y-1.5"><span className="muted text-xs">DataFlow API token (memory only)</span><input aria-label="Operations API token" className="field max-w-md" type="password" autoComplete="off" value={token} onChange={e=>onTokenChange(e.target.value)} placeholder="Enter token to fetch SQL DAG runs"/></label>
   {error&&<div role="alert" className="rounded-lg border border-red-700 bg-red-950/40 p-3 text-sm text-red-300">{error}</div>}
   {observedAt&&<p className="muted text-xs">Last list fetch (browser local time): {observedAt}. No background list polling.</p>}
   {loaded&&<div className="grid gap-3 sm:grid-cols-[1fr_190px]">
    <label className="relative block"><Search className="muted absolute left-3 top-[11px]" size={16}/><input aria-label="Search pipeline runs" className="field pl-9" value={search} onChange={e=>{setSearch(e.target.value);setPage(0)}} placeholder="Run ID, pipeline name, error…"/></label>
    <select aria-label="Filter pipeline status" className="field" value={filter} onChange={e=>{setFilter(e.target.value);setPage(0)}}><option value="all">All statuses</option><option value="queued">Queued</option><option value="running">Running</option><option value="succeeded">Succeeded</option><option value="failed">Failed</option></select>
   </div>}
   {!token&&<p className="muted text-sm">Enter a token to inspect SQL DAG runs.</p>}
   {loaded&&runs.length===0&&<p className="muted rounded-lg border border-border p-5 text-center text-sm">No SQL DAG runs returned.</p>}
   {loaded&&runs.length>0&&filtered.length===0&&<p className="muted rounded-lg border border-border p-5 text-center text-sm">No runs match the filters.</p>}
   {displayed.length>0&&<div className="overflow-x-auto rounded-lg border border-border"><table className="w-full min-w-[680px] text-left text-sm">
    <thead className="enterprise-table-head"><tr><th scope="col" className="px-4 py-3">Pipeline</th><th scope="col" className="px-4 py-3">Run ID</th><th scope="col" className="px-4 py-3">Status</th><th scope="col" className="px-4 py-3 text-right">Action</th></tr></thead>
    <tbody>{displayed.map(r=><tr key={r.run_id} className="enterprise-table-row border-t border-border">
     <td className="px-4 py-3"><div className="text-sm font-medium">{dags.find(d=>d.id===r.dag_id)?.name??"Unknown or deleted DAG"}</div><div className="muted mt-1 break-all text-xs">{r.dag_id??"Unknown"}</div></td>
     <td className="px-4 py-3 font-mono text-xs">{r.run_id.slice(0,8)}…</td>
     <td className="px-4 py-3"><Badge variant={variant(r.status)}>{r.status}</Badge></td>
     <td className="px-4 py-3 text-right"><button className="enterprise-text-action inline-flex items-center gap-1 text-xs font-semibold" type="button" aria-label={`Inspect pipeline run ${r.run_id}`} onClick={()=>{setSelectedId(r.run_id);setDetail(null);void loadDetail(r.run_id)}}><Eye size={14}/>Inspect</button></td>
    </tr>)}</tbody>
   </table></div>}
   {loaded&&runs.length>0&&<div className="flex flex-wrap items-center justify-between gap-2"><span className="muted text-xs">{filtered.length===0?0:actualPage*pageSize+1}–{Math.min((actualPage+1)*pageSize,filtered.length)} of {filtered.length} matching runs</span><div className="flex items-center gap-2"><Button size="sm" variant="outline" onClick={()=>setPage(p=>Math.max(0,p-1))} disabled={actualPage===0}>Previous</Button><span className="muted text-xs tabular-nums">{actualPage+1} / {totalPages}</span><Button size="sm" variant="outline" onClick={()=>setPage(p=>Math.min(totalPages-1,p+1))} disabled={actualPage>=totalPages-1}>Next</Button></div></div>}
  </section>
  {selectedId&&<Card aria-label="Pipeline run inspector">
   <CardHeader><div className="flex flex-wrap items-center justify-between gap-2"><div><CardTitle>Run inspector</CardTitle><CardDescription className="mt-1 break-all font-mono text-xs">{selectedId}</CardDescription></div><div className="flex flex-wrap items-center gap-2">
    {detail&&<Badge variant={variant(detail.status)}>{detail.status}</Badge>}
    <Button size="sm" variant="outline" onClick={()=>void loadDetail(selectedId)} disabled={!token||detailLoading}><RefreshCw size={14} className={detailLoading?"animate-spin":""}/>Refresh detail</Button>
   </div></div></CardHeader>
   <CardContent className="space-y-3">
    {detailLoading&&!detail&&<p className="muted text-sm">Loading run details…</p>}
    {detailError&&<p role="alert" className="rounded-lg border border-red-700 bg-red-950/40 p-3 text-sm text-red-300">{detailError}</p>}
    {detail?.error&&<div className="rounded-lg border border-red-700 bg-red-950/40 p-3 text-sm"><div className="mb-1 flex items-center gap-2 font-semibold text-red-300"><ShieldAlert size={16}/>Pipeline error</div><p className="break-words text-red-300">{detail.error}</p></div>}
    {detail&&<label className="muted flex items-center gap-2 text-xs"><input type="checkbox" checked={autoRefresh} onChange={e=>setAutoRefresh(e.target.checked)}/>Auto-refresh selected queued/running run every 5 seconds</label>}
    {detail&&details.length===0&&<p className="muted rounded-lg border border-border p-4 text-sm">No per-node outcomes have been reported yet. The saved DAG stages are shown below when available.</p>}\n    {detail&&<DagExecutionMap plan={currentDefinition?.nodes} state={detail.result?.state} attempts={detail.result?.attempts} errors={detail.result?.errors}/>}
    <p className="muted text-xs">Attempts and errors come from the stored DAG result. No elapsed time or per-step timestamps are exposed by this API.</p>
   </CardContent>
  </Card>}
 </div>;
}
