import {useEffect,useState} from "react";
import {Database,Play,RefreshCw,Save,Search,Trash2} from "lucide-react";
import {Button} from "./ui/button";
import {labQABatchSyncApi,labSyncSetupApi,managedApi,
 type LabRuntimeConnection,type TableInfo,type ColumnInfo,
 type QABatchStream,type QABatchResult,type ManagedRun} from "../lib/api";

type Props={token:string;adminToken:string;profiles:LabRuntimeConnection[];onCreated:()=>void};
const tableAlias=/^[a-z][a-z0-9_]{0,29}$/;
const targetIdentifier=/^[A-Za-z_][A-Za-z0-9_]{0,63}$/;
function defaultAlias(schema:string,table:string){
 const v=(schema+"_"+table).toLowerCase().replace(/[^a-z0-9_]/g,"_").slice(0,30);
 return /^[a-z]/.test(v)?v:"table_"+v.slice(0,24);
}
export function RuntimeBatchSyncWizard({token,adminToken,profiles,onCreated}:Props){
 const[sourceID,setSourceID]=useState("");
 const[targetID,setTargetID]=useState("");
 const[schema,setSchema]=useState("Erp");
 const[table,setTable]=useState("");
 const[tables,setTables]=useState<TableInfo[]>([]);
 const[columns,setColumns]=useState<ColumnInfo[]>([]);
 const[selected,setSelected]=useState<string[]>([]);
 const[keyColumn,setKeyColumn]=useState("SysRowID");
 const[cursorColumn,setCursorColumn]=useState("SysRevID");
 const requiredCols=[keyColumn,...(cursorColumn?[cursorColumn]:[])];
 const[mode,setMode]=useState<QABatchStream["mode"]>("incremental_rowversion");
 const[alias,setAlias]=useState("part");
 const[layout,setLayout]=useState<"raw_event"|"typed">("raw_event");
 const[targetDatabase,setTargetDatabase]=useState("scratch");
 const[targetNames,setTargetNames]=useState<Record<string,string>>({});
 const[batchSize,setBatchSize]=useState(10000);
 const[workers,setWorkers]=useState(2);
 const[queueSize,setQueueSize]=useState(4);
 const[queue,setQueue]=useState<QABatchStream[]>([]);
 const[results,setResults]=useState<QABatchResult[]>([]);
 const[runs,setRuns]=useState<Record<string,ManagedRun>>({});
 const[busy,setBusy]=useState("");
 const[error,setError]=useState("");
 const[notice,setNotice]=useState("");
 const clearInspect=()=>{setTargetNames({});setTable("");setColumns([]);setSelected([]);setTables([]);setResults([]);setQueue([]);setRuns({});setKeyColumn("SysRowID");setCursorColumn("SysRevID")};
 useEffect(()=>{setSourceID("");setTargetID("");setSchema("Erp");clearInspect()},[token,adminToken]);
 const task=async(name:string,action:()=>Promise<void>)=>{
  setBusy(name);setError("");setNotice("");
  try{await action()}catch(e){setError(String(e))}
  finally{setBusy("")}
 };
 const inspect=()=>task("inspect",async()=>{
  const found=await labSyncSetupApi.columns(sourceID,schema,table,token,adminToken);
  const key=found.columns.find(c=>c.name==="SysRowID")??
   found.columns.find(c=>c.primary_key)??found.columns[0];
  const cursor=found.columns.find(c=>c.name==="SysRevID"&&c.cursor_candidate)??
   found.columns.find(c=>c.cursor_candidate);
  if(!key)throw Error("Source has no visible columns.");
  setKeyColumn(key.name);setCursorColumn(cursor?.name??"");
  if(!cursor)setMode("full_refresh");
  setColumns(found.columns);setSelected(found.columns.map(c=>c.name));
  setTargetNames(Object.fromEntries(found.columns.map(c=>[c.name,c.name])));
  setAlias(defaultAlias(schema,table));
 });
 const addToBatch=()=>{
  setError("");setNotice("");
  if(queue.length>=10||!sourceID||!targetID||!table||!columns.length||
   !keyColumn||!requiredCols.every(c=>selected.includes(c))||
   (mode!=="full_refresh"&&!cursorColumn)||
   (layout==="raw_event"?!tableAlias.test(alias):
    !targetIdentifier.test(alias)||!targetIdentifier.test(targetDatabase)||
    selected.some(s=>!targetIdentifier.test(targetNames[s]??s)))||
   batchSize<100||batchSize>100000||workers<1||workers>32||
   queueSize<1||queueSize>100){setError("Check selections, alias and runtime limits.");return}
  if(queue.some(q=>q.source_schema===schema&&q.source_table===table)){
   setError("This source table is already in the batch.");return
  }
  setQueue(q=>[...q,{source_schema:schema,source_table:table,
   target_database:layout==="raw_event"?"scratch":targetDatabase,target_table:alias,
   destination_layout:layout,
   column_mappings:layout==="typed"?columns.filter(c=>selected.includes(c.name)).map(c=>({
    source:c.name,target:targetNames[c.name]??c.name
   })):undefined,
   selected_columns:columns.filter(c=>selected.includes(c.name)).map(c=>c.name),
   source_key_column:keyColumn,source_cursor_column:cursorColumn,
   mode,batch_size:batchSize,workers,queue_capacity:queueSize}]);
  setTable("");setColumns([]);setSelected([]);setNotice("Added to draft batch (no database writes yet).");
 };
 const save=()=>task("save",async()=>{
  if(!sourceID||!targetID||!queue.length)return;
  const saved=await labQABatchSyncApi.create({
   source_connection_id:sourceID,target_connection_id:targetID,streams:queue},token,adminToken);
  setResults(saved.streams);setQueue([]);
  setNotice(saved.count+" QA streams created + bound atomically. Run them separately.");
  onCreated();
 });
 const runAll=()=>task("run-all",async()=>{
  let accepted=0;const errors:string[]=[];
  // Bounded sequential enqueue; each managed stream has its own durable
  // queue and single-active-stream guard. We never start 10 jobs at once.
  for(const item of results){
   try{
    const r=await managedApi.run(item.stream_id,token);
    setRuns(prev=>({...prev,[item.stream_id]:r}));
    accepted++;
   }catch(e){errors.push(item.source+": "+String(e))}
  }
  setNotice(accepted+" / "+results.length+" streams queued.");
  if(errors.length)throw Error("Some streams were not queued: "+errors.join("; "));
 });
 useEffect(()=>{
  const pending=Object.entries(runs).filter(([,run])=>run.status==="queued"||run.status==="running");
  if(pending.length===0||!token)return;
  let cancelled=false;
  const handle=window.setInterval(()=>{
   for(const [id,run] of pending){
    managedApi.status(run.run_id,token).then(next=>{
     if(!cancelled)setRuns(current=>({...current,[id]:next}));
    }).catch(()=>{/* retain last known status; manual refresh remains available */});
   }
  },4000);
  return()=>{cancelled=true;window.clearInterval(handle)};
 },[runs,token]);
 const start=(id:string)=>task("run-"+id,async()=>{
  const result=await managedApi.run(id,token);
  setRuns(prev=>({...prev,[id]:result}));setNotice("Run queued for "+id);
 });
 const refresh=(id:string)=>task("status-"+id,async()=>{
  const run=runs[id];if(!run)return;
  const result=await managedApi.status(run.run_id,token);
  setRuns(prev=>({...prev,[id]:result}));
 });
 const sources=profiles.filter(p=>p.driver==="sqlserver"&&p.status==="verified");
 const targets=profiles.filter(p=>p.driver==="clickhouse"&&p.status==="verified");
 return <section className="rounded-md border p-4 space-y-4" aria-label="Batch sync setup wizard">
  <div className="flex flex-wrap justify-between gap-2"><div>
   <h4 className="font-semibold flex items-center gap-2"><Database size={16}/>Thiết lập đồng bộ nhiều bảng · QA</h4>
   <p className="muted text-xs mt-1">Select saved connections, configure up to 10 source tables, and choose Full Refresh or Incremental for each.</p>
  </div><span className="rounded border px-2 py-1 text-[11px]">Disposable scratch only</span></div>
  <div className="grid gap-3 md:grid-cols-2">
   <label className="text-xs space-y-1">Verified SQL Server source
    <select className="field" aria-label="Batch source connection" value={sourceID} onChange={e=>{setSourceID(e.target.value);clearInspect()}}>
     <option value="">Select source</option>{sources.map(p=><option key={p.id} value={p.id}>{p.name}</option>)}</select>
   </label>
   <label className="text-xs space-y-1">Verified ClickHouse destination
    <select className="field" aria-label="Batch target connection" value={targetID} onChange={e=>{setTargetID(e.target.value);clearInspect()}}>
     <option value="">Select destination</option>{targets.map(p=><option key={p.id} value={p.id}>{p.name}</option>)}</select>
   </label>
  </div>
  <div className="grid gap-2 md:grid-cols-[1fr_1fr_auto] items-end">
   <label className="text-xs space-y-1">SQL schema
    <input className="field" aria-label="Batch schema" value={schema} onChange={e=>{setSchema(e.target.value);setTables([]);setTable("");setColumns([])}}/>
   </label>
   <label className="text-xs space-y-1">Discovered table
    <select className="field" aria-label="Batch source table" value={table} onChange={e=>{setTable(e.target.value);setColumns([]);setSelected([])}}>
     <option value="">Choose table</option>{tables.map(t=><option key={t.schema+"."+t.name} value={t.name}>{t.schema}.{t.name}</option>)}
    </select>
   </label>
   <div className="flex flex-wrap gap-2">
    <Button type="button" variant="outline" disabled={!!busy||!sourceID||!targetID} onClick={()=>task("discover",async()=>{
     const found=await labSyncSetupApi.tables(sourceID,schema,token,adminToken);setTables(found.tables);setTable("");setColumns([])
    })}><RefreshCw size={14}/>Discover</Button>
    <Button type="button" variant="outline" disabled={!!busy||!table} onClick={inspect}><Search size={14}/>Columns</Button>
   </div>
  </div>
  {columns.length>0&&<>
   <div className="space-y-2">
    <div className="flex items-center justify-between"><span className="text-xs font-semibold">Columns ({selected.length}/{columns.length})</span>
     <Button type="button" size="sm" variant="outline" onClick={()=>setSelected(columns.map(c=>c.name))}>Select all</Button></div>
    <div className="max-h-44 overflow-y-auto border rounded" role="group" aria-label="Batch column selection">
     {columns.map(c=><label key={c.name} className="flex gap-2 items-center text-xs p-2 border-b">
      <input aria-label={"Batch column "+c.name} type="checkbox" checked={selected.includes(c.name)} disabled={requiredCols.includes(c.name)}
       onChange={e=>setSelected(v=>e.target.checked?[...v,c.name]:v.filter(n=>n!==c.name))}/>
      <span className="flex-1 font-mono">{c.name}</span><span className="muted">{c.sql_type}</span>
      {requiredCols.includes(c.name)&&<span className="df-text-accent">required</span>}
     </label>)}
    </div>
   </div>
   <div className="grid gap-2 md:grid-cols-2">
    <label className="text-xs space-y-1">Source UNIQUE key
     <select className="field" aria-label="Batch source key" value={keyColumn} onChange={e=>{
      setKeyColumn(e.target.value);setSelected(v=>v.includes(e.target.value)?v:[...v,e.target.value]);
     }}>{columns.map(c=><option value={c.name} key={c.name}>{c.name} ({c.sql_type})</option>)}</select>
    </label>
    <label className="text-xs space-y-1">Source RowVersion cursor
     <select className="field" aria-label="Batch source cursor" value={cursorColumn} onChange={e=>{
      setCursorColumn(e.target.value);
      if(e.target.value)setSelected(v=>v.includes(e.target.value)?v:[...v,e.target.value]);
     }}>
      <option value="">None · Full only</option>
      {columns.filter(c=>c.cursor_candidate).map(c=><option value={c.name} key={c.name}>{c.name}</option>)}
     </select>
    </label>
    <label className="text-xs space-y-1">Sync Mode
     <select className="field" aria-label="Batch sync mode" value={mode} onChange={e=>setMode(e.target.value as QABatchStream["mode"])}>
      <option value="incremental_rowversion">Incremental · RowVersion</option>
      <option value="full_refresh">Full Refresh · Atomic snapshot replace</option>
      <option value="full_then_incremental" disabled={!cursorColumn}>Full once → Incremental · same snapshot boundary</option>
     </select>
    </label>
 
   </div>
   <div className="grid gap-2 md:grid-cols-3">
    <label className="text-xs space-y-1">Destination layout
     <select className="field" aria-label="Batch destination layout" value={layout}
       onChange={e=>{setLayout(e.target.value as "raw_event"|"typed");setAlias(defaultAlias(schema,table));}}>
      <option value="raw_event">QA raw events · generated table</option>
      <option value="typed">Typed columns · explicit owned table</option>
     </select>
    </label>
    {layout==="typed"&&<label className="text-xs space-y-1">Target database (admin allowlisted)
     <input className="field" aria-label="Batch target database" value={targetDatabase}
       maxLength={64} onChange={e=>setTargetDatabase(e.target.value)}/>
    </label>}
    <label className="text-xs space-y-1">{layout==="typed"?"Destination table":"Destination table alias"}
     <input className="field" aria-label="Batch target alias" value={alias}
       maxLength={layout==="typed"?64:30} onChange={e=>setAlias(e.target.value)}/>
    </label>
   </div>
   {layout==="typed"&&<div className="space-y-2 border rounded p-3">
    <p className="font-semibold text-xs">Source → ClickHouse typed column mapping</p>
    <p className="text-xs muted">Types are inferred and verified by the server. Existing unowned tables cannot be overwritten. For incremental current rows query with FINAL.</p>
    {columns.filter(c=>selected.includes(c.name)).map(col=><label key={col.name} className="grid grid-cols-2 gap-2 items-center text-xs">
     <span className="font-mono">{col.name} · {col.sql_type}</span>
     <input className="field" aria-label={"Target column "+col.name}
       value={targetNames[col.name]??col.name} maxLength={64}
       onChange={e=>setTargetNames(prev=>({...prev,[col.name]:e.target.value}))}/>
    </label>)}
   </div>}
   <p className="muted text-xs">Custom key must be non-null, single-column UNIQUE; incremental needs a native rowversion. The server verifies both against SQL catalog.</p>
   <p className="muted text-xs">Raw-event destinations get private scratch.dfqa_ names; typed destinations use the requested allowlisted database/table and own them with immutable mapping identity. Full Refresh uses staging + EXCHANGE; Incremental uses versioned ReplacingMergeTree (query FINAL for current rows). Source is SELECT-only.</p>
   <div className="grid gap-2 sm:grid-cols-3">
    {([["Batch size",batchSize,setBatchSize,100,100000],["Workers",workers,setWorkers,1,32],["Queue",queueSize,setQueueSize,1,100]] as const).map(([name,value,setter,min,max])=>
     <label key={name} className="text-xs space-y-1">{name}<input className="field" type="number" min={min} max={max} value={value} onChange={e=>setter(Number(e.target.value))}/></label>)}
   </div>
   <Button type="button" variant="outline" disabled={!!busy||queue.length>=10} onClick={addToBatch}>Add configured table to batch</Button>
  </>}
  {queue.length>0&&<div className="space-y-2">
   <p className="text-xs font-semibold">Draft streams ({queue.length}/10) — no writes yet</p>
   {queue.map((s,i)=><div key={s.source_schema+"."+s.source_table} className="flex items-center justify-between gap-2 border rounded p-2 text-xs">
    <span>{s.source_schema}.{s.source_table} → {s.target_database}.{s.target_table} ({s.destination_layout??"raw_event"}) · {s.mode} · {s.selected_columns.length} columns</span>
    <Button type="button" size="sm" variant="outline" onClick={()=>setQueue(list=>list.filter((_,j)=>j!==i))}><Trash2 size={13}/>Remove</Button>
   </div>)}
   <Button type="button" disabled={!!busy||!sourceID||!targetID} onClick={save}><Save size={15}/>Save & Bind {queue.length} streams</Button>
  </div>}
  {results.length>0&&<div className="space-y-2">
   <div className="flex flex-wrap items-center justify-between gap-2">
    <p className="font-semibold text-sm">Saved &amp; bound streams</p>
    <Button type="button" disabled={!!busy||results.some(x=>["queued","running"].includes(runs[x.stream_id]?.status??""))} onClick={runAll}>
     <Play size={14}/>Run All ({results.length}) — sequential enqueue
    </Button>
   </div>
   {results.map(result=><div className="border rounded p-3 space-y-2 text-xs" key={result.stream_id}>
    <p>{result.source} → <code>{result.destination}</code></p>
    <p>{result.mode} · stream <code className="break-all">{result.stream_id}</code></p>
    <div className="flex flex-wrap gap-2">
     <Button type="button" size="sm" disabled={!!busy} onClick={()=>start(result.stream_id)}><Play size={14}/>Run</Button>
     <Button type="button" variant="outline" size="sm" disabled={!!busy||!runs[result.stream_id]} onClick={()=>refresh(result.stream_id)}>Refresh status</Button>
     <span className="self-center">{runs[result.stream_id]?.status??"Not run"} · {runs[result.stream_id]?.rows_written??0} rows</span>
    </div>
    {runs[result.stream_id]?.error&&<p role="alert" className="df-text-error">{runs[result.stream_id]?.error}</p>}
   </div>)}
  </div>}
  {notice&&<p role="status" className="df-text-success text-xs">{notice}</p>}
  {error&&<p role="alert" className="df-text-error text-xs">{error}</p>}
  <p className="muted text-xs">Incremental requires a unique source key and a native rowversion cursor. Full → Incremental is an opt-in one-time bootstrap with a guarded checkpoint. Ordinary Full Refresh alone does not establish an Incremental cursor. Not production-certified.</p>
 </section>
}
