export type RunResult={run_id:string;status:string;source?:string;target?:string;rows_written:number;batches_written:number;elapsed_seconds?:number;upper_exclusive?:string;error?:string|null};export type SyncRequest={source_schema:string;source_table:string;target_database:string;target_table:string;batch_size:number;workers:number;queue_capacity:number;mode?:"full"|"rowversion";lower_cursor?:string;atomic_replace?:boolean};
async function parse<T>(response:Response|Promise<Response>):Promise<T>{const res=await response;if(!res.ok)throw new Error((await res.text()).slice(0,500)||`HTTP ${res.status}`);return res.json() as Promise<T>}
export const api={health:async()=>{const r=await fetch("/healthz");if(!r.ok)throw new Error("Service unavailable");return r.text()},run:(body:SyncRequest,token:string,signal?:AbortSignal)=>parse<RunResult>(fetch("/api/v1/bootstrap/runs",{method:"POST",headers:{"Authorization":`Bearer ${token}`,"Content-Type":"application/json"},body:JSON.stringify(body),signal}).then(r=>r)),status:(id:string,token:string)=>parse<RunResult>(fetch(`/api/v1/bootstrap/status?id=${encodeURIComponent(id)}`,{headers:{Authorization:`Bearer ${token}`}}))};
export type TableInfo={schema:string;name:string};
export type ColumnInfo={name:string;sql_type:string;max_length:number;precision:number;scale:number;nullable:boolean;identity:boolean;primary_key:boolean;cursor_candidate:boolean};
async function getSource<T>(path:string,token:string):Promise<T>{return parse<T>(await fetch(path,{headers:{Authorization:`Bearer ${token}`}}))}
export const sourceApi={
 tables:(schema:string,token:string)=>getSource<TableInfo[]>(`/api/v1/source/tables?schema=${encodeURIComponent(schema)}`,token),
 columns:(schema:string,table:string,token:string)=>getSource<ColumnInfo[]>(`/api/v1/source/columns?schema=${encodeURIComponent(schema)}&table=${encodeURIComponent(table)}`,token),
 rowversionUpper:(token:string)=>getSource<{upper_exclusive:string}>("/api/v1/source/rowversion-upper",token)
};


export type PipelineNode={id:string;job_id:string;requires:string[]};
export type PipelinePlan={nodes:PipelineNode[]};
export type PipelineValidation={valid:boolean;nodes?:number;ready_on_start?:string[];status?:string;error?:string};
export async function validatePipeline(plan:PipelinePlan,token:string):Promise<PipelineValidation>{
 const r=await fetch("/api/v1/pipelines/validate",{method:"POST",headers:{"Authorization":`Bearer ${token}`,"Content-Type":"application/json"},body:JSON.stringify(plan)});
 if(r.status===422)return r.json() as Promise<PipelineValidation>;
 return parse<PipelineValidation>(r);
}

export type HistoryRun={run_id:string;status:string;source:string;target:string;rows_written:number;batches_written:number;error?:string;created_at:string;finished_at?:string};
export type HistoryResponse={runs:HistoryRun[];count:number};
export async function getRunHistory(token:string,limit=20):Promise<HistoryResponse>{
 return parse<HistoryResponse>(await fetch(`/api/v1/bootstrap/history?limit=${limit}`,{headers:{Authorization:`Bearer ${token}`}}))
}

export type ManagedStream={
 id:string;source_schema:string;source_table:string;
 target_database:string;target_table:string;
 batch_size:number;workers:number;queue_capacity:number;lower_cursor?:string
};
export type ManagedRun={
 run_id:string;stream_id?:string;status:string;lower_cursor?:string;
 upper_exclusive?:string;rows_written?:number;batches_written?:number;error?:string
};
export const managedApi={
 list:(token:string)=>getSource<{streams:ManagedStream[]}>("/api/v1/managed/streams",token),
 create:(settings:Omit<ManagedStream,"id"|"lower_cursor">,token:string)=>
  parse<ManagedStream>(fetch("/api/v1/managed/streams",{method:"POST",headers:{Authorization:`Bearer ${token}`,"Content-Type":"application/json"},body:JSON.stringify(settings)})),
 run:(stream_id:string,token:string)=>
  parse<ManagedRun>(fetch("/api/v1/managed/runs",{method:"POST",headers:{Authorization:`Bearer ${token}`,"Content-Type":"application/json"},body:JSON.stringify({stream_id})})),
 status:(id:string,token:string)=>getSource<ManagedRun>(`/api/v1/managed/runs?id=${encodeURIComponent(id)}`,token)
};

export type ManagedSchedule={id:string;stream_id:string;interval_seconds:number;enabled:boolean;next_run_at:string};
export const scheduleApi={
 list:(token:string)=>getSource<{schedules:ManagedSchedule[]}>("/api/v1/managed/schedules",token),
 save:(cfg:{stream_id:string;interval_seconds:number;enabled:boolean},token:string)=>
  parse<ManagedSchedule>(fetch("/api/v1/managed/schedules",{method:"POST",headers:{Authorization:`Bearer ${token}`,"Content-Type":"application/json"},body:JSON.stringify(cfg)}))
};
