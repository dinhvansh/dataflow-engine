import {useState,type ReactNode} from "react";
import {ArrowLeft,ArrowRight,Database,HardDrive,Plus,Settings2,Workflow} from "lucide-react";
import {Button} from "./ui/button";
import type {ReusedConnection} from "./QAConnectionBrowser";
import "./ConnectorHubShellV2.css";

type View="connections"|"new"|"sources"|"destinations"|"advanced";
type ProfileTab="sources"|"destinations";
export type ConnectorHubShellV2Props={
 sessionRole?:"admin"|"operator"|null;
 liveQAManager:ReactNode;
 onProfileTabChange:(tab:ProfileTab)=>void;
 sourceCatalog:ReactNode;
 destinationCatalog:ReactNode;
 sourceExplorer:ReactNode;
 wizard:(go:(tab:ProfileTab)=>void,reusePair:ReusedConnection|null)=>ReactNode;
 runningConnections:(selected:string,onSelect:(key:string)=>void,onAddStreams:(pair:ReusedConnection)=>void,onManageProfiles:()=>void,onCreateConnection:()=>void)=>ReactNode;
 configuredConnections:ReactNode;
 managedStreams:ReactNode;
 draftRegistry:ReactNode;
 routePreview:ReactNode;
 roadmap:ReactNode;
};

/** Keep the profile manager mounted while navigating so a draft or Test state survives. */
export function ConnectorHubShellV2(props:ConnectorHubShellV2Props){
 const[view,setView]=useState<View>("connections");
 const[reusePair,setReusePair]=useState<ReusedConnection|null>(null);
 const[selectedPair,setSelectedPair]=useState("");
 const canManage=props.sessionRole!=="operator";
 const openProfile=(tab:ProfileTab)=>{props.onProfileTabChange(tab);setView(tab)};
 const openNew=()=>{setReusePair(null);setSelectedPair("");setView("new")};
 const openAddStreams=(pair:ReusedConnection)=>{setReusePair(pair);setView("new")};
 const title=view==="new"?(reusePair?"Add streams":"New connection"):view==="sources"?"Sources":view==="destinations"?"Destinations":view==="advanced"?"QA tools":"Connections";
 return <div className="dfh-shell" aria-label="Connections">
  <div className="dfh-sidebar" aria-label="Điều hướng dữ liệu">
   <div className="dfh-sidebar-label">DATA MOVEMENT</div>
   <nav className="dfh-nav">
    <button type="button" className={view==="connections"||view==="new"?"dfh-nav-primary current":"dfh-nav-primary"} aria-current={view==="connections"||view==="new"?"page":undefined} onClick={()=>{setSelectedPair("");setView("connections")}}><Workflow size={19}/> Connections</button>
    <button type="button" className={view==="sources"?"current":""} aria-current={view==="sources"?"page":undefined} onClick={()=>openProfile("sources")}><Database size={17}/> Sources</button>
    <button type="button" className={view==="destinations"?"current":""} aria-current={view==="destinations"?"page":undefined} onClick={()=>openProfile("destinations")}><HardDrive size={17}/> Destinations</button>
   </nav>
   <div className="dfh-sidebar-bottom"><span className="dfh-env-dot"/> Local QA workspace</div>
   <button type="button" className={view==="advanced"?"dfh-tools current":"dfh-tools"} onClick={()=>setView("advanced")}><Settings2 size={16}/> QA tools</button>
  </div>
  <main className="dfh-main">
   {!(view==="connections"&&selectedPair)&&<header className="dfh-heading">
    <div>{view==="new"?<button className="dfh-back" type="button" onClick={()=>setView("connections")}><ArrowLeft size={15}/> Connections</button>:<p className="dfh-kicker">WORKSPACE / {title.toUpperCase()}</p>}
     <h2>{title}</h2><p>{view==="new"?"Chọn nguồn và đích, cấu hình streams, rồi tạo luồng đồng bộ.":view==="sources"?"Quản lý nguồn SQL Server đã lưu và kiểm tra kết nối.":view==="destinations"?"Quản lý đích ClickHouse đã lưu và kiểm tra quyền ghi.":view==="connections"?"Theo dõi và cấu hình luồng dữ liệu giữa source và destination.":"Công cụ kiểm tra QA và cấu hình cũ."}</p></div>
    {canManage&&view==="connections"&&<Button type="button" onClick={openNew}><Plus size={16}/> New connection</Button>}
   </header>}
   <div className="dfh-content" hidden={view!=="connections"}>
    {view==="connections"&&<section aria-label="Danh sách connections">{props.runningConnections(selectedPair,setSelectedPair,openAddStreams,()=>openProfile("sources"),openNew)}</section>}
    {!canManage&&<p className="dfh-access">Administrator quản lý profiles và tạo connections. Operator có thể xem và chạy streams đã được cấp quyền.</p>}
   </div>
   {canManage&&<div className="dfh-content dfh-create" hidden={view!=="new"}><section aria-label="Wizard tạo connection">{props.wizard(openProfile,reusePair)}</section></div>}
   {canManage&&<div className="dfh-content" hidden={view!=="sources"&&view!=="destinations"}>
    <div className="dfh-live-qa">{props.liveQAManager}</div>
    <div className="dfh-next"><Button type="button" variant="outline" onClick={()=>view==="sources"?openProfile("destinations"):openNew()}>{view==="sources"?"Next: Destinations":"Next: New connection"}<ArrowRight size={15}/></Button></div>
    {view==="sources"&&<details className="dfh-legacy"><summary>Khám phá SQL Server đã cấu hình</summary><div className="mt-4">{props.sourceExplorer}</div></details>}
   </div>}
   <div className="dfh-content" hidden={view!=="advanced"}>
    <details className="dfh-legacy"><summary>Toàn bộ managed streams · QA</summary><div className="mt-4">{props.managedStreams}</div></details>
    <section aria-label="Configured connections">{props.configuredConnections}</section>
    {canManage&&<section aria-label="Draft registry">{props.draftRegistry}</section>}
    <details className="dfh-legacy"><summary>Connector catalog và khả năng sắp tới</summary><div className="mt-4 space-y-5">{props.sourceCatalog}{props.destinationCatalog}</div></details>
    <section aria-label="Route preview">{props.routePreview}</section><section aria-label="Roadmap">{props.roadmap}</section>
   </div>
  </main>
 </div>;
}
