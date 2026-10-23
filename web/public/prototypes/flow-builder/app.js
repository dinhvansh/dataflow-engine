/* DataFlow guided UX v2: two genuinely separate, browser-only workflows.
   No fetch/XMLHttpRequest, API credentials, SQL writes or production routes. */
(function(){
"use strict";
var fixtures=[
 {id:"orders",name:"Erp.CustomOrders",label:"Sales Orders",key:"OrderID",cursor:"RowChanged",
  cols:[["OrderID","bigint"],["Customer","nvarchar"],["Region","nvarchar"],["Amount","decimal"],["Status","nvarchar"],["RowChanged","rowversion"]],
  rows:[
   {OrderID:1001,Customer:"Andros Vietnam",Region:"South",Amount:1250000,Status:"Active",RowChanged:"0x000001"},
   {OrderID:1002,Customer:"Alpha Retail",Region:"North",Amount:895000,Status:"Active",RowChanged:"0x000002"},
   {OrderID:1003,Customer:"Minh Phu Foods",Region:"South",Amount:740000,Status:"Cancelled",RowChanged:"0x000003"},
   {OrderID:1004,Customer:"Delta Market",Region:"Central",Amount:2460000,Status:"Active",RowChanged:"0x000004"},
   {OrderID:1005,Customer:"Star Wholesale",Region:"North",Amount:375000,Status:"Active",RowChanged:"0x000005"},
   {OrderID:1006,Customer:"Lotus Food",Region:"South",Amount:132000,Status:"Pending",RowChanged:"0x000006"}
  ]},
 {id:"part",name:"Erp.Part",label:"Product Master",key:"PartNum",cursor:"SysRevID",
  cols:[["PartNum","nvarchar"],["PartDescription","nvarchar"],["ProdCode","nvarchar"],["UnitPrice","decimal"],["Inactive","bit"],["SysRevID","rowversion"]],
  rows:[
   {PartNum:"A-001",PartDescription:"Mango Puree",ProdCode:"FRUIT",UnitPrice:120000,Inactive:0,SysRevID:"0x000001"},
   {PartNum:"A-002",PartDescription:"Strawberry Jam",ProdCode:"JAM",UnitPrice:86000,Inactive:0,SysRevID:"0x000002"},
   {PartNum:"B-010",PartDescription:"Apricot Filling",ProdCode:"BAKE",UnitPrice:103000,Inactive:0,SysRevID:"0x000003"},
   {PartNum:"B-015",PartDescription:"Frozen Banana",ProdCode:"FRUIT",UnitPrice:59000,Inactive:1,SysRevID:"0x000004"}
  ]},
 {id:"snapshot",name:"Erp.CustomSnapshot",label:"Snapshot only",key:"RecordID",cursor:null,
  cols:[["RecordID","bigint"],["Department","nvarchar"],["Owner","nvarchar"],["Cost","decimal"]],
  rows:[{RecordID:21,Department:"Finance",Owner:"Mai",Cost:875000},{RecordID:22,Department:"Sales",Owner:"Khoa",Cost:1320000},{RecordID:23,Department:"IT",Owner:"Van",Cost:645000}]}
];
var kindNames={filter:"Lọc hàng",select:"Chọn cột",rename:"Đổi tên",calculate:"Tính toán",aggregate:"Nhóm & tổng hợp",sort:"Sắp xếp",limit:"Giới hạn"};
var state={
 workspace:"sync",theme:"light",syncStep:0,transformStep:0,
 sync:{name:"SQL Server orders → RAW",source:"orders",columns:fixtures[0].cols.map(function(x){return x[0];}),mode:"full_then_incremental",database:"scratch",rawTable:"raw_orders",run:null},
 rawRegistry:[],
 transform:{name:"Revenue analytics",rawId:"",ops:[],database:"scratch",martTable:"mart_revenue",mapping:{},run:null,showJSON:false}
};
var timer=null,nextOpId=0;
try{if(localStorage.getItem("df-ux-theme")==="dark")state.theme="dark";}catch(_){}
var $=function(id){return document.getElementById(id);};
function E(s){return String(s===undefined||s===null?"":s).replace(/[&<>"']/g,function(c){return {"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c];});}
function note(s){var n=document.createElement("div");n.className="toast";n.textContent=s;$("toast-stack").appendChild(n);setTimeout(function(){n.remove();},3000);}
function source(){return fixtures.find(function(x){return x.id===state.sync.source;})||fixtures[0];}
function raw(){return state.rawRegistry.find(function(x){return x.id===state.transform.rawId;})||null;}
function columns(rows){return rows.length?Object.keys(rows[0]):[];}
function validID(v){return /^[A-Za-z_][A-Za-z0-9_]{0,63}$/.test(v);}
function copyRows(rows){return rows.map(function(x){return Object.assign({},x);});}
function syncRows(){var s=state.sync;return source().rows.map(function(r){var out={};s.columns.forEach(function(k){out[k]=r[k];});return out;});}
function syncErrors(){
 var s=state.sync,t=source(),errors=[];
 if(!s.name.trim())errors.push("Nhập tên Sync pipeline.");
 if(s.columns.indexOf(t.key)<0)errors.push("Phải giữ cột unique key: "+t.key);
 if(s.mode!=="full_refresh"&&(!t.cursor||s.columns.indexOf(t.cursor)<0))errors.push("Incremental yêu cầu RowVersion hợp lệ.");
 if(s.database!=="scratch"||!validID(s.rawTable))errors.push("RAW target chỉ được ở scratch và tên bảng hợp lệ.");
 return errors;
}
function transformRows(){
 var p=raw();if(!p)return [];
 var rows=copyRows(p.rows);
 state.transform.ops.forEach(function(op){if(!op.enabled)return;
   var col=op.column;
   if(op.kind==="filter")rows=rows.filter(function(r){var v=r[col];if(op.condition==="greater")return Number(v)>Number(op.value);if(op.condition==="contains")return String(v||"").toLowerCase().includes(String(op.value).toLowerCase());return String(v)===String(op.value);});
   if(op.kind==="rename")rows=rows.map(function(r){var x=Object.assign({},r);if(col in x&&op.alias&&op.alias!==col){x[op.alias]=x[col];delete x[col];}return x;});
   if(op.kind==="calculate")rows=rows.map(function(r){var x=Object.assign({},r);if(op.alias)x[op.alias]=Number((Number(r[col]||0)*Number(op.value||0)).toFixed(2));return x;});
   if(op.kind==="sort")rows=rows.slice().sort(function(a,b){var x=a[col],y=b[col],v=typeof x==="number"&&typeof y==="number"?x-y:String(x).localeCompare(String(y));return op.direction==="asc"?v:-v;});
   if(op.kind==="limit")rows=rows.slice(0,Math.max(1,Math.min(999,Number(op.value)||1)));
   if(op.kind==="select"){var keep=String(op.columns||"").split(",").map(function(v){return v.trim();}).filter(Boolean);rows=rows.map(function(r){var x={};keep.forEach(function(k){if(k in r)x[k]=r[k];});return x;});}
   if(op.kind==="aggregate"){var groups={};rows.forEach(function(r){var k=String(r[col]);if(!groups[k])groups[k]={group:r[col],sum:0,count:0};groups[k].sum+=Number(r[op.measure]||0);groups[k].count++;});rows=Object.keys(groups).map(function(k){var x={};x[col]=groups[k].group;x[op.alias||"Total"]=Number(groups[k].sum.toFixed(2));x.RowCount=groups[k].count;return x;});}
 });
 return rows;
}
function mapped(){var m={},cols=columns(transformRows());cols.forEach(function(c){m[c]=state.transform.mapping[c]===undefined?c:state.transform.mapping[c];});return m;}
function transformErrors(){var p=raw(),s=state.transform,e=[];if(!p)e.push("Chưa chọn bảng RAW đã Sync thành công.");
 if(!s.name.trim())e.push("Nhập tên Transform pipeline.");
 if(s.database!=="scratch"||!validID(s.martTable))e.push("MART demo chỉ được ở scratch và tên bảng hợp lệ.");
 var names=Object.values(mapped());if(!names.every(validID)||new Set(names.map(function(n){return n.toLowerCase();})).size!==names.length)e.push("Mapping cột trùng hoặc tên không hợp lệ.");
 if(p&&columns(p.rows).length===0)e.push("RAW không có cột dữ liệu.");

 return e;
}
function sel(values,current,attr){return '<select '+(attr||"")+'>'+values.map(function(v){var x=Array.isArray(v)?v[0]:v,y=Array.isArray(v)?v[1]:v;return '<option value="'+E(x)+'"'+(x===current?' selected':'')+'>'+E(y)+'</option>';}).join("")+'</select>';}
function field(label,html,hint){return '<label class="field-label">'+E(label)+(hint?'<small>'+E(hint)+'</small>':'')+html+'</label>';}
function header(n,title,desc,badge){return '<div class="panel-header"><div><div class="section-eyebrow">'+(state.workspace==="sync"?"SYNC WORKFLOW":"TRANSFORM WORKFLOW")+' · STEP '+n+'</div><h2>'+title+'</h2><p>'+desc+'</p></div><span class="status-chip '+(badge==="ready"?"ok":"")+'">'+(badge||"Bản demo")+'</span></div>';}
function section(title,note){return '<div class="section-title"><span>'+title+'</span>'+(note?'<span class="muted-copy">'+note+'</span>':'')+'</div>';}
function grid(rows){var ks=columns(rows);if(!ks.length)return '<div class="empty-transform">Không có dữ liệu hiển thị sau bước xử lý.</div>';return '<div class="grid-scroll"><table class="data-table"><thead><tr>'+ks.map(function(k){return '<th>'+E(k)+'</th>';}).join("")+'</tr></thead><tbody>'+rows.slice(0,8).map(function(r){return '<tr>'+ks.map(function(k){var v=r[k];return '<td'+(typeof v==="number"?' class="mono"':'')+'>'+E(typeof v==="number"?v.toLocaleString("en-US"):v)+'</td>';}).join("")+'</tr>';}).join("")+'</tbody></table></div>';}
function choices(){
 var t=source(),mode=state.sync.mode;return [
 ["full_refresh","Full Refresh","Lấy toàn bộ dữ liệu rồi thay thế bảng RAW được quản lý."],
 ["full_then_incremental","Full một lần → Incremental","Lần đầu đọc toàn bộ; lần sau lấy phần thay đổi theo RowVersion."],
 ["incremental_rowversion","Incremental","Lấy phần thay đổi dựa trên checkpoint hiện có."]
 ].map(function(v){var disabled=!t.cursor&&v[0]!=="full_refresh";return '<label class="choice-row '+(mode===v[0]?'selected':'')+'" style="'+(disabled?'opacity:.48':'')+'"><input name="sync-mode" type="radio" value="'+v[0]+'" '+(mode===v[0]?'checked':'')+(disabled?' disabled':'')+'><span><strong>'+v[1]+'</strong><small>'+v[2]+'</small></span></label>';}).join("");
}
function syncSource(){
 var s=state.sync,t=source();return header(1,"Nguồn dữ liệu · SQL Server","Chỉ chọn nguồn, bảng và cột để đồng bộ. Không có Transform trong quy trình này.","SELECT-only")+
 '<div class="form-grid">'+field("Connection",sel([["qa_sql","SQL Server · DataFlowFixtureQA"]],"qa_sql","disabled"),"Kết nối mock đã lưu")+
 field("Tên Sync pipeline",'<input type="text" data-sync-field="name" value="'+E(s.name)+'">')+'</div>'+
 section("01 · Bảng dữ liệu nguồn", "Discovery · mẫu")+
 '<div class="table-picker">'+fixtures.map(function(x){return '<button type="button" class="table-option '+(x.id===s.source?'active':'')+'" data-action="source" data-value="'+E(x.id)+'"><span class="table-symbol">▤</span><strong>'+E(x.label)+'</strong><small>'+E(x.name)+'</small><small>Key: '+E(x.key)+'</small></button>';}).join("")+'</div>'+
 '<div class="key-info spaced-top"><span class="status-chip ok">Unique key: '+E(t.key)+'</span> '+(t.cursor?'<span class="status-chip ok">RowVersion: '+E(t.cursor)+'</span>':'<span class="status-chip warn">Chỉ hỗ trợ Full Refresh</span>')+'<br>Source read-only. ERP sẽ không bị INSERT/UPDATE/DELETE.</div>'+
 section("02 · Chọn cột",s.columns.length+" cột")+
 '<div class="grid-scroll"><table class="data-table"><thead><tr><th>Chọn</th><th>Tên cột</th><th>Kiểu dữ liệu</th><th>Vai trò</th></tr></thead><tbody>'+t.cols.map(function(c){var required=c[0]===t.key||(s.mode!=="full_refresh"&&c[0]===t.cursor);return '<tr><td><input aria-label="Chọn cột '+E(c[0])+'" data-sync-col="'+E(c[0])+'" type="checkbox" '+(s.columns.includes(c[0])?'checked':'')+(required?' disabled':'')+'></td><td class="mono">'+E(c[0])+'</td><td>'+E(c[1])+'</td><td>'+(c[0]===t.key?'<span class="status-chip ok">KEY</span>':c[0]===t.cursor?'<span class="status-chip">CURSOR</span>':'Data')+'</td></tr>';}).join("")+'</tbody></table></div>'+
 section("03 · Chế độ đồng bộ")+
 choices()+
 section("Preview · đọc từ Source")+
 grid(syncRows());
}
function syncDestination(){
 var s=state.sync;return header(2,"ClickHouse RAW · đích đồng bộ","Chọn nơi lưu dữ liệu gốc. Transform sẽ đọc từ RAW sau khi Sync xong.","No transform")+
 '<div class="form-grid">'+field("Destination connection",sel([["ch","ClickHouse · Isolated QA"]],"ch","disabled"))+
 field("Database",'<input type="text" value="'+E(s.database)+'" data-sync-field="database">',"Prototype chỉ dùng scratch")+
 field("Tên RAW table",'<input type="text" maxlength="64" value="'+E(s.rawTable)+'" data-sync-field="rawTable">')+
 field("Write mode",sel([["raw","Write raw source columns"]],"raw","disabled"))+'</div>'+
 '<div class="destination-summary spaced-top">Đồng bộ <b>'+E(source().name)+'</b> → <code>'+E(s.database)+'.'+E(s.rawTable)+'</code><br><b>'+syncRows().length+' hàng mẫu</b>, '+s.columns.length+' cột. Dữ liệu ở bước này chưa Filter, JOIN hoặc tổng hợp.</div>'+
 section("RAW data preview", "Không thay đổi dữ liệu")+
 grid(syncRows())+
 '<div class="success-inline spaced-top">✓ Đây là ranh giới giữa hai quy trình: Sync xong sẽ tạo một bảng RAW mẫu, sau đó m có thể mở Transform Pipeline riêng để xử lý.</div>';
}
function summary(icon,title,desc){return '<div class="review-block"><span class="review-symbol">'+icon+'</span><div><strong>'+title+'</strong><p>'+desc+'</p></div></div>';}
function statusStages(run,labels){
 return '<div class="run-progress">'+labels.map(function(title,i){var done=run&&(run.done||run.index>i),running=run&&!run.done&&run.index===i;return '<div class="run-stage '+(done?'done':running?'running':'')+'"><strong>'+(done?"✓ ":running?"⟳ ":"○ ")+title+'</strong><small>'+(done?"Demo PASS":running?"Simulating":"Not started")+'</small></div>';}).join("")+'</div>';
}
function syncReview(){
 var s=state.sync,errors=syncErrors(),run=s.run;
 return header(3,"Kiểm tra và chạy Sync","Chỉ chạy Extract → RAW. Không thực hiện Transform, dbt hay Publish MART.",errors.length?"Cần sửa":"ready")+
 summary("▤","Nguồn: "+E(source().name),s.columns.length+" cột · Key "+E(source().key)+" · Mode "+E(s.mode))+
 summary("▣","Đích RAW: "+E(s.database)+"."+E(s.rawTable),syncRows().length+" hàng mẫu · giữ nguyên dữ liệu chưa biến đổi")+
 (errors.length?'<div class="warning-inline">⚠ '+errors.map(E).join("<br>⚠ ")+'</div>':'<div class="success-inline">✓ Sync đã sẵn sàng mô phỏng, không phụ thuộc Transform.<br>✓ SQL Server SELECT-only; không gọi backend.</div>')+
 section("Quá trình đồng bộ",run&&run.done?"Hoàn thành · demo":"Independent job")+
 statusStages(run,["Extract","Load RAW","Verify & Checkpoint"])+
 (run&&run.done?'<div class="success-inline"><b>✓ Đã Sync thành công (mô phỏng).</b> RAW <code>'+E(s.database)+'.'+E(s.rawTable)+'</code> đã xuất hiện trong danh sách đầu vào của Transform Pipeline.</div>':
 '<div class="warning-inline">Mô phỏng trong trình duyệt, không tạo checkpoint hay ClickHouse table thật.</div>')+
 '<div class="flex-gap spaced-top"><button type="button" class="btn btn-primary" data-action="run-sync" '+(errors.length||run&&!run.done?'disabled':'')+'>▷ '+(run&&run.done?"Sync lại (demo)":"Chạy thử Sync")+'</button><button type="button" class="btn btn-outline" data-action="workspace" data-value="transform" '+(!s.run||!s.run.done?'disabled':'')+'>Đến Transform riêng →</button><button type="button" class="btn btn-outline" data-action="export">Xuất Sync JSON ⇩</button></div>';
}
function transformInput(){
 var reg=state.rawRegistry,p=raw(),s=state.transform;
 return header(1,"Chọn bảng RAW đã có","Transform không kết nối trực tiếp tới ERP; chỉ đọc bảng RAW mà Sync đã lưu.","RAW input")+
 '<div class="warning-inline">Hai pipeline độc lập. Transform có thể tạo lúc khác, chạy theo lịch riêng hoặc được kích hoạt sau khi Sync thành công.</div>'+
 section("Bảng RAW đang có",reg.length+" bảng đã Sync (demo)")+
 (reg.length?'<div class="table-picker">'+reg.map(function(x){return '<button type="button" class="table-option '+(s.rawId===x.id?'active':'')+'" data-action="raw" data-value="'+E(x.id)+'"><span class="table-symbol">▣</span><strong>'+E(x.id)+'</strong><small>'+E(x.rows.length)+' rows · '+E(x.sourceName)+'</small><small>Synced: '+E(x.mode)+'</small></button>';}).join("")+'</div>':
 '<div class="empty-transform"><strong>Chưa có bảng RAW nào.</strong>Chạy Sync Pipeline trước để tạo dữ liệu nguồn cho Transform trong bản demo.<div class="spaced-top"><button type="button" class="btn btn-primary" data-action="workspace" data-value="sync">← Đi tới Sync Pipeline</button></div></div>')+
 (p?'<div class="success-inline spaced-top">✓ Chọn <b>'+E(p.id)+'</b>. RAW này giữ nguyên sau khi m chỉnh hoặc chạy lại Transform.</div>'+section("Dữ liệu RAW đầu vào","Snapshot đã Sync")+grid(p.rows):"")+
 section("Tên Transform pipeline")+
 field("Tên model / pipeline",'<input type="text" value="'+E(s.name)+'" data-transform-field="name">');
}
function opFactory(kind){
 var cs=columns(transformRows()),nums=cs.filter(function(x){return /amount|price|cost|total/i.test(x);}),n=nums[0]||cs[0]||"Amount";
 return {id:"step_"+(++nextOpId),kind:kind,enabled:true,column:kind==="filter"?(cs.includes("Status")?"Status":cs[0]):kind==="aggregate"?(cs.includes("Region")?"Region":cs[0]):n,value:kind==="filter"?(cs.includes("Status")?"Active":""):(kind==="limit"?"5":"1.1"),alias:kind==="rename"?"RenamedColumn":kind==="aggregate"?"TotalAmount":"AdjustedAmount",condition:"equals",direction:"desc",measure:n,columns:cs.join(", ")};
}
function opForm(op,idx){
 var cs=columns(raw()?raw().rows:[]),options=cs.map(function(c){return [c,c];}),f=function(name,label,markup){return field(label,markup);};
 var dropdown=function(name,label,choices,value){return f(name,label,sel(choices,value,'data-op-id="'+E(op.id)+'" data-op-field="'+name+'"'));};
 var inp=function(name,label,value,type){return f(name,label,'<input type="'+(type||"text")+'" data-op-id="'+E(op.id)+'" data-op-field="'+name+'" value="'+E(value)+'">');};
 if(op.kind==="filter")return dropdown("column","Cột",options,op.column)+dropdown("condition","Điều kiện",[["equals","Bằng"],["contains","Chứa"],["greater","Lớn hơn"]],op.condition)+inp("value","Giá trị",op.value);
 if(op.kind==="rename")return dropdown("column","Cột nguồn",options,op.column)+inp("alias","Tên mới",op.alias);
 if(op.kind==="calculate")return dropdown("column","Cột số",options,op.column)+inp("value","Nhân hệ số",op.value,"number")+inp("alias","Cột kết quả",op.alias);
 if(op.kind==="aggregate")return dropdown("column","Nhóm theo",options,op.column)+dropdown("measure","Cột tính tổng",options,op.measure)+inp("alias","Tên cột tổng",op.alias);
 if(op.kind==="sort")return dropdown("column","Cột sắp xếp",options,op.column)+dropdown("direction","Thứ tự",[["asc","Tăng dần"],["desc","Giảm dần"]],op.direction);
 if(op.kind==="limit")return inp("value","Số dòng",op.value,"number");
 if(op.kind==="select")return inp("columns","Giữ cột (cách nhau dấu phẩy)",op.columns);
 return "";
}
function transformDesign(){
 var p=raw(),ops=state.transform.ops,rows=transformRows();
 return header(2,"Transform · thiết kế mô hình","Thao tác độc lập trên dữ liệu đã được Sync vào ClickHouse RAW.","Independent model")+
 '<div class="destination-summary"><b>INPUT:</b> '+E(p?p.id:"Chưa chọn RAW")+'<br><b>OUTPUT:</b> '+rows.length+' hàng, '+columns(rows).length+' cột mẫu. Không đọc lại ERP.</div>'+
 '<div class="metrics"><div class="metric"><small>RAW input</small><strong>'+(p?p.rows.length:0)+'</strong><small>rows</small></div><div class="metric"><small>Transform output</small><strong>'+rows.length+'</strong><small>rows</small></div><div class="metric"><small>Operations</small><strong>'+ops.filter(function(op){return op.enabled;}).length+'</strong><small>enabled</small></div></div>'+
 section("Thêm bước biến đổi","Live preview · Browser demo")+
 '<div class="op-gallery">'+Object.keys(kindNames).map(function(k){return '<button class="op-card" type="button" data-action="op-add" data-value="'+k+'" '+(!p?'disabled':'')+'><span class="op-icon">'+({filter:"≡",select:"▦",rename:"Aa",calculate:"∑",aggregate:"◫",sort:"⇅",limit:"№"}[k])+'</span><strong>'+kindNames[k]+'</strong><small>+ Thêm</small></button>';}).join("")+'</div>'+
 section("Các bước Transform",ops.length+" bước")+
 (ops.length?ops.map(function(op,i){return '<div class="transform-row '+(!op.enabled?'disabled':'')+'"><span class="op-number">'+(i+1)+'</span><div><div class="op-head"><strong>'+E(kindNames[op.kind])+'</strong><label class="muted-copy"><input type="checkbox" data-op-enabled="'+E(op.id)+'" '+(op.enabled?'checked':'')+'> Bật</label></div><div class="op-config">'+opForm(op,i)+'</div></div><div class="op-buttons"><button type="button" title="Lên" data-action="op-up" data-value="'+E(op.id)+'" '+(!i?'disabled':'')+'>↑</button><button type="button" title="Xuống" data-action="op-down" data-value="'+E(op.id)+'" '+(i===ops.length-1?'disabled':'')+'>↓</button><button type="button" title="Xóa" data-action="op-remove" data-value="'+E(op.id)+'">×</button></div></div>';}).join(""):'<div class="empty-transform"><strong>Chưa thêm Transform nào</strong>RAW sẽ được giữ nguyên nếu m không thêm thao tác.</div>')+
 section("Preview kết quả Transform",rows.length+" rows (synthetic)")+
 grid(rows)+
 (ops.some(function(op){return op.enabled&&op.kind==="aggregate";})?'<div class="warning-inline spaced-top">Demo cho phép preview Group By; runtime thật phải định nghĩa lại grain/unique key và DQ trước khi publish.</div>':'')+
 '<div class="key-info spaced-top">Khi triển khai thật, các bước xử lý được thực thi bởi dbt/ClickHouse sau khi Sync hoàn thành, không trong vòng lặp extract.</div>';
}
function transformDestination(){
 var s=state.transform,rows=transformRows(),map=mapped();
 return header(3,"Lưu kết quả · MART","Đây là bảng kết quả sau Transform, không phải bảng RAW nguồn.","Output model")+
 '<div class="form-grid">'+field("Output connection",sel([["clickhouse","ClickHouse · QA"]],"clickhouse","disabled"))+
 field("Database",'<input type="text" value="'+E(s.database)+'" data-transform-field="database">',"Prototype giới hạn scratch")+
 field("Table MART",'<input type="text" maxlength="64" data-transform-field="martTable" value="'+E(s.martTable)+'">')+
 field("Publish policy",sel([["verified","Verify quality → Publish"]],"verified","disabled"))+'</div>'+
 '<div class="destination-summary spaced-top"><b>RAW input:</b> '+E(s.rawId||"None")+'<br><b>MART output:</b> <code>'+E(s.database)+'.'+E(s.martTable)+'</code><br>Transform và MART có run/status độc lập với Sync.</div>'+
 section("Mapping kết quả → ClickHouse MART",Object.keys(map).length+" cột")+
 '<div class="grid-scroll"><table class="data-table mapping-table"><thead><tr><th>Cột sau Transform</th><th>→</th><th>Cột tại MART</th></tr></thead><tbody>'+
 Object.keys(map).map(function(k){return '<tr><td class="mono">'+E(k)+'</td><td>→</td><td><input type="text" aria-label="MART column '+E(k)+'" data-map="'+E(k)+'" value="'+E(map[k])+'"></td></tr>';}).join("")+'</tbody></table></div>'+
 section("Preview dữ liệu xuất bản",rows.length+" rows")+
 grid(rows.map(function(row){var r={};Object.keys(map).forEach(function(k){r[map[k]]=row[k];});return r;}))+
 '<div class="key-info spaced-top">Chạy thật phải kiểm tra Quality, quyền ghi, bảng thuộc DataFlow và kiểm soát publish. Prototype này chỉ hiển thị ý tưởng, không gọi API.</div>';
}
function transformReview(){
 var t=state.transform,p=raw(),e=transformErrors(),run=t.run;
 return header(4,"Kiểm tra & chạy Transform","Bắt đầu công việc độc lập từ RAW → dbt → Quality → MART, không Sync lại.",e.length?"Cần lưu ý":"ready")+
 summary("▣","Đọc RAW: "+E(p?p.id:"Chưa chọn"),"Lấy dữ liệu đã có trong ClickHouse, không query ERP")+
 summary("⧉","Transform: "+t.ops.filter(function(op){return op.enabled;}).length+" bước",t.ops.filter(function(op){return op.enabled;}).map(function(op){return E(kindNames[op.kind]);}).join(" → ")||"Không biến đổi")+
 summary("▤","Xuất bản MART: "+E(t.database)+"."+E(t.martTable),"Kiểm tra Quality trước khi publish · "+transformRows().length+" hàng mẫu")+
 (e.length?'<div class="warning-inline">⚠ '+e.map(E).join("<br>⚠ ")+'</div>':'<div class="success-inline">✓ Transform chạy độc lập. Sync không chạy lại.<br>✓ Đây vẫn là bản demo, không ghi lên database thật.</div>')+
 section("Transform run status",run&&run.done?"Đã mô phỏng xong":"Independent run")+
 statusStages(run,["Read RAW","Execute models","Quality check","Publish MART"])+
 (run&&run.done?'<div class="success-inline"><b>✓ Đã mô phỏng Transform thành công.</b> Kết quả '+transformRows().length+' hàng vào '+E(t.database)+'.'+E(t.martTable)+'. Không có dữ liệu thật nào bị ghi.</div>':
 '<div class="warning-inline">Không tích hợp trực tiếp dbt API hoặc Create/Publish backend ở prototype này.</div>')+
 '<div class="flex-gap spaced-top"><button type="button" class="btn btn-primary" data-action="run-transform" '+(e.length||run&&!run.done?'disabled':'')+'>▷ '+(run&&run.done?"Chạy lại Transform (demo)":"Mô phỏng Transform")+'</button><button class="btn btn-outline" type="button" data-action="export">Xuất Transform JSON ⇩</button></div>';
}
function drawStepper(){
 var sync=state.workspace==="sync",step=sync?state.syncStep:state.transformStep;
 var list=sync?[["Source","Bảng & cột"],["RAW Destination","ClickHouse RAW"],["Review & Sync","Extract → Load"]]:
 [["Choose RAW","Bảng đã đồng bộ"],["Transform","Visual model"],["MART Destination","Bảng kết quả"],["Review & Run","dbt → Publish"]];
 $("stepper").innerHTML=list.map(function(x,i){return '<button type="button" class="step '+(i===step?'active':i<step?'done':'')+'" data-action="step" data-value="'+i+'"><span class="step-num">'+(i<step?'✓':i+1)+'</span><span><strong>'+x[0]+'</strong><small>'+x[1]+'</small></span></button>';}).join("");
}
function drawDiagram(){
 var sync=state.workspace==="sync",p=raw();
 var nodes=sync?[["▤","SQL SERVER SOURCE",source().name,"SELECT-only · "+state.sync.columns.length+" cols"],["▣","CLICKHOUSE RAW",state.sync.database+"."+state.sync.rawTable,"Chỉ dữ liệu nguồn · không Transform"]]:
 [["▣","CLICKHOUSE RAW",p?p.id:"Chưa chọn bảng RAW","Đầu vào đã Sync"],["⧉","TRANSFORM / dbt",state.transform.ops.length+" thao tác","Đọc RAW · SQL trên ClickHouse"],["▣","CLICKHOUSE MART",state.transform.database+"."+state.transform.martTable,"Quality → Publish"]];
 var lane=$("flow-diagram");lane.classList.toggle("two-nodes",sync);
 lane.innerHTML=nodes.map(function(n,i){return (i?'<div class="flow-arrow">⟶</div>':'')+'<div class="flow-node '+(sync||p?'has-data':'')+'"><span class="node-icon">'+n[0]+'</span><div style="min-width:0"><div class="tag">'+n[1]+'</div><strong title="'+E(n[2])+'">'+E(n[2])+'</strong><small>'+E(n[3])+'</small></div></div>';}).join("");
 var run=sync?state.sync.run:state.transform.run;$("flow-state").textContent=run&&run.done?"SIMULATED PASS":run?"SIMULATING":"NOT STARTED · "+(sync?"SYNC":"TRANSFORM");
}
function help(){
 var sync=state.workspace==="sync",st=sync?state.syncStep:state.transformStep;
 var content=sync?[
 ["Source phải đơn giản","Chọn SQL Server, table, columns, full/delta. Không cấu hình JOIN/Filter tại đây.","Cột key + cursor quan trọng cho incremental."],
 ["RAW là vùng dữ liệu gốc","RAW có nhiệm vụ nhận dữ liệu chưa biến đổi. Một bảng RAW có thể được nhiều model đọc lại.","Sync retry/checkpoint không liên quan lịch chạy Transform."],
 ["Chạy Sync một mình","Sync chỉ Extract → Load → Verify. Sau khi mô phỏng chạy xong, bảng RAW được thêm vào danh sách cho Transform.","Đây không phải run trên backend thật."]
 ][st]:[
 ["Transform đọc từ RAW","RAW phải tồn tại trước khi model có thể đọc. Dùng bảng đã được Sync mà không phải cấu hình SQL Server lần nữa.","M có thể chạy nhiều Transform model khác nhau trên cùng một RAW."],
 ["Transform là bước riêng","Filter, Select, Rename, Calculate, Aggregate, Sort và Limit được preview bằng JavaScript trong demo.","Các thao tác thật do dbt/ClickHouse đảm nhiệm; không chạy song song lúc Sync."],
 ["Bảng MART khác RAW","MART chỉ chứa kết quả mô hình hóa. Không ghi đè RAW khi thay đổi Transform.","Chỉ publish sau kiểm tra chất lượng và chính sách an toàn."],
 ["Chạy Transform không Sync","Run riêng, retry riêng, status riêng. Sau này có thể tự động kích hoạt sau khi Sync hoàn tất.","Chế độ tự động sẽ là orchestration tùy chọn, không phải một pipeline ghép cứng."]
 ][st];
 $("context").innerHTML='<div class="help-number">GUIDED UX / '+(sync?"SYNC":"TRANSFORM")+'</div><h3>'+content[0]+'</h3><p>'+content[1]+'</p>'+
 '<div class="tip"><b>Lưu ý:</b><br>'+content[2]+'</div>'+
 '<div class="divider"></div><h3>Trạng thái độc lập</h3>'+
 '<div class="checkline"><span class="check '+(state.sync.run&&state.sync.run.done?"":"wait")+'">'+(state.sync.run&&state.sync.run.done?"✓":"○")+'</span> Sync · '+(state.sync.run&&state.sync.run.done?"Đã mô phỏng xong":"Chưa chạy")+'</div>'+
 '<div class="checkline"><span class="check '+(state.rawRegistry.length?"":"wait")+'">'+(state.rawRegistry.length?"✓":"○")+'</span> RAW available · '+state.rawRegistry.length+' bảng</div>'+
 '<div class="checkline"><span class="check '+(state.transform.run&&state.transform.run.done?"":"wait")+'">'+(state.transform.run&&state.transform.run.done?"✓":"○")+'</span> Transform · '+(state.transform.run&&state.transform.run.done?"Đã mô phỏng xong":"Chưa chạy")+'</div>'+
 '<div class="tip"><b>Không tạo pipeline thật.</b><br>Trạng thái và hàng dữ liệu chỉ tồn tại trong phiên demo của trình duyệt.</div>';
}
function blueprint(){
 if(state.workspace==="sync"){var s=state.sync;return {prototype:true,workflow:"SYNC",execution:"simulation_only",name:s.name,source:{connection:"mock-sqlserver",table:source().name,columns:s.columns,mode:s.mode,key:source().key,cursor:source().cursor},raw_destination:{database:s.database,table:s.rawTable},transform:"none"};}
 var t=state.transform;return {prototype:true,workflow:"TRANSFORM",execution:"simulation_only",name:t.name,input_raw_table:t.rawId,steps:t.ops,output_mart:{database:t.database,table:t.martTable,mapping:mapped()},sync_execution:"not_required"};
}
function controls(){
 var sync=state.workspace==="sync",step=sync?state.syncStep:state.transformStep,max=sync?2:3,disabled=sync&&step===0&&syncErrors().some(function(e){return /key|cursor|RowVersion|Nguồn/.test(e);});
 $("footer-controls").innerHTML='<div class="muted-copy"> '+(sync?"Sync":"Transform")+' · '+(step+1)+' / '+(max+1)+' · demo độc lập</div><div class="right-controls">'+(step?'<button class="btn btn-outline" data-action="back">← Quay lại</button>':'<button class="btn btn-outline" data-action="save">Lưu bản nháp</button>')+(step<max?'<button class="btn btn-primary" data-action="next" '+(disabled?'disabled':'')+'>Tiếp tục →</button>':'<button class="btn btn-outline" data-action="export">Xuất cấu hình ⇩</button>')+'</div>';
}
function render(){
 document.documentElement.classList.toggle("dark",state.theme==="dark");
 $("theme-toggle").textContent=state.theme==="dark"?"☀":"◐";
 var sync=state.workspace==="sync",step=sync?state.syncStep:state.transformStep;
 $("workspace-title").textContent=sync?"Đồng bộ dữ liệu · Sync":"Xử lý dữ liệu · Transform";
 $("workspace-subtitle").textContent=sync?"SQL Server → ClickHouse RAW. Sync xong là hoàn tất, không bắt buộc Transform.":"Chọn bảng RAW đã có → dbt/SQL → Quality → MART. Không Sync lại.";
 document.querySelectorAll('[data-action="workspace"]').forEach(function(el){el.classList.toggle("active",el.getAttribute("data-value")===state.workspace);el.classList.toggle("selected",el.getAttribute("data-value")===state.workspace);});
 drawStepper();drawDiagram();
 $("stage").innerHTML=(sync?[syncSource,syncDestination,syncReview]:[transformInput,transformDesign,transformDestination,transformReview])[step]();
 help();controls();
}
function workspace(which){if(which!=="sync"&&which!=="transform")return;state.workspace=which;render();}
function stepChange(n){var key=state.workspace==="sync"?"syncStep":"transformStep";state[key]=Math.max(0,Math.min(state.workspace==="sync"?2:3,n));render();}
function begin(which){
 var sync=which==="sync",issues=sync?syncErrors():transformErrors();if(issues.length){note("Cần sửa: "+issues[0]);return;}
 if(timer!==null){clearInterval(timer);timer=null;}
 var target=sync?state.sync:state.transform;target.run={index:0,done:false};render();
 var count=sync?3:4;timer=setInterval(function(){
  target.run.index++;if(target.run.index>=count){clearInterval(timer);timer=null;target.run.done=true;
    if(sync){var s=state.sync,id=s.database+"."+s.rawTable;state.rawRegistry=state.rawRegistry.filter(function(x){return x.id!==id;});state.rawRegistry.push({id:id,sourceName:source().name,mode:s.mode,rows:syncRows()});if(state.transform.rawId===id)state.transform.run=null;if(!state.transform.rawId)state.transform.rawId=id;note("SYNC PASS · RAW có sẵn cho Transform (demo).");}
    else note("TRANSFORM PASS · MART được mô phỏng, không ghi DB.");
  }render();
 },550);
}
function handle(action,value){
 if(action==="workspace"){workspace(value);return;}
 if(action==="step"){stepChange(Number(value));return;}
 if(action==="next"){if(state.workspace==="sync"&&state.syncStep===0&&syncErrors().some(function(e){return /key|RowVersion/.test(e);}))return note("Chưa đủ key/cursor.");if(state.workspace==="transform"&&state.transformStep===0&&!raw())return note("Chạy Sync trước để tạo RAW.");stepChange((state.workspace==="sync"?state.syncStep:state.transformStep)+1);return;}
 if(action==="back"){stepChange((state.workspace==="sync"?state.syncStep:state.transformStep)-1);return;}
 if(action==="source"){var t=fixtures.find(function(x){return x.id===value;});if(!t)return;state.sync.source=t.id;state.sync.columns=t.cols.map(function(c){return c[0];});state.sync.mode=t.cursor?"full_then_incremental":"full_refresh";state.sync.rawTable="raw_"+t.id.toLowerCase();state.sync.run=null;render();return;}
 if(action==="raw"){state.transform.rawId=value;state.transform.ops=[];state.transform.mapping={};state.transform.run=null;render();return;}
 if(action==="op-add"){if(state.transform.ops.length>=12)return note("Giới hạn 12 bước demo.");state.transform.ops.push(opFactory(value));state.transform.run=null;render();return;}
 if(action==="op-remove"||action==="op-up"||action==="op-down"){var ops=state.transform.ops,i=ops.findIndex(function(o){return o.id===value;});if(i<0)return;if(action==="op-remove")ops.splice(i,1);else{var j=i+(action==="op-up"?-1:1);if(j<0||j>=ops.length)return;ops.splice(j,0,ops.splice(i,1)[0]);}state.transform.run=null;render();return;}
 if(action==="run-sync"||action==="run-transform"){begin(action==="run-sync"?"sync":"transform");return;}
 if(action==="theme"){state.theme=state.theme==="dark"?"light":"dark";try{localStorage.setItem("df-ux-theme",state.theme);}catch(_){}render();return;}
 if(action==="export"){var blob=new Blob([JSON.stringify(blueprint(),null,2)],{type:"application/json"}),url=URL.createObjectURL(blob),a=document.createElement("a");a.href=url;a.download="dataflow-"+state.workspace+"-prototype.json";document.body.appendChild(a);a.click();a.remove();setTimeout(function(){URL.revokeObjectURL(url);},400);note("Đã xuất JSON demo, không phải backend payload.");return;}
 if(action==="save"){try{localStorage.setItem("df-ux-draft-"+state.workspace,JSON.stringify(blueprint()));note("Đã lưu bản nháp "+state.workspace+" trên trình duyệt.");}catch(e){note("Storage bị chặn. Chọn xuất JSON.");}return;}
 if(action==="reset"){if(!confirm("Xóa toàn bộ trạng thái demo Sync và Transform?"))return;if(timer!==null)clearInterval(timer);timer=null;location.reload();return;}
}
document.addEventListener("click",function(e){var el=e.target.closest("[data-action]");if(el)handle(el.getAttribute("data-action"),el.getAttribute("data-value"));});
document.addEventListener("change",function(e){var el=e.target;
 if(el.hasAttribute("data-sync-field")){state.sync[el.getAttribute("data-sync-field")]=el.value;state.sync.run=null;render();return;}
 if(el.hasAttribute("data-transform-field")){state.transform[el.getAttribute("data-transform-field")]=el.value;state.transform.run=null;render();return;}
 if(el.hasAttribute("data-sync-col")){var c=el.getAttribute("data-sync-col"),t=source();if(c===t.key||(c===t.cursor&&state.sync.mode!=="full_refresh"))return;state.sync.columns=el.checked?state.sync.columns.concat(c):state.sync.columns.filter(function(x){return x!==c;});state.sync.run=null;render();return;}
 if(el.name==="sync-mode"){state.sync.mode=el.value;var c=source().cursor;if(c&&state.sync.mode!=="full_refresh"&&!state.sync.columns.includes(c))state.sync.columns.push(c);state.sync.run=null;render();return;}
 if(el.hasAttribute("data-op-enabled")){var op=state.transform.ops.find(function(o){return o.id===el.getAttribute("data-op-enabled");});if(op)op.enabled=el.checked;state.transform.run=null;render();return;}
 if(el.hasAttribute("data-op-field")){var op=state.transform.ops.find(function(o){return o.id===el.getAttribute("data-op-id");});if(op)op[el.getAttribute("data-op-field")]=el.value;state.transform.run=null;render();return;}
 if(el.hasAttribute("data-map")){state.transform.mapping[el.getAttribute("data-map")]=el.value;state.transform.run=null;render();}
});
render();
})();