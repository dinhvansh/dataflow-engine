import {useEffect,useMemo,useState} from "react";
import {Database,Play,RefreshCw,Save,Search,ShieldCheck} from "lucide-react";
import {Button} from "./ui/button";
import {labSyncSetupApi,managedApi,type ColumnInfo,type LabRuntimeConnection,type ManagedRun,type TableInfo} from "../lib/api";

type Props={token:string;adminToken:string;profiles:LabRuntimeConnection[];onCreated:()=>void};
const mandatory=["SysRowID","SysRevID"];

// A complete *QA incremental* path: verified connection -> discovery on that
// connection -> selected columns -> atomic stream+binding -> managed Run.
// Full refresh and general destinations are intentionally not advertised as
// runtime-bound features until the engine supports those configurations.
export function RuntimeSyncSetupWizard({token,adminToken,profiles,onCreated}:Props){
 const [sourceID,setSourceID]=useState("");
 const [targetID,setTargetID]=useState("");
 const [schema,setSchema]=useState("Erp");
 const [table,setTable]=useState("");
 const [tables,setTables]=useState<TableInfo[]>([]);
 const [columns,setColumns]=useState<ColumnInfo[]>([]);
 const [selected,setSelected]=useState<string[]>([]);
 const [query,setQuery]=useState("");
 const [batch,setBatch]=useState(10000);
 const [workers,setWorkers]=useState(2);
 const [queue,setQueue]=useState(4);
 const [busy,setBusy]=useState("");
 const [notice,setNotice]=useState("");
 const [error,setError]=useState("");
 const [streamID,setStreamID]=useState("");
 const [run,setRun]=useState<ManagedRun|null>(null);
 const verifiedSources=profiles.filter(p=>p.driver==="sqlserver"&&p.status==="verified");
 const verifiedTargets=profiles.filter(p=>p.driver==="clickhouse"&&p.status==="verified");
 const canSave=!!sourceID&&!!targetID&&!!table&&columns.length>0&&
    mandatory.every(n=>selected.includes(n))&&batch>=100&&batch<=100000&&
    workers>=1&&workers<=32&&queue>=1&&queue<=100&&!busy;
 const filtered=useMemo(()=>columns.filter(c=>(c.name+" "+c.sql_type).toLowerCase().includes(query.trim().toLowerCase())),[columns,query]);
 const clearStream=()=>{setStreamID("");setRun(null);setNotice("");setError("")};
 useEffect(()=>{setSourceID("");setTargetID("");setTable("");setTables([]);setColumns([]);setSelected([]);clearStream()},[token,adminToken]);
 useEffect(()=>{
  if(!run||!["queued","running"].includes(run.status)||!token)return;
  let cancelled=false;
  const handle=window.setInterval(()=>{
   managedApi.status(run.run_id,token).then(r=>{if(!cancelled)setRun(r)}).catch(()=>{});
  },4000);
  return()=>{cancelled=true;window.clearInterval(handle)};
 },[run?.run_id,run?.status,token]);
 const task=async(id:string,fn:()=>Promise<void>)=>{
  setBusy(id);setError("");setNotice("");
  try{await fn()}catch(e){setError(String(e))}
  finally{setBusy("")}
 };
 const discover=()=>task("discover",async()=>{
  if(!sourceID)throw Error("Choose a tested SQL Server profile.");
  const r=await labSyncSetupApi.tables(sourceID,schema,token,adminToken);
  setTables(r.tables);setTable("");setColumns([]);setSelected([]);clearStream();
 });
 const inspect=()=>task("inspect",async()=>{
  if(!table)throw Error("Choose a source table.");
  const r=await labSyncSetupApi.columns(sourceID,schema,table,token,adminToken);
  const key=r.columns.find(c=>c.name==="SysRowID"&&c.sql_type.toLowerCase()==="uniqueidentifier");
  const cursor=r.columns.find(c=>c.name==="SysRevID"&&c.cursor_candidate);
  if(!key||!cursor)throw Error("This table does not support the current RowVersion managed sync (SysRowID + SysRevID required).");
  setColumns(r.columns);setSelected(r.columns.map(c=>c.name));clearStream();
 });
 const save=()=>task("save",async()=>{
  if(!canSave)return;
  const r=await labSyncSetupApi.create({
   source_connection_id:sourceID,target_connection_id:targetID,source_schema:schema,
   source_table:table,selected_columns:columns.filter(c=>selected.includes(c.name)).map(c=>c.name),
   target_database:"scratch",target_table:"dataflow_raw_events",
   mode:"incremental_rowversion",batch_size:batch,workers,queue_capacity:queue
  },token,adminToken);
  setStreamID(r.stream_id);setNotice("Saved and bound stream "+r.stream_id+". Ready for QA incremental Run.");onCreated();
 });
 const runStream=()=>task("run",async()=>{
  if(!streamID)throw Error("Save a bound stream first.");
  const r=await managedApi.run(streamID,token);setRun(r);setNotice("Managed run queued; status refreshes automatically.");
 });
 return <section className="rounded-md border p-4 space-y-4" aria-label="QA sync setup wizard">
  <div className="flex flex-wrap items-start justify-between gap-2">
   <div><h4 className="font-semibold flex items-center gap-2"><Database size={16}/>Sync Setup · saved connections</h4>
    <p className="muted text-xs mt-1">Configure a new read-only SQL Server stream, save + bind atomically, then run to ClickHouse scratch. No credential re-entry.</p></div>
   <span className="rounded border px-2 py-1 text-[11px]">QA Incremental</span>
  </div>
  {verifiedSources.length===0||verifiedTargets.length===0?
   <p className="df-text-warning text-xs">Add and Test both SQL Server and ClickHouse profiles above before creating a Sync Job.</p>:null}
  <div className="grid gap-3 md:grid-cols-2">
   <label className="text-xs space-y-1">1. Verified source connection
    <select className="field" aria-label="Wizard source connection" value={sourceID} onChange={e=>{setSourceID(e.target.value);setTables([]);setTable("");setColumns([]);setSelected([]);clearStream()}}>
     <option value="">Choose SQL Server</option>{verifiedSources.map(p=><option key={p.id} value={p.id}>{p.name} · rev {p.revision}</option>)}
    </select>
   </label>
   <label className="text-xs space-y-1">2. Verified destination
    <select className="field" aria-label="Wizard target connection" value={targetID} onChange={e=>{setTargetID(e.target.value);clearStream()}}>
     <option value="">Choose ClickHouse</option>{verifiedTargets.map(p=><option key={p.id} value={p.id}>{p.name} · rev {p.revision}</option>)}
    </select>
   </label>
  </div>
  <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] items-end">
   <label className="text-xs space-y-1">3. SQL Schema
    <input className="field" aria-label="Wizard schema" value={schema} onChange={e=>{setSchema(e.target.value);setTables([]);setTable("");setColumns([]);setSelected([]);clearStream()}}/>
   </label>
   <label className="text-xs space-y-1">Table
    <select className="field" aria-label="Wizard source table" value={table} onChange={e=>{setTable(e.target.value);setColumns([]);setSelected([]);clearStream()}}>
     <option value="">Choose table</option>{tables.map(t=><option key={t.schema+"."+t.name} value={t.name}>{t.schema}.{t.name}</option>)}
    </select>
   </label>
   <div className="flex gap-2"><Button type="button" variant="outline" disabled={!!busy||!sourceID} onClick={discover}><RefreshCw size={14}/>Discover</Button><Button type="button" variant="outline" disabled={!!busy||!table} onClick={inspect}><Search size={14}/>Columns</Button></div>
  </div>
  {columns.length>0&&<div className="space-y-2">
   <div className="flex flex-wrap items-center justify-between gap-2">
    <span className="text-xs font-semibold">4. Select columns ({selected.length}/{columns.length})</span>
    <Button type="button" variant="outline" size="sm" onClick={()=>{setSelected(columns.map(c=>c.name));clearStream()}}>Select all</Button>
   </div>
   <input type="search" className="field" aria-label="Wizard column filter" value={query} placeholder="Filter columns..." onChange={e=>setQuery(e.target.value)}/>
   <div className="max-h-52 overflow-y-auto rounded border" role="group" aria-label="Wizard column selection">
    {filtered.map(c=><label className="border-b p-2 flex items-center gap-3 text-xs" key={c.name}>
     <input type="checkbox" aria-label={"Wizard column "+c.name} checked={selected.includes(c.name)} disabled={mandatory.includes(c.name)} onChange={e=>{setSelected(old=>e.target.checked?[...old,c.name]:old.filter(v=>v!==c.name));clearStream()}}/>
     <span className="font-mono flex-1 truncate">{c.name}</span><span className="muted">{c.sql_type}</span>
     {mandatory.includes(c.name)&&<span className="df-text-accent">key/cursor</span>}
    </label>)}
   </div>
  </div>}
  <div className="grid gap-3 md:grid-cols-4">
   <label className="text-xs space-y-1">5. Sync mode
    <select className="field" aria-label="Wizard sync mode" value="incremental_rowversion" disabled>
     <option value="incremental_rowversion">Incremental · RowVersion</option>
    </select>
   </label>
   {([["Batch size",batch,setBatch,100,100000],["Workers",workers,setWorkers,1,32],["Queue",queue,setQueue,1,100]] as const).map(([name,value,setValue,min,max])=>
    <label key={name} className="text-xs space-y-1">{name}
     <input className="field" aria-label={"Wizard "+name} type="number" min={min} max={max} value={value}
      onChange={e=>{setValue(Number(e.target.value));clearStream()}}/>
    </label>)}
  </div>
  <p className="muted text-xs">Destination: scratch.dataflow_raw_events. Full Refresh, other cursor types, custom destination mappings and multi-table batch Save are separate runtime capabilities and are not enabled in this QA wizard.</p>
  <div className="flex flex-wrap gap-2">
   <Button type="button" disabled={!canSave||!!streamID} onClick={save}><Save size={15}/>{busy==="save"?"Saving…":"Save stream + Bind"}</Button>
   <Button type="button" disabled={!streamID||!!busy} onClick={runStream}><Play size={15}/>Run selected stream</Button>
  </div>
  {error&&<p role="alert" className="df-text-error text-xs">{error}</p>}
  {notice&&<p role="status" className="df-text-success text-xs">{notice}</p>}
  {streamID&&<div className="text-xs break-all">Saved stream ID: <code>{streamID}</code></div>}
  {run&&<div className="rounded border p-3 space-y-1 text-xs"><p><ShieldCheck size={14} className="inline"/> Run status: <strong>{run.status}</strong></p>
    <p>Run: <code>{run.run_id}</code></p><p>Rows: {run.rows_written??0} · Batches: {run.batches_written??0}</p>
    {run.error&&<p className="df-text-error">{run.error}</p>}
  </div>}
 </section>;
}
