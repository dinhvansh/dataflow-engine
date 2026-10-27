import {useEffect,useMemo,useRef,useState} from "react";
import {ArrowLeft,ArrowRight,Plus,Search} from "lucide-react";
import {labConnectionOverviewApi,type QAConnectionOverview,type QAConnectionStream} from "../lib/api";
import {Button} from "./ui/button";

export type ReusedConnection={connectionID:string;connectionName:string;sourceID:string;targetID:string;sourceName:string;targetName:string;existingTables:string[]};
const pairKey=(c:QAConnectionOverview)=>c.id;
const recent=(date?:string)=>{const age=Date.now()-Date.parse(date||"");return Number.isFinite(age)&&age>=-60000&&age<=1800000};
const dateLabel=(value?:string)=>value?new Date(value).toLocaleString("vi-VN"):"—";
const statusLabel=(status:string)=>({succeeded:"Thành công",failed:"Lỗi",running:"Đang chạy",queued:"Đang chờ"} as Record<string,string>)[status]||status;
const runLabel=(s:QAConnectionStream)=>!s.binding_current?"Cần kiểm tra liên kết":s.latest_run?.status==="succeeded"?"Lần chạy thành công":s.latest_run?.status==="failed"?"Lần chạy lỗi":s.latest_run?.status==="running"?"Đang chạy":s.latest_run?.status==="queued"?"Đang chờ":"Chưa chạy";

export function QAConnectionBrowser({token,adminToken,refreshKey,selected,onSelect,onAddStreams,onManageProfiles}:{token:string;adminToken:string;refreshKey:number;selected:string;onSelect:(key:string)=>void;onAddStreams:(pair:ReusedConnection)=>void;onManageProfiles:()=>void}){
 const[items,setItems]=useState<QAConnectionOverview[]|null>(null);
 const[query,setQuery]=useState("");
 const[loading,setLoading]=useState(false);
 const[error,setError]=useState("");
 const request=useRef(0);
 const load=async()=>{
  const current=++request.current;
  if(!token||!adminToken){setItems(null);setError("");setLoading(false);return}
  setItems(null);setLoading(true);setError("");
  try{const result=await labConnectionOverviewApi.list(token,adminToken);if(current===request.current)setItems(result.connections)}
  catch{if(current===request.current){setItems(null);setError("Không tải được connections QA. Kiểm tra quyền quản trị và dịch vụ QA.")}}
  finally{if(current===request.current)setLoading(false)}
 };
 useEffect(()=>{void load();return()=>{request.current++}},[token,adminToken,refreshKey]);
 const detail=items?.find(c=>pairKey(c)===selected);
 const visible=useMemo(()=>items?.filter(c=>(c.name+" "+c.source_name+" "+c.target_name+" "+c.streams.map(s=>s.source_schema+"."+s.source_table).join(" ")).toLowerCase().includes(query.toLowerCase().trim()))??[],[items,query]);
 const canReuse=!!detail&&detail.source_status==="verified"&&detail.target_status==="verified"&&recent(detail.source_checked_at)&&recent(detail.target_checked_at);
 return <div className="dfh-list" aria-label="QA connections">
  {detail?<>
   <div className="dfh-detail-top"><button type="button" className="dfh-back" onClick={()=>onSelect("")}><ArrowLeft size={15}/> Connections</button><span>/</span><span>{detail.name}</span></div>
   <div className="dfh-detail-heading"><div><h3>{detail.name}</h3><p>{detail.source_name} → {detail.target_name} · {detail.streams.length} streams</p></div>
    {canReuse?<Button type="button" onClick={()=>onAddStreams({connectionID:detail.id,connectionName:detail.name,sourceID:detail.source_connection_id,targetID:detail.target_connection_id,sourceName:detail.source_name,targetName:detail.target_name,existingTables:detail.streams.map(s=>s.source_schema+"."+s.source_table)})}><Plus size={16}/> Thêm stream</Button>:<Button type="button" variant="outline" onClick={onManageProfiles}>Test lại profiles để thêm stream</Button>}
   </div>
   {!canReuse&&<p className="dfh-data-note">Profile nguồn và đích cần được Test thành công trong 30 phút gần đây trước khi thêm stream.</p>}
   <div className="dfh-table-wrap"><table className="dfh-table"><thead><tr><th>Stream nguồn</th><th>Bảng đích</th><th>Lần chạy gần nhất</th><th>Lịch</th></tr></thead><tbody>{detail.streams.map(s=><tr key={s.id}><td><strong>{s.source_schema}.{s.source_table}</strong><small>{runLabel(s)}</small></td><td>{s.target_database}.{s.target_table}</td><td>{s.latest_run?<>{s.latest_run.rows_written.toLocaleString("vi-VN")} dòng<small>{dateLabel(s.latest_run.finished_at||s.latest_run.requested_at)}</small></>:"Chưa có lần chạy"}</td><td>{s.schedule_seconds>0?(s.schedule_enabled?`Mỗi ${s.schedule_seconds/3600} giờ`:"Đã tắt"):"Thủ công"}</td></tr>)}</tbody></table></div>
   <p className="dfh-data-note">Trạng thái hiển thị theo lần chạy gần nhất và phiên bản liên kết đã lưu; không suy diễn kết nối đang online.</p>
  </>:<>
   <div className="dfh-list-tools"><label className="dfh-search"><Search size={17} aria-hidden="true"/><input type="search" aria-label="Tìm QA connection" placeholder="Tìm nguồn, đích hoặc stream…" value={query} onChange={e=>setQuery(e.target.value)}/></label><Button type="button" variant="outline" size="sm" disabled={loading||!adminToken} onClick={()=>void load()}>{loading?"Đang tải…":"Làm mới"}</Button></div>
   {!adminToken&&<p className="dfh-empty">Mở Profiles và nhập quyền quản trị QA để xem các connection đã lưu.</p>}
   {error&&<p role="alert" className="df-text-error text-sm">{error}</p>}
   {loading&&!items&&<p role="status" className="dfh-empty">Đang tải connections…</p>}
   {items&&<p className="dfh-count">{visible.length} connections đã lưu</p>}
   {items?.length===0&&<p className="dfh-empty">Chưa có connection nào dùng cặp profile QA đã lưu.</p>}
   {items&&items.length>0&&visible.length===0&&<p className="dfh-empty">Không có kết quả phù hợp.</p>}
   {visible.length>0&&<div className="dfh-table-wrap"><table className="dfh-table"><thead><tr><th>Connection</th><th>Streams</th><th>Lần chạy gần nhất</th></tr></thead><tbody>{visible.map(c=>{const latest=c.streams.flatMap(s=>s.latest_run?[s.latest_run]:[]).sort((a,b)=>Date.parse(b.requested_at)-Date.parse(a.requested_at))[0];return <tr key={pairKey(c)}><td><button type="button" className="dfh-open" onClick={()=>onSelect(pairKey(c))}>{c.name}</button><small>{c.source_name} <ArrowRight size={12} aria-hidden="true"/> {c.target_name}</small></td><td>{c.streams.length}</td><td>{latest?<>{statusLabel(latest.status)}<small>{dateLabel(latest.finished_at||latest.requested_at)}</small></>:"Chưa chạy"}</td></tr>})}</tbody></table></div>}
  </>}
 </div>;
}
