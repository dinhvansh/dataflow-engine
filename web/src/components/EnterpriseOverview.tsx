import {Activity,ArrowRight,CheckCircle2,Clock3,Database,GitBranch,Layers3,Play,Server,ShieldAlert,Workflow} from "lucide-react";
import {Button} from "./ui/button";
import {Badge} from "./ui/badge";
import {Card,CardContent,CardDescription,CardHeader,CardTitle} from "./ui/card";
import type {RunResult} from "../lib/api";

export type DashboardDestination="connections"|"jobs"|"pipelines"|"transform"|"lineage"|"runs";
type Props={health:string;run:RunResult|null;onNavigate:(view:DashboardDestination)=>void};

const entryPoints=[
 {id:"connections" as const,title:"Connection Hub",caption:"Browse available source and destination adapters",icon:Database,detail:"7 source types · 4 destination types in preview"},
 {id:"jobs" as const,title:"Sync Jobs",caption:"Configure and inspect a scratch-only ingestion run",icon:Play,detail:"Full snapshot · Rowversion"},
 {id:"pipelines" as const,title:"Pipelines",caption:"Design and validate job dependencies",icon:GitBranch,detail:"SQL DAG · Dependency workflow"},
 {id:"transform" as const,title:"Transform Studio",caption:"Develop SQL and visual transforms",icon:Layers3,detail:"dbt · Preview · Validation"},
];
const engineStages=[
 {name:"Source",caption:"SQL Server",icon:Database},
 {name:"Extract",caption:"Go reader",icon:Server},
 {name:"Process",caption:"Bounded queue",icon:Workflow},
 {name:"Destination",caption:"ClickHouse",icon:Layers3},
];
export function EnterpriseOverview({health,run,onNavigate}:Props){
 const status=health==="online"?"Connected":health==="offline"?"Unavailable":"Checking";
 const statusBadge=health==="online"?"success":health==="offline"?"warning":"neutral";
 return <div className="space-y-6">
  <section className="enterprise-hero rounded-2xl p-5 sm:p-7">
   <div className="flex flex-col justify-between gap-5 lg:flex-row lg:items-center">
    <div className="max-w-2xl space-y-3">
     <div className="flex flex-wrap items-center gap-2"><Badge variant="default">DATA INTEGRATION</Badge><Badge variant="neutral">Experimental workspace</Badge></div>
     <h2 className="text-2xl font-bold tracking-tight sm:text-3xl">One workspace for your data pipelines.</h2>
     <p className="muted text-sm leading-relaxed">Discover sources, ingest into ClickHouse, build data models and inspect execution. Only configured/test-enabled operations are executable; planned connectors never create real jobs.</p>
     <div className="flex flex-wrap gap-2 pt-1"><Button type="button" onClick={()=>onNavigate("connections")}><Database size={16}/>Explore connections <ArrowRight size={15}/></Button><Button type="button" variant="outline" onClick={()=>onNavigate("jobs")}>Configure trial sync</Button></div>
    </div>
    <div className="enterprise-hero-status rounded-xl border p-4 lg:w-[270px]">
     <div className="muted mb-3 text-xs font-semibold uppercase tracking-widest">Service endpoint</div>
     <div className="flex items-center justify-between gap-2"><span className="flex items-center gap-2 text-sm font-semibold"><span className={health==="online"?"h-2 w-2 rounded-full bg-emerald-400":health==="offline"?"h-2 w-2 rounded-full bg-red-400":"h-2 w-2 rounded-full bg-amber-400"}/>DataFlow API</span><Badge variant={statusBadge}>{status}</Badge></div>
     <div className="muted mt-3 text-xs">Live result from GET /healthz. It does not verify the underlying databases.</div>
    </div>
   </div>
  </section>

  <div className="grid gap-4 md:grid-cols-3">
   <Card><CardHeader className="pb-2"><CardDescription><Server className="mr-2 inline" size={15}/>Engine health</CardDescription><CardTitle className="text-2xl">{status}</CardTitle></CardHeader><CardContent className="muted text-xs">Current API health check</CardContent></Card>
   <Card><CardHeader className="pb-2"><CardDescription><Activity className="mr-2 inline" size={15}/>Last observed run</CardDescription><CardTitle className="text-2xl">{run?.status??"Not inspected"}</CardTitle></CardHeader><CardContent className="muted text-xs">Current browser session only</CardContent></Card>
   <Card><CardHeader className="pb-2"><CardDescription><Workflow className="mr-2 inline" size={15}/>Rows written</CardDescription><CardTitle className="text-2xl tabular-nums">{run?run.rows_written.toLocaleString():"—"}</CardTitle></CardHeader><CardContent className="muted text-xs">Last inspected run · no global aggregation</CardContent></Card>
  </div>

  <section aria-labelledby="quick-access-title" className="space-y-3">
   <div className="flex items-end justify-between"><div><h2 id="quick-access-title" className="text-lg font-semibold">Quick access</h2><p className="muted mt-1 text-sm">Jump into the tools you use most.</p></div><span className="muted hidden text-xs sm:block">WORKSPACE MODULES</span></div>
   <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
    {entryPoints.map(item=>{const Icon=item.icon;return <button type="button" key={item.id} onClick={()=>onNavigate(item.id)} className="enterprise-link-card group rounded-xl border p-4 text-left">
     <div className="flex items-start justify-between"><div className="enterprise-icon flex h-10 w-10 items-center justify-center rounded-lg"><Icon size={19}/></div><ArrowRight className="muted transition-transform group-hover:translate-x-1" size={17}/></div>
     <h3 className="mt-4 text-sm font-semibold">{item.title}</h3><p className="muted mt-1 min-h-[40px] text-xs leading-relaxed">{item.caption}</p><div className="enterprise-card-rule mt-3 pt-3 text-[11px]">{item.detail}</div>
    </button>})}
   </div>
  </section>

  <div className="grid gap-4 xl:grid-cols-[minmax(0,1.5fr)_minmax(300px,1fr)]">
   <Card><CardHeader><CardTitle className="flex items-center gap-2"><Workflow size={18}/>Active engine architecture</CardTitle><CardDescription>The currently implemented SQL Server → ClickHouse processing path.</CardDescription></CardHeader><CardContent>
    <div className="grid grid-cols-1 gap-2 sm:grid-cols-4">
     {engineStages.map((stage,i)=>{const Icon=stage.icon;return <div key={stage.name} className="relative rounded-lg border border-border p-3">
      <div className="flex items-center gap-2"><Icon className="text-primary" size={17}/><span className="text-xs font-semibold">{stage.name}</span></div>
      <p className="muted mt-2 text-xs">{stage.caption}</p>
      {i<engineStages.length-1&&<ArrowRight className="enterprise-stage-arrow absolute -right-3 top-5 z-10 hidden bg-card text-primary sm:block" size={16}/>}
     </div>})}
    </div>
    <div className="mt-4 flex items-start gap-2 rounded-lg border border-border p-3"><ShieldAlert size={16} className="mt-0.5 shrink-0 text-amber-400"/><p className="muted text-xs leading-relaxed">Production cutover is not approved. Hard-delete propagation and independent multi-source connection instances are not supported end-to-end. Only run against explicitly allowed scratch destinations.</p></div>
   </CardContent></Card>
   <Card><CardHeader><CardTitle className="flex items-center gap-2"><CheckCircle2 size={18}/>Capabilities</CardTitle><CardDescription>Supported now versus future delivery.</CardDescription></CardHeader><CardContent className="space-y-3">
    <div className="flex items-center justify-between gap-2 text-sm"><span>SQL Server discovery</span><Badge variant="success">Available</Badge></div>
    <div className="flex items-center justify-between gap-2 text-sm"><span>Scratch ClickHouse writer</span><Badge variant="success">Available</Badge></div>
    <div className="flex items-center justify-between gap-2 text-sm"><span>Multi-source instances</span><Badge variant="neutral"><Clock3 size={12}/>Planned</Badge></div>
    <div className="flex items-center justify-between gap-2 text-sm"><span>Physical delete to targets</span><Badge variant="warning">Not ready</Badge></div>
    <Button type="button" variant="outline" className="mt-2 w-full" onClick={()=>onNavigate("lineage")}>Inspect data lineage <ArrowRight size={15}/></Button>
   </CardContent></Card>
  </div>
 </div>;
}
