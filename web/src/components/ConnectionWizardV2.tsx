import {useEffect,useRef,useState} from "react";
import {Button} from "./ui/button";
import {labQABatchSyncApi,labSyncSetupApi,managedApi,scheduleApi,type ColumnInfo,type TableInfo,type LabRuntimeConnection,type QABatchStream,type QABatchResult,type ManagedRun} from "../lib/api";
import "./ConnectionWizardV2.css";

type Mode="full_refresh"|"incremental_rowversion";
type Row={schema:string;name:string;enabled:boolean;status:"loading"|"ready"|"error";error:string;columns:ColumnInfo[];selected:string[];key:string;cursor:string;automatic:boolean;mode:Mode};
type Props={token:string;adminToken:string;profiles:LabRuntimeConnection[];onCreated?:()=>void;onGoToTab?:(tab:"sources"|"destinations")=>void};
type Frequency="manual"|"3600"|"21600"|"86400";
type Filter="all"|"selected"|"unselected";
const rowId=(r:Pick<Row,"schema"|"name">)=>r.schema+"."+r.name;
const nativeCursor=(c:ColumnInfo)=>c.cursor_candidate&&["rowversion","timestamp"].includes(c.sql_type.toLowerCase());
const dateCursor=(c:ColumnInfo)=>["datetime","datetime2","smalldatetime","datetimeoffset"].includes(c.sql_type.toLowerCase());
const verified=(p:LabRuntimeConnection)=>{const ms=Date.now()-Date.parse(p.checked_at||"");return p.status==="verified"&&Number.isFinite(ms)&&ms>=-60000&&ms<=1800000};
const alias=(schema:string,table:string)=>{let v=(schema+"_"+table).toLowerCase().replace(/[^a-z0-9_]/g,"_").slice(0,30);return /^[a-z]/.test(v)?v:"t"+v.slice(0,29)};
const effective=(r:Row)=>r.columns.filter(c=>r.selected.includes(c.name)||c.name===r.key||(!!r.cursor&&c.name===r.cursor)).map(c=>c.name);
const rowProblem=(r:Row)=>{if(!r.enabled)return "";if(r.status!=="ready")return r.status==="error"?r.error:"Chưa tải metadata";const k=r.columns.find(c=>c.name===r.key);if(!k||k.nullable)return "Cần khóa chính không nullable";if(effective(r).length<1||effective(r).length>256)return "Cần 1–256 cột";if(r.mode==="incremental_rowversion"&&!r.columns.some(c=>c.name===r.cursor&&nativeCursor(c)))return "Incremental bị chặn: thiếu rowversion/timestamp hợp lệ";return ""};
const errorText=(e:unknown)=>e instanceof Error?e.message:String(e);
const isLocked=(r:Row,c:string)=>c===r.key||!!r.cursor&&c===r.cursor;
export function ConnectionWizardV2({token,adminToken,profiles,onCreated,onGoToTab}:Props){
 const[step,setStep]=useState<1|2|3|4>(1);
 const[sourceID,setSourceID]=useState("");const[targetID,setTargetID]=useState("");
 const[schema,setSchema]=useState("Erp");const[loaded,setLoaded]=useState(false);
 const[loadedAt,setLoadedAt]=useState("");const[rows,setRows]=useState<Row[]>([]);
 const[loading,setLoading]=useState(false);const[busy,setBusy]=useState(false);
 const[error,setError]=useState("");const[frequency,setFrequency]=useState<Frequency>("manual");
 const[created,setCreated]=useState<QABatchResult[]>([]);const[scheduleErrors,setScheduleErrors]=useState<string[]>([]);
 const[runs,setRuns]=useState<Record<string,{run?:ManagedRun;error?:string}>>({});
 const[modalId,setModalId]=useState("");const[query,setQuery]=useState("");const[filter,setFilter]=useState<Filter>("all");
 const generation=useRef(0);
 const src=profiles.find(p=>p.id===sourceID),dst=profiles.find(p=>p.id===targetID);
 const srcOk=!!src&&src.driver==="sqlserver"&&verified(src),dstOk=!!dst&&dst.driver==="clickhouse"&&verified(dst);
 const selectedRows=rows.filter(r=>r.enabled);
 const problems=selectedRows.map(r=>({name:rowId(r),error:rowProblem(r)})).filter(r=>r.error);
 const canCreate=loaded&&selectedRows.length>0&&selectedRows.length<=10&&problems.length===0&&srcOk&&dstOk;
 const modal=rows.find(r=>rowId(r)===modalId);
 const shown=modal?.columns.filter(c=>{if(!(c.name+" "+c.sql_type).toLowerCase().includes(query.toLowerCase()))return false;const chosen=effective(modal).includes(c.name);return filter==="all"||filter==="selected"&&chosen||filter==="unselected"&&!chosen})||[];
 const update=(id:string,fn:(r:Row)=>Row)=>setRows(old=>old.map(r=>rowId(r)===id?fn(r):r));
 const invalidate=()=>{generation.current++;setLoaded(false);setRows([]);setLoadedAt("");setModalId("");setCreated([]);setRuns({});setScheduleErrors([]);setError("")};
 useEffect(()=>{invalidate();setSourceID("");setTargetID("");setStep(1)},[token,adminToken]);
 useEffect(()=>{if(!modalId)return;const handler=(ev:KeyboardEvent)=>{if(ev.key==="Escape")setModalId("")};document.addEventListener("keydown",handler);return()=>document.removeEventListener("keydown",handler)},[modalId]);
 const fetchColumns=async(r:Row,seq:number,activeSource:string,activeSchema:string)=>{
  const id=rowId(r);update(id,x=>({...x,status:"loading",error:""}));
  try{const result=await labSyncSetupApi.columns(activeSource,activeSchema,r.name,token,adminToken);if(seq!==generation.current)return;const cols=result.columns;if(!cols.length)throw Error("API trả về 0 cột");
   const key=cols.find(c=>c.name==="SysRowID"&&!c.nullable)||cols.find(c=>c.primary_key&&!c.nullable)||cols.find(c=>!c.nullable);
   update(id,x=>({...x,status:"ready",columns:cols,selected:cols.map(c=>c.name),key:key?.name||"",cursor:"",automatic:false,mode:"full_refresh",error:""}));
  }catch(e){if(seq===generation.current)update(id,x=>({...x,status:"error",error:errorText(e)}))}
 };
 const discover=async()=>{if(!srcOk||loading||!schema.trim())return;invalidate();const seq=generation.current,activeSource=sourceID,activeSchema=schema.trim();setLoading(true);
  try{const result=await labSyncSetupApi.tables(activeSource,activeSchema,token,adminToken);if(seq!==generation.current)return;
   const list:Row[]=result.tables.map((t:TableInfo,i)=>({schema:t.schema,name:t.name,enabled:i<10,status:"loading",error:"",columns:[],selected:[],key:"",cursor:"",automatic:false,mode:"full_refresh"}));
   setRows(list);setLoaded(true);setLoadedAt(new Date().toLocaleString("vi-VN"));
   for(const r of list.slice(0,10)){if(seq!==generation.current)break;await fetchColumns(r,seq,activeSource,activeSchema)}
  }catch(e){if(seq===generation.current){setError("Discovery lỗi: "+errorText(e));setLoaded(false)}}
  finally{if(seq===generation.current)setLoading(false)}
 };
 const enable=(r:Row,yes:boolean)=>{if(yes&&selectedRows.length>=10){setError("Tối đa 10 bảng mỗi batch");return}setError("");update(rowId(r),x=>({...x,enabled:yes}));if(yes&&r.status==="error")void fetchColumns(r,generation.current,sourceID,schema.trim())};
 const setMode=(r:Row,mode:Mode)=>{if(mode==="incremental_rowversion"&&!r.columns.some(nativeCursor))return;update(rowId(r),x=>{const first=x.columns.find(nativeCursor);const auto=mode==="incremental_rowversion"&&!x.cursor&&!!first;const cursor=auto?first!.name:x.cursor;return {...x,mode,cursor,automatic:auto||x.automatic,selected:Array.from(new Set([...x.selected,...(cursor?[cursor]:[])]))}})};
 const changeCursor=(r:Row,name:string)=>update(rowId(r),x=>({...x,cursor:name,automatic:false,selected:Array.from(new Set([...x.selected,...(name?[name]:[])]))}));
 const changeKey=(r:Row,name:string)=>update(rowId(r),x=>({...x,key:name,selected:Array.from(new Set([...x.selected,...(name?[name]:[])]))}));
 const choose=(r:Row,names:string[],yes:boolean)=>update(rowId(r),x=>{const editable=names.filter(n=>!isLocked(x,n));return {...x,selected:yes?Array.from(new Set([...x.selected,...editable])):x.selected.filter(n=>!editable.includes(n))}});
 const create=async()=>{if(!canCreate||busy||created.length)return;setBusy(true);setError("");setScheduleErrors([]);
  const streams:QABatchStream[]=selectedRows.map(r=>({source_schema:r.schema,source_table:r.name,target_database:"scratch",target_table:alias(r.schema,r.name),destination_layout:"raw_event",selected_columns:effective(r),source_key_column:r.key,source_cursor_column:r.cursor,mode:r.mode,batch_size:10000,workers:2,queue_capacity:4}));
  try{const result=await labQABatchSyncApi.create({source_connection_id:sourceID,target_connection_id:targetID,streams},token,adminToken);setCreated(result.streams);onCreated?.();
   if(frequency!=="manual"){const outcomes=await Promise.allSettled(result.streams.map(item=>scheduleApi.save({stream_id:item.stream_id,interval_seconds:Number(frequency),enabled:true},token)));
    setScheduleErrors(outcomes.flatMap((o,i)=>o.status==="rejected"?[result.streams[i].source+": "+errorText(o.reason)]:[]))}
  }catch(e){setError("Tạo batch lỗi: "+errorText(e))}finally{setBusy(false)}
 };
 const run=async(id:string)=>{setRuns(old=>({...old,[id]:{error:"Đang gửi Run…"}}));try{const result=await managedApi.run(id,token);setRuns(old=>({...old,[id]:{run:result}}))}catch(e){setRuns(old=>({...old,[id]:{error:errorText(e)}}))}};
 useEffect(()=>{const active=Object.entries(runs).filter(([,x])=>x.run&&["queued","running"].includes(x.run.status));if(!active.length)return;
  let cancelled=false;const handle=window.setInterval(()=>{void Promise.all(active.map(async([id,x])=>{try{const next=await managedApi.status(x.run!.run_id,token);if(!cancelled)setRuns(old=>({...old,[id]:{run:next}}))}catch(e){if(!cancelled)setRuns(old=>({...old,[id]:{run:x.run,error:"Polling lỗi: "+errorText(e)}}))}}))},4000);
  return()=>{cancelled=true;window.clearInterval(handle)}},[runs,token]);
 const choices=(driver:"sqlserver"|"clickhouse")=>profiles.filter(p=>p.driver===driver);
 const cursorSelect=(r:Row)=><><select className="field" value={r.cursor} aria-label={"Cursor "+r.name} onChange={e=>changeCursor(r,e.target.value)}><option value="">Không có</option>
  {r.columns.filter(c=>nativeCursor(c)||dateCursor(c)).map(c=><option key={c.name} value={c.name} disabled={!nativeCursor(c)}>{c.name} ({c.sql_type}){dateCursor(c)?" · API chưa hỗ trợ":""}</option>)}</select>
  {r.automatic&&r.cursor&&<small className="dfw-note">Cursor tự chọn</small>}</>;
 return <section className="dfw-root space-y-4" aria-label="Wizard tạo connection QA">
  <header><h3 className="font-semibold text-lg">Tạo connection · QA</h3><p className="muted text-xs">SQL Server → ClickHouse scratch RAW · tối đa 10 streams</p></header>
  <nav className="dfw-steps">{["Source","Destination","Schema & Streams","Lịch & tạo"].map((x,i)=><div key={x} className={step===i+1?"dfw-step current":"dfw-step"}><b>{i+1}</b>{x}</div>)}</nav>
  {error&&<p role="alert" className="dfw-alert">{error}</p>}
  {step===1&&<div className="space-y-3"><h4 className="font-semibold">1. Chọn Source SQL Server đã Test</h4><p className="muted text-xs">Chỉ profile verified mới được dùng. Profile khác cần Test trong Live QA manager.</p>
   {!choices("sqlserver").length&&<p className="dfw-empty">Chưa có SQL Server profile từ API.</p>}
   {choices("sqlserver").map(p=><label className="dfw-profile" key={p.id}><input type="radio" name="source-v2" checked={sourceID===p.id} disabled={!verified(p)} onChange={()=>{invalidate();setSourceID(p.id)}}/><span><strong>{p.name}</strong><small>{p.database} · {p.host}:{p.port}</small></span><small>{verified(p)?"Verified":"cần Test"}</small></label>)}
   {onGoToTab&&<Button variant="outline" type="button" onClick={()=>onGoToTab("sources")}>Đi tới Sources</Button>}
  </div>}
  {step===2&&<div className="space-y-3"><h4 className="font-semibold">2. Chọn Destination ClickHouse đã Test</h4><p className="muted text-xs">Đích luôn là scratch RAW; chưa Test thì không được chọn.</p>
   {!choices("clickhouse").length&&<p className="dfw-empty">Chưa có ClickHouse profile từ API.</p>}
   {choices("clickhouse").map(p=><label className="dfw-profile" key={p.id}><input type="radio" name="target-v2" checked={targetID===p.id} disabled={!verified(p)} onChange={()=>{invalidate();setTargetID(p.id)}}/><span><strong>{p.name}</strong><small>{p.database} · {p.host}:{p.port}</small></span><small>{verified(p)?"Verified":"cần Test"}</small></label>)}
   {onGoToTab&&<Button variant="outline" type="button" onClick={()=>onGoToTab("destinations")}>Đi tới Destinations</Button>}
  </div>}
  {step===3&&<div className="space-y-4"><div className="dfw-discovery space-y-3"><h4 className="font-semibold">3. Discovery schema</h4><div className="flex gap-3 flex-wrap items-end"><label className="flex-1 min-w-[180px]">SQL schema<input className="field mt-1" aria-label="Discovery schema" value={schema} onChange={e=>{setSchema(e.target.value);invalidate()}}/></label>
   <Button variant="outline" type="button" disabled={!srcOk||loading||!schema.trim()} onClick={()=>void discover()}>{loading?"Đang tải…":"Tải / làm mới schema"}</Button></div>
   <p className="muted text-xs">Chỉ đọc metadata bằng SELECT. Đổi schema phải tải lại. Không dùng ERP thật.</p>
   {loaded&&<p role="status">{rows.length} bảng · {rows.reduce((n,r)=>n+(r.status==="ready"?r.columns.length:0),0)} cột đã tải · {loadedAt}</p>}</div>
   {!loaded?<p className="dfw-empty">Chưa tải schema. Streams chưa được hiển thị.</p>:<div className="dfw-streams">{rows.map(r=><div className="dfw-stream" key={rowId(r)}>
    <label className="flex gap-2 items-center"><input type="checkbox" checked={r.enabled} aria-label={"Chọn stream "+rowId(r)} onChange={e=>enable(r,e.target.checked)}/><strong>{rowId(r)}</strong></label>
    <div className="dfw-stream-fields"><label>Sync mode<select className="field" aria-label={"Sync mode "+r.name} value={r.mode} disabled={!r.enabled||r.status!=="ready"} onChange={e=>setMode(r,e.target.value as Mode)}>
     <option value="full_refresh">Full refresh</option><option value="incremental_rowversion" disabled={!r.columns.some(nativeCursor)}>Incremental - RowVersion</option></select></label>
     <label>Cursor{cursorSelect(r)}</label><label>Khóa chính<select className="field" value={r.key} aria-label={"Khóa chính "+r.name} disabled={!r.enabled||r.status!=="ready"} onChange={e=>changeKey(r,e.target.value)}><option value="">Chưa chọn</option>{r.columns.filter(c=>!c.nullable).map(c=><option key={c.name} value={c.name}>{c.name}</option>)}</select></label>
     <div><Button type="button" size="sm" variant="outline" disabled={!r.enabled||r.status!=="ready"} onClick={()=>{setModalId(rowId(r));setFilter("all");setQuery("")}}>Chọn cột · {effective(r).length}/{r.columns.length}</Button><small className="muted block">{effective(r).length}/{r.columns.length} cột được sync</small></div>
    </div>
    {r.enabled&&r.status==="error"&&<Button type="button" variant="outline" size="sm" onClick={()=>void fetchColumns(r,generation.current,sourceID,schema.trim())}>Tải lại cột</Button>}
    {r.enabled&&r.status==="ready"&&!r.columns.some(nativeCursor)&&<p className="dfw-warning">Incremental bị chặn: không có rowversion/timestamp hợp lệ; Full refresh vẫn có thể chọn.</p>}
    {r.enabled&&rowProblem(r)&&<p className="dfw-warning" role="alert">{rowProblem(r)}</p>}
    </div>)}</div>}
  </div>}
  {step===4&&<div className="space-y-3"><h4 className="font-semibold">4. Review & tạo</h4><p>Nguồn: <b>{src?.name||"Chưa chọn"}</b> · Đích: <b>{dst?.name||"Chưa chọn"} / scratch RAW</b></p>
   {selectedRows.map(r=><div className="dfw-review" key={rowId(r)}><div><b>{rowId(r)}</b><small>{r.mode} · cursor {r.cursor||"không có"} · {effective(r).length} cột</small></div><small>scratch.dfqa_{alias(r.schema,r.name)}_&lt;stream-id&gt;</small></div>)}
   <p className="muted text-xs">Server thêm 12 ký tự stream ID vào tên bảng RAW. Đây là giới hạn API hiện tại.</p>
   {problems.length>0&&<div role="alert" className="dfw-alert"><b>Các stream cần sửa:</b><ul className="list-disc pl-6">{problems.map(p=><li key={p.name}>{p.name}: {p.error}</li>)}</ul></div>}
   <label className="block">Lịch chạy<select className="field mt-1" value={frequency} onChange={e=>setFrequency(e.target.value as Frequency)}><option value="manual">Thủ công</option><option value="3600">Mỗi giờ</option><option value="21600">Mỗi 6 giờ</option><option value="86400">Hằng ngày</option></select></label>
   <Button type="button" disabled={!canCreate||busy||created.length>0} onClick={()=>void create()}>{busy?"Đang tạo…":"Tạo"}</Button>
   {created.length>0&&<div role="status" className="space-y-2"><b>Đã tạo {created.length} streams</b>
    {scheduleErrors.length>0&&<div className="dfw-warning" role="alert">Lưu lịch lỗi nhưng batch đã được tạo: {scheduleErrors.join("; ")}</div>}
    {created.map(c=><div key={c.stream_id} className="dfw-review"><div><b>{c.source}</b><small>{c.destination} · {c.mode}</small>{runs[c.stream_id]?.run&&<small>Run: {runs[c.stream_id].run?.status}</small>}{runs[c.stream_id]?.error&&<small role="alert">{runs[c.stream_id].error}</small>}</div>
     <Button type="button" size="sm" variant="outline" disabled={!!runs[c.stream_id]?.run&&["queued","running"].includes(runs[c.stream_id].run!.status)} onClick={()=>void run(c.stream_id)}>Run</Button></div>)}</div>}
  </div>}
  <footer className="flex justify-between gap-3 border-t pt-3"><Button variant="outline" type="button" disabled={step===1||busy||created.length>0} onClick={()=>setStep((step-1) as 1|2|3|4)}>Quay lại</Button>
   {step<4&&<Button type="button" disabled={loading||busy||step===1&&!srcOk||step===2&&!dstOk||step===3&&!canCreate} onClick={()=>setStep((step+1) as 1|2|3|4)}>Tiếp tục</Button>}</footer>
  {modal&&<div className="dfw-modal-backdrop" role="presentation" onMouseDown={e=>{if(e.target===e.currentTarget)setModalId("")}}><div role="dialog" aria-modal="true" aria-label={"Chọn cột "+rowId(modal)} className="dfw-modal">
   <div className="dfw-modal-head"><div><h3 className="font-semibold">Chọn cột · {rowId(modal)}</h3><small className="muted">{effective(modal).length}/{modal.columns.length} cột được sync</small></div><Button type="button" variant="outline" onClick={()=>setModalId("")}>Đóng</Button></div>
   <div className="dfw-modal-tools"><label>Cursor{cursorSelect(modal)}</label><input type="search" aria-label="Tìm kiếm cột" className="field" placeholder="Tìm tên cột…" value={query} onChange={e=>setQuery(e.target.value)}/>
    <div className="dfw-chips">{([["all","Tất cả"],["selected","Đã chọn"],["unselected","Chưa chọn"]] as const).map(([id,label])=><button type="button" key={id} className={filter===id?"dfw-chip active":"dfw-chip"} aria-pressed={filter===id} onClick={()=>setFilter(id)}>{label}</button>)}</div>
    <div className="flex gap-2 flex-wrap items-center"><small>Đang hiển {shown.length}/{modal.columns.length} cột</small><Button type="button" variant="outline" size="sm" onClick={()=>choose(modal,shown.map(c=>c.name),true)}>Chọn {shown.filter(c=>!effective(modal).includes(c.name)&&!isLocked(modal,c.name)).length} cột đang hiển</Button>
    <Button type="button" variant="outline" size="sm" onClick={()=>choose(modal,modal.columns.map(c=>c.name),true)}>Chọn tất cả</Button><Button type="button" variant="outline" size="sm" onClick={()=>choose(modal,modal.columns.map(c=>c.name),false)}>Bỏ chọn tất cả</Button></div>
   </div>
   {modal.columns.length>100&&<p className="dfw-warning">Trên 100 cột: cần virtualize ở giai đoạn tiếp theo.</p>}
   <div className="dfw-modal-scroll"><div className="dfw-column-grid">{shown.map(c=><label className="dfw-column" key={c.name}><input type="checkbox" checked={effective(modal).includes(c.name)} disabled={isLocked(modal,c.name)} onChange={e=>choose(modal,[c.name],e.target.checked)}/><span className="dfw-colname">{c.name}<small className="muted">{c.sql_type}</small></span>{isLocked(modal,c.name)&&<span className="dfw-locked">Khoá</span>}</label>)}</div></div>
   <div className="dfw-modal-foot"><small>Cột khóa và cursor luôn sync</small><Button type="button" onClick={()=>setModalId("")}>Xong</Button></div>
  </div></div>}
 </section>
}
