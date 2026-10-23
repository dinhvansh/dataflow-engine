/* DataFlow UX demo ONLY. No backend calls, credential handling, or ERP writes. */
(function(){
"use strict";
var tables=[
 {id:"CustomOrders",schema:"Erp",name:"CustomOrders",label:"Sales Orders",desc:"Orders · unique OrderID · native RowChanged",key:"OrderID",cursor:"RowChanged",
  columns:[["OrderID","bigint"],["Customer","nvarchar"],["Region","nvarchar"],["Amount","decimal(18,2)"],["Status","nvarchar"],["RowChanged","rowversion"]],
  rows:[
   {OrderID:1001,Customer:"Andros Vietnam",Region:"South",Amount:1250000,Status:"Active",RowChanged:"0x0000000000000001"},
   {OrderID:1002,Customer:"Alpha Retail",Region:"North",Amount:895000,Status:"Active",RowChanged:"0x0000000000000002"},
   {OrderID:1003,Customer:"Minh Phu Foods",Region:"South",Amount:740000,Status:"Cancelled",RowChanged:"0x0000000000000003"},
   {OrderID:1004,Customer:"Delta Market",Region:"Central",Amount:2460000,Status:"Active",RowChanged:"0x0000000000000004"},
   {OrderID:1005,Customer:"Star Wholesale",Region:"North",Amount:375000,Status:"Active",RowChanged:"0x0000000000000005"},
   {OrderID:1006,Customer:"Lotus Food",Region:"South",Amount:132000,Status:"Pending",RowChanged:"0x0000000000000006"}
  ]},
 {id:"Part",schema:"Erp",name:"Part",label:"Product Master",desc:"Parts · single unique PartNum · native SysRevID",key:"PartNum",cursor:"SysRevID",
  columns:[["PartNum","nvarchar"],["PartDescription","nvarchar"],["ProdCode","nvarchar"],["UnitPrice","decimal(18,2)"],["Inactive","bit"],["SysRevID","rowversion"]],
  rows:[
   {PartNum:"A-001",PartDescription:"Mango Puree 1kg",ProdCode:"FRUIT",UnitPrice:120000,Inactive:0,SysRevID:"0x0000000000000001"},
   {PartNum:"A-002",PartDescription:"Strawberry Jam 500g",ProdCode:"JAM",UnitPrice:86000,Inactive:0,SysRevID:"0x0000000000000002"},
   {PartNum:"B-010",PartDescription:"Apricot Filling",ProdCode:"BAKE",UnitPrice:103000,Inactive:0,SysRevID:"0x0000000000000003"},
   {PartNum:"B-015",PartDescription:"Frozen Banana",ProdCode:"FRUIT",UnitPrice:59000,Inactive:1,SysRevID:"0x0000000000000004"}
  ]},
 {id:"CustomSnapshot",schema:"Erp",name:"CustomSnapshot",label:"Snapshot Only",desc:"Records · unique RecordID · no RowVersion",key:"RecordID",cursor:null,
  columns:[["RecordID","bigint"],["Department","nvarchar"],["Owner","nvarchar"],["Cost","decimal(18,2)"]],
  rows:[
   {RecordID:21,Department:"Finance",Owner:"Mai",Cost:875000},
   {RecordID:22,Department:"Sales",Owner:"Khoa",Cost:1320000},
   {RecordID:23,Department:"IT",Owner:"Van",Cost:645000}
  ]}
];
var defaults=function(){return {step:0,theme:"light",name:"Sales order analytics",sourceConnection:"qa_sqlserver",table:"CustomOrders",selected:["OrderID","Customer","Region","Amount","Status","RowChanged"],mode:"full_then_incremental",ops:[],targetConnection:"qa_clickhouse",database:"scratch",targetTable:"dfqa_orders_curated",layout:"typed",mapping:{},lastRun:null,showJSON:false,showDraft:false};};
var state=defaults();
var runTimer=null, seq=0;
try{if(localStorage.getItem("df-ux-theme")==="dark")state.theme="dark";}catch(_){}
function getTable(){return tables.find(function(t){return t.id===state.table;})||tables[0];}
function esc(v){return String(v===null||v===undefined?"":v).replace(/[&<>"']/g,function(ch){return {"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[ch];});}
function attr(v){return esc(v);}
function notify(message){var el=document.getElementById("toast-stack"),n=document.createElement("div");n.className="toast";n.textContent=message;el.appendChild(n);setTimeout(function(){n.remove();},3400);}
function listColumns(rows){return rows.length?Object.keys(rows[0]):[];}
function originalRows(){var t=getTable();return t.rows.map(function(r){var out={};state.selected.forEach(function(c){out[c]=r[c];});return out;});}
function selectedKeyOk(){var t=getTable();return state.selected.indexOf(t.key)>=0&&(state.mode==="full_refresh"||!!t.cursor&&state.selected.indexOf(t.cursor)>=0);}
function validateSource(){return !!state.sourceConnection&&selectedKeyOk()&&state.selected.length>0&&(state.mode==="full_refresh"||!!getTable().cursor);}
function safeId(s){return /^[A-Za-z_][A-Za-z0-9_]{0,63}$/.test(s);}
function isSafeDest(){return safeId(state.database)&&safeId(state.targetTable)&&state.database==="scratch"&&!!state.targetConnection&&
 (state.layout==="raw_event"||state.layout==="typed")&&
 Object.keys(destinationMapping()).every(function(k){return safeId(destinationMapping()[k]);})&&
 new Set(Object.values(destinationMapping()).map(function(s){return s.toLowerCase();})).size===Object.keys(destinationMapping()).length;}
function colType(c){var origin=getTable().columns.find(function(x){return x[0]===c;});if(origin)return origin[1];if(c.toLowerCase().indexOf("total")>=0||c.toLowerCase().indexOf("amount")>=0||c.toLowerCase().indexOf("price")>=0||c.toLowerCase().indexOf("cost")>=0)return "decimal(18,2)";return "nvarchar"; }
function destinationMapping(){var cols=listColumns(outputRows());var map={};cols.forEach(function(c){map[c]=state.mapping[c]===undefined?c:state.mapping[c];});return map;}
function executeOps(){var rows=originalRows();state.ops.forEach(function(op){
 if(!op.enabled)return;
 var col=op.column;
 if(op.kind==="filter"){rows=rows.filter(function(r){var v=r[col];if(op.compare==="greater")return Number(v)>Number(op.value);if(op.compare==="contains")return String(v===undefined?"":v).toLowerCase().includes(String(op.value).toLowerCase());return String(v)===String(op.value);});}
 if(op.kind==="rename"){rows=rows.map(function(r){var t=Object.assign({},r);if(col in t&&op.alias&&op.alias!==col){t[op.alias]=t[col];delete t[col];}return t;});}
 if(op.kind==="calculate"){rows=rows.map(function(r){var t=Object.assign({},r);if(op.alias)t[op.alias]=Number((Number(r[col]||0)*Number(op.value||0)).toFixed(2));return t;});}
 if(op.kind==="sort"){rows=rows.slice().sort(function(a,b){var av=a[col],bv=b[col],result=(typeof av==="number"&&typeof bv==="number")?av-bv:String(av).localeCompare(String(bv));return op.direction==="desc"?-result:result;});}
 if(op.kind==="limit"){rows=rows.slice(0,Math.max(1,Math.min(1000,Number(op.value)||1)));}
 if(op.kind==="select"){var names=String(op.columns||"").split(",").map(function(x){return x.trim();}).filter(Boolean);if(names.length)rows=rows.map(function(r){var t={};names.forEach(function(k){if(k in r)t[k]=r[k];});return t;});}
 if(op.kind==="aggregate"){var groups={};rows.forEach(function(r){var key=String(r[col]);if(!groups[key])groups[key]={count:0,sum:0,value:r[col]};groups[key].count++;groups[key].sum+=Number(r[op.measure]||0);});rows=Object.keys(groups).map(function(k){var t={};t[col]=groups[k].value;t[op.alias||"TotalAmount"]=Number(groups[k].sum.toFixed(2));t["RowCount"]=groups[k].count;return t;});}
 });return rows;}
function outputRows(){return executeOps();}
function columnsForCurrentStep(index){var rows=originalRows();state.ops.slice(0,index).filter(function(op){return op.enabled;}).forEach(function(op){var dummy=state.ops;state.ops=[op];rows=executeOpsPreview(rows,op);state.ops=dummy;});return listColumns(rows);}
function executeOpsPreview(rows,op){var original=originalRows;/* no side effects: scope this op to current rows */
 var s=state.ops;var savedSelected=state.selected;var inRows=rows.map(function(r){return Object.assign({},r);});
 state.ops=[];var kind=op.kind;
 if(kind==="filter")return inRows.filter(function(r){if(op.compare==="greater")return Number(r[op.column])>Number(op.value);if(op.compare==="contains")return String(r[op.column]).toLowerCase().includes(String(op.value).toLowerCase());return String(r[op.column])===String(op.value);});
 if(kind==="rename")return inRows.map(function(r){if(!(op.column in r)||!op.alias||op.column===op.alias)return r;var t=Object.assign({},r);t[op.alias]=t[op.column];delete t[op.column];return t;});
 if(kind==="calculate")return inRows.map(function(r){var t=Object.assign({},r);t[op.alias]=Number((Number(r[op.column]||0)*Number(op.value||0)).toFixed(2));return t;});
 if(kind==="sort")return inRows.sort(function(a,b){var x=a[op.column],y=b[op.column],z=typeof x==="number"&&typeof y==="number"?x-y:String(x).localeCompare(String(y));return op.direction==="desc"?-z:z;});
 if(kind==="limit")return inRows.slice(0,Math.max(1,Math.min(1000,Number(op.value)||1)));
 if(kind==="select"){var cs=String(op.columns||"").split(",").map(function(x){return x.trim();});return inRows.map(function(r){var t={};cs.forEach(function(k){if(k in r)t[k]=r[k];});return t;});}
 if(kind==="aggregate"){var by={};inRows.forEach(function(r){var v=String(r[op.column]);if(!by[v])by[v]={sum:0,count:0,key:r[op.column]};by[v].sum+=Number(r[op.measure]||0);by[v].count++;});return Object.keys(by).map(function(k){var t={};t[op.column]=by[k].key;t[op.alias||"TotalAmount"]=Number(by[k].sum.toFixed(2));t.RowCount=by[k].count;return t;});}
 return inRows;
}
function transformedRows(){var rows=originalRows();state.ops.forEach(function(op){if(op.enabled)rows=executeOpsPreview(rows,op);});return rows;}
function outputRows(){return transformedRows();}
function uiSelect(options,value,attrs){return '<select '+(attrs||"")+'>'+options.map(function(o){var v=Array.isArray(o)?o[0]:o,txt=Array.isArray(o)?o[1]:o;return '<option value="'+attr(v)+'"'+(v===value?' selected':'')+'>'+esc(txt)+'</option>';}).join("")+'</select>';}
function dataGrid(rows,includeCheckbox){var cols=listColumns(rows);if(!cols.length)return '<div class="empty-transform">Không có hàng sau bước xử lý. Hãy điều chỉnh filter hoặc chọn bảng khác.</div>';
 var head=cols.map(function(c){return '<th>'+esc(c)+' <span class="mono">'+esc(colType(c))+'</span></th>';}).join("");
 return '<div class="grid-scroll"><table class="data-table"><thead><tr>'+head+'</tr></thead><tbody>'+
 rows.slice(0,8).map(function(row){return '<tr>'+cols.map(function(c){var v=row[c];return '<td'+(typeof v==="number"?' class="mono"':'')+'>'+esc(typeof v==="number"?v.toLocaleString("en-US"):v)+'</td>';}).join("")+'</tr>';}).join("")+'</tbody></table></div>';
}
function sectionTitle(name,trailing){return '<div class="section-title"><span>'+name+'</span>'+(trailing||"")+'</div>';}
function panelHeader(number,title,desc,extra){return '<div class="panel-header"><div><div class="section-eyebrow">STEP '+number+' / 4</div><h2>'+title+'</h2><p>'+desc+'</p></div>'+(extra||"")+'</div>';}
function renderStepper(){var names=[["Nguồn dữ liệu","Connect & Extract"],["Xử lý dữ liệu","Visual Transform"],["Nơi lưu","Destination & Mapping"],["Kiểm tra & chạy","Review & Simulate"]];
 document.getElementById("stepper").innerHTML=names.map(function(item,i){return '<button type="button" class="step '+(i===state.step?'active':i<state.step?'done':'')+'" data-action="step" data-value="'+i+'" aria-current="'+(i===state.step?'step':'false')+'"><span class="step-num">'+(i<state.step?'✓':i+1)+'</span><span><strong>'+item[0]+'</strong><small>'+item[1]+'</small></span></button>';}).join("");}
function renderDiagram(){var t=getTable(),s=state.ops.filter(function(o){return o.enabled;}).length,cols=listColumns(outputRows()).length;var data=[
 ["▤","SOURCE",t.schema+"."+t.name,state.selected.length+" cột · "+(state.mode==="full_refresh"?"Full only":"RowVersion")],
 ["⧉","TRANSFORM",s?s+" bước xử lý":"Không thay đổi dữ liệu",cols+" cột đầu ra"],
 ["▣","DESTINATION",state.database+"."+state.targetTable,state.layout==="typed"?"Typed table":"Raw events"]
 ];
 document.getElementById("flow-diagram").innerHTML=data.map(function(d,i){var node='<div class="flow-node has-data"><span class="node-icon">'+d[0]+'</span><div style="min-width:0"><div class="tag">'+d[1]+'</div><strong title="'+attr(d[2])+'">'+esc(d[2])+'</strong><small>'+esc(d[3])+'</small></div></div>';return (i?'<div class="flow-arrow">⟶</div>':'')+node;}).join("");
 var tag=state.lastRun&&state.lastRun.done?"Mô phỏng hoàn tất":state.lastRun?"Đang mô phỏng":"Bản nháp · không chạy backend";document.getElementById("flow-state").textContent=tag;}
function renderSource(){
 var t=getTable(),cursorAvailable=!!t.cursor,sourceRows=originalRows(),columns=t.columns;
 return panelHeader(1,"Chọn nguồn và dữ liệu cần lấy","Khám phá bảng SQL Server và quyết định lấy dữ liệu theo cách nào.",'<span class="status-chip ok">✓ Source read-only</span>')+
 '<div class="form-grid">'+
 '<label class="field-label">Source connection <small>Kết nối đã lưu (dữ liệu giả lập)</small>'+uiSelect([["qa_sqlserver","ERP SQL Server · DataFlowFixtureQA"]],state.sourceConnection,'data-field="sourceConnection"')+'</label>'+
 '<label class="field-label">Tên pipeline <small>Để tìm lại trong danh sách Job</small><input type="text" data-field="name" maxlength="70" value="'+attr(state.name)+'"></label>'+
 '</div>'+
 sectionTitle("01 · Chọn bảng từ SQL Server",'<span class="status-chip">Schema Erp</span>')+
 '<div class="table-picker">'+tables.map(function(x){return '<button type="button" class="table-option '+(x.id===state.table?'active':'')+'" data-action="table" data-value="'+attr(x.id)+'"><span class="table-symbol">▤</span><strong>'+esc(x.label)+'</strong><small>Erp.'+esc(x.name)+'</small><small>'+esc(x.desc)+'</small></button>';}).join("")+'</div>'+
 '<div class="key-info spaced-top"><span class="status-chip ok">✓ Unique key: '+esc(t.key)+'</span> &nbsp; '+(cursorAvailable?'<span class="status-chip ok">✓ RowVersion: '+esc(t.cursor)+'</span>':'<span class="status-chip warn">! Không có RowVersion · chỉ Full Refresh</span>')+'<br>Hệ thống đọc dữ liệu bằng tài khoản SELECT-only; prototype không gửi kết nối đến server.</div>'+
 sectionTitle("02 · Chọn các cột cần lấy",'<span class="column-count">'+state.selected.length+' / '+columns.length+' cột được chọn</span>')+
 '<div class="grid-scroll"><table class="data-table"><thead><tr><th>Chọn</th><th>Tên cột</th><th>Kiểu dữ liệu</th><th>Vai trò</th></tr></thead><tbody>'+columns.map(function(c){var required=c[0]===t.key||(state.mode!=="full_refresh"&&c[0]===t.cursor);return '<tr><td><input type="checkbox" aria-label="Chọn cột '+attr(c[0])+'" data-column="'+attr(c[0])+'" '+(state.selected.indexOf(c[0])>=0?'checked':'')+(required?' disabled':'')+'></td><td class="mono">'+esc(c[0])+'</td><td>'+esc(c[1])+'</td><td>'+(c[0]===t.key?'<span class="status-chip ok">UNIQUE KEY</span>':c[0]===t.cursor?'<span class="status-chip">ROWVERSION</span>':'<span class="muted-copy">Data field</span>')+'</td></tr>';}).join("")+'</tbody></table></div>'+
 sectionTitle("03 · Chọn cách đồng bộ")+
 '<div>'+[
  ["full_refresh","Full Refresh","Đọc toàn bộ, thay thế dữ liệu đích khi chạy lại."],
  ["full_then_incremental","Full → Incremental","Đọc toàn bộ một lần, các lần sau chỉ lấy phần thay đổi."],
  ["incremental_rowversion","Incremental RowVersion","Chỉ đọc phần thay đổi kể từ checkpoint đã lưu."]
 ].map(function(m){var unavailable=!cursorAvailable&&m[0]!=="full_refresh";return '<label class="choice-row '+(state.mode===m[0]?'selected':'')+'" style="'+(unavailable?'opacity:.5':'')+'"><input type="radio" name="syncmode" value="'+m[0]+'" data-sync-mode '+(state.mode===m[0]?'checked':'')+(unavailable?' disabled':'')+'><span><strong>'+m[1]+'</strong><small>'+m[2]+(unavailable?' · yêu cầu RowVersion':'')+'</small></span></label>';}).join("")+'</div>'+
 sectionTitle("Xem trước dữ liệu nguồn",'<span class="status-chip">Mẫu '+sourceRows.length+' hàng · không truy cập ERP</span>')+
 dataGrid(sourceRows);
}
function opText(kind){return ({filter:"Lọc dữ liệu",rename:"Đổi tên cột",calculate:"Cột tính toán",aggregate:"Nhóm & tổng hợp",sort:"Sắp xếp",limit:"Giới hạn dòng",select:"Chọn cột"})[kind]||kind;}
function makeOp(kind){var t=getTable(),cols=listColumns(outputRows()),numeric=cols.find(function(x){return /amount|price|cost|total/i.test(x);})||cols[0]||t.key;return {id:"op"+(++seq),kind:kind,enabled:true,column:kind==="filter"?(cols.indexOf("Status")>=0?"Status":cols[0]):kind==="aggregate"?(cols.indexOf("Region")>=0?"Region":cols[0]):numeric,value:kind==="filter"?(cols.indexOf("Status")>=0?"Active":""):(kind==="limit"?"5":"1.1"),alias:kind==="rename"?"RenamedColumn":kind==="aggregate"?"TotalAmount":"AdjustedAmount",compare:"equals",measure:numeric,direction:"desc",columns:cols.join(", ")};}
function opSettings(op,idx){
 var cols=columnsForCurrentStep(idx),opts=cols.map(function(c){return [c,c];}),num=cols.filter(function(c){return /amount|price|cost|total/i.test(c);});
 var field=function(label,content){return '<label class="field-label">'+label+content+'</label>';};
 var sel=function(fieldName,label,items,value){return field(label,uiSelect(items.length?items:[["","No fields"]],value,'data-op-id="'+attr(op.id)+'" data-op-field="'+fieldName+'"'));};
 var txt=function(fieldName,label,value,type){return field(label,'<input type="'+(type||'text')+'" data-op-id="'+attr(op.id)+'" data-op-field="'+fieldName+'" value="'+attr(value)+'">');};
 if(op.kind==="filter")return sel("column","Cột để lọc",opts,op.column)+sel("compare","Điều kiện",[["equals","Bằng"],["contains","Chứa"],["greater","Lớn hơn"]],op.compare)+txt("value","Giá trị",op.value);
 if(op.kind==="rename")return sel("column","Cột nguồn",opts,op.column)+txt("alias","Tên cột mới",op.alias);
 if(op.kind==="calculate")return sel("column","Cột số nguồn",opts,op.column)+txt("value","Hệ số nhân",op.value,"number")+txt("alias","Tên cột mới",op.alias);
 if(op.kind==="aggregate")return sel("column","Nhóm theo",opts,op.column)+sel("measure","Cột cộng tổng",num.length?num.map(function(c){return [c,c];}):opts,op.measure)+txt("alias","Tên cột tổng",op.alias);
 if(op.kind==="sort")return sel("column","Sắp xếp theo",opts,op.column)+sel("direction","Thứ tự",[["asc","Tăng dần"],["desc","Giảm dần"]],op.direction);
 if(op.kind==="limit")return txt("value","Giữ số hàng đầu tiên",op.value,"number");
 if(op.kind==="select")return txt("columns","Tên cột giữ lại (ngăn cách bằng dấu phẩy)",op.columns);
 return "";
}
function renderTransform(){
 var rows=outputRows(),origin=originalRows(),ops=[
 ["filter","≡","Lọc hàng"],["select","▦","Chọn cột"],["rename","Aa","Đổi tên"],["calculate","∑","Tính toán"],["aggregate","◫","Gộp nhóm"],["sort","⇅","Sắp xếp"],["limit","№","Giới hạn"]
 ];
 return panelHeader(2,"Biến đổi dữ liệu","Thêm từng bước xử lý và nhìn thấy kết quả trước/sau ngay lập tức.",'<span class="status-chip">'+state.ops.length+' bước cấu hình</span>')+
 '<div class="metrics"><div class="metric"><small>Dữ liệu đầu vào</small><strong>'+origin.length+'</strong><small>hàng mẫu</small></div><div class="metric"><small>Dữ liệu đầu ra</small><strong>'+rows.length+'</strong><small>hàng sau xử lý</small></div><div class="metric"><small>Số cột kết quả</small><strong>'+listColumns(rows).length+'</strong><small>cột sau xử lý</small></div></div>'+
 sectionTitle("01 · Chọn thao tác xử lý",'<span class="muted-copy">Nhấn để thêm vào quy trình</span>')+
 '<div class="op-gallery">'+ops.map(function(o){return '<button type="button" class="op-card" data-action="add-op" data-value="'+o[0]+'"><span class="op-icon">'+o[1]+'</span><strong>'+o[2]+'</strong><small>+ Thêm bước</small></button>';}).join("")+'</div>'+
 sectionTitle("02 · Các bước đã thiết lập",'<span class="status-chip">'+state.ops.length+' bước · theo thứ tự từ trên xuống</span>')+
 (state.ops.length?state.ops.map(function(op,i){return '<div class="transform-row '+(!op.enabled?'disabled':'')+'"><span class="op-number">'+(i+1)+'</span><div><div class="op-head"><strong>'+esc(opText(op.kind))+'</strong><label class="muted-copy"><input type="checkbox" data-op-enable="'+attr(op.id)+'" '+(op.enabled?'checked':'')+'> Bật</label></div><div class="op-config">'+opSettings(op,i)+'</div></div><div class="op-buttons"><button type="button" data-action="move-op" data-value="'+attr(op.id)+':-1" aria-label="Chuyển lên" title="Lên" '+(i===0?'disabled':'')+'>↑</button><button type="button" data-action="move-op" data-value="'+attr(op.id)+':1" aria-label="Chuyển xuống" title="Xuống" '+(i===state.ops.length-1?'disabled':'')+'>↓</button><button type="button" data-action="remove-op" data-value="'+attr(op.id)+'" aria-label="Xóa bước" title="Xóa">×</button></div></div>';}).join(""):'<div class="empty-transform"><strong>Chưa có bước biến đổi</strong>Dữ liệu sẽ đi từ Source sang Destination mà không thay đổi.<br>Hãy thử thêm “Lọc hàng” hoặc “Cột tính toán” ở phía trên.</div>')+
 sectionTitle("03 · Kết quả trực tiếp",'<span class="status-chip ok">PREVIEW · '+rows.length+' rows</span>')+
 '<div class="row-actions"><p class="muted-copy">Bảng bên dưới là kết quả tính toán từ dữ liệu giả lập. Không chạy dbt thật.</p><span class="legend"><span><span class="dot"></span>Đã cập nhật</span></span></div>'+
 dataGrid(rows)+
 (state.ops.some(function(op){return op.enabled&&op.kind==="aggregate";})?'<div class="warning-inline spaced-top">Lưu ý: Group By thay đổi khóa của dữ liệu. Muốn Incremental thực tế cần cấu hình grain và unique key của bảng tổng hợp trước khi chạy.</div>':'');
}
function renderDestination(){
 var out=outputRows(),cols=listColumns(out),mapped=destinationMapping();
 return panelHeader(3,"Chọn nơi lưu dữ liệu","Chọn ClickHouse, bảng đích và đối chiếu tên cột trước khi ghi.",'<span class="status-chip ok">✓ QA warehouse</span>')+
 '<div class="form-grid"><label class="field-label">Destination connection <small>Đích thử nghiệm được quản lý</small>'+uiSelect([["qa_clickhouse","ClickHouse · Isolated QA"]],state.targetConnection,'data-field="targetConnection"')+'</label>'+
 '<label class="field-label">Kiểu bảng lưu <small>Typed: từng cột; raw-event: metadata + payload</small>'+uiSelect([["typed","Typed table · columns + types"],["raw_event","Raw events · event payload"]],state.layout,'data-field="layout"')+'</label>'+
 '<label class="field-label">Database <small>Prototype chỉ cho phép scratch (QA)</small><input type="text" value="'+attr(state.database)+'" data-field="database"></label>'+
 '<label class="field-label">Table name <small>Bảng demo mới, không được trùng bảng có sẵn</small><input type="text" value="'+attr(state.targetTable)+'" data-field="targetTable" maxlength="64"></label></div>'+
 '<div class="destination-summary spaced-top"><div class="hint-label">Đường dẫn bảng đích</div><b><code>'+esc(state.database)+'.'+esc(state.targetTable)+'</code></b><br>Dữ liệu: <b>'+out.length+' hàng mẫu</b> · '+cols.length+' cột · Chế độ <b>'+esc(state.mode)+'</b></div>'+
 sectionTitle("Mapping cột · nguồn → đích",'<span class="status-chip">'+cols.length+' cột</span>')+
 '<p class="muted-copy">Sửa tên cột đích ở bên phải; tên cột SQL Server gốc không thay đổi. Kiểu dữ liệu thật sẽ do server đối chiếu với catalog.</p>'+
 '<div class="grid-scroll"><table class="data-table mapping-table"><thead><tr><th>Cột sau Transform</th><th>Kiểu suy đoán</th><th>Tên cột ClickHouse</th></tr></thead><tbody>'+cols.map(function(c){return '<tr><td><b class="mono">'+esc(c)+'</b></td><td>'+esc(colType(c))+'</td><td><input type="text" data-map-column="'+attr(c)+'" aria-label="Destination column '+attr(c)+'" maxlength="64" value="'+attr(mapped[c])+'"></td></tr>';}).join("")+'</tbody></table></div>'+
 sectionTitle("Chính sách ghi dữ liệu")+
 '<div class="choice-row selected"><span class="check">✓</span><span><strong>'+esc(state.mode==="full_refresh"?"Full Replace":state.mode==="full_then_incremental"?"Full once → Incremental":"Incremental by RowVersion")+'</strong><small>'+esc(state.mode==="full_refresh"?"Lần chạy sau thay thế bản chụp toàn bảng.":state.mode==="full_then_incremental"?"Checkpoint chụp tại snapshot; những lần sau đọc thay đổi.":"Các thay đổi được ghi theo version; tra cứu current rows bằng FINAL.")+'</small></span></div>'+
 (!isSafeDest()?'<div class="warning-inline">Bảng đích chưa hợp lệ: chỉ chấp nhận database scratch, tên bảng/cột dạng chữ và số, không trùng tên.</div>':'<div class="success-inline">✓ Cấu hình hợp lệ ở mức prototype. Runtime thật sẽ kiểm tra ownership, grants, các kiểu được hỗ trợ và giới hạn bảo mật.</div>')+
 sectionTitle("Xem trước dữ liệu sau mapping")+
 dataGrid(out.map(function(row){var t={};Object.keys(mapped).forEach(function(k){t[mapped[k]]=row[k];});return t;}));
}
function getIssues(){var issues=[];if(!validateSource())issues.push("Source phải chọn đủ key và RowVersion nếu chạy incremental.");if(!isSafeDest())issues.push("Tên database/table hoặc mapping cột ClickHouse không hợp lệ.");if(!state.name.trim())issues.push("Tên pipeline không được để trống.");if(!listColumns(outputRows()).length)issues.push("Transform phải xuất ít nhất một cột.");if(state.ops.some(function(o){return o.enabled&&o.kind==="aggregate";})&&state.mode!=="full_refresh")issues.push("Pipeline aggregate + Incremental cần thiết kế lại khóa/grain (prototype chưa hỗ trợ chạy thật).");return issues;}
function blueprint(){return {prototype:true,execution:"simulation_only",version:"UX-2026-10-23",name:state.name,source:{profile:state.sourceConnection,driver:"sqlserver",schema:getTable().schema,table:state.table,columns:state.selected,unique_key:getTable().key,rowversion:getTable().cursor,sync_mode:state.mode},transform:{mode:"visual",steps:state.ops.map(function(o){return Object.assign({},o);}),preview_rows:outputRows().length},destination:{profile:state.targetConnection,driver:"clickhouse",database:state.database,table:state.targetTable,layout:state.layout,columns:destinationMapping()},safety:{read_only_source:true,destination_requires_ownership:true,production_execution:false}};}
function renderReview(){
 var rows=outputRows(),issues=getIssues(),t=getTable(),o=state.ops.filter(function(x){return x.enabled;});
 var run=state.lastRun,stages=["Đọc nguồn","Xử lý dữ liệu","Mapping & ghi đích","Xác minh kết quả"];
 return panelHeader(4,"Kiểm tra trước khi chạy","Đây là bản tổng hợp giúp m hiểu chính xác pipeline sẽ làm gì.",'<span class="status-chip '+(!issues.length?'ok':'warn')+'">'+(!issues.length?'✓ Sẵn sàng mô phỏng':issues.length+' mục cần xử lý')+'</span>')+
 '<div class="review-block"><span class="review-symbol">▤</span><div><strong>01 · Đọc '+esc(t.schema+'.'+t.name)+'</strong><p>Kết nối SQL Server QA · '+state.selected.length+' cột · Khóa '+esc(t.key)+' · '+esc(state.mode)+'.<br>'+t.rows.length+' hàng dữ liệu mẫu, chỉ đọc.</p></div></div>'+
 '<div class="review-block"><span class="review-symbol">⧉</span><div><strong>02 · '+o.length+' bước Transform</strong><p>'+(o.length?o.map(function(x){return esc(opText(x.kind));}).join(" → "):"Không thay đổi dữ liệu đầu vào")+'<br>Kết quả xem trước: '+rows.length+' hàng / '+listColumns(rows).length+' cột.</p></div></div>'+
 '<div class="review-block"><span class="review-symbol">▣</span><div><strong>03 · Lưu vào '+esc(state.database+'.'+state.targetTable)+'</strong><p>ClickHouse QA · '+(state.layout==="typed"?"bảng typed có cột riêng":"raw events")+' · '+Object.keys(destinationMapping()).length+' column mappings. Môi trường thật phải kiểm tra bảng được DataFlow quản lý.</p></div></div>'+
 sectionTitle("Kiểm tra an toàn")+
 (issues.length?'<div class="warning-inline">⚠ '+issues.map(esc).join("<br>⚠ ")+'</div>':'<div class="success-inline">✓ Cấu hình demo đạt các điều kiện cơ bản.<br>✓ Source chỉ đọc · Destination scratch · Không có API ghi trong prototype.<br>✓ Dữ liệu và kết quả đều giả lập.</div>')+
 sectionTitle("Chạy thử pipeline",'<span class="status-chip">'+(run&&run.done?'SIMULATED SUCCESS':run?'SIMULATING':'NOT STARTED')+'</span>')+
 '<div class="run-progress">'+stages.map(function(s,i){var done=run&&(run.done||i<run.index),active=run&&!run.done&&i===run.index;return '<div class="run-stage '+(done?'done':active?'running':'')+'"><strong>'+(done?"✓ ":active?"⟳ ":"○ ")+s+'</strong><small>'+(done?"Hoàn thành · demo":active?"Đang mô phỏng":"Chưa bắt đầu")+'</small></div>';}).join("")+'</div>'+
 (run&&run.done?'<div class="success-inline"><b>✓ Mô phỏng hoàn tất</b> · '+rows.length+' hàng mẫu được xử lý trên trình duyệt. Không có request nào được gửi tới SQL Server / ClickHouse.</div>':'<div class="warning-inline">Nút bên dưới chỉ mô phỏng trải nghiệm vận hành. Nó không tạo job, checkpoint, bảng dữ liệu hoặc bản ghi thật.</div>')+
 '<div class="flex-gap spaced-top"><button class="btn btn-primary" data-action="simulate" '+(issues.length||run&&!run.done?'disabled':'')+'>▷ '+(run&&run.done?"Chạy lại mô phỏng":"Mô phỏng chạy Pipeline")+'</button><button class="btn btn-outline" data-action="export">⇩ Xuất Blueprint JSON</button><button class="btn btn-outline" data-action="json">'+(state.showJSON?"Ẩn":"Xem")+" JSON</button></div>'+
 (state.showJSON?'<pre class="code-preview">'+esc(JSON.stringify(blueprint(),null,2))+'</pre>':'');
}
function renderHelp(){
 var t=getTable();var tips=[
 ["Vì sao bắt đầu từ Source?","Trước tiên hãy chọn kết nối đã kiểm tra, sau đó chọn bảng và cột cần đọc. Các cột khóa/cursor được đánh dấu rõ để tránh cấu hình sai.","Không có RowVersion vẫn dùng được Full Refresh, nhưng không dùng Full → Incremental."],
 ["Tại sao có Transform?","Mỗi thao tác là một bước. Thêm, bật/tắt, đổi thứ tự và xem sự khác biệt ngay dưới bảng preview.","Các phép biến đổi trong trang là JavaScript demo. Backend Transform Studio và dbt hiện có giới hạn riêng."],
 ["Mapping là gì?","Là đối chiếu tên cột kết quả với cột ClickHouse. Ví dụ Amount → Revenue. Không phải đổi tên cột trong ERP.","Chỉ chọn database scratch ở prototype. Bảng có sẵn không thuộc DataFlow sẽ bị runtime từ chối."],
 ["Trước khi Run thật","Kiểm tra toàn bộ sơ đồ và cảnh báo rồi mới thực hiện. Sau này đây sẽ là nơi xem log, row count và checkpoint.","Ở prototype mọi nút Run đều là mô phỏng. Xuất JSON để review nhưng JSON chưa phải payload API tạo pipeline."]
 ];
 var tip=tips[state.step],stats=state.step===1?["Cột nguồn",String(state.selected.length),"Cột sau xử lý",String(listColumns(outputRows()).length)]:state.step===0?["Bảng đang chọn",t.name,"Sync",state.mode==="full_refresh"?"Full":state.mode==="full_then_incremental"?"Full → Delta":"Delta"]:state.step===2?["Database",state.database,"Ghi đích",state.layout==="typed"?"Typed":"Raw events"]:["Trạng thái",getIssues().length?"Cần sửa":"Ready","Rows",String(outputRows().length)];
 document.getElementById("context").innerHTML='<div class="help-number">CONTEXTUAL HELP / 0'+(state.step+1)+'</div><h3>'+tip[0]+'</h3><p>'+tip[1]+'</p>'+
 '<div class="subtle-box"><div class="hint-label">'+esc(stats[0])+'</div><div class="value-bold">'+esc(stats[1])+'</div></div>'+
 '<div class="subtle-box spaced-top"><div class="hint-label">'+esc(stats[2])+'</div><div class="value-bold">'+esc(stats[3])+'</div></div>'+
 '<div class="tip"><span class="help-strong">Lưu ý</span><br>'+tip[2]+'</div>'+
 '<div class="divider"></div><h3>Checklist</h3>'+
 '<div class="checkline"><span class="check '+(!validateSource()?'wait':'')+'">'+(validateSource()?"✓":"○")+'</span><span>Đã chọn nguồn, bảng và khóa đồng bộ</span></div>'+
 '<div class="checkline"><span class="check">✓</span><span>Transform xem trước được từ dữ liệu mẫu</span></div>'+
 '<div class="checkline"><span class="check '+(!isSafeDest()?'wait':'')+'">'+(isSafeDest()?"✓":"○")+'</span><span>Tên bảng ClickHouse đích hợp lệ</span></div>'+
 '<div class="checkline"><span class="check '+(getIssues().length?'wait':'')+'">'+(!getIssues().length?"✓":"○")+'</span><span>Sẵn sàng tới Review</span></div>';
}
function renderFooter(){var step=state.step;document.getElementById("footer-controls").innerHTML='<div class="muted-copy">Bước '+(step+1)+' / 4 · Mọi thao tác ở đây là prototype.</div><div class="right-controls">'+(step>0?'<button class="btn btn-outline" type="button" data-action="back">← Quay lại</button>':'<button class="btn btn-outline" type="button" data-action="save">Lưu bản nháp</button>')+(step<3?'<button class="btn btn-primary" type="button" data-action="next">Tiếp tục →</button>':'<button class="btn btn-outline" type="button" data-action="export">Xuất cấu hình ⇩</button>')+'</div>';}
function render(){document.documentElement.classList.toggle("dark",state.theme==="dark");document.getElementById("theme-toggle").textContent=state.theme==="light"?"◐":"☀";renderStepper();renderDiagram();document.getElementById("stage").innerHTML=[renderSource,renderTransform,renderDestination,renderReview][state.step]();renderHelp();renderFooter();}
function changeTable(id){var t=tables.find(function(x){return x.id===id;});if(!t)return;state.table=id;state.selected=t.columns.map(function(x){return x[0];});state.mode=t.cursor?"full_then_incremental":"full_refresh";state.ops=[];state.mapping={};state.targetTable="dfqa_"+id.toLowerCase()+"_curated";state.lastRun=null;notify("Đã đổi sang Erp."+t.name+" · cấu hình Transform được làm mới.");render();}
function changeStep(n){state.step=Math.max(0,Math.min(3,n));render();document.querySelector(".stepper").scrollIntoView({behavior:"smooth",block:"start"});}
function doAction(action,v){
 if(action==="step"){changeStep(Number(v));return;}
 if(action==="table"){changeTable(v);return;}
 if(action==="theme"){state.theme=state.theme==="light"?"dark":"light";try{localStorage.setItem("df-ux-theme",state.theme);}catch(_){}render();return;}
 if(action==="next"){if(state.step===0&&!validateSource()){notify("Hãy chọn đủ unique key/cursor phù hợp trước khi tiếp tục.");return;}if(state.step===2&&!isSafeDest()){notify("Tên bảng đích hoặc mapping chưa hợp lệ.");return;}changeStep(state.step+1);return;}
 if(action==="back"){changeStep(state.step-1);return;}
 if(action==="add-op"){if(state.ops.length>=12){notify("Prototype giới hạn 12 bước Transform.");return;}state.ops.push(makeOp(v));state.lastRun=null;render();return;}
 if(action==="remove-op"){state.ops=state.ops.filter(function(o){return o.id!==v;});state.lastRun=null;render();return;}
 if(action==="move-op"){var parts=v.split(":"),i=state.ops.findIndex(function(o){return o.id===parts[0];}),j=i+Number(parts[1]);if(i>=0&&j>=0&&j<state.ops.length){var op=state.ops.splice(i,1)[0];state.ops.splice(j,0,op);}state.lastRun=null;render();return;}
 if(action==="json"){state.showJSON=!state.showJSON;render();return;}
 if(action==="save"){try{localStorage.setItem("df-ux-demo-draft",JSON.stringify(blueprint()));notify("Đã lưu cấu hình demo trên trình duyệt này. Không lưu trên server.");}catch(e){notify("Trình duyệt chặn local storage. Hãy chọn Xuất Blueprint JSON.");}return;}
 if(action==="reset"){if(!confirm("Đặt lại toàn bộ cấu hình prototype?"))return;if(runTimer)clearInterval(runTimer);var theme=state.theme;state=defaults();state.theme=theme;notify("Đã đặt lại prototype.");render();return;}
 if(action==="export"){var data=JSON.stringify(blueprint(),null,2),blob=new Blob([data],{type:"application/json"}),url=URL.createObjectURL(blob),a=document.createElement("a");a.href=url;a.download="dataflow-pipeline-ux-blueprint.json";document.body.appendChild(a);a.click();a.remove();setTimeout(function(){URL.revokeObjectURL(url);},800);notify("Đã xuất JSON (chỉ là bản cấu hình prototype).");return;}
 if(action==="simulate"){var issues=getIssues();if(issues.length){notify("Còn "+issues.length+" mục cần sửa trong cấu hình.");return;}if(runTimer)clearInterval(runTimer);state.lastRun={index:0,done:false};state.step=3;render();runTimer=setInterval(function(){if(!state.lastRun){clearInterval(runTimer);return;}state.lastRun.index++;if(state.lastRun.index>=4){state.lastRun.done=true;clearInterval(runTimer);notify("Mô phỏng hoàn tất · không ghi vào hệ thống thật.");}render();},680);return;}
}
document.addEventListener("click",function(e){var el=e.target.closest("[data-action]");if(!el)return;doAction(el.getAttribute("data-action"),el.getAttribute("data-value"));});
document.addEventListener("change",function(e){var el=e.target;
 if(el.hasAttribute("data-column")){var name=el.getAttribute("data-column"),t=getTable(),required=name===t.key||state.mode!=="full_refresh"&&name===t.cursor;if(required)return;state.selected=el.checked?state.selected.concat(name):state.selected.filter(function(x){return x!==name;});state.lastRun=null;render();return;}
 if(el.hasAttribute("data-sync-mode")){state.mode=el.value;var t=getTable();if(t.cursor&&state.mode!=="full_refresh"&&state.selected.indexOf(t.cursor)<0)state.selected.push(t.cursor);state.lastRun=null;render();return;}
 if(el.hasAttribute("data-op-enable")){var op=state.ops.find(function(o){return o.id===el.getAttribute("data-op-enable");});if(op)op.enabled=el.checked;state.lastRun=null;render();return;}
 if(el.hasAttribute("data-op-field")){var op=state.ops.find(function(o){return o.id===el.getAttribute("data-op-id");});if(op)op[el.getAttribute("data-op-field")]=el.value;state.lastRun=null;render();return;}
 if(el.hasAttribute("data-map-column")){state.mapping[el.getAttribute("data-map-column")]=el.value;state.lastRun=null;render();return;}
 if(el.hasAttribute("data-field")){var field=el.getAttribute("data-field");state[field]=el.value;state.lastRun=null;render();return;}
});
render();
})();