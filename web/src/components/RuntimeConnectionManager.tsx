import {useCallback,useEffect,useState} from "react";
import {Ban,CheckCircle2,Database,KeyRound,Link2,Pencil,Plus,RefreshCw,ShieldAlert,TestTube2} from "lucide-react";
import {labRuntimeConnectionsApi,managedApi,type LabRuntimeConnection,type LabRuntimeInput,type ManagedStream} from "../lib/api";
import {Button} from "./ui/button";
import {RuntimeSyncSetupWizard} from "./RuntimeSyncSetupWizard";
import {RuntimeBatchSyncWizard} from "./RuntimeBatchSyncWizard";

// QA-only: authenticated profile operations are handled by the Go backend.
// Credential values never appear in API GET payloads, URL or localStorage.
// The owner must configure exact approved endpoints and encryption key server-side.
type WizardQAState={profiles:LabRuntimeConnection[];adminToken:string};
export function RuntimeConnectionManager({token,sessionAdmin=false,onQAState,refreshKey=0,showLegacyWizards=true,preferredDriver}:{
 token:string;sessionAdmin?:boolean;onQAState?:(state:WizardQAState)=>void;
 refreshKey?:number;showLegacyWizards?:boolean;preferredDriver?:LabRuntimeInput["driver"];
}){
 const[adminToken,setAdminToken]=useState(sessionAdmin?"session":"");
 const[profiles,setProfiles]=useState<LabRuntimeConnection[]>([]);
 useEffect(()=>{onQAState?.({profiles,adminToken})},[profiles,adminToken,onQAState]);
 const[streams,setStreams]=useState<ManagedStream[]>([]);
 const[loaded,setLoaded]=useState(false);
 const[busy,setBusy]=useState("");
 const[error,setError]=useState("");
 const[notice,setNotice]=useState("");
 const[form,setForm]=useState<LabRuntimeInput>({name:"",driver:"sqlserver",host:"",port:1433,database:"DataFlowFixtureQA",username:"qa_erp_reader",password:""});
 const[editing,setEditing]=useState<LabRuntimeConnection|null>(null);
 useEffect(()=>{if(!preferredDriver||editing)return;setForm(p=>({...p,driver:preferredDriver,port:preferredDriver==="sqlserver"?1433:8123,database:preferredDriver==="sqlserver"?"DataFlowFixtureQA":"scratch",username:preferredDriver==="sqlserver"?"qa_erp_reader":"default",password:""}))},[preferredDriver,editing]);
 const[confirmRevoke,setConfirmRevoke]=useState("");
 const[streamID,setStreamID]=useState("");
 const[sourceID,setSourceID]=useState("");
 const[targetID,setTargetID]=useState("");
 const modify=<K extends keyof LabRuntimeInput>(key:K,value:LabRuntimeInput[K])=>setForm(p=>({...p,[key]:value}));
 const reload=useCallback(async()=>{
  if(!token||!adminToken)return;
  setBusy("load");setError("");setNotice("");
  try{
   const [p,s]=await Promise.all([labRuntimeConnectionsApi.list(token,adminToken),managedApi.list(token)]);
   setProfiles(p.profiles);setStreams(s.streams);setLoaded(true);
  }catch{setProfiles([]);setLoaded(false);setError("Lab runtime manager unavailable. Check admin permission, migrations, backend feature flags and approved endpoints.");}
  finally{setBusy("");}
 },[token,adminToken]);
 useEffect(()=>{if(sessionAdmin&&token&&adminToken)void reload()},[sessionAdmin,reload,token,adminToken]);
 useEffect(()=>{if(refreshKey>0&&loaded&&token&&adminToken)void reload()},[refreshKey]);
 const create=async()=>{
  if(!token||!adminToken||(!editing&&!form.password))return;
  setBusy("save");setError("");setNotice("");
  try{
   const payload={...form};
   if(editing){await labRuntimeConnectionsApi.update(editing.id,editing.revision,{name:payload.name,host:payload.host,port:payload.port,database:payload.database,username:payload.username,password:payload.password},token,adminToken)}
   else{await labRuntimeConnectionsApi.create(payload,token,adminToken)}
   setForm(p=>({...p,name:"",password:""})); // remove secret from form state after save
   setEditing(null);
   setNotice("Profile saved as DRAFT. Run real Test before binding or running a stream.");
   const result=await labRuntimeConnectionsApi.list(token,adminToken);
   setProfiles(result.profiles);
  }catch{setError("Could not save profile. Check revision, active runs, approved endpoint and credential store.");}
  finally{setBusy("");}
 };
 const edit=(p:LabRuntimeConnection)=>{setEditing(p);setConfirmRevoke("");setForm({name:p.name,driver:p.driver,host:p.host,port:p.port,database:p.database,username:p.username,password:""});setError("");setNotice("Editing invalidates previous verification and bindings. Leave password blank to retain the encrypted credential.")};
 const revoke=async(p:LabRuntimeConnection)=>{
  setBusy("revoke-"+p.id);setError("");setNotice("");
  try{await labRuntimeConnectionsApi.revoke(p.id,p.revision,token,adminToken);setConfirmRevoke("");setEditing(null);setForm(f=>({...f,password:""}));setProfiles((await labRuntimeConnectionsApi.list(token,adminToken)).profiles);setNotice("Profile revoked; saved streams and checkpoints remain, but bound runs are blocked.")}
  catch{setError("Revocation refused. Check revision and active runs.")}
  finally{setBusy("")}
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
   {!sessionAdmin&&<label className="space-y-1"><span className="muted text-xs">Connection admin credential · separate from lab API token</span>
    <input className="field" type="password" autoComplete="off" aria-label="Live connection admin credential" value={adminToken} onChange={e=>{setAdminToken(e.target.value);setProfiles([]);setLoaded(false);setError("");}}/>
   </label>}
   <Button variant="outline" size="sm" type="button" disabled={!token||!adminToken||!!busy} onClick={()=>void reload()}><RefreshCw size={15}/> Load connections</Button>
  </div>
  {error&&<p className="df-text-error text-xs" role="alert">{error}</p>}
  {notice&&<p className="df-text-success text-xs" role="status">{notice}</p>}
  {loaded&&<>
   <div className="grid gap-3 lg:grid-cols-2">
    <div className="space-y-3 rounded-md border p-3">
     <h4 className="font-semibold text-sm">{editing?"Edit connection · revision "+editing.revision:"Add connection"}</h4>
     <label className="block text-xs space-y-1">Connection name<input className="field" aria-label="Live connection name" value={form.name} maxLength={80} onChange={e=>modify("name",e.target.value)}/></label>
     <label className="block text-xs space-y-1">Database type<select className="field" aria-label="Live connection driver" value={form.driver} disabled={!!editing} onChange={e=>{const driver=e.target.value as LabRuntimeInput["driver"];setForm(p=>({...p,driver,port:driver==="sqlserver"?1433:8123,database:driver==="sqlserver"?"DataFlowFixtureQA":"scratch",username:driver==="sqlserver"?"qa_erp_reader":"default",password:""}))}}>
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
     <label className="block text-xs space-y-1">Password{editing?" · optional replacement":""}<input className="field" aria-label="Live connection password" type="password" autoComplete="new-password" value={form.password} onChange={e=>modify("password",e.target.value)}/></label>
     <div className="flex gap-2"><Button type="button" disabled={!!busy||!form.name.trim()||!form.host||(!editing&&!form.password)} onClick={()=>void create()}><Plus size={15}/>{busy==="save"?"Saving…":editing?"Save revision":"Save encrypted draft"}</Button>{editing&&<Button type="button" variant="outline" onClick={()=>{setEditing(null);setForm(p=>({...p,name:"",password:""}))}}>Cancel edit</Button>}</div>
    </div>
    <div className="space-y-3 rounded-md border p-3" aria-label="Saved QA profiles">
     <h4 className="font-semibold text-sm">Saved profiles ({profiles.length})</h4>
     {profiles.length===0&&<p className="muted text-xs">No saved connections.</p>}
     {profiles.map(p=><div key={p.id} className="rounded border p-3 space-y-2">
      <div className="flex items-start justify-between gap-2"><strong className="text-sm">{p.name}</strong><span className={p.status==="verified"?"df-text-success text-xs":"muted text-xs"}>{p.status==="verified"?"Verified":p.status==="revoked"?"Revoked":"Draft"}</span></div>
      <p className="muted text-xs">{p.driver} · {p.database} · revision {p.revision}</p>
      {p.status!=="revoked"&&<div className="flex flex-wrap gap-2"><Button type="button" size="sm" variant="outline" disabled={!!busy} onClick={()=>void test(p.id)}><TestTube2 size={15}/>{busy===p.id?"Testing…":"Test real connection"}</Button><Button type="button" size="sm" variant="outline" disabled={!!busy} onClick={()=>edit(p)}><Pencil size={14}/>Edit</Button><Button type="button" size="sm" variant="outline" disabled={!!busy} onClick={()=>setConfirmRevoke(p.id)}><Ban size={14}/>Revoke</Button></div>}
      {confirmRevoke===p.id&&<div className="space-y-2 rounded border p-2 text-xs" role="group" aria-label={"Confirm revoke "+p.name}><p>Revoke revision {p.revision}? Bound streams will stop running; metadata and checkpoints stay intact.</p><div className="flex gap-2"><Button type="button" size="sm" disabled={!!busy} onClick={()=>void revoke(p)}>Confirm revoke</Button><Button type="button" size="sm" variant="outline" onClick={()=>setConfirmRevoke("")}>Cancel</Button></div></div>}
     </div>)}
    </div>
   </div>
   {showLegacyWizards&&<>
    <RuntimeBatchSyncWizard token={token} adminToken={adminToken} profiles={profiles} onCreated={()=>void reload()}/>
    <RuntimeSyncSetupWizard token={token} adminToken={adminToken} profiles={profiles} onCreated={()=>void reload()}/>
   </>}
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
