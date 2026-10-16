import {useEffect,useMemo,useState,type DragEvent} from "react";
import {ArrowDown,ArrowUp,Braces,CheckCircle2,ChevronDown,Code2,Database,Eye,Filter,GripVertical,Layers3,Plus,RefreshCw,Save,ShieldAlert,Table2,Trash2,Undo2} from "lucide-react";
import {Button} from "./ui/button";
import {transformApi,type TableRun,type DbtRun,type TransformColumn,type TransformKind,type TransformModel,type TransformStep,type SavedTransform,type CompileResult,type PreviewResult} from "../lib/transformApi";

const operationDetails:{kind:TransformKind;name:string;hint:string}[]=[
 {kind:"filter",name:"Lọc hàng",hint:"Filter rows"},
 {kind:"select",name:"Chọn cột",hint:"Choose columns"},
 {kind:"rename",name:"Đổi tên cột",hint:"Rename"},
 {kind:"calculate",name:"Cột tính toán",hint:"Calculated column"},
 {kind:"aggregate",name:"Nhóm & tổng hợp",hint:"Group By"},
 {kind:"sort",name:"Sắp xếp",hint:"Sort"},
 {kind:"limit",name:"Giới hạn dòng",hint:"Keep top rows"}
];
const labels:Record<TransformKind,string>=Object.fromEntries(operationDetails.map(o=>[o.kind,o.name])) as Record<TransformKind,string>;
const defaultModel:TransformModel={name:"my_first_transform",mode:"visual",source:{database:"scratch",table:"dataflow_raw_events"},steps:[],sql:""};
const kinds=new Set<TransformKind>(operationDetails.map(o=>o.kind));
let nextID=0;
function makeStep(kind:TransformKind,columns:TransformColumn[]):TransformStep{
 const first=columns[0]?.name||"_pk";
 const second=columns[1]?.name||"_sysrevid";
 const id="step_"+String(++nextID);
 switch(kind){
  case "filter":return {id,type:kind,enabled:true,column:first,operator:"equals",value:"",value_type:"text"};
  case "select":return {id,type:kind,enabled:true,columns:[first,second]};
  case "rename":return {id,type:kind,enabled:true,column:first,alias:first+"_new"};
  case "calculate":return {id,type:kind,enabled:true,column:first,alias:"calculated_value",formula_operator:"multiply",value:"1"};
  case "aggregate":return {id,type:kind,enabled:true,column:first,group_by:[second],function:"count",alias:"total_rows"};
  case "sort":return {id,type:kind,enabled:true,column:first,direction:"ASC"};
  case "limit":return {id,type:kind,enabled:true,limit:100};
 }
}
function TextField({label,value,onChange,placeholder}:{label:string;value:string;onChange:(next:string)=>void;placeholder?:string}){
 return <label className="block space-y-1"><span className="text-xs text-slate-400">{label}</span><input className="field" value={value} placeholder={placeholder} onChange={e=>onChange(e.target.value)}/></label>;
}
function StepFields({step,columns,onChange}:{step:TransformStep;columns:TransformColumn[];onChange:(patch:Partial<TransformStep>)=>void}){
 const name=(v:string)=>onChange({column:v});
 const columnPicker=(label:string,current:string,cb:(v:string)=>void)=><label className="block space-y-1"><span className="text-xs text-slate-400">{label}</span>{columns.length>0?
  <select className="field" value={current} onChange={e=>cb(e.target.value)}>{!columns.some(c=>c.name===current)&&<option value={current}>{current}</option>}{columns.map(c=><option key={c.name} value={c.name}>{c.name} · {c.type}</option>)}</select>:
  <input className="field" value={current} onChange={e=>cb(e.target.value)} placeholder="Tên cột"/>}</label>;
 const selector=(label:string,value:string,items:[string,string][],cb:(s:string)=>void)=><label className="block space-y-1"><span className="text-xs text-slate-400">{label}</span><select className="field" value={value} onChange={e=>cb(e.target.value)}>{items.map(([v,t])=><option key={v} value={v}>{t}</option>)}</select></label>;
 switch(step.type){
  case "filter":return <div className="space-y-3">
   {columnPicker("Cột",step.column||"",name)}
   {selector("Điều kiện",step.operator||"equals",[["equals","Bằng (=)"],["not_equals","Khác (!=)"],["greater","Lớn hơn (>)"],["less","Nhỏ hơn (<)"],["greater_or_equal","≥"],["less_or_equal","≤"],["contains","Chứa ký tự"],["is_null","Là NULL"],["is_not_null","Không NULL"]],s=>onChange({operator:s}))}
   {!["is_null","is_not_null"].includes(step.operator||"")&&<><TextField label="Giá trị so sánh" value={step.value||""} onChange={s=>onChange({value:s})}/>{selector("Kiểu dữ liệu",step.value_type||"text",[["text","Văn bản"],["number","Số"]],s=>onChange({value_type:s}))}</>}
  </div>;
  case "select":return <div className="space-y-2">
   <p className="text-xs text-slate-400">Chọn các cột muốn giữ. Có thể nhập tên cột cách nhau bằng dấu phẩy.</p>
   <textarea className="field min-h-20 text-sm" value={(step.columns||[]).join(", ")} onChange={e=>onChange({columns:e.target.value.split(",").map(s=>s.trim()).filter(Boolean)})}/>
   {columns.length>0&&<div className="flex max-h-32 flex-wrap gap-1 overflow-y-auto">{columns.map(c=><button type="button" key={c.name} className={"rounded border px-2 py-1 text-xs "+((step.columns||[]).includes(c.name)?"border-sky-400 bg-sky-500/20":"border-slate-700 bg-slate-800")} onClick={()=>onChange({columns:(step.columns||[]).includes(c.name)?(step.columns||[]).filter(n=>n!==c.name):[...(step.columns||[]),c.name]})}>{c.name}</button>)}</div>}
  </div>;
  case "rename":return <div className="space-y-3">{columnPicker("Tên cột hiện tại",step.column||"",name)}<TextField label="Tên mới" value={step.alias||""} onChange={s=>onChange({alias:s})}/></div>;
  case "calculate":return <div className="space-y-3">{columnPicker("Cột số nguồn",step.column||"",name)}{selector("Phép tính",step.formula_operator||"multiply",[["add","Cộng (+)"],["subtract","Trừ (−)"],["multiply","Nhân (×)"],["divide","Chia (÷)"]],s=>onChange({formula_operator:s}))}<TextField label="Giá trị số" value={step.value||""} onChange={s=>onChange({value:s})}/><TextField label="Tên cột mới" value={step.alias||""} onChange={s=>onChange({alias:s})}/></div>;
  case "aggregate":return <div className="space-y-3">
   <TextField label="Nhóm theo cột (cách nhau bằng dấu phẩy)" value={(step.group_by||[]).join(", ")} onChange={s=>onChange({group_by:s.split(",").map(v=>v.trim()).filter(Boolean)})}/>
   {selector("Hàm tổng hợp",step.function||"count",[["count","Đếm dòng"],["sum","Tổng SUM"],["avg","Trung bình AVG"],["min","Nhỏ nhất MIN"],["max","Lớn nhất MAX"]],s=>onChange({function:s}))}
   {step.function!=="count"&&columnPicker("Cột giá trị",step.column||"",name)}
   <TextField label="Tên cột kết quả" value={step.alias||""} onChange={s=>onChange({alias:s})}/>
  </div>;
  case "sort":return <div className="space-y-3">{columnPicker("Cột sắp xếp",step.column||"",name)}{selector("Thứ tự",step.direction||"ASC",[["ASC","Tăng dần"],["DESC","Giảm dần"]],s=>onChange({direction:s}))}</div>;
  case "limit":return <label className="block space-y-1"><span className="text-xs text-slate-400">Số dòng tối đa (1–10000)</span><input type="number" className="field" min={1} max={10000} value={step.limit??100} onChange={e=>onChange({limit:Number(e.target.value)})}/></label>;
 }
}
export function TransformStudio({token,onTokenChange}:{token:string;onTokenChange:(value:string)=>void}){
 const[model,setModel]=useState<TransformModel>(defaultModel);
 const[columns,setColumns]=useState<TransformColumn[]>([]);
 const[compiled,setCompiled]=useState<CompileResult|null>(null);
 const[preview,setPreview]=useState<PreviewResult|null>(null);
 const[saved,setSaved]=useState<SavedTransform[]>([]);
 const[selected,setSelected]=useState<string>("source");
 const[dragged,setDragged]=useState<string|null>(null);
 const[busy,setBusy]=useState(false);
 const[error,setError]=useState("");
 const[info,setInfo]=useState("");
 const[sqlDirty,setSQLDirty]=useState(false);
 const[compileError,setCompileError]=useState("");
 const[savedModelID,setSavedModelID]=useState("");
 const[unsaved,setUnsaved]=useState(true);
 const[dbtRun,setDbtRun]=useState<DbtRun|null>(null);
 const[tableRun,setTableRun]=useState<TableRun|null>(null);
 const[uniqueKey,setUniqueKey]=useState("");
 const[tableBusy,setTableBusy]=useState(false);
 const active=useMemo(()=>model.steps.filter(s=>s.enabled!==false),[model.steps]);
 const selectedStep=model.steps.find(s=>s.id===selected);
 const selectedIndex=selected==="source"?0:Math.max(0,active.findIndex(s=>s.id===selected)+1);
 const displayedFields=preview?.meta?.length?preview.meta.map(m=>m.name):Object.keys(preview?.data?.[0]||{});
 const patchModel=(patch:Partial<TransformModel>)=>{setModel(old=>({...old,...patch}));setPreview(null);setSQLDirty(true);setUnsaved(true);setError("");};
 const patchStep=(id:string,patch:Partial<TransformStep>)=>patchModel({steps:model.steps.map(s=>s.id===id?{...s,...patch}:s)});
 const addStep=(type:TransformKind,position?:number)=>{
  if(model.steps.length>=30){setError("Chỉ hỗ trợ tối đa 30 bước");return}
  const step=makeStep(type,columns);
  // Persisted models may already contain step_1 etc. Never reuse IDs.
  while(model.steps.some(existing=>existing.id===step.id)){step.id="step_"+String(++nextID)}
  const next=[...model.steps];next.splice(position??next.length,0,step);
  patchModel({steps:next,mode:"visual"});setSelected(step.id);
 };
 const shift=(id:string,delta:number)=>{
  const current=model.steps.findIndex(s=>s.id===id);const nextIndex=current+delta;
  if(current<0||nextIndex<0||nextIndex>=model.steps.length)return;
  const next=[...model.steps];[next[current],next[nextIndex]]=[next[nextIndex],next[current]];
  patchModel({steps:next});
 };
 const dropped=(event:DragEvent<HTMLElement>,index:number)=>{
  event.preventDefault();const fromPalette=event.dataTransfer.getData("application/x-dataflow-tool");
  if(kinds.has(fromPalette as TransformKind)){addStep(fromPalette as TransformKind,index);return}
  const id=event.dataTransfer.getData("application/x-dataflow-step");
  const original=model.steps.findIndex(s=>s.id===id);
  if(original<0)return;
  const next=[...model.steps];const [step]=next.splice(original,1);
  const newIndex=original<index?Math.max(0,index-1):index;
  next.splice(newIndex,0,step);patchModel({steps:next});setSelected(id);setDragged(null);
 };
 useEffect(()=>{
  if(!token)return;
  let live=true;const timeout=window.setTimeout(()=>{
   void transformApi.compile(model,token).then(result=>{if(live){setCompiled(result);setSQLDirty(false);setCompileError("")}}).catch((cause:unknown)=>{if(live){setCompiled(null);setSQLDirty(true);setCompileError(String(cause))}});
  },320);
  return()=>{live=false;window.clearTimeout(timeout)};
 },[model,token]);
 useEffect(()=>{
  if(!dbtRun||!token||!["queued","running"].includes(dbtRun.status))return;
  let live=true;
  const timer=window.setInterval(()=>{
   void transformApi.dbtStatus(dbtRun.run_id,token).then(x=>{if(live)setDbtRun(x)}).catch(()=>{});
  },3000);
  return()=>{live=false;window.clearInterval(timer)};
 },[dbtRun?.run_id,dbtRun?.status,token]);
 const executeView=async()=>{
  if(!savedModelID||unsaved){setError("Lưu model trước khi tạo View");return}
  setBusy(true);setError("");setInfo("");
  try{const r=await transformApi.dbtRun(savedModelID,token);setDbtRun(r);setInfo("Đã xếp hàng dbt View, xem trạng thái bên dưới. Worker cần đang chạy.");}
  catch(e){setError(String(e))}
  finally{setBusy(false)}
 };
 useEffect(()=>{
  if(!tableRun||!token||!["queued","building","publishing","rolling_back"].includes(tableRun.state))return;
  let live=true;
  const poll=window.setInterval(()=>{
   void transformApi.tableStatus(tableRun.run_id,token).then(r=>{if(live&&r.runs[0])setTableRun(r.runs[0])}).catch(()=>{});
  },3000);
  return()=>{live=false;window.clearInterval(poll)};
 },[tableRun?.run_id,tableRun?.state,token]);
 const stageTable=async()=>{
  if(!savedModelID||unsaved||model.mode!=="visual"){setError("Save a Visual model before building a table");return}
  setTableBusy(true);setError("");setInfo("");
  try{
   const queued=await transformApi.tableStage(savedModelID,token,uniqueKey.trim());
   setTableRun(queued);
   setInfo("Đã xếp hàng dựng staging table. Dữ liệu báo cáo hiện tại chưa bị thay đổi.");
  }catch(e){setError(String(e))}
  finally{setTableBusy(false)}
 };
 const tableAction=async(action:"publish"|"rollback")=>{
  if(!tableRun)return;
  const msg=action==="publish"?"Xác nhận publish bảng đã kiểm tra vào scratch? Bảng đang dùng sẽ được giữ làm bản rollback.":"Xác nhận khôi phục bảng cũ? Chỉ dùng khi bản trước còn nguyên.";
  if(!window.confirm(msg))return;
  setTableBusy(true);setError("");
  try{
   const result=await transformApi.tableAction(tableRun.run_id,action,token);
   setTableRun(result);setInfo(action==="publish"?"Đã xếp hàng Publish; đang kiểm tra UUID và thay bảng an toàn.":"Đã xếp hàng Rollback; đang kiểm tra UUID bản cũ.");
  }catch(e){setError(String(e))}
  finally{setTableBusy(false)}
 };
 const fetchColumns=async()=>{
  if(!token){setError("Nhập API token trước");return}
  setBusy(true);setError("");setInfo("");
  try{const result=await transformApi.columns(model.source,token);setColumns(result.columns);setInfo("Đã nhận "+result.columns.length+" cột từ ClickHouse")}
  catch(e){setError(String(e))}
  finally{setBusy(false)}
 };
 const doPreview=async()=>{
  if(!token){setError("Nhập API token trước");return}
  setBusy(true);setError("");setInfo("");
  try{
   const index=model.mode==="sql"?undefined:selectedIndex;
   const data=await transformApi.preview(model,token,index,50);
   setPreview(data);setInfo("Preview trả về "+data.rows+" dòng. Không ghi dữ liệu.")
  }catch(e){setError(String(e));setPreview(null)}
  finally{setBusy(false)}
 };
 const doSave=async()=>{
  if(!token){setError("Nhập API token trước");return}
  setBusy(true);setError("");setInfo("");
  try{
   const r=await transformApi.save(model,token);
   setInfo("Đã lưu model "+r.name+" (chỉ metadata, chưa tạo bảng)");setSavedModelID(r.id);setUnsaved(false);setTableRun(null);
   setSaved((await transformApi.models(token)).models);
  }catch(e){setError(String(e))}
  finally{setBusy(false)}
 };
 const loadSaved=async()=>{
  if(!token)return;setBusy(true);setError("");
  try{setSaved((await transformApi.models(token)).models)}catch(e){setError(String(e))}finally{setBusy(false)}
 };
 const toggleSQL=(mode:"visual"|"sql")=>{
  if(mode===model.mode)return;
  patchModel({mode,sql:mode==="sql"?(compiled?.sql||model.sql||"SELECT 1"):model.sql});setSelected("source");
 };
 return <div className="space-y-4">
  <section className="section flex flex-wrap items-center justify-between gap-4">
   <div><h2 className="flex items-center gap-2 text-lg font-semibold"><Layers3 size={19}/>Transform Studio <span className="rounded border border-amber-700/50 px-2 py-0.5 text-xs text-amber-300">v1.0 Lineage Preview</span></h2><p className="muted mt-1 text-sm">Giống Power Query: thêm từng bước, kéo đổi thứ tự, xem dữ liệu, hoặc tự viết SQL.</p></div>
   <div className="flex flex-wrap gap-2">
    <Button variant="outline" disabled={busy||!token} onClick={loadSaved}><RefreshCw size={14}/>Models</Button>
    <Button variant="outline" disabled={busy||!token} onClick={doPreview}><Eye size={15}/>Preview bước</Button>
    <Button disabled={busy||!token} onClick={doSave}><Save size={15}/>Lưu Model</Button><Button variant="outline" disabled={busy||!token||unsaved||!savedModelID||model.mode!=="visual"} onClick={executeView}><Database size={15}/>Build View (dbt)</Button><Button variant="outline" disabled={busy||tableBusy||!token||unsaved||!savedModelID||model.mode!=="visual"} onClick={stageTable}><Table2 size={15}/>Build Staging Table</Button>
   </div>
  </section>
  <div className="flex items-start gap-2 rounded-lg border border-amber-800/50 bg-amber-500/10 p-3 text-sm text-amber-200">
   <ShieldAlert className="shrink-0" size={18}/><span>Giai đoạn thử nghiệm: Preview vẫn chỉ SELECT. Có thể Build View hoặc dựng Safe Staging Table trong scratch, nhưng Table chỉ được Publish khi kiểm tra xong và được xác nhận thủ công; chưa bật Incremental, Mart hoặc production. Build View cần lưu model trước.</span>
  </div>
  <section className="section grid gap-3 md:grid-cols-[1fr_1fr_1fr]">
   <TextField label="Tên model" value={model.name} onChange={name=>patchModel({name})} placeholder="revenue_by_part"/>
   <TextField label="ClickHouse database (scratch)" value={model.source.database} onChange={database=>patchModel({source:{...model.source,database}})}/>
   <TextField label="Bảng nguồn" value={model.source.table} onChange={table=>{patchModel({source:{...model.source,table}});setColumns([])}}/>
   <label className="block space-y-1 md:col-span-3">
    <span className="text-xs text-slate-400">Hoặc chọn View đầu ra của model khác để nối Flow (tự tạo dependency)</span>
    <select className="field" value={model.source.table.startsWith("dfv_")?model.source.table:""} onChange={e=>{
      if(!e.target.value)return;
      patchModel({source:{database:"scratch",table:e.target.value}});
      setColumns([]);
      setInfo("Nguồn mới: scratch."+e.target.value+". Lưu Model để cập nhật Lineage. Build upstream trước khi chạy downstream.");
    }}>
     <option value="">— Chọn model đã lưu (bấm Models để tải danh sách) —</option>
     {saved.filter(s=>s.name!==model.name&&s.model.mode==="visual"&&/^[a-z][a-z0-9_]{0,85}$/.test(s.name)).map(s=><option key={s.id} value={"dfv_"+s.name}>{s.name} → scratch.dfv_{s.name}</option>)}
    </select>
   </label>
   <label className="block space-y-1 md:col-span-2"><span className="text-xs text-slate-400">API token · lưu trong bộ nhớ trình duyệt</span><input type="password" autoComplete="off" className="field" value={token} onChange={e=>onTokenChange(e.target.value)} placeholder="Bearer token"/></label>
   <div className="flex items-end"><Button variant="outline" disabled={!token||busy} onClick={fetchColumns}><Database size={15}/>Lấy danh sách cột</Button></div>
   {saved.length>0&&<div className="md:col-span-3 flex flex-wrap gap-2"><span className="muted text-xs self-center">Model đã lưu:</span>{saved.map(entry=><button key={entry.id} className="rounded-md border border-slate-700 bg-slate-800 px-3 py-1.5 text-xs hover:border-sky-500" onClick={()=>{setModel(entry.model);setSelected("source");setPreview(null);setCompiled(null);setColumns([]);setSavedModelID(entry.id);setUnsaved(false);setDbtRun(null);setTableRun(null);setInfo("Đã tải model "+entry.name)}}>{entry.name}</button>)}</div>}
  </section>
  <div className="grid gap-4 xl:grid-cols-[205px_minmax(0,1fr)_335px]">
   <aside className="section space-y-3">
    <h3 className="font-semibold">Thao tác</h3><p className="muted text-xs">Kéo sang Applied Steps hoặc bấm +</p>
    {operationDetails.map(op=><button key={op.kind} draggable={model.mode==="visual"} onDragStart={e=>e.dataTransfer.setData("application/x-dataflow-tool",op.kind)} disabled={model.mode!=="visual"} onClick={()=>addStep(op.kind)} className="flex w-full items-center gap-2 rounded-lg border border-slate-700 bg-slate-800/60 p-2.5 text-left transition hover:border-sky-500 hover:bg-slate-700/50 disabled:opacity-40"><GripVertical size={14} className="text-slate-500"/><div className="min-w-0 flex-1"><div className="text-sm font-medium">{op.name}</div><div className="muted text-xs">{op.hint}</div></div><Plus size={15}/></button>)}
    <div className="border-t border-slate-700 pt-3"><h4 className="mb-2 text-xs font-semibold">Cột nguồn</h4>{columns.length?columns.map(c=><div key={c.name} className="break-all border-b border-slate-800 py-1.5 text-xs">{c.name}<span className="muted ml-1">{c.type}</span></div>):<p className="muted text-xs">Bấm "Lấy danh sách cột" để thấy schema.</p>}</div>
   </aside>
   <section className="section min-w-0 space-y-3">
    <div className="flex flex-wrap items-center justify-between gap-2"><h3 className="flex items-center gap-2 font-semibold"><Table2 size={18}/>Data Preview</h3><div className="flex rounded-lg border border-slate-700 p-1 text-xs"><button className={"rounded px-3 py-1.5 "+(model.mode==="visual"?"bg-sky-500/20 text-sky-300":"muted")} onClick={()=>toggleSQL("visual")}>Visual</button><button className={"rounded px-3 py-1.5 "+(model.mode==="sql"?"bg-sky-500/20 text-sky-300":"muted")} onClick={()=>toggleSQL("sql")}>SQL Code</button></div></div>
    {model.mode==="sql"?<div className="space-y-2"><div className="flex items-center gap-2 text-xs text-slate-400"><Code2 size={15}/>Chế độ code: tự sửa câu SQL SELECT</div><textarea spellCheck={false} className="field min-h-64 font-mono text-xs leading-6" value={model.sql||""} onChange={e=>patchModel({sql:e.target.value})}/></div>:<div className="flex items-center gap-2 rounded-md border border-slate-800 bg-slate-900/70 p-2 text-xs"><Filter size={15}/><span>Đang xem: <b>{selected==="source"?"Source":labels[selectedStep?.type||"filter"]||"Kết quả cuối"}</b></span><span className="muted ml-auto">{active.length} bước đang bật</span></div>}
    {preview?<div className="space-y-3"><div className="flex items-center justify-between text-xs"><span className="text-emerald-400">Read-only preview · {preview.rows} dòng</span><span className="muted">Tối đa 50 dòng / lần</span></div><div className="max-h-[420px] overflow-auto rounded-lg border border-slate-700"><table className="w-full min-w-[560px] whitespace-nowrap text-left text-xs"><thead className="sticky top-0 bg-slate-800">{displayedFields.map(col=><th className="border-b border-slate-700 px-3 py-2" key={col}>{col}</th>)}</thead><tbody>{(preview.data||[]).map((row,i)=><tr key={i} className="border-b border-slate-800">{displayedFields.map(col=><td className="max-w-60 truncate px-3 py-2" key={col} title={String(row[col]??"NULL")}>{row[col]===null?<i className="muted">NULL</i>:typeof row[col]==="object"?JSON.stringify(row[col]):String(row[col]??"")}</td>)}</tr>)}</tbody></table>{(!preview.data||preview.data.length===0)&&<p className="muted p-6 text-center text-sm">Không có dòng dữ liệu; xem schema nếu có.</p>}</div></div>:
    <div className="flex min-h-44 flex-col items-center justify-center gap-2 rounded-lg border border-dashed border-slate-700 bg-slate-900/40 p-6 text-center"><Eye size={26} className="text-slate-500"/><b className="text-sm">Xem dữ liệu sau từng bước</b><p className="muted max-w-sm text-xs">Chọn Source hoặc Applied Step bên phải rồi bấm Preview bước. Dữ liệu không được lưu hay chỉnh sửa.</p><Button variant="outline" disabled={busy||!token} onClick={doPreview}>Chạy Preview</Button></div>}
    <div className="border-t border-slate-700 pt-3"><div className="mb-2 flex items-center gap-2 font-semibold text-sm"><Braces size={16}/>Generated SQL {sqlDirty&&<span className="muted text-xs">(đang cập nhật)</span>}</div><pre className="max-h-72 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-slate-950 p-3 font-mono text-xs leading-5 text-sky-200">{compiled?.sql||"Chọn nguồn hoặc nhập API token để tạo SQL."}</pre>{compileError&&<p role="alert" className="mt-2 text-xs text-rose-300">Lỗi bước biến đổi: {compileError}</p>}<p className="muted mt-2 text-xs">Chuyển sang SQL Code để sửa trực tiếp. SQL code độc lập với Applied Steps.</p></div>
   </section>
   <aside className="section min-w-0 space-y-3">
    <div className="flex items-center justify-between"><h3 className="font-semibold">Applied Steps</h3><span className="muted text-xs">Kéo đổi thứ tự</span></div>
    <button className={"flex w-full items-center gap-2 rounded-md border p-3 text-left text-sm "+(selected==="source"?"border-sky-500 bg-sky-500/10":"border-slate-700 bg-slate-800/40")} onClick={()=>{setSelected("source");setPreview(null)}}><Database size={16}/><b>Source</b><span className="muted ml-auto text-xs">0</span></button>
    <div className="min-h-32 space-y-1.5 rounded-lg border border-dashed border-slate-700 p-1.5" onDragOver={e=>e.preventDefault()} onDrop={e=>dropped(e,model.steps.length)}>
     {model.steps.length===0&&<div className="muted flex min-h-24 items-center justify-center p-3 text-center text-xs">Thả Filter, Select, Group By... vào đây.</div>}
     {model.steps.map((step,i)=><div key={step.id} className={dragged===step.id?"opacity-40":""} onDragOver={e=>e.preventDefault()} onDrop={e=>{e.stopPropagation();dropped(e,i)}}><div draggable={model.mode==="visual"} onDragStart={e=>{setDragged(step.id);e.dataTransfer.setData("application/x-dataflow-step",step.id)}} onDragEnd={()=>setDragged(null)} className={"flex items-center gap-1 rounded-md border p-1.5 "+(selected===step.id?"border-sky-500 bg-sky-500/10":"border-slate-700 bg-slate-800/50")}><GripVertical className="cursor-grab text-slate-500" size={14}/><input aria-label={"Bật tắt "+labels[step.type]} type="checkbox" checked={step.enabled!==false} onChange={e=>patchStep(step.id,{enabled:e.target.checked})}/><button className={"min-w-0 flex-1 truncate px-1 text-left text-xs "+(step.enabled===false?"text-slate-500 line-through":"")} onClick={()=>{setSelected(step.id);setPreview(null)}}><span className="muted mr-1">{i+1}.</span>{labels[step.type]}</button><button title="Lên" aria-label="Move step up" disabled={i===0} onClick={()=>shift(step.id,-1)}><ArrowUp size={13}/></button><button title="Xuống" aria-label="Move step down" disabled={i===model.steps.length-1} onClick={()=>shift(step.id,1)}><ArrowDown size={13}/></button><button title="Xóa bước" aria-label="Delete step" className="text-rose-300" onClick={()=>{patchModel({steps:model.steps.filter(s=>s.id!==step.id)});setSelected("source")}}><Trash2 size={13}/></button></div></div>)}
    </div>
    {selectedStep&&model.mode==="visual"&&<div className="space-y-3 border-t border-slate-700 pt-3"><div className="flex items-center gap-2 text-sm font-semibold"><ChevronDown size={16}/>{labels[selectedStep.type]}</div><StepFields step={selectedStep} columns={columns} onChange={patch=>patchStep(selectedStep.id,patch)}/></div>}
    <div className="border-t border-slate-700 pt-3 text-xs"><div className="flex items-center gap-2 text-emerald-300"><CheckCircle2 size={15}/>Step changes are non-destructive</div><p className="muted mt-2">Preview chỉ đọc. Save chỉ lưu công thức. dbt materialization sẽ là giai đoạn tiếp theo sau khi kiểm chứng.</p><button className="muted mt-3 flex items-center gap-1 underline" onClick={()=>{setModel(defaultModel);setSavedModelID("");setUnsaved(true);setDbtRun(null);setTableRun(null);setUniqueKey("");setColumns([]);setSelected("source");setCompiled(null);setPreview(null);setError("");setInfo("")}}><Undo2 size={13}/>Reset Studio</button></div>
   </aside>
  </div>
  {error&&<div role="alert" className="rounded-md border border-red-700 bg-red-950/50 p-3 text-sm text-red-300">{error}</div>}
  <section className="section flex flex-wrap items-end gap-3">
   <label className="block min-w-48 flex-1 space-y-1"><span className="text-xs text-slate-400">Safe Table v1.1 · Khóa duy nhất (tùy chọn, để phát hiện trùng)</span><input className="field" value={uniqueKey} onChange={e=>setUniqueKey(e.target.value)} placeholder="Ví dụ: InvoiceNum"/></label>
   <div className="max-w-md text-xs text-slate-400">Build staging → kiểm tra rows + optional unique key → Publish thủ công. Nếu gặp lỗi trước Publish, bảng đang dùng không thay đổi. TEST scratch only.</div>
  </section>
  {tableRun&&<section className="section space-y-3 text-sm">
   <div className="flex items-center justify-between"><b>Safe Table Build</b><span className="rounded border border-slate-600 px-2 py-1 font-mono text-xs">{tableRun.state}</span></div>
   <div className="text-xs text-slate-400">Run {tableRun.run_id} · Stage {tableRun.stage} · Target {tableRun.target}</div>
   {tableRun.rows!==undefined&&<div className="text-xs">Data Quality: {tableRun.rows.toLocaleString()} rows verified {tableRun.has_backup?"· Backup available":""}</div>}
   {tableRun.error&&<div role="alert" className="text-rose-300">{tableRun.error}</div>}
   {tableRun.log_tail&&<pre className="max-h-36 overflow-auto whitespace-pre-wrap bg-slate-950 p-2 text-xs">{tableRun.log_tail}</pre>}
   <div className="flex gap-2 flex-wrap">
    <Button disabled={tableBusy||tableRun.state!=="ready"} onClick={()=>void tableAction("publish")}>Approve & Publish</Button>
    <Button variant="outline" disabled={tableBusy||tableRun.state!=="published"||!tableRun.has_backup} onClick={()=>void tableAction("rollback")}>Rollback previous table</Button>
   </div>
   <p className="text-xs text-amber-200">Chỉ áp dụng cho bản TEST; chưa xác nhận an toàn với dashboard production hoặc Epicor.</p>
  </section>}
  {dbtRun&&<div className="rounded-lg border border-slate-700 bg-slate-900/70 p-3 text-sm">
  <div className="flex flex-wrap items-center justify-between gap-2"><b>dbt View materialization</b><span className="rounded border border-slate-600 px-2 py-1 font-mono text-xs">{dbtRun.status}</span></div>
  <p className="muted mt-2 text-xs">Run: {dbtRun.run_id} · Destination: {dbtRun.target||("scratch.dfv_"+model.name)}</p>
  {dbtRun.error&&<p className="mt-2 text-xs text-rose-300">{dbtRun.error}</p>}
  {dbtRun.log_tail&&<pre className="mt-2 max-h-40 overflow-auto whitespace-pre-wrap break-all rounded bg-slate-950 p-3 text-xs">{dbtRun.log_tail}</pre>}
 </div>}
 {info&&<div role="status" className="rounded-md border border-emerald-800 bg-emerald-950/40 p-3 text-sm text-emerald-300">{info}</div>}
 </div>;
}
