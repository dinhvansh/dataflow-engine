import {useEffect,useRef,useState} from "react";
import {Database,HardDrive,RefreshCw,Server,ShieldCheck} from "lucide-react";
import {connectionApi,type ConfiguredConnection} from "../lib/api";
import {Button} from "./ui/button";

// Observational only: values come from a fixed, redacted, authenticated
// backend inventory. This view never asks for or renders connection secrets.
export function ConfiguredConnections({token,onTokenChange}:{token:string;onTokenChange:(next:string)=>void}){
 const[items,setItems]=useState<ConfiguredConnection[]|null>(null);
 const[loading,setLoading]=useState(false);
 const[error,setError]=useState("");
 const[testing,setTesting]=useState("");
 const[testResults,setTestResults]=useState<Record<string,boolean>>({});
 const testConnection=async(id:string)=>{setTesting(id);setError("");try{const result=await connectionApi.testConfigured(id,token);setTestResults(p=>({...p,[id]:result.passed}));}catch{setError("QA test unavailable. Check isolated lab flags, permissions and backend configuration.");}finally{setTesting("");}};
 const requestId=useRef(0);
 useEffect(()=>{requestId.current++;setItems(null);setError("");setLoading(false)},[token]);
 const changeToken=(value:string)=>{
  requestId.current++;
  setItems(null);setError("");setLoading(false);onTokenChange(value);
 };
 const refresh=async()=>{
  if(!token||loading)return;
  const current=++requestId.current;
  setLoading(true);setError("");
  try{
   const result=await connectionApi.list(token);
   if(current===requestId.current)setItems(result.connections);
  }catch{
   if(current===requestId.current){setItems(null);setError("Cannot load connection inventory. Check API token and backend availability.");}
  }finally{if(current===requestId.current)setLoading(false);}
 };
 const statusLabel=(state:ConfiguredConnection["configuration_state"])=>state==="configured"?"Configured · not tested":"Not configured";
 return <section className="hub-workspace space-y-4 rounded-lg border p-4 sm:p-5" aria-label="Configured connection inventory">
  <div className="flex flex-wrap items-start justify-between gap-3">
   <div><p className="hub-eyebrow mb-2">CONNECTION MANAGER / V3.1</p>
    <h3 className="flex items-center gap-2 text-lg font-semibold"><ShieldCheck size={18}/>Server configuration inventory</h3>
    <p className="muted mt-1 max-w-3xl text-xs leading-5">Read-only inventory of backend-configured SQL Server, ClickHouse and internal PostgreSQL slots. These server slots are separate from saved QA profiles; configured does not mean tested. No credentials, DSN or hostnames are returned.</p>
   </div>
   <span className="df-pipeline-stat">INVENTORY ONLY</span>
  </div>
  <div className="flex flex-wrap items-center gap-3"><p className="muted text-xs">{token==="session"?"Uses your signed-in QA session.":"Uses the in-memory Lab API token from the header."}</p><Button type="button" variant="outline" disabled={!token||loading} onClick={()=>{void refresh();}} aria-label="Refresh configured connections"><RefreshCw size={15}/>{loading?"Loading…":"Load inventory"}</Button></div>
  {!token&&<p className="muted text-xs" role="status">Enter an API token to read server-managed connections.</p>}
  {error&&<p role="alert" className="rounded border df-border-error df-bg-error-soft p-3 text-xs df-text-error">{error}</p>}
  {items&&<div className="grid gap-3 md:grid-cols-3">
   {items.map(item=>{
    const Icon=item.driver==="sqlserver"?Server:item.driver==="clickhouse"?HardDrive:Database;
    return <article key={item.id} className="hub-connector-card rounded border p-4 space-y-2" aria-label={"Connection "+item.id}>
     <div className="flex flex-wrap items-center justify-between gap-2">
      <Icon size={17} className="hub-accent"/>
      <span className={item.configuration_state==="configured"?"df-text-success text-xs font-semibold":"df-text-warning text-xs font-semibold"}>{statusLabel(item.configuration_state)}</span>
     </div>
     <h4 className="font-semibold text-sm">{item.name}</h4>
     <p className="muted text-xs">{item.driver} · {item.kind}</p>
     <p className="muted text-xs leading-5">{item.usage}</p>
     <div className="border-t df-border-neutral pt-2 space-y-2"><p className="text-xs muted">Server-managed single instance · isolated QA check only</p>{item.kind!=="internal"&&<Button type="button" size="sm" variant="outline" disabled={!token||!!testing||item.configuration_state!=="configured"} onClick={()=>void testConnection(item.id)}>{testing===item.id?"Testing…":"Test QA connection"}</Button>}{Object.prototype.hasOwnProperty.call(testResults,item.id)&&<p role="status" className={testResults[item.id]?"df-text-success text-xs":"df-text-error text-xs"}>{testResults[item.id]?"QA driver check PASS":"QA driver/permission check FAIL"}</p>}</div>
    </article>;
   })}
  </div>}
  {items&&<p className="muted text-xs" role="status">{items.length} server slots listed. This is the server slot inventory, not the saved QA profile list. Authorized Administrators can manage saved SQL Server and ClickHouse QA profiles above.</p>}
 </section>;
}
