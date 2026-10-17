import {useEffect,useState} from "react";
import {AlertTriangle,CheckCircle2,Database,Play,RefreshCw,RotateCcw} from "lucide-react";
import {Button} from "./ui/button";
import {transformApi,type IncrementalRun} from "../lib/transformApi";

const working=new Set(["queued","building","publishing","rolling_back"]);
export function IncrementalStudio({token,onTokenChange}:{token:string;onTokenChange:(value:string)=>void}){
 const[target,setTarget]=useState("dft_sales_demo");
 const[delta,setDelta]=useState("dfd_sales_demo");
 const[key,setKey]=useState("InvoiceNum");
 const[cursor,setCursor]=useState("0");
 const[latest,setLatest]=useState<IncrementalRun|null>(null);
 const[history,setHistory]=useState<IncrementalRun[]>([]);
 const[busy,setBusy]=useState(false);
 const[error,setError]=useState("");
 const[info,setInfo]=useState("");
 const refresh=async()=>{
  if(!token)return;
  try{
   const res=await transformApi.incrementalStatus(token);
   setHistory(res.runs);
   if(latest){
    const next=res.runs.find(r=>r.run_id===latest.run_id);
    if(next)setLatest(next);
   }
  }catch(e){setError(String(e))}
 };
 useEffect(()=>{
  if(!latest||!token||!working.has(latest.state))return;
  let live=true;
  const timer=window.setInterval(()=>{
   void transformApi.incrementalStatus(token,latest.run_id).then(res=>{
    if(live&&res.runs[0])setLatest(res.runs[0]);
   }).catch(e=>{if(live)setError(String(e))});
  },2500);
  return()=>{live=false;window.clearInterval(timer)};
 },[latest?.run_id,latest?.state,token]);
 const build=async()=>{
  if(!token){setError("Nhập API token");return}
  setBusy(true);setError("");setInfo("");
  try{
   const queued=await transformApi.incrementalRun(target.trim(),delta.trim(),key.trim(),cursor.trim(),token);
   setLatest(queued);
   setInfo("Đã xếp hàng cập nhật delta. Cursor chưa thay đổi; chờ DQ rồi mới Publish.");
  }catch(e){setError(String(e))}
  finally{setBusy(false)}
 };
 const action=async(type:"publish"|"rollback")=>{
  if(!latest)return;
  const message=type==="publish"?
    "Xác nhận Publish bản Incremental đã kiểm tra? Cursor sẽ tăng sau khi đổi bảng thành công.":
    "Xác nhận Rollback bản dữ liệu trước? Cursor cũng sẽ quay về mốc cũ.";
  if(!window.confirm(message))return;
  setBusy(true);setError("");
  try{
   const res=await transformApi.incrementalAction(latest.run_id,type,token);
   setLatest({...latest,...res});
   setInfo(type==="publish"?"Đã duyệt Publish, đang kiểm tra UUID và cập nhật checkpoint.":"Đã yêu cầu Rollback về dữ liệu và cursor trước.");
  }catch(e){setError(String(e))}
  finally{setBusy(false)}
 };
 return <div className="df-studio-workspace df-incremental-workspace space-y-4">
  <div className="df-studio-hero section space-y-2">
   <p className="df-studio-kicker mb-2">OPERATIONS / INCREMENTAL / 01</p><h2 className="flex items-center gap-2 text-xl font-bold"><Database size={20}/>Safe Incremental CDC · v1.3</h2>
   <p className="muted text-sm">Cập nhật bản ghi thêm/sửa/xóa từ ClickHouse delta, dựng bản staging rồi kiểm tra trước khi Publish. Phiên bản TEST copy-on-write; chưa phải tối ưu 7 triệu dòng Epicor.</p>
   <div className="flex gap-2 rounded-lg border df-border-warning df-bg-warning-soft p-3 text-xs df-text-warning">
    <AlertTriangle size={17} className="shrink-0"/>Chỉ dùng database scratch Atomic và bảng đích dft_* đã tồn tại. Delta phải có đúng các cột của bảng đích, sau đó là _df_version (UInt64) và _df_deleted (UInt8). Version phải tăng đơn điệu và delta phải append-only.
   </div>
  </div>
  <div className="section grid gap-4 md:grid-cols-2">
   <label className="space-y-1"><span className="muted text-xs">API bearer token</span><input type="password" className="field" value={token} onChange={e=>onTokenChange(e.target.value)} autoComplete="off"/></label>
   <label className="space-y-1"><span className="muted text-xs">Khóa duy nhất</span><input className="field" value={key} onChange={e=>setKey(e.target.value)} placeholder="InvoiceNum"/></label>
   <label className="space-y-1"><span className="muted text-xs">Target (bảng vật lý hiện có trong scratch)</span><input className="field font-mono" value={target} onChange={e=>setTarget(e.target.value)}/></label>
   <label className="space-y-1"><span className="muted text-xs">Delta / CDC table trong scratch</span><input className="field font-mono" value={delta} onChange={e=>setDelta(e.target.value)}/></label>
   <label className="space-y-1"><span className="muted text-xs">Cursor ban đầu (chỉ sử dụng khi đăng ký target lần đầu)</span><input className="field font-mono" value={cursor} onChange={e=>setCursor(e.target.value)} placeholder="0"/></label>
   <div className="flex flex-wrap items-end gap-2">
    <Button disabled={busy||!token} onClick={()=>void build()}><Play size={15}/> Build Delta Staging</Button>
    <Button variant="outline" disabled={busy||!token} onClick={()=>void refresh()}><RefreshCw size={15}/>History</Button>
   </div>
  </div>
  {error&&<div role="alert" className="rounded border df-border-error p-3 text-sm df-text-error">{error}</div>}
  {info&&<div role="status" className="rounded border df-border-accent p-3 text-sm df-text-accent">{info}</div>}
  {latest&&<div className="section space-y-3">
   <div className="flex flex-wrap items-center justify-between"><h3 className="font-semibold">Incremental Run</h3><span className="rounded border df-border-neutral px-2 py-1 font-mono text-xs">{latest.state}</span></div>
   <p className="muted break-all font-mono text-xs">{latest.run_id}</p>
   <div className="grid gap-2 sm:grid-cols-4">
    {([["Lower Cursor",latest.cursor_lower],["Upper Cursor",latest.cursor_upper],["Delta events",latest.delta_rows],["Output rows",latest.rows_verified]] as const).map(([label,value])=>
     <div key={label} className="rounded-lg df-surface-soft p-3"><p className="muted text-xs">{label}</p><p className="mt-1 font-mono text-sm">{value??"—"}</p></div>)}
   </div>
   {latest.error&&<p role="alert" className="text-xs df-text-error">{latest.error}</p>}
   <div className="flex flex-wrap gap-2">
    <Button disabled={busy||latest.state!=="ready"} onClick={()=>void action("publish")}><CheckCircle2 size={15}/>Approve & Publish + Cursor</Button>
    <Button variant="outline" disabled={busy||latest.state!=="published"||!latest.has_backup} onClick={()=>void action("rollback")}><RotateCcw size={15}/>Rollback Data + Cursor</Button>
   </div>
   <p className="muted text-xs">Checkpoint chỉ tăng khi Publish hoàn tất. Nếu không có sự kiện mới, run kết thúc với trạng thái no_changes.</p>
  </div>}
  {history.length>0&&<div className="section"><h3 className="mb-2 font-semibold">Run History</h3>
   <div className="max-h-72 overflow-auto">{history.map(item=>
    <button key={item.run_id} className="flex w-full flex-wrap items-center justify-between gap-2 border-b df-border-neutral px-2 py-2 text-left text-xs df-hover-bg-neutral" onClick={()=>setLatest(item)}>
     <span className="font-mono">{item.target}</span><span>{item.cursor_lower||"?"} → {item.cursor_upper||"?"}</span><span>{item.state}</span>
    </button>)}</div>
  </div>}
 </div>;
}
