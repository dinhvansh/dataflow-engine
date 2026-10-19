import {useState} from "react";
import {CheckCircle2,Database,KeyRound,Link2,Plus,RefreshCw,ShieldAlert,TestTube2} from "lucide-react";
import {labRuntimeConnectionsApi,managedApi,type LabRuntimeConnection,type LabRuntimeInput,type ManagedStream} from "../lib/api";
import {Button} from "./ui/button";

// QA-only: authenticated profile operations are handled by the Go backend.
// Credential values never appear in API GET payloads, URL or localStorage.
// The owner must configure exact approved endpoints and encryption key server-side.
export function RuntimeConnectionManager({token}:{token:string}){
 const[adminToken,setAdminToken]=useState("");
 const[profiles,setProfiles]=useState<LabRuntimeConnection[]>([]);
 const[streams,setStreams]=useState<ManagedStream[]>([]);
 const[loaded,setLoaded]=useState(false);
 const[busy,setBusy]=useState("");
 const[error,setError]=useState("");
 const[notice,setNotice]=useState("");
 const[form,setForm]=useState<LabRuntimeInput>({name:"",driver:"sqlserver",host:"",port:1433,database:"DataFlowFixtureQA",username:"qa_erp_reader",password:""});
 const[streamID,setStreamID]=useState("");
 const[sourceID,setSourceID]=useState("");
 const[targetID,setTargetID]=useState("");
 const modify=<K extends keyof LabRuntimeInput>(key:K,value:LabRuntimeInput[K])=>setForm(p=>({...p,[key]:value}));
 const reload=async()=>{
  if(!token||!adminToken)return;
  setBusy("load");setError("");setNotice("");
  try{
   const [p,s]=await Promise.all([labRuntimeConnectionsApi.list(token,adminToken),managedApi.list(token)]);
   setProfiles(p.profiles);setStreams(s.streams);setLoaded(true);
  }catch{setProfiles([]);setLoaded(false);setError("Lab runtime manager unavailable. Check admin permission, migrations, backend feature flags and approved endpoints.");}
  finally{setBusy("");}
 };
 const create=async()=>{
  if(!token||!adminToken||!form.password)return;
  setBusy("save");setError("");setNotice("");
  try{
   const payload={...form};
   await labRuntimeConnectionsApi.create(payload,token,adminToken);
   setForm(p=>({...p,name:"",password:""})); // remove secret from form state after save
   setNotice("Encrypted profile saved as DRAFT. Test it before binding to a stream.");
   const result=await labRuntimeConnectionsApi.list(token,adminToken);
   setProfiles(result.profiles);
  }catch{setError("Could not save profile. The server may reject non-approved endpoints, credentials or missing encryption key.");}
  finally{setBusy("");}
 };
 const test=async(id:string)=>{
  setBusy(id);setError("");setNotice("");
  try{
   const result=await labRuntimeConnectionsApi.test(id,token,adminToken);
   setNotice(result.passed?"Real driver/permission test PASSED.":"Real driver/permission test FAILED. Verify only approved QA credentials.");
   const listed=await labRuntimeConnectionsApi.list(token,adminToken);
   setProfiles(listed.profiles);
  }catch{setError("QA driver test failed or feature is disabled. No credentials were returned.");}
  finally{setBusy("");}
 };
 const bind=async()=>{
  if(!streamID||!sourceID||!targetID)return;
  setBusy("bind");setError("");setNotice("");
  try{
   await labRuntimeConnectionsApi.bind(streamID,sourceID,targetID,token,adminToken);
   setNotice("Verified connection pair bound to the selected scratch stream. Open Pipelines to run the existing Core pipeline.");
  }catch{setError("Binding refused. Both profiles must pass a recent test, use allowed QA endpoints and an existing scratch stream.");}
  finally{setBusy("");}
 };
 return <section className="hub-workspace rounded-lg border p-4 sm:p-5 space-y-4" aria-label="Live lab connection manager">
  <div className="flex flex-wrap items-start justify-between gap-3">
   <div><p className="hub-eyebrow mb-2">CONNECTION MANAGER / LIVE QA</p><h3 className="text-lg font-semibold flex items-center gap-2"><Database size={18}/> SQL Server / ClickHouse connections</h3>
    <p className="muted mt-1 text-xs leading-5">Add, encrypted Save, real driver Test and Bind to a managed scratch stream. Strictly isolated QA-only; host:port must be approved in the backend. Never use production ERP credentials.</p></div>
   <span className="df-pipeline-stat">ISOLATED QA · ADMIN ONLY</span>
  </div>
  <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end">
   <label className="space-y-1"><span className="muted text-xs">Connection admin credential · separate from lab API token</span>
    <input className="field" type="password" autoComplete="off" aria-label="Live connection admin credential" value={adminToken} onChange={e=>{setAdminToken(e.target.value);setProfiles([]);setLoaded(false);setError("");}}/>
   </label>
   <Button variant="outline" size="sm" type="button" disabled={!token||!adminToken||!!busy} onClick={()=>void reload()}><RefreshCw size={15}/> Load connections</Button>
  </div>
  {error&&<p className="df-text-error text-xs" role="alert">{error}</p>}
  {notice&&<p className="df-text-success text-xs" role="status">{notice}</p>}
  {loaded&&<>
   <div className="grid gap-3 lg:grid-cols-2">
    <div className="space-y-3 rounded-md border p-3">
     <h4 className="font-semibold text-sm">Add connection</h4>
     <label className="block text-xs space-y-1">Connection name<input className="field" aria-label="Live connection name" value={form.name} maxLength={80} onChange={e=>modify("name",e.target.value)}/></label>
     <label className="block text-xs space-y-1">Database type<select className="field" aria-label="Live connection driver" value={form.driver} onChange={e=>{const driver=e.target.value as LabRuntimeInput["driver"];setForm(p=>({...p,driver,port:driver==="sqlserver"?1433:8123,database:driver==="sqlserver"?"DataFlowFixtureQA":"scratch",username:driver==="sqlserver"?"qa_erp_reader":"default",password:""}))}}>
      <option value="sqlserver">Microsoft SQL Server · source</option><option value="clickhouse">ClickHouse · scratch destination</option>
     </select></label>
     <div className="grid grid-cols-[minmax(0,1fr)_110px] gap-2">
      <label className="block text-xs space-y-1">Approved host<input className="field" aria-label="Live connection host" value={form.host} onChange={e=>modify("host",e.target.value)} placeholder="QA host only"/></label>
      <label className="block text-xs space-y-1">Port<input type="number" className="field" aria-label="Live connection port" min={1} max={65535} value={form.port} onChange={e=>modify("port",Number(e.target.value))}/></label>
     </div>
     <div className="grid gap-2 sm:grid-cols-2">
      <label className="block text-xs space-y-1">Database<input className="field" aria-label="Live connection database" value={form.database} onChange={e=>modify("database",e.target.value)}/></label>
      <label className="block text-xs space-y-1">Username<input className="field" aria-label="Live connection username" autoComplete="off" value={form.username} onChange={e=>modify("username",e.target.value)}/></label>
     </div>
     <label className="block text-xs space-y-1">Password<input className="field" aria-label="Live connection password" type="password" autoComplete="new-password" value={form.password} onChange={e=>modify("password",e.target.value)}/></label>
     <Button type="button" disabled={!!busy||!form.name.trim()||!form.host||!form.password} onClick={()=>void create()}><Plus size={15}/>{busy==="save"?"Saving…":"Save encrypted draft"}</Button>
    </div>
    <div className="space-y-3 rounded-md border p-3">
     <h4 className="font-semibold text-sm">Saved profiles ({profiles.length})</h4>
     {profiles.length===0&&<p className="muted text-xs">No saved connections.</p>}
     {profiles.map(p=><div key={p.id} className="rounded border p-3 space-y-2">
      <div className="flex items-start justify-between gap-2"><strong className="text-sm">{p.name}</strong><span className={p.status==="verified"?"df-text-success text-xs":"muted text-xs"}>{p.status==="verified"?"Verified":"Draft"}</span></div>
      <p className="muted text-xs">{p.driver} · {p.database} · revision {p.revision}</p>
      <Button type="button" size="sm" variant="outline" disabled={!!busy} onClick={()=>void test(p.id)}><TestTube2 size={15}/>{busy===p.id?"Testing…":"Test real connection"}</Button>
     </div>)}
    </div>
   </div>
   <div className="rounded-md border p-3 space-y-3">
    <h4 className="font-semibold text-sm flex items-center gap-2"><Link2 size={16}/> Bind verified connections to scratch pipeline</h4>
    <div className="grid gap-2 md:grid-cols-3">
     <label className="text-xs space-y-1">Managed scratch stream<select aria-label="Runtime bind stream" className="field" value={streamID} onChange={e=>setStreamID(e.target.value)}><option value="">Select stream</option>{streams.filter(s=>s.target_database==="scratch"&&s.target_table==="dataflow_raw_events").map(s=><option key={s.id} value={s.id}>{s.source_schema}.{s.source_table} · {s.id.slice(0,8)}</option>)}</select></label>
     <label className="text-xs space-y-1">SQL Server source<select aria-label="Runtime bind source" className="field" value={sourceID} onChange={e=>setSourceID(e.target.value)}><option value="">Select source</option>{profiles.filter(p=>p.driver==="sqlserver"&&p.status==="verified").map(p=><option key={p.id} value={p.id}>{p.name}</option>)}</select></label>
     <label className="text-xs space-y-1">ClickHouse destination<select aria-label="Runtime bind target" className="field" value={targetID} onChange={e=>setTargetID(e.target.value)}><option value="">Select target</option>{profiles.filter(p=>p.driver==="clickhouse"&&p.status==="verified").map(p=><option key={p.id} value={p.id}>{p.name}</option>)}</select></label>
    </div>
    <Button type="button" disabled={!!busy||!streamID||!sourceID||!targetID} onClick={()=>void bind()}><CheckCircle2 size={15}/> Bind to pipeline</Button>
    <p className="muted text-xs">The Core pipeline executes the same saved stream ID. Only recently verified QA profiles are allowed; changing a profile invalidates the saved binding.</p>
   </div>
  </>}
  <p className="muted text-xs flex items-start gap-2"><ShieldAlert size={15}/>Backend must have isolated QA flags, exact host:port allowlists and a private AES-256 key set. No production-grade IdP or Vault certification is claimed.</p>
 </section>
}
