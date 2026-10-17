import {useMemo,useState} from "react";
import {ArrowDown,ArrowRight,Box,CheckCircle2,Clock3,Database,FileSpreadsheet,HardDrive,Info,Layers3,LockKeyhole,Network,Search,Server,ShieldCheck,Workflow,X} from "lucide-react";
import {SourceExplorer} from "./SourceExplorer";
import {Badge} from "./ui/badge";
import {Button} from "./ui/button";
import {Card,CardContent,CardDescription,CardHeader,CardTitle} from "./ui/card";

// Presentation-only catalog. It is NOT a connection registry: no credentials,
// driver selection or new transport endpoints are persisted or invoked here.
type Capability="available"|"planned";
type ConnectorKind="source"|"destination";
type Connector={id:string;name:string;kind:ConnectorKind;type:string;description:string;capability:Capability;mode:string};
const catalog:Connector[]=[
 {id:"sqlserver",name:"Microsoft SQL Server",kind:"source",type:"Database",description:"Read-only schema discovery and current single-DSN ingestion path. Full snapshot and rowversion are supported under the existing test guardrails.",capability:"available",mode:"Snapshot · Rowversion"},
 {id:"postgres-source",name:"PostgreSQL",kind:"source",type:"Database",description:"Reserved source adapter. The current PostgreSQL instance is metadata storage, not a connected source.",capability:"planned",mode:"Full · Incremental"},
 {id:"mysql",name:"MySQL / MariaDB",kind:"source",type:"Database",description:"Future relational database adapter; no driver or credentials API available.",capability:"planned",mode:"Full · CDC"},
 {id:"oracle",name:"Oracle Database",kind:"source",type:"Database",description:"Reserved connector for enterprise database sources. No driver is installed.",capability:"planned",mode:"Full · Incremental"},
 {id:"files",name:"CSV / Excel Files",kind:"source",type:"Files",description:"Planned local file and import discovery. Upload and filesystem access are not implemented.",capability:"planned",mode:"Batch files"},
 {id:"rest",name:"REST / HTTP API",kind:"source",type:"API",description:"Future paginated API polling, with credentials to be managed on the server.",capability:"planned",mode:"API polling"},
 {id:"s3",name:"S3 / Object Storage",kind:"source",type:"Object storage",description:"Reserved object storage source for future partition discovery.",capability:"planned",mode:"Batch / Object"},
 {id:"clickhouse",name:"ClickHouse",kind:"destination",type:"Warehouse",description:"Existing scratch-only ClickHouse destination. This is not a selectable independent connection instance.",capability:"available",mode:"Scratch tables"},
 {id:"postgres-dest",name:"PostgreSQL",kind:"destination",type:"Database",description:"Currently the control-plane metadata database, not a general-purpose destination.",capability:"planned",mode:"Data sink"},
 {id:"s3-dest",name:"S3 / Object Storage",kind:"destination",type:"Object storage",description:"Planned object output, such as parquet/CSV export.",capability:"planned",mode:"Object export"},
 {id:"mysql-dest",name:"MySQL",kind:"destination",type:"Database",description:"Reserved external database sink; data writes are not implemented.",capability:"planned",mode:"Data sink"},
];
const supportedRoute={source:"sqlserver",target:"clickhouse"} as const;
const definitions=[
 {id:"definitions",label:"Connector definitions",detail:"Catalog and supported capabilities"},
 {id:"instances",label:"Connection instances",detail:"Independent credentials / endpoints · planned"},
 {id:"bindings",label:"Route bindings",detail:"Source → destination mapping · planned"},
 {id:"jobs",label:"Managed operations",detail:"Existing test-only syncs are configured elsewhere"},
] as const;
function typeIcon(type:string){
 if(type==="Files")return FileSpreadsheet;
 if(type==="API")return Network;
 if(type==="Object storage")return HardDrive;
 if(type==="Warehouse")return Layers3;
 return Database;
}
function StatusBadge({available}:{available:boolean}){
 return <Badge variant={available?"success":"neutral"}>{available?<CheckCircle2 size={12}/>:<Clock3 size={12}/>} {available?"Available · limited":"Planned · no driver"}</Badge>;
}
function Metric({label,value,detail,icon:Icon}:{label:string;value:number;detail:string;icon:typeof Database}){
 return <Card className="hub-metric"><CardHeader className="pb-2"><CardDescription className="flex items-center gap-2"><Icon size={15}/>{label}</CardDescription><CardTitle className="text-2xl tabular-nums">{value.toString().padStart(2,"0")}</CardTitle></CardHeader><CardContent className="muted text-xs">{detail}</CardContent></Card>;
}
export function ConnectorHub({token,onTokenChange}:{token:string;onTokenChange:(value:string)=>void}){
 const[tab,setTab]=useState<"sources"|"destinations"|"routes">("sources");
 const[query,setQuery]=useState("");
 const[filter,setFilter]=useState<"all"|Capability>("all");
 const[showExplorer,setShowExplorer]=useState(true);
 const[selectedSource,setSelectedSource]=useState("sqlserver");
 const[selectedTarget,setSelectedTarget]=useState("clickhouse");
 const sources=catalog.filter(c=>c.kind==="source");
 const targets=catalog.filter(c=>c.kind==="destination");
 const filtered=useMemo(()=>catalog.filter(c=>(tab==="destinations"?c.kind==="destination":c.kind==="source")&&(filter==="all"||c.capability===filter)&&(c.name+" "+c.type+" "+c.description+" "+c.mode).toLowerCase().includes(query.trim().toLowerCase())),[tab,query,filter]);
 const source=catalog.find(c=>c.id===selectedSource)!;
 const target=catalog.find(c=>c.id===selectedTarget)!;
 const supported=selectedSource===supportedRoute.source&&selectedTarget===supportedRoute.target;
 const pickTab=(next:typeof tab)=>{setTab(next);setQuery("");setFilter("all")};
 return <div className="space-y-5">
  <section className="hub-hero space-y-5 rounded-lg border p-5 sm:p-6" aria-label="Connector Hub overview">
   <div className="flex flex-wrap items-start justify-between gap-4">
    <div className="max-w-3xl">
     <p className="hub-eyebrow mb-2">CONNECTIVITY / CATALOG 01</p>
     <h2 className="flex items-center gap-3 text-2xl font-bold tracking-tight"><span className="hub-hero-symbol"><Network size={21}/></span><span>Connector Hub</span></h2>
     <p className="muted mt-2 max-w-2xl text-sm leading-relaxed">Design for many sources and destinations. Today, SQL Server → ClickHouse is the only existing engine path; other adapters are future slots, not live connections.</p>
    </div>
    <Badge variant="neutral"><Info size={13}/> UI catalog · no credentials saved</Badge>
   </div>
   <div className="grid gap-3 sm:grid-cols-3">
    <Metric label="Source adapters" value={sources.length} detail="1 limited source currently available" icon={Server}/>
    <Metric label="Destinations" value={targets.length} detail="1 scratch-only writer available" icon={Database}/>
    <Metric label="Underlying paths" value={1} detail="SQL Server → ClickHouse · test only" icon={Workflow}/>
   </div>
  </section>
  <section className="hub-workspace space-y-4">
   <div className="hub-tabs flex flex-wrap gap-1 rounded-lg border p-1" role="tablist" aria-label="Connector catalog sections">
    {([{id:"sources",label:"Sources",count:sources.length},{id:"destinations",label:"Destinations",count:targets.length},{id:"routes",label:"Data routes",count:1}] as const).map(section=><button type="button" key={section.id} role="tab" aria-selected={tab===section.id} className={`hub-tab inline-flex items-center gap-2 rounded px-4 py-2.5 text-sm font-semibold ${tab===section.id?"is-active":""}`} onClick={()=>pickTab(section.id)}>{section.label}<span className="hub-tab-count text-[10px] tabular-nums">{section.count.toString().padStart(2,"0")}</span></button>)}
   </div>
   {tab!=="routes"&&<>
    <div className="hub-filter-panel flex flex-col gap-3 rounded-lg border p-4 lg:flex-row lg:items-end lg:justify-between">
     <div className="space-y-2 lg:min-w-[340px] lg:flex-1">
      <div className="flex flex-wrap items-center justify-between gap-2"><h3 className="font-semibold">{tab==="sources"?"Source connector catalog":"Destination connector catalog"}</h3><span className="muted hub-mono text-[11px]">{filtered.length} MATCHING</span></div>
      <label className="relative block"><Search size={16} className="hub-search-icon absolute left-3 top-3"/><input aria-label="Search connectors" className="field pl-10 pr-9" value={query} onChange={e=>setQuery(e.target.value)} placeholder="Search adapters, modes, storage…"/>{query&&<button type="button" className="hub-clear-search absolute right-2 top-2 rounded p-1" aria-label="Clear connector search" onClick={()=>setQuery("")}><X size={14}/></button>}</label>
     </div>
     <div className="hub-filter-controls flex flex-wrap gap-2" role="group" aria-label="Connector availability filter">
      {([{id:"all",label:"All"},{id:"available",label:"Available"},{id:"planned",label:"Planned"}] as const).map(item=><button type="button" key={item.id} aria-pressed={filter===item.id} className={`hub-filter rounded border px-3 py-2 text-xs font-semibold ${filter===item.id?"is-active":""}`} onClick={()=>setFilter(item.id)}>{item.label}</button>)}
     </div>
    </div>
    {filtered.length>0?<div className="grid gap-3 lg:grid-cols-2">{filtered.map(item=>{const Icon=typeIcon(item.type);const available=item.capability==="available";return <article key={item.id} className={`hub-connector-card space-y-4 rounded-lg border p-4 sm:p-5 ${available?"is-ready":""}`}>
      <div className="flex flex-wrap items-start gap-3"><div className="hub-icon-tile flex h-11 w-11 shrink-0 items-center justify-center rounded-md"><Icon size={21}/></div><div className="min-w-[150px] flex-1 space-y-1"><h4 className="text-sm font-bold">{item.name}</h4><p className="muted text-xs">{item.type} · {item.mode}</p></div><StatusBadge available={available}/></div>
      <p className="muted min-h-[42px] text-sm leading-relaxed">{item.description}</p>
      <div className="hub-card-footer flex flex-wrap items-center justify-between gap-3 border-t pt-3"><span className="muted text-xs">{available?"Uses existing backend configuration":"No connection configuration exists"}</span>
       {item.id==="sqlserver"?<Button type="button" size="sm" variant="outline" aria-expanded={showExplorer} onClick={()=>setShowExplorer(p=>!p)}>{showExplorer?"Hide explorer":"Open live explorer"}<ArrowRight size={14}/></Button>:item.id==="clickhouse"?<span className="hub-available-caption flex items-center gap-1 text-xs font-medium"><ShieldCheck size={15}/>Scratch writer available</span>:<span className="muted flex items-center gap-1 text-xs"><LockKeyhole size={14}/>Not configurable yet</span>}
      </div>
     </article>})}</div>:<div className="hub-empty rounded-lg border p-8 text-center"><Search className="muted mx-auto mb-3" size={25}/><p className="font-semibold">No matching connectors</p><p className="muted mt-1 text-sm">Try another keyword or availability filter.</p><Button type="button" variant="outline" size="sm" className="mt-4" onClick={()=>{setQuery("");setFilter("all")}}>Reset filters</Button></div>}
    {tab==="sources"&&showExplorer&&<div className="hub-explorer-wrap rounded-lg border p-3 sm:p-4" aria-label="Existing SQL Server Explorer">
      <div className="mb-3 flex flex-wrap items-center gap-2 text-sm font-semibold"><ShieldCheck size={17} className="hub-accent"/>Existing SQL Server Explorer <Badge variant="neutral">BACKEND-CONFIGURED DSN</Badge></div>
      <SourceExplorer token={token} onTokenChange={onTokenChange}/>
     </div>}
   </>}
   {tab==="routes"&&<div className="space-y-4">
    <div className="hub-route-panel rounded-lg border p-4 sm:p-6">
     <div className="mb-5 flex flex-wrap items-start justify-between gap-2"><div><p className="hub-eyebrow mb-2">ROUTING / CONCEPT ONLY</p><h3 className="text-lg font-bold">Source → Destination</h3><p className="muted mt-1 text-sm">Select an adapter pair to preview its support status. No route or job is created.</p></div><Badge variant="neutral"><LockKeyhole size={12}/> Read-only preview</Badge></div>
     <div className="hub-route-grid grid gap-3 sm:grid-cols-[minmax(0,1fr)_72px_minmax(0,1fr)] sm:items-center">
      <label className="hub-route-endpoint space-y-2 rounded-lg border p-4"><span className="hub-eyebrow">SOURCE / FROM</span><div className="flex items-center gap-2"><Database size={18} className="hub-accent"/><span className="text-sm font-semibold">Data origin</span></div><select aria-label="Route source" className="field" value={selectedSource} onChange={e=>setSelectedSource(e.target.value)}>{sources.map(x=><option key={x.id} value={x.id}>{x.name}{x.capability==="planned"?" (planned)":""}</option>)}</select><StatusBadge available={source.capability==="available"}/></label>
      <div className="hub-route-arrow flex items-center justify-center"><ArrowRight size={27} className="hidden sm:block"/><ArrowDown size={27} className="sm:hidden"/></div>
      <label className="hub-route-endpoint space-y-2 rounded-lg border p-4"><span className="hub-eyebrow">DESTINATION / TO</span><div className="flex items-center gap-2"><Layers3 size={18} className="hub-accent"/><span className="text-sm font-semibold">Data warehouse</span></div><select aria-label="Route destination" className="field" value={selectedTarget} onChange={e=>setSelectedTarget(e.target.value)}>{targets.map(x=><option key={x.id} value={x.id}>{x.name}{x.capability==="planned"?" (planned)":""}</option>)}</select><StatusBadge available={target.capability==="available"}/></label>
     </div>
     <div className={`hub-route-message mt-5 rounded-md border p-4 ${supported?"supported":"planned"}`} role="status"><div className="flex items-center gap-2 font-semibold">{supported?<CheckCircle2 size={17}/>:<Clock3 size={17}/>} {supported?"Supported underlying path · test only":"Planned route · execution unavailable"}</div><p className="muted mt-1 text-xs leading-relaxed">{supported?"This is the existing single SQL Server backend DSN to scratch ClickHouse. Multiple source/destination instances are not supported. Configure actual allowed syncs in Sync Jobs.":"This pairing is a visual preview only. No secrets, connectors, streams or destination jobs are saved."}</p></div>
    </div>
    <div className="hub-workflow-rail rounded-lg border p-5"><div className="mb-4 flex items-center gap-2 font-semibold"><Box size={18} className="hub-accent"/>Connector architecture roadmap</div><div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">{definitions.map((x,i)=><div key={x.id} className="hub-roadmap-step rounded-md border p-3"><div className="hub-eyebrow">STAGE {String(i+1).padStart(2,"0")}</div><h4 className="mt-2 text-sm font-semibold">{x.label}</h4><p className="muted mt-1 text-xs">{x.detail}</p></div>)}</div><p className="muted mt-4 text-xs">All future stages require independent backend APIs, permissions and QA. The current UI does not provision them.</p></div>
   </div>}
  </section>
 </div>;
}
