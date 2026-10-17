import {useState} from "react";
import {Database,RefreshCw,TableProperties,ShieldCheck,Search} from "lucide-react";
import {Button} from "./ui/button";
import {sourceApi,type TableInfo,type ColumnInfo} from "../lib/api";
type Props={token:string;onTokenChange:(value:string)=>void};
export function SourceExplorer({token,onTokenChange}:Props){
 const[schema,setSchema]=useState("Erp");
 const[table,setTable]=useState("PartTran");
 const[tables,setTables]=useState<TableInfo[]>([]);
 const[columns,setColumns]=useState<ColumnInfo[]>([]);
 const[tableFilter,setTableFilter]=useState("");
 const[columnFilter,setColumnFilter]=useState("");
 const matchingTables=tables.filter(t=>`${t.schema}.${t.name}`.toLowerCase().includes(tableFilter.trim().toLowerCase()));
 const matchingColumns=columns.filter(c=>`${c.name} ${c.sql_type}`.toLowerCase().includes(columnFilter.trim().toLowerCase()));
 const[upper,setUpper]=useState("");
 const[loading,setLoading]=useState(false);
 const[error,setError]=useState("");
 const action=async(fn:()=>Promise<void>)=>{setLoading(true);setError("");try{await fn()}catch(e){setError(String(e))}finally{setLoading(false)}};
 const list=()=>action(async()=>{setTables(await sourceApi.tables(schema,token));setColumns([])});
 const inspect=()=>action(async()=>{setColumns(await sourceApi.columns(schema,table,token));setColumnFilter("")});
 const watermark=()=>action(async()=>{setUpper((await sourceApi.rowversionUpper(token)).upper_exclusive)});
 return <div className="space-y-5">
  <div className="section space-y-4">
   <div className="flex items-center gap-2 text-lg font-semibold"><Database size={18}/>SQL Server Source Explorer</div>
   <p className="muted text-sm">Read-only metadata discovery. It never modifies Epicor tables, enables CDC, or saves credentials.</p>
   <div className="grid gap-3 sm:grid-cols-3">
    <div className="sm:col-span-3"><label className="muted mb-1 block text-xs">DataFlow API bearer token (memory only)</label><input aria-label="Source API token" className="field" type="password" autoComplete="off" value={token} onChange={e=>onTokenChange(e.target.value)}/></div>
    <div><label className="muted mb-1 block text-xs">SQL Schema</label><input aria-label="SQL Server schema" className="field" value={schema} onChange={e=>setSchema(e.target.value)} /></div>
    <div><label className="muted mb-1 block text-xs">Table</label><input aria-label="SQL Server table" className="field" value={table} onChange={e=>setTable(e.target.value)}/></div>
    <div className="flex items-end gap-2"><Button disabled={loading||!token} variant="outline" onClick={list}><RefreshCw size={15}/>List</Button><Button disabled={loading||!token} onClick={inspect}><TableProperties size={15}/>Inspect</Button></div>
   </div>
   <Button disabled={loading||!token} variant="outline" onClick={watermark}><ShieldCheck size={16}/>Read safe rowversion boundary</Button>
   {upper&&<div className="hub-source-meta rounded-md border px-3 py-2 text-sm">MIN_ACTIVE_ROWVERSION(): <code className="font-mono">{upper}</code><p className="muted mt-1 text-xs">This is a database-wide boundary, not a completed sync checkpoint.</p></div>}
   {error&&<p role="alert" className="hub-source-error rounded-md border p-3 text-sm">{error}</p>}
  </div>
  {tables.length>0&&<section className="section space-y-3" aria-label="Available SQL Server tables">
   <div className="flex flex-wrap items-center justify-between gap-2"><h3 className="font-semibold">Available tables ({tables.length})</h3><span className="muted text-xs">{matchingTables.length} shown</span></div>
   <div className="relative"><Search className="muted pointer-events-none absolute left-3 top-3" size={15}/><input className="field pl-9" type="search" aria-label="Filter available tables" value={tableFilter} onChange={e=>setTableFilter(e.target.value)} placeholder="Filter schema or table name"/></div>
   {matchingTables.length>0?<div className="flex max-h-56 flex-wrap gap-2 overflow-y-auto">{matchingTables.map(t=><button type="button" key={t.schema+"."+t.name} aria-pressed={schema===t.schema&&table===t.name} className={`hub-source-chip rounded border px-3 py-1.5 text-xs ${schema===t.schema&&table===t.name?"is-selected":""}`} onClick={()=>{setSchema(t.schema);setTable(t.name);setColumns([]);setColumnFilter("")}}>{t.name}</button>)}</div>
   :<p className="muted rounded-md border border-border p-4 text-center text-sm">No tables match this filter.</p>}
  </section>}
  {columns.length>0&&<section className="section min-w-0 space-y-3" aria-label="Inspected SQL Server columns">
   <div className="flex flex-wrap items-center justify-between gap-3"><h3 className="font-semibold">{schema}.{table} — {columns.length} columns</h3><span className="muted text-xs">{matchingColumns.length} shown</span></div>
   <div className="relative"><Search className="muted pointer-events-none absolute left-3 top-3" size={15}/><input className="field pl-9" type="search" aria-label="Filter inspected columns" value={columnFilter} onChange={e=>setColumnFilter(e.target.value)} placeholder="Filter column name or SQL type"/></div>
   {matchingColumns.length>0?<div className="max-h-[440px] overflow-auto rounded-md border border-border"><table className="w-full min-w-[550px] text-left text-sm"><thead className="hub-source-th sticky top-0 z-10"><tr className="border-b border-border text-xs"><th scope="col" className="p-3">Column</th><th scope="col" className="p-3">SQL Type</th><th scope="col" className="p-3">Nullable</th><th scope="col" className="p-3">PK</th><th scope="col" className="p-3">Cursor</th></tr></thead><tbody>{matchingColumns.map(c=><tr key={c.name} className="hub-source-tr border-b"><td className="hub-source-column p-3 font-mono">{c.name}</td><td className="p-3">{c.sql_type}</td><td className="p-3">{c.nullable?"Yes":"No"}</td><td className="p-3">{c.primary_key?"PK":"—"}</td><td className="p-3">{c.cursor_candidate?<span className="hub-source-cursor">rowversion</span>:"—"}</td></tr>)}</tbody></table></div>
   :<p className="muted rounded-md border border-border p-4 text-center text-sm">No columns match this filter.</p>}
  </section>}
 </div>;
}
