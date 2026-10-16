import {useCallback,useEffect,useMemo,useState} from "react";
import {AlertTriangle,ArrowRight,Database,GitBranch,RefreshCw,Search,ShieldCheck,Workflow} from "lucide-react";
import {Button} from "./ui/button";
import {transformApi,type DbtRun,type LineageGraph,type LineageNode} from "../lib/transformApi";

type Positioned={node:LineageNode;x:number;y:number;depth:number};
const NODE_WIDTH=218,NODE_HEIGHT=82,X_STEP=270,Y_STEP=100;
function calculate(graph:LineageGraph):{items:Positioned[];width:number;height:number;positions:Map<string,Positioned>}{
 const nodes=graph.nodes.slice(0,100);
 const levels=new Map<string,number>();
 const depth=(id:string,trail=new Set<string>()):number=>{
  if(levels.has(id))return levels.get(id)!;
  if(trail.has(id))return 0;
  trail.add(id);
  const parents=graph.edges.filter(e=>e.to===id).map(e=>e.from);
  const level=parents.length>0?Math.max(...parents.map(id=>depth(id,new Set(trail))))+1:0;
  levels.set(id,level);return level;
 };
 for(const n of nodes)depth(n.id);
 const columns=new Map<number,LineageNode[]>();
 for(const n of nodes){const l=levels.get(n.id)||0;columns.set(l,[...(columns.get(l)||[]),n])}
 for(const list of columns.values())list.sort((a,b)=>a.label.localeCompare(b.label));
 const items:Positioned[]=[];
 let height=0,maxDepth=0;
 for(const [d,list] of columns){
  maxDepth=Math.max(maxDepth,d);height=Math.max(height,list.length*Y_STEP+24);
  list.forEach((node,i)=>items.push({node,x:24+d*X_STEP,y:20+i*Y_STEP,depth:d}));
 }
 const positions=new Map(items.map(p=>[p.node.id,p]));
 return {items,width:Math.max(660,(maxDepth+1)*X_STEP+44),height:Math.max(200,height),positions};
}
function downstream(graph:LineageGraph,id:string):Set<string>{
 const affected=new Set<string>();const todo=[id];
 while(todo.length){const current=todo.shift()!;
  for(const e of graph.edges){if(e.from===current&&!affected.has(e.to)&&e.to!==id){affected.add(e.to);todo.push(e.to)}}
 }
 return affected;
}
export function DataLineage({token,onTokenChange}:{token:string;onTokenChange:(t:string)=>void}){
 const[graph,setGraph]=useState<LineageGraph|null>(null);
 const[runs,setRuns]=useState<DbtRun[]>([]);
 const[selected,setSelected]=useState<string>("");
 const[filter,setFilter]=useState("");
 const[loading,setLoading]=useState(false);
 const[error,setError]=useState("");
 const refresh=useCallback(async()=>{
  if(!token){setError("Nhập API token để xem Lineage");return}
  setLoading(true);setError("");
  try{
   const [lineage,history]=await Promise.all([transformApi.lineage(token),transformApi.dbtHistory(token).catch(()=>({runs:[]}))]);
   setGraph(lineage);setRuns(history.runs);
   setSelected(old=>lineage.nodes.some(n=>n.id===old)?old:(lineage.nodes.find(n=>n.kind==="model")?.id||""));
  }catch(e){setError(String(e))}
  finally{setLoading(false)}
 },[token]);
 useEffect(()=>{if(token)void refresh()},[refresh,token]);
 const layout=useMemo(()=>graph?calculate(graph):null,[graph]);
 const current=graph?.nodes.find(n=>n.id===selected);
 const affected=useMemo(()=>graph&&current?downstream(graph,current.id):new Set<string>(),[graph,current]);
 const parents=graph?.edges.filter(e=>e.to===selected).map(e=>graph.nodes.find(n=>n.id===e.from)?.label||e.from)||[];
 const children=graph?.edges.filter(e=>e.from===selected).map(e=>graph.nodes.find(n=>n.id===e.to)?.label||e.to)||[];
 const newestRun=current?.kind==="model"?runs.find(r=>r.model_name===current.label):undefined;
 const matches=(n:LineageNode)=>filter.trim()===""||n.label.toLowerCase().includes(filter.toLowerCase());
 const nodeStatus=(node:LineageNode)=>{
  if(node.status==="missing"||node.status==="missing_dependency"||node.status==="missing_source")return "border-rose-600/60 bg-rose-950/30";
  if(node.status==="untracked_sql")return "border-amber-700/50 bg-amber-900/10";
  return node.kind==="source"?"border-slate-600 bg-slate-800/80":"border-sky-700/60 bg-[#10293c]";
 };
 return <div className="space-y-4">
  <div className="section flex flex-wrap items-center justify-between gap-3">
   <div><h2 className="flex items-center gap-2 text-lg font-semibold"><GitBranch size={20}/>Data Lineage & Impact Analysis</h2><p className="muted mt-1 text-sm">Xem dữ liệu đi qua những model nào và biết sửa một model sẽ ảnh hưởng đến đâu.</p></div>
   <Button variant="outline" disabled={loading||!token} onClick={refresh}><RefreshCw size={16}/>Refresh Graph</Button>
  </div>
  <div className="section grid items-end gap-3 md:grid-cols-[1fr_1fr]">
   <label className="space-y-1"><span className="muted text-xs">API bearer token</span><input type="password" autoComplete="off" value={token} className="field" onChange={e=>onTokenChange(e.target.value)}/></label>
   <label className="space-y-1"><span className="muted text-xs">Tìm model hoặc bảng nguồn</span><div className="relative"><Search className="absolute left-3 top-3 text-slate-500" size={15}/><input className="field pl-9" value={filter} onChange={e=>setFilter(e.target.value)} placeholder="vd: revenue, part, inventory"/></div></label>
  </div>
  <div className="flex items-start gap-2 rounded-lg border border-sky-800/50 bg-sky-950/20 p-3 text-xs text-sky-100"><ShieldCheck size={17} className="shrink-0"/>Graph được suy ra từ model Visual đã lưu: nguồn <code>scratch.dfv_model</code> tạo dependency đến model tương ứng. Lineage này chỉ phân tích metadata, <b>không tự build lại hoặc tự chạy dbt</b>. SQL Code là vùng chưa thể truy vết đầy đủ.</div>
  {error&&<div role="alert" className="rounded-lg border border-rose-700 p-3 text-sm text-rose-300">{error}</div>}
  {graph&&layout&&<>
   <div className="grid gap-3 sm:grid-cols-3">
    <div className="section"><div className="muted text-xs">Models</div><div className="mt-1 text-2xl font-semibold">{graph.nodes.filter(n=>n.kind==="model").length}</div></div>
    <div className="section"><div className="muted text-xs">Dependencies</div><div className="mt-1 text-2xl font-semibold">{graph.edges.filter(e=>e.from.startsWith("model:")).length}</div></div>
    <div className="section"><div className="muted text-xs">Needs attention</div><div className="mt-1 text-2xl font-semibold">{graph.problems.length}</div></div>
   </div>
   <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_300px]">
    <div className="section min-w-0">
     <div className="mb-3 flex flex-wrap items-center justify-between gap-2"><h3 className="flex items-center gap-2 font-semibold"><Workflow size={18}/>Dependency Map</h3><span className="muted text-xs">Bấm một node để xem impact · cuộn ngang khi đồ thị lớn</span></div>
     <div className="max-h-[680px] overflow-auto rounded-lg border border-slate-700 bg-slate-950/60">
      {graph.nodes.length>100&&<p className="p-2 text-xs text-amber-300">Chỉ hiển thị 100 node đầu để giữ giao diện mượt; backend vẫn phân tích đủ toàn bộ danh sách.</p>}
      <div className="relative" style={{width:layout.width,height:layout.height}}>
       <svg aria-hidden="true" className="absolute inset-0 pointer-events-none" width={layout.width} height={layout.height}>
        <defs><marker id="dataflow-lineage-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="5" markerHeight="5" orient="auto-start-reverse"><path d="M0,0 L10,5 L0,10 Z" fill="#64748b"/></marker></defs>
        {graph.edges.map((edge,i)=>{const start=layout.positions.get(edge.from),end=layout.positions.get(edge.to);if(!start||!end)return null;
         const x1=start.x+NODE_WIDTH,x2=end.x,y1=start.y+NODE_HEIGHT/2,y2=end.y+NODE_HEIGHT/2,mid=(x1+x2)/2;
         const hot=selected===edge.from||selected===edge.to||affected.has(edge.to);
         return <path key={i} d={`M ${x1} ${y1} C ${mid} ${y1}, ${mid} ${y2}, ${x2-7} ${y2}`} stroke={hot?"#38bdf8":"#64748b"} strokeWidth={hot?2.2:1.3} strokeOpacity={hot?1:0.55} fill="none" markerEnd="url(#dataflow-lineage-arrow)"/>;
        })}
       </svg>
       {layout.items.map(({node,x,y})=>{
        const active=node.id===selected,hot=affected.has(node.id);
        const faded=!matches(node)&&!active&&!hot;
        return <button key={node.id} type="button" title={node.label} style={{position:"absolute",left:x,top:y,width:NODE_WIDTH,height:NODE_HEIGHT}} onClick={()=>setSelected(node.id)}
         className={`rounded-lg border px-3 py-2 text-left transition ${nodeStatus(node)} ${active?"ring-2 ring-sky-400":hot?"ring-1 ring-amber-400":""} ${faded?"opacity-35":""} hover:border-sky-400 focus-visible:outline focus-visible:outline-2 focus-visible:outline-sky-400`}>
         <div className="mb-1 flex items-center gap-1.5 text-xs text-slate-400">{node.kind==="source"?<Database size={14}/>:<GitBranch size={14}/>}<span>{node.kind==="source"?"Data Source":node.kind==="missing"?"Missing Model":"Transform Model"}</span></div>
         <div className="truncate text-sm font-semibold">{node.label}</div>
         <div className="mt-1 truncate text-[10px] text-slate-400">{node.status==="ready"?"Defined":node.status.replaceAll("_"," ")}</div>
        </button>;
       })}
      </div>
     </div>
     {graph.problems.length>0&&<div className="mt-3 space-y-1 rounded-md border border-amber-800/50 bg-amber-950/20 p-3 text-xs">{graph.problems.slice(0,12).map((p,i)=><div key={i} className="flex items-start gap-2 text-amber-200"><AlertTriangle size={14} className="shrink-0"/><span><b>{p.model}</b>: {p.detail}</span></div>)}</div>}
    </div>
    <div className="section min-w-0 space-y-3">
     <h3 className="font-semibold">Model Inspector</h3>
     {!current?<p className="muted text-sm">Chọn một model trên sơ đồ.</p>:<>
      <div className="rounded-lg border border-slate-700 bg-slate-800/50 p-3"><div className="muted text-xs">{current.kind}</div><p className="mt-1 break-all font-semibold">{current.label}</p><p className="muted mt-1 text-xs">{current.target||"External / input source"}</p></div>
      <div><p className="mb-2 text-xs font-semibold text-slate-300">Đọc dữ liệu từ</p>{parents.length?parents.map(x=><div key={x} className="my-1 rounded bg-slate-800 p-2 text-xs">{x}</div>):<p className="muted text-xs">Không có upstream model được khai báo.</p>}</div>
      <div><p className="mb-2 text-xs font-semibold text-slate-300">Model trực tiếp phía sau</p>{children.length?children.map(x=><div key={x} className="my-1 flex items-center gap-2 rounded bg-slate-800 p-2 text-xs"><ArrowRight size={13}/>{x}</div>):<p className="muted text-xs">Không có downstream trực tiếp.</p>}</div>
      {current.kind==="model"&&<div className="rounded-lg border border-amber-800/50 bg-amber-950/20 p-3"><p className="text-xs font-semibold text-amber-200">Impact nếu sửa model này</p><p className="mt-1 text-2xl font-semibold">{[...affected].filter(id=>id.startsWith("model:")).length}</p><p className="muted text-xs">model downstream có thể cần kiểm tra hoặc build lại</p><div className="mt-2 flex flex-wrap gap-1">{[...affected].filter(id=>id.startsWith("model:")).sort().map(id=><span key={id} className="rounded border border-amber-800 px-2 py-1 text-[11px] text-amber-200">{id.replace("model:","")}</span>)}</div></div>}
      {newestRun&&<div className="border-t border-slate-700 pt-3 text-xs"><b>dbt run gần nhất</b><p className="muted mt-1">{newestRun.status} · {newestRun.target||"scratch"}</p><p className="muted mt-1">Trạng thái run gần nhất không có nghĩa model hiện tại đã build đúng phiên bản SQL mới nhất.</p></div>}
     </>}
    </div>
   </div>
  </>}
  {!graph&&!loading&&!error&&<div className="section muted text-center text-sm">Chưa tải graph. Nhập API token và bấm Refresh Graph.</div>}
 </div>;
}
