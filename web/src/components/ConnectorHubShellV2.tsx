import {useState,type ReactNode} from "react";
import {ArrowLeft,Plus} from "lucide-react";
import {Button} from "./ui/button";
import type {ReusedConnection} from "./QAConnectionBrowser";
import "./ConnectorHubShellV2.css";

type View="connections"|"new"|"profiles"|"sources"|"destinations"|"advanced";
type Jump=(tab:"sources"|"destinations")=>void;
export type ConnectorHubShellV2Props={
 sessionRole?:"admin"|"operator"|null;
 liveQAManager:ReactNode;
 sourceCatalog:ReactNode;
 destinationCatalog:ReactNode;
 sourceExplorer:ReactNode;
 wizard:(go:Jump,reusePair:ReusedConnection|null)=>ReactNode;
 runningConnections:(selected:string,onSelect:(key:string)=>void,onAddStreams:(pair:ReusedConnection)=>void,onManageProfiles:()=>void)=>ReactNode;
 configuredConnections:ReactNode;
 draftRegistry:ReactNode;
 routePreview:ReactNode;
 roadmap:ReactNode;
};

/** Keep the manager and wizard mounted so verified profiles and the draft survive navigation. */
export function ConnectorHubShellV2(props:ConnectorHubShellV2Props){
 const[view,setView]=useState<View>("connections");
 const[reusePair,setReusePair]=useState<ReusedConnection|null>(null);
 const[selectedPair,setSelectedPair]=useState("");
 const canManage=props.sessionRole!=="operator";
 const go:Jump=()=>setView("profiles");
 const nav=(next:View)=>setView(next);
 const openNew=()=>{setReusePair(null);setSelectedPair("");setView("new")};
 const openAddStreams=(pair:ReusedConnection)=>{setReusePair(pair);setView("new")};
 return <div className="dfh-shell" aria-label="Connections">
  <header className="dfh-heading">
   <div><h2>{view==="new"?(reusePair?"Thêm stream":"New connection"):view==="profiles"?"Connection profiles":view==="sources"?"Sources":view==="destinations"?"Destinations":view==="advanced"?"QA tools":"Connections"}</h2>
    <p>{view==="new"?"Thiết lập nguồn, đích, streams và lịch chạy.":"Đồng bộ dữ liệu SQL Server → ClickHouse trong môi trường QA."}</p></div>
   {view==="connections"&&canManage&&<Button type="button" onClick={openNew}><Plus size={16}/> New connection</Button>}
  </header>
  <nav className="dfh-nav" aria-label="Điều hướng kết nối">
   {([ ["connections","Connections"],["sources","Sources"],["destinations","Destinations"]] as const).map(([id,label])=><button type="button" key={id} aria-label={id==="connections"?"All connections":undefined} className={view===id?"current":""} aria-current={view===id?"page":undefined} onClick={()=>nav(id)}>{label}</button>)}
   {canManage&&<button type="button" className={view==="profiles"?"current":""} aria-current={view==="profiles"?"page":undefined} onClick={()=>nav("profiles")}>Profiles</button>}
  </nav>
  {view==="new"&&<div className="dfh-breadcrumb"><button type="button" onClick={()=>nav("connections")}><ArrowLeft size={15}/> Connections</button><span>/</span><span>{reusePair?"Thêm stream":"New connection"}</span></div>}
  <div className="dfh-content" hidden={view!=="connections"}>
   {view==="connections"&&<section aria-label="Danh sách connections">{props.runningConnections(selectedPair,setSelectedPair,openAddStreams,()=>nav("profiles"))}</section>}
   {!canManage&&<p className="dfh-access">Bạn đang ở vai trò Operator. Administrator quản lý và tạo QA connections; bạn có thể xem streams đã cấu hình.</p>}
  </div>
  {canManage&&<div className="dfh-content dfh-create" hidden={view!=="new"}><section aria-label="Wizard tạo connection">{props.wizard(go,reusePair)}</section></div>}
  {canManage&&<div className="dfh-content" hidden={view!=="profiles"}>
   <p className="dfh-help">Tạo hoặc kiểm tra profile SQL Server và ClickHouse tại đây. Sau khi Test thành công, quay lại New connection để chọn profile.</p>
   <div className="dfh-live-qa">{props.liveQAManager}</div>
   <Button type="button" variant="outline" onClick={()=>nav("new")}>Quay lại New connection</Button>
  </div>}
  <div className="dfh-content" hidden={view!=="sources"}>
   {view==="sources"&&<section aria-label="Source catalog">{props.sourceCatalog}</section>}
   {view==="sources"&&<section aria-label="Source explorer">{props.sourceExplorer}</section>}
  </div>
  <div className="dfh-content" hidden={view!=="destinations"}>
   {view==="destinations"&&<section aria-label="Destination catalog">{props.destinationCatalog}</section>}
  </div>
  <div className="dfh-content" hidden={view!=="advanced"}>
   <section aria-label="Configured connections">{props.configuredConnections}</section>
   {canManage&&<section aria-label="Draft registry">{props.draftRegistry}</section>}
   <section aria-label="Route preview">{props.routePreview}</section>
   <section aria-label="Roadmap">{props.roadmap}</section>
  </div>
  <footer className="dfh-footer"><button type="button" onClick={()=>nav("advanced")}>QA details &amp; legacy tools</button></footer>
 </div>;
}
