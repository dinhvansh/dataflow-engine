import {useEffect,useState} from "react";
import {Workflow,Play,RefreshCw,Save,ShieldAlert} from "lucide-react";
import {Button} from "./ui/button";
import {sqlDAGApi,type ExecutableDAG,type DAGRun,type SQLJob} from "../lib/api";
export function ExecutablePipelines({token}:{token:string}){
 const[name,setName]=useState("revenue_reporting");
 const[enabled,setEnabled]=useState(false);
 const[jobs,setJobs]=useState<SQLJob[]>([]);
 const[dags,setDags]=useState<ExecutableDAG[]>([]);
 const[raw,setRaw]=useState('{"nodes":[]}');
 const[run,setRun]=useState<DAGRun|null>(null);
 const[busy,setBusy]=useState(false);const[error,setError]=useState("");const[notice,setNotice]=useState("");
 const refresh=async()=>{setBusy(true);setError("");try{const [a,b]=await Promise.all([sqlDAGApi.jobs(token),sqlDAGApi.dags(token)]);setJobs(a.jobs);setDags(b.pipelines)}catch(e){setError(String(e))}finally{setBusy(false)}};
 const save=async()=>{setBusy(true);setError("");setNotice("");try{
  const parsed=JSON.parse(raw);
  if(!Array.isArray(parsed.nodes))throw new Error("JSON must contain nodes array");
  const result=await sqlDAGApi.saveDAG({name,enabled,nodes:parsed.nodes},token);
  setNotice(`Saved pipeline: ${result.id}`);setDags((await sqlDAGApi.dags(token)).pipelines);
 }catch(e){setError(String(e))}finally{setBusy(false)}};
 const start=async(id:string)=>{setBusy(true);setError("");try{setRun(await sqlDAGApi.start(id,token))}catch(e){setError(String(e))}finally{setBusy(false)}};
 useEffect(()=>{
  if(!token||!run||!["queued","running"].includes(run.status))return;
  let mounted=true;
  const handle=setInterval(()=>{void sqlDAGApi.run(run.run_id,token).then(r=>{if(mounted)setRun(r)}).catch(()=>{})},3000);
  return()=>{mounted=false;clearInterval(handle)};
 },[token,run?.run_id,run?.status]);
 return <div className="df-studio-workspace df-executable-workspace space-y-5">
  <section className="df-studio-panel section space-y-4">
   <div className="flex items-center gap-2 font-semibold"><Workflow size={19}/>Executable SQL DAG (test mode)</div>
   <p className="flex items-start gap-2 rounded-md border border-amber-800/50 bg-amber-950/20 p-3 text-sm text-amber-200"><ShieldAlert size={18} className="shrink-0"/>This executor runs enabled SELECT-only ClickHouse jobs. It is asynchronous but in-process; it is not a durable distributed scheduler. Existing DAG Validator is separate.</p>
   <div className="grid gap-3 sm:grid-cols-2"><label className="space-y-1"><span className="muted text-xs">Pipeline name</span><input className="field" value={name} onChange={e=>setName(e.target.value)}/></label><label className="flex items-center gap-2 pt-5 text-sm"><input type="checkbox" checked={enabled} onChange={e=>setEnabled(e.target.checked)}/>Enabled for test execution</label></div>
   <label className="block space-y-1"><span className="muted text-xs">DAG JSON (each job_id is registered SQL Job UUID)</span><textarea className="field min-h-[230px] font-mono text-xs" spellCheck={false} value={raw} onChange={e=>setRaw(e.target.value)}/></label>
   <div className="flex flex-wrap gap-2"><Button disabled={!token||busy} onClick={save}><Save size={15}/>Save Pipeline</Button><Button variant="outline" disabled={!token||busy} onClick={refresh}><RefreshCw size={15}/>Refresh Jobs & Pipelines</Button></div>
   {notice&&<p className="text-sm text-emerald-300">{notice}</p>}{error&&<p role="alert" className="text-sm text-red-300">{error}</p>}
  </section>
  {jobs.length>0&&<section className="section"><p className="mb-2 text-sm font-semibold">Available SQL Jobs</p><div className="space-y-1">{jobs.map(j=><div key={j.id} className="text-xs"><span className={j.enabled?"text-emerald-400":"muted"}>{j.enabled?"●":"○"}</span> {j.name} — <code className="muted">{j.id}</code></div>)}</div></section>}
  {dags.length>0&&<section className="section"><p className="mb-3 font-semibold">Saved DAGs</p><div className="space-y-2">{dags.map(p=><div className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-slate-700 bg-slate-800/50 p-3" key={p.id}><div><b className="text-sm">{p.name}</b><div className="muted text-xs">{p.nodes.length} nodes · {p.enabled?"enabled":"disabled"}</div></div><Button size="sm" disabled={!token||busy||!p.enabled} onClick={()=>start(p.id)}><Play size={14}/>Run</Button></div>)}</div></section>}
  {run&&<section className="section"><div className="font-semibold">Pipeline run: {run.status}</div><p className="muted mt-1 break-all font-mono text-xs">{run.run_id}</p>{run.error&&<p className="mt-2 text-sm text-red-300">{run.error}</p>}{run.result?.state&&<div className="mt-3 space-y-2">{Object.entries(run.result.state).map(([node,status])=><div key={node} className="flex justify-between border-b border-slate-800 py-2 text-sm"><span>{node}</span><span>{status} (attempts: {run.result?.attempts?.[node]??0})</span></div>)}</div>}</section>}
 </div>;
}
