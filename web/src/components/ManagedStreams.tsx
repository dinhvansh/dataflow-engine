import {ReconciliationPanel} from "./ReconciliationPanel";
import {SchedulePanel} from "./SchedulePanel";
import {useEffect,useState} from "react";
import {Activity,Database,Play,RefreshCw,ShieldCheck,Search} from "lucide-react";
import {Button} from "./ui/button";
import {managedApi,sourceApi,type TableInfo,type ColumnInfo,type ManagedStream,type ManagedRun} from "../lib/api";

type Settings=Omit<ManagedStream,"id"|"lower_cursor">;
const initial:Settings={source_schema:"Erp",source_table:"Part",target_database:"scratch",target_table:"dataflow_raw_events",batch_size:10000,workers:2,queue_capacity:4};
export function ManagedStreams({token}:{token:string}){
 const[config,setConfig]=useState<Settings>(initial);
 const[streams,setStreams]=useState<ManagedStream[]>([]);
 const[tables,setTables]=useState<TableInfo[]>([]);
 const[columns,setColumns]=useState<ColumnInfo[]>([]);
 const[selected,setSelected]=useState<string[]>([]);
 const[inspected,setInspected]=useState("");
 const[columnQuery,setColumnQuery]=useState("");
 const[run,setRun]=useState<ManagedRun|null>(null);
 const[busy,setBusy]=useState("");
 const[error,setError]=useState("");
 const[notice,setNotice]=useState("");
 const edit=(key:Exclude<keyof Settings,"selected_columns">,value:string)=>{
  if(key==="source_schema"||key==="source_table"){setColumns([]);setSelected([]);setInspected("");setTables(key==="source_schema"?[]:tables)}
  setConfig(prev=>({...prev,[key]:typeof prev[key]==="number"?Number(value):value}));
 };
 const task=async(name:string,fn:()=>Promise<void>)=>{setBusy(name);setError("");setNotice("");try{await fn()}catch(e){setError(String(e))}finally{setBusy("")}};
 const load=()=>task("list",async()=>{const x=await managedApi.list(token);setStreams(x.streams)});
 const discover=()=>task("discover",async()=>{const found=await sourceApi.tables(config.source_schema,token);setTables(found)});
 const inspect=()=>task("inspect",async()=>{
  const found=await sourceApi.columns(config.source_schema,config.source_table,token);
  if(!found.some(c=>c.name==="SysRevID"&&c.cursor_candidate)||!found.some(c=>c.name==="SysRowID"&&c.sql_type==="uniqueidentifier"))throw Error("This stream requires SysRevID (rowversion) and SysRowID (uniqueidentifier).");
  setColumns(found);setSelected(found.map(c=>c.name));setInspected(config.source_schema+"."+config.source_table);
 });
 const validSelection=inspected===config.source_schema+"."+config.source_table&&columns.length>0&&selected.length>0&&selected.includes("SysRevID")&&selected.includes("SysRowID");
 const create=()=>task("create",async()=>{
  if(!validSelection)throw Error("Inspect the selected table and choose valid columns before saving.");
  // A missing projection means legacy all-column behavior; a subset is persisted.
  const payload:Settings={...config,selected_columns:selected.length===columns.length?undefined:columns.filter(c=>selected.includes(c.name)).map(c=>c.name)};
  const result=await managedApi.create(payload,token);
  setNotice(`Stream saved: ${result.id}`);setStreams((await managedApi.list(token)).streams)
 });
 const execute=(id:string)=>task("execute",async()=>{const r=await managedApi.run(id,token);setRun(r);setNotice("Run queued; inspect status below.")});
 const refresh=()=>task("refresh",async()=>{if(!run)return;setRun(await managedApi.status(run.run_id,token))});
 useEffect(()=>{
  if(!token||!run||!["queued","running"].includes(run.status))return;
  let cancelled=false;
  const poll=async()=>{try{const next=await managedApi.status(run.run_id,token);if(!cancelled)setRun(next)}catch{/* manual refresh displays errors */}};
  const interval=setInterval(()=>{void poll()},4000);
  return()=>{cancelled=true;clearInterval(interval)};
 },[run?.run_id,run?.status,token]);
 return <section className="df-studio-workspace df-managed-workspace section mt-5 space-y-4">
  <header className="flex flex-wrap items-center justify-between gap-2"><div><h2 className="flex items-center gap-2 text-lg font-semibold"><Database size={18}/>Stream Configuration · RowVersion</h2><p className="muted mt-1 text-sm">Persistent PostgreSQL cursor · rowversion + SysRowID · ClickHouse logical dedup.</p></div><span className="rounded-md df-bg-warning-soft px-3 py-1 text-xs df-text-warning">Scratch only</span></header>
  <p className="rounded-md border df-border-warning df-bg-warning-soft p-3 text-xs df-text-warning">The backend must explicitly enable DATAFLOW_ALLOW_TEST_LOAD and DATAFLOW_ENABLE_MANAGED_TEST. This reads Epicor SELECT-only and appends to scratch.dataflow_raw_events. It does not detect physical deletes. No production cutover.</p>
  <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
   {(["source_schema","source_table","target_database","target_table"] as const).map(k=><label key={k} className="space-y-1"><span className="muted text-xs">{k.replaceAll("_"," ")}</span><input className="field" value={config[k]} onChange={e=>edit(k,e.target.value)}/></label>)}
  </div>
  <section className="rounded-md border p-4 space-y-3" aria-label="Airbyte-style source discovery and field projection">
   <div className="flex flex-wrap items-center justify-between gap-3"><div><h3 className="font-semibold text-sm">Discover source schema &amp; columns</h3><p className="muted text-xs">Read-only metadata. Select fields for this stream; cursor/key columns remain mandatory.</p></div>
    <div className="flex flex-wrap gap-2"><Button type="button" variant="outline" disabled={!!busy||!token} onClick={discover}><RefreshCw size={14}/>Discover tables</Button><Button type="button" variant="outline" disabled={!!busy||!token||!config.source_table} onClick={inspect}><Search size={14}/>Inspect columns</Button></div>
   </div>
   {tables.length>0&&<label className="block space-y-1 text-xs">Tables in {config.source_schema}
    <select className="field" aria-label="Discovered source table" value={config.source_table} onChange={e=>edit("source_table",e.target.value)}>
     {!tables.some(t=>t.name===config.source_table)&&<option value={config.source_table}>{config.source_table}</option>}
     {tables.map(t=><option key={t.schema+"."+t.name} value={t.name}>{t.schema}.{t.name}</option>)}
    </select></label>}
   {inspected===config.source_schema+"."+config.source_table&&columns.length>0&&<>
    <div className="flex flex-wrap items-center justify-between gap-3"><p className="text-xs font-semibold">{selected.length} / {columns.length} columns</p>
     <Button type="button" size="sm" variant="outline" onClick={()=>setSelected(columns.map(c=>c.name))}>Select all</Button></div>
    <input className="field" type="search" aria-label="Filter source columns" placeholder="Filter columns by name or type" value={columnQuery} onChange={e=>setColumnQuery(e.target.value)}/>
    <div className="max-h-64 overflow-y-auto rounded border" role="group" aria-label="Column projection">
    {columns.filter(c=>(c.name+" "+c.sql_type).toLowerCase().includes(columnQuery.toLowerCase())).map(c=>{
     const required=c.name==="SysRowID"||c.name==="SysRevID";
     return <label key={c.name} className="flex items-center gap-3 border-b p-2 text-xs">
      <input type="checkbox" aria-label={"Sync column "+c.name} checked={selected.includes(c.name)} disabled={required} onChange={e=>setSelected(old=>e.target.checked?[...old,c.name]:old.filter(v=>v!==c.name))}/>
      <span className="min-w-0 flex-1 truncate font-mono">{c.name}</span><span className="muted">{c.sql_type}</span>{required&&<span className="df-text-accent">required</span>}
     </label>;
    })}</div>
    <p className="muted text-xs">Existing stream projection is immutable after Save. For the current managed engine only RowVersion incremental is supported; Full Refresh is a separate one-time bootstrap path.</p>
   </>}
  </section>
  <div className="grid gap-3 sm:grid-cols-3">
   {(["batch_size","workers","queue_capacity"] as const).map(k=><label key={k} className="space-y-1"><span className="muted text-xs">{k.replaceAll("_"," ")}</span><input className="field" type="number" min={k==="batch_size"?100:1} max={k==="batch_size"?100000:k==="workers"?32:100} value={config[k]} onChange={e=>edit(k,e.target.value)}/></label>)}
  </div>
  <div className="flex flex-wrap gap-2"><Button disabled={!!busy||!token||!validSelection} onClick={create}><ShieldCheck size={15}/>Save selected stream</Button><Button variant="outline" disabled={!!busy||!token} onClick={load}><RefreshCw size={15}/>Refresh Streams</Button></div>
  {notice&&<p className="text-xs df-text-success">{notice}</p>}
  {error&&<p role="alert" className="rounded-md df-bg-error-soft p-3 text-sm df-text-error">{error}</p>}
  {streams.length>0&&<div className="overflow-x-auto"><table className="w-full min-w-[690px] text-left text-xs"><thead><tr className="border-b df-border-neutral df-text-neutral"><th className="py-3">Source → Target</th><th>Checkpoint (lower inclusive)</th><th>Performance</th><th>Action</th></tr></thead><tbody>{streams.map(s=><tr key={s.id} className="border-b df-border-neutral"><td className="py-3">{s.source_schema}.{s.source_table}<span className="muted block">{s.target_database}.{s.target_table} · {s.selected_columns?.length?s.selected_columns.length+" selected columns":"all columns"}</span></td><td className="font-mono df-text-accent">{s.lower_cursor||"—"}</td><td>{s.batch_size} batch · {s.workers} workers · {s.queue_capacity} queue</td><td><Button size="sm" disabled={!!busy||!token} onClick={()=>execute(s.id)}><Play size={14}/>Run</Button></td></tr>)}</tbody></table></div>}
  {run&&<div className="rounded-lg border df-border-neutral df-surface-soft p-4"><div className="mb-2 flex items-center justify-between gap-2"><span className="flex items-center gap-2 font-semibold"><Activity size={16}/>Async run · {run.status}</span><Button size="sm" variant="outline" disabled={!!busy} onClick={refresh}><RefreshCw size={14}/>Status</Button></div><p className="muted break-all text-xs">Run ID: {run.run_id}</p><div className="mt-3 grid gap-2 text-sm sm:grid-cols-2"><p>Read boundary: <code>{run.lower_cursor||"—"}</code></p><p>Next lower: <code>{run.upper_exclusive||"—"}</code></p><p>Rows: {run.rows_written??0}</p><p>Batches: {run.batches_written??0}</p></div>{run.error&&<p className="mt-2 text-sm df-text-error">{run.error}</p>}</div>}
  <SchedulePanel token={token} streams={streams}/>
  <ReconciliationPanel token={token} streams={streams}/>
 </section>;
}
