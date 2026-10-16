import {useState} from "react";
import {Database,RefreshCw,TableProperties,ShieldCheck} from "lucide-react";
import {Button} from "./ui/button";
import {sourceApi,type TableInfo,type ColumnInfo} from "../lib/api";
type Props={token:string;onTokenChange:(value:string)=>void};
export function SourceExplorer({token,onTokenChange}:Props){
 const[schema,setSchema]=useState("Erp");
 const[table,setTable]=useState("PartTran");
 const[tables,setTables]=useState<TableInfo[]>([]);
 const[columns,setColumns]=useState<ColumnInfo[]>([]);
 const[upper,setUpper]=useState("");
 const[loading,setLoading]=useState(false);
 const[error,setError]=useState("");
 const action=async(fn:()=>Promise<void>)=>{setLoading(true);setError("");try{await fn()}catch(e){setError(String(e))}finally{setLoading(false)}};
 const list=()=>action(async()=>{setTables(await sourceApi.tables(schema,token));setColumns([])});
 const inspect=()=>action(async()=>{setColumns(await sourceApi.columns(schema,table,token))});
 const watermark=()=>action(async()=>{setUpper((await sourceApi.rowversionUpper(token)).upper_exclusive)});
 return <div className="space-y-5">
  <div className="section space-y-4">
   <div className="flex items-center gap-2 text-lg font-semibold"><Database size={18}/>SQL Server Source Explorer</div>
   <p className="muted text-sm">Read-only metadata discovery. It never modifies Epicor tables, enables CDC, or saves credentials.</p>
   <div className="grid gap-3 sm:grid-cols-3">
    <div className="sm:col-span-3"><label className="mb-1 block text-xs text-slate-400">DataFlow API bearer token (memory only)</label><input className="field" type="password" autoComplete="off" value={token} onChange={e=>onTokenChange(e.target.value)}/></div>
    <div><label className="mb-1 block text-xs text-slate-400">SQL Schema</label><input className="field" value={schema} onChange={e=>setSchema(e.target.value)} /></div>
    <div><label className="mb-1 block text-xs text-slate-400">Table</label><input className="field" value={table} onChange={e=>setTable(e.target.value)}/></div>
    <div className="flex items-end gap-2"><Button disabled={loading||!token} variant="outline" onClick={list}><RefreshCw size={15}/>List</Button><Button disabled={loading||!token} onClick={inspect}><TableProperties size={15}/>Inspect</Button></div>
   </div>
   <Button disabled={loading||!token} variant="outline" onClick={watermark}><ShieldCheck size={16}/>Read safe rowversion boundary</Button>
   {upper&&<div className="rounded-lg border border-sky-700/40 bg-sky-500/10 px-3 py-2 text-sm">MIN_ACTIVE_ROWVERSION(): <code className="font-mono">{upper}</code><p className="muted mt-1 text-xs">This is a database-wide boundary, not a completed sync checkpoint.</p></div>}
   {error&&<p role="alert" className="rounded-md bg-red-950 p-3 text-sm text-red-300">{error}</p>}
  </div>
  {tables.length>0&&<div className="section"><h3 className="mb-3 font-semibold">Available tables ({tables.length})</h3><div className="flex max-h-56 flex-wrap gap-2 overflow-y-auto">{tables.map(t=><button key={t.schema+"."+t.name} className="rounded-md border border-slate-700 bg-slate-800 px-3 py-1.5 text-xs hover:border-sky-500" onClick={()=>{setSchema(t.schema);setTable(t.name);setColumns([])}}>{t.name}</button>)}</div></div>}
  {columns.length>0&&<div className="section overflow-x-auto"><h3 className="mb-3 font-semibold">{schema}.{table} — {columns.length} columns</h3><table className="w-full min-w-[550px] text-left text-sm"><thead><tr className="border-b border-slate-700 text-xs text-slate-400"><th className="py-2">Column</th><th>SQL Type</th><th>Nullable</th><th>PK</th><th>Cursor</th></tr></thead><tbody>{columns.map(c=><tr key={c.name} className="border-b border-slate-800"><td className="py-2 font-mono text-sky-200">{c.name}</td><td>{c.sql_type}</td><td>{c.nullable?"Yes":"No"}</td><td>{c.primary_key?"PK":"—"}</td><td>{c.cursor_candidate?<span className="text-emerald-400">rowversion</span>:"—"}</td></tr>)}</tbody></table></div>}
 </div>;
}
