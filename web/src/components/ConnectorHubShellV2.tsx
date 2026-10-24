import {useState,type ReactNode} from "react";
import "./ConnectorHubShellV2.css";
type Tab="connections"|"sources"|"destinations";
type Jump=(tab:"sources"|"destinations")=>void;
export type ConnectorHubShellV2Props={
 sessionRole?:"admin"|"operator"|null;
 liveQAManager:ReactNode;
 sourceCatalog:ReactNode;
 destinationCatalog:ReactNode;
 sourceExplorer:ReactNode;
 wizard:(go:Jump)=>ReactNode;
 runningConnections:ReactNode;
 configuredConnections:ReactNode;
 draftRegistry:ReactNode;
 routePreview:ReactNode;
 roadmap:ReactNode;
};
/** All tab panels remain mounted; switching tabs preserves wizard state. */
export function ConnectorHubShellV2(props:ConnectorHubShellV2Props){
 const[tab,setTab]=useState<Tab>("sources");
 const go:Jump=next=>setTab(next);
 return <div className="dfh-shell" aria-label="Connector Hub">
  <header className="dfh-heading"><h2>Connector Hub</h2><p>Quản lý kết nối SQL Server → ClickHouse trong môi trường QA/lab.</p></header>
  {props.sessionRole!=="operator"&&<div className="dfh-live-qa">{props.liveQAManager}</div>}
  <div className="dfh-tabs" role="tablist" aria-label="Các phần kết nối">
   {([["connections","Connections"],["sources","Sources"],["destinations","Destinations"]] as const).map(([id,label])=>
    <button key={id} type="button" role="tab" id={"dfh-tab-"+id} aria-controls={"dfh-panel-"+id}
     aria-selected={tab===id} className={tab===id?"dfh-tab current":"dfh-tab"} onClick={()=>setTab(id)}>{label}</button>)}
  </div>
  <div id="dfh-panel-connections" className="dfh-content" role="tabpanel" aria-labelledby="dfh-tab-connections" hidden={tab!=="connections"}>
   {props.sessionRole!=="operator"&&<section aria-label="Wizard tạo connection">{props.wizard(go)}</section>}
   <section aria-label="Connections đang chạy">{props.runningConnections}</section>
   <section aria-label="Configured connections">{props.configuredConnections}</section>
   {props.sessionRole!=="operator"&&<section aria-label="Draft registry">{props.draftRegistry}</section>}
   <section aria-label="Route preview">{props.routePreview}</section>
   <section aria-label="Roadmap">{props.roadmap}</section>
  </div>
  <div id="dfh-panel-sources" className="dfh-content" role="tabpanel" aria-labelledby="dfh-tab-sources" hidden={tab!=="sources"}>
   <section aria-label="Source catalog">{props.sourceCatalog}</section>
   <section aria-label="SourceExplorer">{props.sourceExplorer}</section>
  </div>
  <div id="dfh-panel-destinations" className="dfh-content" role="tabpanel" aria-labelledby="dfh-tab-destinations" hidden={tab!=="destinations"}>
   <section aria-label="Destination catalog">{props.destinationCatalog}</section>
  </div>
 </div>;
}