export type RunResult={run_id:string;status:string;source?:string;target?:string;rows_written:number;batches_written:number;elapsed_seconds?:number;error?:string|null};export type SyncRequest={source_schema:string;source_table:string;target_database:string;target_table:string;batch_size:number;workers:number;queue_capacity:number};
async function parse<T>(res:Response):Promise<T>{if(!res.ok)throw new Error((await res.text()).slice(0,500)||`HTTP ${res.status}`);return res.json() as Promise<T>}
export const api={health:async()=>{const r=await fetch("/healthz");if(!r.ok)throw new Error("Service unavailable");return r.text()},run:(body:SyncRequest,token:string,signal?:AbortSignal)=>parse<RunResult>(fetch("/api/v1/bootstrap/runs",{method:"POST",headers:{"Authorization":`Bearer ${token}`,"Content-Type":"application/json"},body:JSON.stringify(body),signal}).then(r=>r)),status:(id:string,token:string)=>parse<RunResult>(fetch(`/api/v1/bootstrap/status?id=${encodeURIComponent(id)}`,{headers:{Authorization:`Bearer ${token}`}}))};
export type TableInfo={schema:string;name:string};
export type ColumnInfo={name:string;sql_type:string;max_length:number;precision:number;scale:number;nullable:boolean;identity:boolean;primary_key:boolean;cursor_candidate:boolean};
async function getSource<T>(path:string,token:string):Promise<T>{return parse<T>(await fetch(path,{headers:{Authorization:`Bearer ${token}`}}))}
export const sourceApi={
 tables:(schema:string,token:string)=>getSource<TableInfo[]>(`/api/v1/source/tables?schema=${encodeURIComponent(schema)}`,token),
 columns:(schema:string,table:string,token:string)=>getSource<ColumnInfo[]>(`/api/v1/source/columns?schema=${encodeURIComponent(schema)}&table=${encodeURIComponent(table)}`,token),
 rowversionUpper:(token:string)=>getSource<{upper_exclusive:string}>("/api/v1/source/rowversion-upper",token)
};

