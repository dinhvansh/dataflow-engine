import {useMemo,useRef,useState} from "react";
import {CheckCircle2,Code2,Database,FileCode2,Info,Play,RefreshCw,Save,Search,ShieldAlert,TerminalSquare} from "lucide-react";
import {Button} from "./ui/button";
import {Badge} from "./ui/badge";
import {sqlDAGApi,type SQLJob} from "../lib/api";
export function SQLJobStudio({token,onTokenChange}:{token:string;onTokenChange:(token:string)=>void}){
 const[name,setName]=useState("revenue_quality_check");
 const[database,setDatabase]=useState("scratch");
 const[query,setQuery]=useState("SELECT count() AS total_rows FROM scratch.dataflow_raw_events");
 const[enabled,setEnabled]=useState(false);
 const[jobs,setJobs]=useState<SQLJob[]>([]);
 const[loaded,setLoaded]=useState(false);
 const[search,setSearch]=useState("");
 const[onlyEnabled,setOnlyEnabled]=useState(false);
 const[editingId,setEditingId]=useState("");
 const[error,setError]=useState(""),[notice,setNotice]=useState(""),[busy,setBusy]=useState(false);
 const gutterRef=useRef<HTMLDivElement>(null);
 const lineCount=query.split("\n").length;
 const filtered=useMemo(()=>jobs.filter(j=>(!onlyEnabled||j.enabled)&&(j.name+" "+j.id+" "+j.database).toLowerCase().includes(search.trim().toLowerCase())),[jobs,onlyEnabled,search]);
 const load=async()=>{setBusy(true);setError("");try{setJobs((await sqlDAGApi.jobs(token)).jobs);setLoaded(true)}catch(e){setError(String(e))}finally{setBusy(false)}};
 const save=async()=>{setBusy(true);setError("");setNotice("");try{
  const result=await sqlDAGApi.saveJob({name,database,query,enabled},token);
  setEditingId(result.id);setNotice(`Saved SQL Job ${result.name} (${result.id})`);
  setJobs((await sqlDAGApi.jobs(token)).jobs);setLoaded(true);
 }catch(e){setError(String(e))}finally{setBusy(false)}};
 const selectJob=(j:SQLJob)=>{setEditingId(j.id);setName(j.name);setDatabase(j.database);setQuery(j.query);setEnabled(j.enabled);setError("");setNotice("")};
 return <div className="df-studio-workspace space-y-5">
  <section className="df-studio-hero section flex flex-wrap items-start justify-between gap-4">
   <div><p className="df-studio-kicker mb-2">DEVELOPMENT / SQL / 01</p><h2 className="flex items-center gap-2 text-xl font-bold"><TerminalSquare size={20}/>SQL Job Studio</h2>
    <p className="muted mt-2 max-w-2xl text-sm">Author SELECT-only ClickHouse jobs for existing SQL DAGs; saving a definition never runs it.</p></div><Badge variant="neutral"><Database size={13}/>Scratch only</Badge>
  </section>
  <div className="df-studio-notice flex items-start gap-2 rounded-md border p-3 text-sm" role="note"><ShieldAlert size={17} className="mt-0.5 shrink-0"/><span>Only a single SELECT query through a restricted ClickHouse account and scratch database. No INSERT, ALTER, DROP or production jobs. Execution remains controlled by registered DAGs.</span></div>
  <div className="grid min-w-0 gap-4 xl:grid-cols-[minmax(0,1.7fr)_minmax(310px,1fr)]">
   <section className="df-studio-panel section min-w-0 space-y-4" aria-label="SQL job editor">
    <div className="flex flex-wrap items-center justify-between gap-2"><h3 className="flex items-center gap-2 font-semibold"><Code2 size={17}/>Query editor</h3><Badge variant={enabled?"success":"neutral"}>{enabled?"Enabled for test":"Disabled"}</Badge></div>
    <label className="block space-y-1"><span className="df-studio-label">DataFlow API token · browser memory only</span><input type="password" className="field" autoComplete="off" aria-label="SQL Jobs API token" value={token} onChange={e=>onTokenChange(e.target.value)}/></label>
    <div className="grid gap-3 sm:grid-cols-2">
     <label className="block space-y-1"><span className="df-studio-label">Job name</span><input className="field" aria-label="SQL job name" value={name} onChange={e=>setName(e.target.value)}/></label>
     <label className="block space-y-1"><span className="df-studio-label">ClickHouse database</span><input className="field" aria-label="SQL job database" value={database} onChange={e=>setDatabase(e.target.value)}/></label>
    </div>
    <div className="df-studio-code-box overflow-hidden rounded-md border"><div className="df-studio-code-toolbar flex items-center justify-between px-3 py-2"><span className="flex items-center gap-2 text-xs font-semibold"><FileCode2 size={14}/>query.sql</span><span className="df-studio-kicker">CLICKHOUSE / SELECT</span></div>
     <div className="df-studio-code-editor flex min-w-0">
      <div ref={gutterRef} aria-hidden="true" className="df-studio-line-numbers min-h-[270px] shrink-0 overflow-hidden py-4 pr-2 text-right font-mono text-xs leading-6">
       {Array.from({length:lineCount},(_,i)=><div key={i}>{i+1}</div>)}
      </div>
      <label htmlFor="df-sql-job-query" className="sr-only">SQL SELECT query</label>
      <textarea id="df-sql-job-query" spellCheck={false} wrap="off" className="df-studio-sql-area block min-h-[270px] min-w-0 flex-1 resize-y overflow-auto p-4 pl-3 font-mono text-xs leading-6 outline-none" value={query} onChange={e=>setQuery(e.target.value)} onScroll={e=>{if(gutterRef.current)gutterRef.current.scrollTop=e.currentTarget.scrollTop}}/>
     </div>
     <div className="df-studio-code-toolbar flex items-center justify-between px-3 py-2 text-[11px]"><span>Read-only query contract</span><span className="tabular-nums">{lineCount} lines · {query.length} chars</span></div>
    </div>
    <label className="df-studio-check flex items-start gap-3 rounded-md border p-3 text-sm"><input type="checkbox" className="mt-0.5" checked={enabled} onChange={e=>setEnabled(e.target.checked)}/><span><b>Enable this test SQL Job</b><span className="muted mt-1 block text-xs">Enabling a saved definition does not enqueue or execute a DAG.</span></span></label>
    <div className="flex flex-wrap items-center gap-2"><Button disabled={!token||busy} onClick={save}><Save size={15}/>{busy?"Working…":"Save SQL Job"}</Button><Button variant="outline" disabled={!token||busy} onClick={load}><RefreshCw size={15}/>Refresh Jobs</Button>{editingId&&<span className="muted ml-auto font-mono text-[11px]">{editingId.slice(0,12)}…</span>}</div>
    {notice&&<p role="status" className="df-studio-success rounded-md border p-3 text-xs"><CheckCircle2 size={14} className="mr-1 inline"/>{notice}</p>}
    {error&&<p role="alert" className="df-studio-error rounded-md border p-3 text-xs">{error}</p>}
   </section>
   <aside className="df-studio-panel section min-w-0 space-y-4" aria-label="Registered SQL jobs">
    <div className="flex items-start justify-between gap-2"><div><h3 className="font-semibold">Registered SQL Jobs</h3><p className="muted mt-1 text-xs">{loaded?`${jobs.length} loaded jobs`:"Load saved jobs to browse and edit."}</p></div><Badge variant="neutral">{filtered.length} shown</Badge></div>
    <div className="relative"><Search size={16} className="muted pointer-events-none absolute left-3 top-3"/><input aria-label="Search SQL jobs" className="field pl-10 pr-16" value={search} onChange={e=>setSearch(e.target.value)} placeholder="Search name, database, ID"/>{search&&<button type="button" className="df-studio-clear-search absolute right-2 top-1/2 -translate-y-1/2 rounded px-2 py-1 text-xs" aria-label="Clear SQL job search" onClick={()=>setSearch("")}>Clear</button>}</div>
    <label className="muted flex items-center gap-2 text-xs"><input type="checkbox" checked={onlyEnabled} onChange={e=>setOnlyEnabled(e.target.checked)}/>Enabled only</label>
    {!loaded&&<div className="df-studio-empty rounded-md border p-6 text-center"><Database className="muted mx-auto mb-2" size={25}/><p className="text-sm font-semibold">Nothing loaded yet</p><p className="muted mt-1 text-xs">Refresh Jobs uses the existing registry API.</p></div>}
    {loaded&&jobs.length===0&&<p className="df-studio-empty rounded-md border p-5 text-center text-sm">No registered SQL jobs.</p>}
    {loaded&&jobs.length>0&&filtered.length===0&&<p className="df-studio-empty rounded-md border p-5 text-center text-sm">No matching jobs.</p>}
    {filtered.length>0&&<div className="max-h-[620px] space-y-2 overflow-y-auto">{filtered.map(j=><button type="button" key={j.id} onClick={()=>selectJob(j)} className={`df-studio-registry-item w-full rounded-md border p-3 text-left ${editingId===j.id?"is-active":""}`}>
     <div className="flex flex-wrap items-start justify-between gap-2"><strong className="break-all text-sm">{j.name}</strong><Badge variant={j.enabled?"success":"neutral"}>{j.enabled?"Enabled":"Disabled"}</Badge></div>
     <div className="muted mt-2 flex flex-wrap items-center gap-2 text-xs"><Database size={12}/>{j.database}<span className="font-mono">{j.id.slice(0,8)}…</span></div>
     <span className="df-studio-link mt-3 inline-flex items-center gap-1 text-xs font-semibold"><Play size={12}/>Load definition for editing</span>
    </button>)}</div>}
    <div className="df-studio-muted-note flex items-start gap-2 border-t pt-3 text-xs"><Info size={15} className="shrink-0"/><span>Loading a job fills the editor only. Use Save SQL Job to submit changes.</span></div>
   </aside>
  </div>
 </div>;
}
