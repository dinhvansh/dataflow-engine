import {useEffect,useState} from "react";
import {Database,Play,RefreshCw,Save,ShieldAlert} from "lucide-react";
import {Button} from "./ui/button";
import {sqlDAGApi,type SQLJob} from "../lib/api";

export function SQLJobStudio({token,onTokenChange}:{token:string;onTokenChange:(token:string)=>void}){
 const[name,setName]=useState("revenue_quality_check");
 const[database,setDatabase]=useState("scratch");
 const[query,setQuery]=useState("SELECT count() AS total_rows FROM scratch.dataflow_raw_events");
 const[enabled,setEnabled]=useState(false);
 const[jobs,setJobs]=useState<SQLJob[]>([]);
 const[error,setError]=useState("");const[notice,setNotice]=useState("");const[busy,setBusy]=useState(false);
 const load=async()=>{setBusy(true);setError("");try{setJobs((await sqlDAGApi.jobs(token)).jobs)}catch(e){setError(String(e))}finally{setBusy(false)}};
 const save=async()=>{setBusy(true);setError("");setNotice("");try{
  const result=await sqlDAGApi.saveJob({name,database,query,enabled},token);
  setNotice(`Saved SQL Job ${result.name} (${result.id})`);
  setJobs((await sqlDAGApi.jobs(token)).jobs);
 }catch(e){setError(String(e))}finally{setBusy(false)}};
 return <div className="space-y-5">
  <section className="section space-y-4">
   <h2 className="flex items-center gap-2 text-lg font-semibold"><Database size={19}/>SQL Job Studio (read-only preview)</h2>
   <div className="flex items-start gap-2 rounded-md border border-amber-700/50 bg-amber-950/20 p-3 text-sm text-amber-200"><ShieldAlert size={17} className="shrink-0"/><span>Only single SELECT queries, restricted ClickHouse read-only account, scratch database, with results capped at 200 rows. No INSERT, ALTER, DROP or production jobs. Jobs execute as steps inside a registered DAG.</span></div>
   <div><label className="muted mb-1 block text-xs">DataFlow API token</label><input type="password" className="field" autoComplete="off" value={token} onChange={e=>onTokenChange(e.target.value)}/></div>
   <div className="grid gap-3 sm:grid-cols-2">
    <label className="space-y-1"><span className="muted text-xs">Job name</span><input className="field" value={name} onChange={e=>setName(e.target.value)}/></label>
    <label className="space-y-1"><span className="muted text-xs">ClickHouse database</span><input className="field" value={database} onChange={e=>setDatabase(e.target.value)}/></label>
   </div>
   <label className="block space-y-1"><span className="muted text-xs">SQL SELECT query</span>
    <textarea className="field min-h-[220px] font-mono text-xs leading-5" spellCheck={false} value={query} onChange={e=>setQuery(e.target.value)}/>
   </label>
   <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={enabled} onChange={e=>setEnabled(e.target.checked)}/>Enable this test SQL Job for pipeline execution</label>
   <div className="flex flex-wrap gap-2">
    <Button disabled={!token||busy} onClick={save}><Save size={15}/>Save SQL Job</Button>
    <Button variant="outline" disabled={!token||busy} onClick={load}><RefreshCw size={15}/>Refresh Jobs</Button>
   </div>
   {notice&&<p className="text-xs text-emerald-300">{notice}</p>}
   {error&&<p role="alert" className="text-xs text-red-300">{error}</p>}
  </section>
  <section className="section overflow-x-auto">
   <h3 className="mb-3 font-semibold">Registered SQL Jobs</h3>
   {jobs.length===0?<p className="muted text-sm">Use Refresh Jobs to load the persisted SQL job registry.</p>:
   <table className="w-full min-w-[620px] text-left text-xs"><thead><tr className="border-b border-slate-700 text-slate-400"><th className="py-2">Name / UUID</th><th>Database</th><th>Enabled</th><th>Action</th></tr></thead><tbody>{jobs.map(j=><tr key={j.id} className="border-b border-slate-800">
    <td className="py-3"><b>{j.name}</b><div className="muted font-mono">{j.id}</div></td><td>{j.database}</td><td>{j.enabled?"Yes":"No"}</td><td><Button size="sm" variant="outline" onClick={()=>{setName(j.name);setDatabase(j.database);setQuery(j.query);setEnabled(j.enabled)}}><Play size={13}/>Edit</Button></td>
   </tr>)}</tbody></table>}
  </section>
 </div>;
}
