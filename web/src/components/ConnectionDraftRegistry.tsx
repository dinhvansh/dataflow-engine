import {useEffect,useRef,useState} from "react";
import {Plus,RefreshCw,ShieldAlert} from "lucide-react";
import {connectionDefinitionsApi,type DraftConnection} from "../lib/api";
import {Button} from "./ui/button";

// Draft records are disconnected metadata; this editor cannot configure a host,
// capture database credentials, bind jobs or trigger a network connection.
export function ConnectionDraftRegistry({token}:{token:string}){
 const[adminToken,setAdminToken]=useState("");
 const[name,setName]=useState("");
 const[driver,setDriver]=useState<"sqlserver"|"clickhouse">("sqlserver");
 const[definitions,setDefinitions]=useState<DraftConnection[]|null>(null);
 const[writeEnabled,setWriteEnabled]=useState(false);
 const[busy,setBusy]=useState(false);
 const[error,setError]=useState("");
 const[notice,setNotice]=useState("");
 const requestSerial=useRef(0);
 useEffect(()=>{
  requestSerial.current++;
  setDefinitions(null);setWriteEnabled(false);
  setAdminToken("");setError("");setNotice("");setBusy(false);
 },[token]);
 const reload=async()=>{
  if(!token||busy)return;
  const id=++requestSerial.current;
  setBusy(true);setError("");
  try{
   const response=await connectionDefinitionsApi.list(token,adminToken);
   if(requestSerial.current===id){
    setDefinitions(response.definitions);
    setWriteEnabled(response.write_enabled);
   }
  }catch{
   if(requestSerial.current===id){
    setDefinitions(null);setWriteEnabled(false);
    setError("Could not load draft registry. Check the API token, feature flag and backend migration.");
   }
  }finally{if(requestSerial.current===id)setBusy(false);}
 };
 const create=async()=>{
  if(!token||!adminToken||!writeEnabled||busy)return;
  const label=name.trim();
  if(!/^[A-Za-z][A-Za-z0-9 _.-]{0,79}$/.test(label)){
   setError("Name must start with a letter and use 1–80 letters, numbers, spaces, dots, dashes or underscores.");
   return;
  }
  const id=++requestSerial.current;
  setBusy(true);setError("");setNotice("");
  try{
   const kind=driver==="sqlserver"?"source":"destination";
   const created=await connectionDefinitionsApi.create({name:label,driver,kind},token,adminToken);
   if(requestSerial.current!==id)return;
   const listed=await connectionDefinitionsApi.list(token,adminToken);
   if(requestSerial.current!==id)return;
   setDefinitions(listed.definitions);setWriteEnabled(listed.write_enabled);
   setNotice("Created inactive draft: "+created.name+". No database connection was attempted.");
   setName("");
  }catch{
   if(requestSerial.current===id)setError("Could not create definition (check admin permission, duplicate name or feature flag).");
  }finally{if(requestSerial.current===id)setBusy(false);}
 };
 return <section className="hub-workspace rounded-lg border p-4 sm:p-5 space-y-4" aria-label="Connection draft registry">
  <div className="flex flex-wrap items-start justify-between gap-3">
   <div>
    <p className="hub-eyebrow mb-2">CONNECTION MANAGER / V3.2A</p>
    <h3 className="text-lg font-semibold">Draft connection definitions</h3>
    <p className="muted mt-1 max-w-3xl text-xs leading-5">Register multiple named source/destination slots in the isolated lab control plane. Drafts are inactive: no endpoint, credential, Test Connection, Sync Job binding or execution is available.</p>
   </div>
   <span className="df-pipeline-stat">LAB METADATA · DRAFT ONLY</span>
  </div>
  <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end">
   <label className="space-y-1"><span className="muted text-xs">Connection admin token · separate elevated permission, memory only</span>
    <input className="field" aria-label="Connection registry admin token" autoComplete="off" type="password" value={adminToken}
     onChange={event=>{requestSerial.current++;setAdminToken(event.target.value);setDefinitions(null);setWriteEnabled(false);setError("");setNotice("");setBusy(false);}}/>
   </label>
   <Button type="button" size="sm" variant="outline" disabled={!token||busy} onClick={()=>{void reload();}}><RefreshCw size={15}/>Load drafts</Button>
  </div>
  {definitions&&<div className="space-y-3">
   <div className="flex flex-wrap items-end gap-3">
    <label className="min-w-[180px] flex-1 space-y-1"><span className="muted text-xs">Connection display name</span>
     <input aria-label="Draft connection name" className="field" maxLength={80} value={name} onChange={event=>setName(event.target.value)} placeholder="QA Analytics Source"/>
    </label>
    <label className="min-w-[180px] space-y-1"><span className="muted text-xs">Connector type</span>
     <select aria-label="Draft connection type" className="field" value={driver} onChange={event=>setDriver(event.target.value as "sqlserver"|"clickhouse")}>
      <option value="sqlserver">SQL Server · source</option>
      <option value="clickhouse">ClickHouse · destination</option>
     </select>
    </label>
    <Button type="button" size="sm" disabled={busy||!writeEnabled||!name.trim()} onClick={()=>{void create();}}><Plus size={15}/>Create draft</Button>
   </div>
   {!writeEnabled&&<p className="df-text-warning flex items-start gap-2 text-xs"><ShieldAlert size={15} className="shrink-0"/>Registry create is disabled. Backend requires the explicit feature flag and a separate connection admin token; viewing drafts never grants permission to create.</p>}
   <div className="overflow-x-auto rounded border df-border-neutral">
    <table className="w-full min-w-[475px] text-xs">
     <thead className="enterprise-table-head"><tr><th className="p-3 text-left">Name</th><th className="p-3 text-left">Driver / role</th><th className="p-3 text-left">State</th><th className="p-3 text-left">Scope</th></tr></thead>
     <tbody>{definitions.map(d=><tr key={d.id} className="border-t df-border-neutral" aria-label={"Draft definition "+d.name}>
      <td className="p-3 font-semibold break-words">{d.name}<span className="muted mt-1 block break-all font-mono text-[10px]">{d.id}</span></td>
      <td className="p-3">{d.driver} / {d.kind}</td>
      <td className="p-3 df-text-warning">DRAFT · inactive</td>
      <td className="p-3">{d.workspace}</td>
     </tr>)}</tbody>
    </table>
   </div>
   {definitions.length===0&&<p className="muted text-xs">No draft connection definitions in the registry.</p>}
   <p className="muted text-xs">{definitions.length} drafts shown · maximum 100 latest entries · cannot be used to run jobs</p>
  </div>}
  {notice&&<p role="status" className="df-text-success text-xs">{notice}</p>}
  {error&&<p role="alert" className="df-text-error text-xs">{error}</p>}
 </section>;
}
