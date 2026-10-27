import {useEffect,useMemo,useRef,useState} from "react";
import {ArrowLeft,ArrowRight,CalendarClock,Database,HardDrive,Plus,RefreshCw,Search} from "lucide-react";
import {labConnectionOverviewApi,managedApi,type ManagedRun,type QAConnectionOverview,type QAConnectionStream} from "../lib/api";
import {Button} from "./ui/button";

export type ReusedConnection={connectionID:string;connectionName:string;sourceID:string;targetID:string;sourceName:string;targetName:string;existingTables:string[]};
const pairKey=(c:QAConnectionOverview)=>c.id;
const recent=(date?:string)=>{const age=Date.now()-Date.parse(date||"");return Number.isFinite(age)&&age>=-60000&&age<=1800000};
const dateLabel=(value?:string)=>value?new Date(value).toLocaleString("vi-VN"):"—";
const scheduleLabel=(s:QAConnectionStream)=>s.schedule_seconds>0?(s.schedule_enabled?`Mỗi ${s.schedule_seconds/3600} giờ`:"Đã tắt"):"Thủ công";
const statusLabel=(status:string)=>({succeeded:"Thành công",failed:"Lỗi",running:"Đang chạy",queued:"Đang chờ"} as Record<string,string>)[status]||status;
const displayName=(name:string)=>name.replace(/^Airbyte(?:\s+flow)?/i,"DataFlow").replace(/ · [0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i,"");
const runLabel=(s:QAConnectionStream)=>!s.revision_current?"Cần tạo liên kết mới":!s.binding_current?"Cần Test lại profiles":s.latest_run?.status==="succeeded"?"Lần chạy thành công":s.latest_run?.status==="failed"?"Lần chạy lỗi":s.latest_run?.status==="running"?"Đang chạy":s.latest_run?.status==="queued"?"Đang chờ":"Chưa chạy";

export function QAConnectionBrowser({token,adminToken,refreshKey,selected,onSelect,onAddStreams,onManageProfiles,onCreateConnection}:{token:string;adminToken:string;refreshKey:number;selected:string;onSelect:(key:string)=>void;onAddStreams:(pair:ReusedConnection)=>void;onManageProfiles:()=>void;onCreateConnection:()=>void}){
 const[items,setItems]=useState<QAConnectionOverview[]|null>(null);
 const[query,setQuery]=useState("");
 const[loading,setLoading]=useState(false);
 const[error,setError]=useState("");
 const[syncRuns,setSyncRuns]=useState<Record<string,{run?:ManagedRun;error?:string;pending?:boolean}>>({});
 const[detailTab,setDetailTab]=useState<"overview"|"streams"|"settings">("overview");
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
 useEffect(()=>setDetailTab("overview"),[selected]);
 const visible=useMemo(()=>items?.filter(c=>(c.name+" "+c.source_name+" "+c.target_name+" "+c.streams.map(s=>s.source_schema+"."+s.source_table).join(" ")).toLowerCase().includes(query.toLowerCase().trim()))??[],[items,query]);
 const hasStaleBindings=!!detail&&detail.streams.some(s=>!s.revision_current);
 const canReuse=!!detail&&!hasStaleBindings&&detail.source_status==="verified"&&detail.target_status==="verified"&&recent(detail.source_checked_at)&&recent(detail.target_checked_at);
 const syncReady=!!detail&&detail.streams.length>0&&detail.streams.every(s=>s.binding_current)&&canReuse;
 const syncError=detail?.streams.map(s=>syncRuns[s.id]?.error).find(Boolean);
 const latest=detail?.streams.flatMap(s=>s.latest_run?[s.latest_run]:[]).sort((a,b)=>Date.parse(b.requested_at)-Date.parse(a.requested_at))[0];
 const isSyncing=!!detail&&detail.streams.some(s=>syncRuns[s.id]?.pending||["queued","running"].includes(syncRuns[s.id]?.run?.status||""));
 const syncNow=async(streamId:string)=>{
  setSyncRuns(old=>({...old,[streamId]:{pending:true}}));
  try{const run=await managedApi.run(streamId,token);setSyncRuns(old=>({...old,[streamId]:{run}}))}
  catch(e){setSyncRuns(old=>({...old,[streamId]:{error:e instanceof Error?e.message:String(e)}}))}
 };
 const syncAll=()=>{if(!detail||!syncReady||isSyncing)return;detail.streams.forEach(s=>void syncNow(s.id));setDetailTab("overview")};
 useEffect(()=>{
  const active=Object.entries(syncRuns).filter(([,state])=>state.run&&!state.error&&["queued","running"].includes(state.run.status));
  if(!active.length)return;
  let cancelled=false;
  const interval=window.setInterval(()=>{void Promise.all(active.map(async([id,state])=>{
   try{const next=await managedApi.status(state.run!.run_id,token);if(!cancelled)setSyncRuns(old=>({...old,[id]:{run:next}}))}
   catch(e){if(!cancelled)setSyncRuns(old=>({...old,[id]:{run:state.run,error:e instanceof Error?e.message:String(e)}}))}
  }))},4000);
  return()=>{cancelled=true;window.clearInterval(interval)};
 },[syncRuns,token]);
 return <div className="dfh-list" aria-label="QA connections">
  {detail?<>
    <div className="dfh-detail-top"><button type="button" className="dfh-back" onClick={()=>onSelect("")}><ArrowLeft size={15}/> Connections</button><span>/</span><span title={detail.name}>{displayName(detail.name)}</span></div>
    <section className="dfh-connection-hero" aria-label="Chi tiết connection">
     <div className="dfh-connection-hero-top"><div><p className="dfh-kicker">CONNECTION</p><h2 title={detail.name}>{displayName(detail.name)}</h2></div><div className="dfh-connection-actions"><span className={syncReady?"dfh-status ready":"dfh-status stale"}>{syncReady?"Ready to sync":hasStaleBindings?"Needs setup":"Retest profiles"}</span><Button type="button" disabled={isSyncing||!syncReady} onClick={syncAll}><RefreshCw size={16}/>{isSyncing?"Đang đồng bộ…":"Sync now"}</Button></div></div>
     <div className="dfh-connection-route"><div className="dfh-route-end"><span className="dfh-route-icon"><Database size={20}/></span><div><small>SOURCE · SQL SERVER</small><strong>{detail.source_name}</strong></div></div><ArrowRight className="dfh-route-arrow" size={20}/><div className="dfh-route-end"><span className="dfh-route-icon"><HardDrive size={20}/></span><div><small>DESTINATION · CLICKHOUSE</small><strong>{detail.target_name}</strong></div></div></div>
    </section>
   {syncError&&<p role="alert" className="df-text-error text-sm">{syncError}</p>}
   {hasStaleBindings?<p className="dfh-data-note" role="status">Liên kết của một số stream đã cũ vì profile từng thay đổi hoặc Test thất bại. Test lại không khôi phục liên kết cũ; hãy tạo connection mới cho các stream đó.</p>:!canReuse&&<p className="dfh-data-note" role="status">Profile nguồn và đích cần được Test thành công trong 30 phút gần đây trước khi thêm stream hoặc chạy sync. Vào Sources và Destinations để Test lại.</p>}
   <div className="dfh-detail-tabs" role="tablist" aria-label="Chi tiết connection">{(["overview","streams","settings"] as const).map(tab=><button key={tab} type="button" role="tab" aria-selected={detailTab===tab} className={detailTab===tab?"active":""} onClick={()=>setDetailTab(tab)}>{tab==="overview"?"Overview":tab==="streams"?"Streams":"Settings"}{tab==="streams"&&<span>{detail.streams.length}</span>}</button>)}</div>
   {detailTab==="overview"&&<section className="dfh-overview" role="tabpanel" aria-label="Overview">
    <div className="dfh-summary-grid"><article><small>LAST SYNC</small><strong>{isSyncing?"Đang đồng bộ":latest?statusLabel(latest.status):"Chưa chạy"}</strong><span>{latest?dateLabel(latest.finished_at||latest.requested_at):"Chưa có lần đồng bộ"}</span></article><article><small>RECORDS IN LAST RUN</small><strong>{latest?latest.rows_written.toLocaleString("vi-VN"):"—"}</strong><span>{latest?"Dòng ghi trong lần chạy gần nhất":"Chưa có dữ liệu chạy"}</span></article><article><small>STREAMS</small><strong>{detail.streams.length}</strong><span>{detail.streams.filter(s=>s.binding_current).length} sẵn sàng đồng bộ</span></article></div>
    <div className="dfh-overview-card"><div><h3>Streams được đồng bộ</h3><p>Danh sách nguồn dữ liệu trong connection này.</p></div><button type="button" className="dfh-open" onClick={()=>setDetailTab("streams")}>Xem streams <ArrowRight size={15}/></button><div className="dfh-overview-streams">{detail.streams.map(s=><div key={s.id}><span><strong>{s.source_schema}.{s.source_table}</strong><small>→ {s.target_database}.{s.target_table}</small></span><span className={s.binding_current?"dfh-status ready":"dfh-status stale"}>{s.binding_current?"Ready":s.revision_current?"Retest":"Needs setup"}</span></div>)}</div></div>
   </section>}
   {detailTab==="streams"&&<div className="dfh-stream-toolbar"><div><h3>Streams</h3><p>Chọn từng stream để xem trạng thái và đồng bộ riêng.</p></div>{canReuse?<Button type="button" variant="outline" onClick={()=>onAddStreams({connectionID:detail.id,connectionName:detail.name,sourceID:detail.source_connection_id,targetID:detail.target_connection_id,sourceName:detail.source_name,targetName:detail.target_name,existingTables:detail.streams.map(s=>s.source_schema+"."+s.source_table)})}><Plus size={16}/> Thêm stream</Button>:<Button type="button" variant="outline" onClick={hasStaleBindings?onCreateConnection:onManageProfiles}>{hasStaleBindings?"Tạo connection mới":"Test lại profiles"}</Button>}</div>}
   {detailTab==="settings"&&<section className="dfh-settings" role="tabpanel" aria-label="Settings"><div className="dfh-settings-card"><div><CalendarClock size={19}/><h3>Lịch đồng bộ</h3></div>{detail.streams.map(s=><p key={s.id}><span>{s.source_schema}.{s.source_table}</span><strong>{scheduleLabel(s)}</strong></p>)}</div><div className="dfh-settings-card"><div><Database size={19}/><h3>Profiles</h3></div><p><span>Source</span><strong>{detail.source_name}</strong></p><p><span>Destination</span><strong>{detail.target_name}</strong></p><Button type="button" size="sm" variant="outline" onClick={onManageProfiles}>Quản lý profiles <ArrowRight size={15}/></Button></div></section>}
   {detailTab==="streams"&&<><div className="dfh-table-wrap" role="tabpanel" aria-label="Streams"><table className="dfh-table dfh-stream-table"><thead><tr><th>Stream nguồn</th><th>Bảng đích</th><th>Lần chạy gần nhất</th><th>Lịch</th><th>Đồng bộ</th></tr></thead><tbody>{detail.streams.map(s=>{const current=syncRuns[s.id];const active=current?.pending||current?.run&&["queued","running"].includes(current.run.status);return <tr key={s.id}><td><strong>{s.source_schema}.{s.source_table}</strong><small>{current?.run?statusLabel(current.run.status):runLabel(s)}</small></td><td>{s.target_database}.{s.target_table}</td><td>{current?.run?<>{current.run.rows_written?.toLocaleString("vi-VN")??0} dòng<small>{statusLabel(current.run.status)} · {current.run.run_id.slice(0,8)}…</small></>:s.latest_run?<>{s.latest_run.rows_written.toLocaleString("vi-VN")} dòng<small>{dateLabel(s.latest_run.finished_at||s.latest_run.requested_at)}</small></>:"Chưa có lần chạy"}</td><td>{scheduleLabel(s)}</td><td><Button type="button" size="sm" variant="outline" disabled={!canReuse||!s.binding_current||!!active} onClick={()=>void syncNow(s.id)}>{active?"Đang chạy…":"Sync now"}</Button>{current?.error&&<small role="alert" className="df-text-error">{current.error}</small>}</td></tr>})}</tbody></table></div><p className="dfh-data-note">Trạng thái hiển thị theo lần chạy gần nhất và phiên bản liên kết đã lưu; không suy diễn kết nối đang online.</p></>}
  </>:<>
   <div className="dfh-list-tools"><label className="dfh-search"><Search size={17} aria-hidden="true"/><input type="search" aria-label="Tìm QA connection" placeholder="Tìm nguồn, đích hoặc stream…" value={query} onChange={e=>setQuery(e.target.value)}/></label><Button type="button" variant="outline" size="sm" disabled={loading||!adminToken} onClick={()=>void load()}>{loading?"Đang tải…":"Làm mới"}</Button></div>
   {!adminToken&&<p className="dfh-empty">Mở Profiles và nhập quyền quản trị QA để xem các connection đã lưu.</p>}
   {error&&<p role="alert" className="df-text-error text-sm">{error}</p>}
   {loading&&!items&&<p role="status" className="dfh-empty">Đang tải connections…</p>}
   {items&&<p className="dfh-count">{visible.length} connections đã lưu</p>}
   {items?.length===0&&<p className="dfh-empty">Chưa có connection nào dùng cặp profile QA đã lưu.</p>}
   {items&&items.length>0&&visible.length===0&&<p className="dfh-empty">Không có kết quả phù hợp.</p>}
   {visible.length>0&&<div className="dfh-table-wrap"><table className="dfh-table dfh-connection-table">
    <thead><tr><th>Connection</th><th>Source</th><th>Destination</th><th>Last sync</th></tr></thead>
    <tbody>{visible.map(c=>{
     const latest=c.streams.flatMap(s=>s.latest_run?[s.latest_run]:[]).sort((a,b)=>Date.parse(b.requested_at)-Date.parse(a.requested_at))[0];
     const current=c.streams.every(s=>s.binding_current);
     const stale=c.streams.some(s=>!s.revision_current);
     return <tr key={pairKey(c)}>
      <td><button type="button" className="dfh-open" title={displayName(c.name)} onClick={()=>onSelect(pairKey(c))}>{displayName(c.name)}</button><div className="dfh-connection-meta"><span>{c.streams.length} {c.streams.length===1?"stream":"streams"}</span><span className={current?"dfh-status ready":"dfh-status stale"}>{current?"Valid":stale?"Needs setup":"Retest profiles"}</span></div></td>
      <td className="dfh-profile-name">{c.source_name}</td>
      <td className="dfh-profile-name">{c.target_name}</td>
      <td>{latest?<>{statusLabel(latest.status)}<small>{dateLabel(latest.finished_at||latest.requested_at)}</small></>:"Chưa chạy"}</td>
     </tr>;
    })}</tbody>
   </table></div>}
  </>}
 </div>;
}
