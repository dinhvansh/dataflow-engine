import {useMemo,useState} from "react";
import {ArrowDown,ArrowRight,Box,CheckCircle2,Clock3,Database,FileSpreadsheet,HardDrive,Info,Layers3,Network,Search,Server,ShieldCheck,Workflow} from "lucide-react";
import {SourceExplorer} from "./SourceExplorer";
import {Button} from "./ui/button";

// This is a capability catalog, NOT saved connection credentials or a
// multi-endpoint connector API. Only the existing SQL Server Explorer is live.
type Capability="available"|"planned";
type Connector={id:string;name:string;kind:"source"|"destination";type:string;description:string;capability:Capability;mode:string};
const catalog:Connector[]=[
 {id:"sqlserver",name:"Microsoft SQL Server",kind:"source",type:"Database",description:"Read-only schema discovery, full snapshot and rowversion. Uses the single backend DSN configured by the operator.",capability:"available",mode:"Snapshot · Rowversion"},
 {id:"postgres-source",name:"PostgreSQL",kind:"source",type:"Database",description:"Future source connector; separate from the current control-plane PostgreSQL.",capability:"planned",mode:"Full · Incremental (planned)"},
 {id:"mysql",name:"MySQL / MariaDB",kind:"source",type:"Database",description:"Reserved adapter slot for relational sources.",capability:"planned",mode:"Full · CDC (planned)"},
 {id:"oracle",name:"Oracle Database",kind:"source",type:"Database",description:"Future enterprise database source.",capability:"planned",mode:"Full · Incremental (planned)"},
 {id:"files",name:"CSV / Excel Files",kind:"source",type:"File",description:"Future upload or filesystem ingestion; not wired to an API.",capability:"planned",mode:"Batch files (planned)"},
 {id:"rest",name:"REST / HTTP API",kind:"source",type:"API",description:"Future paginated REST polling with credentials managed server-side.",capability:"planned",mode:"API polling (planned)"},
 {id:"s3",name:"S3 / Object Storage",kind:"source",type:"Object storage",description:"Future object-store ingestion with partition discovery.",capability:"planned",mode:"Batch / Object (planned)"},
 {id:"clickhouse",name:"ClickHouse",kind:"destination",type:"Warehouse",description:"Current scratch destination for the tested SQL Server loading workflow.",capability:"available",mode:"Scratch tables"},
 {id:"postgres-dest",name:"PostgreSQL",kind:"destination",type:"Database",description:"Used internally for metadata today. External destination writes are not implemented.",capability:"planned",mode:"Data sink (planned)"},
 {id:"s3-dest",name:"S3 / Object Storage",kind:"destination",type:"Object storage",description:"Reserved slot for future parquet/CSV batch export.",capability:"planned",mode:"Object export (planned)"},
 {id:"mysql-dest",name:"MySQL",kind:"destination",type:"Database",description:"Future database destination adapter.",capability:"planned",mode:"Data sink (planned)"},
];
const initialRoutes=[{source:"sqlserver",target:"clickhouse",name:"SQL Server → ClickHouse",detail:"Existing single-source engine path; configure execution under Sync Jobs.",supported:true}];
function connectorIcon(item:Connector){
 if(item.type==="File")return FileSpreadsheet;
 if(item.type==="API")return Network;
 if(item.type==="Object storage")return HardDrive;
 return Database;
}
function StateTag({supported}:{supported:boolean}){return <span className={supported?"rounded-md border border-emerald-600/40 bg-emerald-500/10 px-2 py-1 text-xs text-emerald-300":"rounded-md border border-slate-600 bg-slate-800 px-2 py-1 text-xs text-slate-300"}>{supported?"Available · limited":"Planned · no driver"}</span>}
export function ConnectorHub({token,onTokenChange}:{token:string;onTokenChange:(value:string)=>void}){
 const[tab,setTab]=useState<"sources"|"destinations"|"routes">("sources");
 const[query,setQuery]=useState("");
 const[expanded,setExpanded]=useState("sqlserver");
 const[showExplorer,setShowExplorer]=useState(true);
 const[selectedSource,setSelectedSource]=useState("sqlserver");
 const[selectedTarget,setSelectedTarget]=useState("clickhouse");
 const filtered=useMemo(()=>catalog.filter(c=>(tab==="destinations"?c.kind==="destination":c.kind==="source")&&(c.name+" "+c.type+" "+c.description).toLowerCase().includes(query.toLowerCase())),[tab,query]);
 const src=catalog.find(x=>x.id===selectedSource)!;
 const dst=catalog.find(x=>x.id===selectedTarget)!;
 const executable=src.id==="sqlserver"&&dst.id==="clickhouse";
 return <div className="space-y-5">
  <div className="section space-y-4">
   <div className="flex flex-wrap items-start justify-between gap-3">
    <div><div className="flex items-center gap-2 text-lg font-semibold"><Layers3 size={20}/>Connector Hub</div><p className="muted mt-1 text-sm">Design for many sources → many destinations. Existing SQL Server → ClickHouse operations are preserved; future connectors are placeholders only.</p></div>
    <span className="inline-flex items-center gap-1.5 rounded-md border border-amber-700/40 bg-amber-500/10 px-2.5 py-1.5 text-xs text-amber-200"><Info size={14}/>Catalog preview · not a credential registry</span>
   </div>
   <div className="grid gap-3 md:grid-cols-3">
    <div className="rounded-lg border border-slate-700 bg-slate-800/50 p-3"><div className="muted flex items-center gap-2 text-xs"><Server size={15}/>Source adapters</div><div className="mt-1 text-2xl font-semibold">{catalog.filter(x=>x.kind==="source").length}</div><div className="muted text-xs">1 currently supported</div></div>
    <div className="rounded-lg border border-slate-700 bg-slate-800/50 p-3"><div className="muted flex items-center gap-2 text-xs"><Database size={15}/>Destination adapters</div><div className="mt-1 text-2xl font-semibold">{catalog.filter(x=>x.kind==="destination").length}</div><div className="muted text-xs">1 scratch destination</div></div>
    <div className="rounded-lg border border-slate-700 bg-slate-800/50 p-3"><div className="muted flex items-center gap-2 text-xs"><Workflow size={15}/>Executable connector paths</div><div className="mt-1 text-2xl font-semibold">1</div><div className="muted text-xs">Existing SQL Server → ClickHouse</div></div>
   </div>
   <div className="flex flex-wrap gap-2" role="tablist" aria-label="Connector catalog sections">
    {([{id:"sources",label:"Sources"},{id:"destinations",label:"Destinations"},{id:"routes",label:"Data routes"}] as const).map(x=><button key={x.id} type="button" role="tab" aria-selected={tab===x.id} onClick={()=>{setTab(x.id);setQuery("");setExpanded(x.id==="sources"?"sqlserver":"clickhouse")}} className={tab===x.id?"rounded-md bg-sky-500/15 px-4 py-2 text-sm font-semibold text-sky-300 ring-1 ring-sky-600/40":"rounded-md border border-slate-700 px-4 py-2 text-sm text-slate-300 hover:bg-slate-800"}>{x.label}</button>)}
   </div>
  </div>
  {tab!=="routes"&&<>
   <div className="section space-y-3">
    <div className="flex items-center justify-between gap-3"><h2 className="font-semibold">{tab==="sources"?"Source connector catalog":"Destination connector catalog"}</h2><span className="muted text-xs">Drivers are added independently</span></div>
    <label className="relative block"><Search size={16} className="absolute left-3 top-3 text-slate-400"/><input aria-label="Search connectors" className="field pl-10" value={query} onChange={e=>setQuery(e.target.value)} placeholder="Search database, files, APIs…"/></label>
   </div>
   <div className="grid gap-3 lg:grid-cols-2">
    {filtered.map(item=>{const Icon=connectorIcon(item);return <div key={item.id} className="section space-y-3">
     <div className="flex items-start gap-3"><div className="rounded-lg bg-slate-800 p-2.5 text-sky-300"><Icon size={20}/></div><div className="min-w-0 flex-1"><div className="font-semibold">{item.name}</div><p className="muted text-xs">{item.type} · {item.mode}</p></div><StateTag supported={item.capability==="available"}/></div>
     <p className="muted min-h-[44px] text-sm">{item.description}</p>
     <div className="flex flex-wrap items-center justify-between gap-3 border-t border-slate-800 pt-3">
      <span className="muted text-xs">{item.capability==="planned"?"No connector config, secrets or runtime is created.":"Uses existing single backend connection."}</span>
      {item.id==="sqlserver"?<Button size="sm" variant="outline" onClick={()=>{setShowExplorer(x=>!x);setExpanded(item.id)}}>{showExplorer?"Hide explorer":"Open live explorer"} <ArrowRight size={14}/></Button>:item.id==="clickhouse"?<span className="text-xs text-emerald-300"><CheckCircle2 className="mr-1 inline" size={14}/>Scratch writer available</span>:<span className="muted inline-flex items-center gap-1 text-xs"><Clock3 size={14}/>Coming later</span>}
     </div>
    </div>})}
   </div>
   {filtered.length===0&&<div className="section muted text-sm">No matching connector.</div>}
   {tab==="sources"&&showExplorer&&expanded==="sqlserver"&&<div className="rounded-lg border border-sky-600/40 p-3"><div className="mb-3 flex items-center gap-2 text-sm font-semibold text-sky-200"><ShieldCheck size={16}/>Existing SQL Server Explorer · backend-configured DSN</div><SourceExplorer token={token} onTokenChange={onTokenChange}/></div>}
  </>}
  {tab==="routes"&&<div className="space-y-4">
   <div className="section space-y-4">
    <div className="flex items-center gap-2 text-lg font-semibold"><Network size={18}/>Source → Destination routing</div>
    <p className="muted text-sm">Preview how future connector instances will be paired. Selecting a pair does not save a connection or create a job; operational syncs remain under Sync Jobs.</p>
    <div className="grid gap-3 sm:grid-cols-[1fr_auto_1fr] sm:items-end">
     <label className="space-y-1.5"><span className="muted text-xs">From source</span><select aria-label="Route source" className="field" value={selectedSource} onChange={e=>setSelectedSource(e.target.value)}>{catalog.filter(x=>x.kind==="source").map(x=><option value={x.id} key={x.id}>{x.name}{x.capability==="planned"?" (planned)":""}</option>)}</select></label>
     <div className="flex justify-center pb-2"><ArrowRight size={20} className="hidden text-sky-400 sm:block"/><ArrowDown size={20} className="text-sky-400 sm:hidden"/></div>
     <label className="space-y-1.5"><span className="muted text-xs">To destination</span><select aria-label="Route destination" className="field" value={selectedTarget} onChange={e=>setSelectedTarget(e.target.value)}>{catalog.filter(x=>x.kind==="destination").map(x=><option value={x.id} key={x.id}>{x.name}{x.capability==="planned"?" (planned)":""}</option>)}</select></label>
    </div>
    <div className={executable?"rounded-lg border border-emerald-700/40 bg-emerald-950/20 p-3 text-sm":"rounded-lg border border-amber-700/40 bg-amber-950/20 p-3 text-sm"}>
     <div className="font-semibold">{executable?"Supported underlying path":"Planned route — execution unavailable"}</div>
     <p className="muted mt-1 text-xs">{executable?"The backend currently uses one configured SQL Server DSN and writes to scratch ClickHouse. Multiple independent connector instances are not supported yet.":"This pair is a UI preview only. No API call, credentials, new stream or connection is created."}</p>
    </div>
   </div>
   <div className="section space-y-3"><h3 className="font-semibold">Existing route</h3>{initialRoutes.map(r=><div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-slate-700 bg-slate-800/40 p-3" key={r.name}><div><div className="flex items-center gap-2 text-sm font-semibold"><Database size={16}/>{r.name}</div><p className="muted mt-1 text-xs">{r.detail}</p></div><StateTag supported={r.supported}/></div>)}</div>
   <div className="section"><div className="flex items-start gap-3"><Box className="mt-0.5 shrink-0 text-sky-300" size={19}/><div><p className="font-semibold">Future connector architecture</p><p className="muted mt-1 text-sm">Connector definitions → secure connection instances → table/stream discovery → source-to-destination bindings → managed sync jobs → observability. These layers need dedicated backend APIs and permissions before they can become functional.</p></div></div></div>
  </div>}
 </div>;
}
