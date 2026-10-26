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
 batch_size:number;workers:number;queue_capacity:number;selected_columns?:string[];lower_cursor?:string
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

export type ReconciliationRun={
 run_id:string;stream_id:string;status:string;previous_snapshot_id?:string;
 scanned_rows:number;verified_rows:number;
 added_keys:number|null;deleted_keys:number|null;changed_versions:number|null;
 error?:string;detection_only?:boolean
};
export const reconciliationApi={
 list:(token:string)=>getSource<{runs:ReconciliationRun[]}>("/api/v1/reconcile/runs",token),
 run:(stream_id:string,token:string)=>parse<ReconciliationRun>(fetch("/api/v1/reconcile/runs",{
  method:"POST",headers:{Authorization:`Bearer ${token}`,"Content-Type":"application/json"},
  body:JSON.stringify({stream_id})
 })),
 status:(runID:string,token:string)=>getSource<ReconciliationRun>(`/api/v1/reconcile/runs?id=${encodeURIComponent(runID)}`,token)
};

export type SQLJob={id:string;name:string;database:string;query:string;enabled:boolean};
export type ExecutableDAG={id:string;name:string;enabled:boolean;nodes:PipelineNode[]};
export type DAGRun={run_id:string;dag_id?:string;status:string;result?:{state:Record<string,string>;attempts:Record<string,number>;errors:Record<string,string>};error?:string};
export const sqlDAGApi={
 jobs:(token:string)=>getSource<{jobs:SQLJob[]}>("/api/v1/sql/jobs",token),
 saveJob:(job:Omit<SQLJob,"id">,token:string)=>parse<SQLJob>(fetch("/api/v1/sql/jobs",{method:"POST",headers:{"Authorization":`Bearer ${token}`,"Content-Type":"application/json"},body:JSON.stringify(job)})),
 dags:(token:string)=>getSource<{pipelines:ExecutableDAG[]}>("/api/v1/sql/dags",token),
 saveDAG:(dag:Omit<ExecutableDAG,"id">,token:string)=>parse<ExecutableDAG>(fetch("/api/v1/sql/dags",{method:"POST",headers:{"Authorization":`Bearer ${token}`,"Content-Type":"application/json"},body:JSON.stringify(dag)})),
 start:(dag_id:string,token:string)=>parse<DAGRun>(fetch("/api/v1/sql/dag-runs",{method:"POST",headers:{"Authorization":`Bearer ${token}`,"Content-Type":"application/json"},body:JSON.stringify({dag_id})})),
 run:(runID:string,token:string)=>getSource<DAGRun>(`/api/v1/sql/dag-runs?id=${encodeURIComponent(runID)}`,token),
 history:(token:string)=>getSource<{runs:DAGRun[]}>("/api/v1/sql/dag-runs",token)
};

export type ConfiguredConnection={
 id:string;name:string;kind:"source"|"destination"|"internal";
 driver:"sqlserver"|"clickhouse"|"postgresql";
 usage:string;configuration_state:"configured"|"not_configured";
 management_mode:"server_environment";supports_multiple_instances:boolean;
 supports_test_connection:boolean;
};
export const connectionApi={
 list:(token:string)=>getSource<{connections:ConfiguredConnection[]}>("/api/v1/connections",token),
 testConfigured:(id:string,token:string)=>parse<{id:string;passed:boolean;environment:string;tested_at:string}>(fetch("/api/v1/connections/test-configured",{method:"POST",headers:{"Authorization":`Bearer ${token}`,"Content-Type":"application/json"},body:JSON.stringify({id})}))
};

export type DraftConnection={
 id:string;name:string;driver:"sqlserver"|"clickhouse";kind:"source"|"destination";
 workspace:"lab";status:"draft";revision:number;created_at:string;
};
type DraftConnectionInput={name:string;driver:"sqlserver"|"clickhouse";kind:"source"|"destination"};
const registryHeaders=(token:string,adminToken:string)=>({
 "Authorization":"Bearer "+token,...(adminToken?{"X-DataFlow-Connection-Admin":adminToken}:{})
});
export const connectionDefinitionsApi={
 list:(token:string,adminToken:string)=>parse<{definitions:DraftConnection[];write_enabled:boolean}>(
  fetch("/api/v1/connections/definitions",{headers:registryHeaders(token,adminToken)})),
 create:(draft:DraftConnectionInput,token:string,adminToken:string)=>parse<DraftConnection>(
  fetch("/api/v1/connections/definitions",{method:"POST",
   headers:{...registryHeaders(token,adminToken),"Content-Type":"application/json"},
   body:JSON.stringify(draft)}))
};

export type CoreGraphNode={
 id:string;type:"SOURCE"|"SYNC"|"TRANSFORM"|"QUALITY"|"PUBLISH";
 requires:string[];stream_id?:string;model_id?:string;unique_column?:string;
 x?:number;y?:number;
};
export type CoreGraphDefinition={nodes:CoreGraphNode[]};
export type SavedCoreGraph={
 id:string;name:string;version:number;graph:CoreGraphDefinition;
 graph_digest:string;plan_digest:string;
 stream_id:string;model_id:string;unique_column:string;
};
export type ValidatedCoreGraph={
 valid:true;digest:string;model_plan_digest:string;
 stream_id:string;model_id:string;unique_column:string;
};
export const coreGraphApi={
 list:(token:string)=>getSource<{graphs:SavedCoreGraph[]}>("/api/v1/core/graphs",token),
 validate:(name:string,graph:CoreGraphDefinition,token:string)=>
  parse<ValidatedCoreGraph>(fetch("/api/v1/core/graphs",{
   method:"POST",headers:{"Authorization":`Bearer ${token}`,"Content-Type":"application/json"},
   body:JSON.stringify({action:"validate",name,graph,expected_version:0})
  })),
 save:(name:string,graph:CoreGraphDefinition,expected_version:number,token:string)=>
  parse<SavedCoreGraph>(fetch("/api/v1/core/graphs",{
   method:"POST",headers:{"Authorization":`Bearer ${token}`,"Content-Type":"application/json"},
   body:JSON.stringify({action:"save",name,graph,expected_version})
  }))
};

export type CoreStepObservation={index:number;model_name:string;status:string;rows_verified:number;error:string};
export type CorePipelineRun={
 run_id:string;status:"waiting_sync"|"waiting_flow"|"publishing"|"succeeded"|"failed"|"rolled_back";
 stream_id?:string;target_model_name?:string;
 managed_run_id:string;managed_status?:string;model_flow_id?:string;
 model_flow_status?:string;rows_verified?:number;stage?:string;target?:string;
 error?:string;has_backup?:boolean;dependency_order?:string[];steps?:CoreStepObservation[];
 graph_id?:string;graph_version?:number;graph_digest?:string;
 graph_snapshot?:CoreGraphDefinition|null;
};
export const corePipelineApi={
 start:(stream_id:string,model_id:string,unique_column:string,token:string,graph?:SavedCoreGraph)=>
  parse<CorePipelineRun>(fetch("/api/v1/core/runs",{
   method:"POST",
   headers:{"Authorization":`Bearer ${token}`,"Content-Type":"application/json"},
   body:JSON.stringify({stream_id,model_id,unique_column,...(graph?{graph_id:graph.id,graph_version:graph.version}:{})})
  })),
 status:(run_id:string,token:string)=>
  getSource<CorePipelineRun>(`/api/v1/core/runs?id=${encodeURIComponent(run_id)}`,token),
 rollback:(run_id:string,token:string)=>
  parse<{run_id:string;status:"rolled_back";target:string;checkpoint_changed:boolean}>(
   fetch("/api/v1/core/actions",{
    method:"POST",
    headers:{"Authorization":`Bearer ${token}`,"Content-Type":"application/json"},
    body:JSON.stringify({run_id,action:"rollback",confirm:"ROLLBACK"})
   }))
};

export type LabRuntimeConnection={
 id:string;name:string;driver:"sqlserver"|"clickhouse";kind:"source"|"destination";
 host:string;port:number;database:string;username:string;status:"draft"|"verified"|"revoked";
 revision:number;checked_at?:string
};
export type LabRuntimeInput={
 name:string;driver:"sqlserver"|"clickhouse";host:string;port:number;
 database:string;username:string;password:string
};
const runtimeHeaders=(token:string,adminToken:string)=>({
 "Authorization":`Bearer ${token}`,"X-DataFlow-Connection-Admin":adminToken,"Content-Type":"application/json"
});
export type LabStreamSetup={
 source_connection_id:string;target_connection_id:string;
 source_schema:string;source_table:string;selected_columns:string[];
 target_database:"scratch";target_table:"dataflow_raw_events";
 mode:"incremental_rowversion";batch_size:number;workers:number;queue_capacity:number
};
export const labSyncSetupApi={
 tables:(sourceID:string,schema:string,token:string,adminToken:string)=>
  parse<{tables:TableInfo[]}>(fetch("/api/v1/connections/runtime/discover?source_connection_id="+encodeURIComponent(sourceID)+"&schema="+encodeURIComponent(schema),{
   headers:runtimeHeaders(token,adminToken)})),
 columns:(sourceID:string,schema:string,table:string,token:string,adminToken:string)=>
  parse<{columns:ColumnInfo[]}>(fetch("/api/v1/connections/runtime/discover?source_connection_id="+encodeURIComponent(sourceID)+"&schema="+encodeURIComponent(schema)+"&table="+encodeURIComponent(table),{
   headers:runtimeHeaders(token,adminToken)})),
 create:(config:LabStreamSetup,token:string,adminToken:string)=>
  parse<{stream_id:string;bound:boolean;mode:string}>(fetch("/api/v1/connections/runtime/streams",{
   method:"POST",headers:runtimeHeaders(token,adminToken),body:JSON.stringify(config)}))
};
export type QABatchStream={
 source_schema:string;source_table:string;selected_columns:string[];
 mode:"incremental_rowversion"|"full_refresh"|"full_then_incremental";
 target_database:string;target_table:string;
 destination_layout?:"raw_event"|"typed";
 column_mappings?:{source:string;target:string}[];
 source_key_column?:string;source_cursor_column?:string;
 batch_size:number;workers:number;queue_capacity:number
};
export type QABatchResult={stream_id:string;source:string;mode:string;destination:string};
export const labQABatchSyncApi={
 create:(request:{source_connection_id:string;target_connection_id:string;streams:QABatchStream[]},token:string,adminToken:string)=>
  parse<{streams:QABatchResult[];count:number}>(fetch("/api/v1/connections/runtime/streams/batch",{
   method:"POST",headers:runtimeHeaders(token,adminToken),body:JSON.stringify(request)}))
};
export const labRuntimeConnectionsApi={
 list:(token:string,adminToken:string)=>parse<{profiles:LabRuntimeConnection[]}>(
  fetch("/api/v1/connections/runtime",{headers:runtimeHeaders(token,adminToken)})),
 create:(payload:LabRuntimeInput,token:string,adminToken:string)=>parse<LabRuntimeConnection>(
  fetch("/api/v1/connections/runtime",{method:"POST",headers:runtimeHeaders(token,adminToken),body:JSON.stringify(payload)})),
 update:(id:string,revision:number,payload:Omit<LabRuntimeInput,"driver">,token:string,adminToken:string)=>parse<LabRuntimeConnection>(
  fetch("/api/v1/connections/runtime",{method:"PATCH",headers:runtimeHeaders(token,adminToken),body:JSON.stringify({id,revision,...payload})})),
 revoke:async(id:string,revision:number,token:string,adminToken:string)=>{
  const response=await fetch("/api/v1/connections/runtime",{method:"DELETE",headers:runtimeHeaders(token,adminToken),body:JSON.stringify({id,revision})});
  if(!response.ok)throw Error((await response.text()).slice(0,500)||`HTTP ${response.status}`);
 },
 test:(id:string,token:string,adminToken:string)=>parse<{id:string;passed:boolean;tested_at:string}>(
  fetch("/api/v1/connections/runtime/test",{method:"POST",headers:runtimeHeaders(token,adminToken),body:JSON.stringify({id})})),
 bind:(stream_id:string,source_connection_id:string,target_connection_id:string,token:string,adminToken:string)=>parse<{bound:boolean;stream_id:string}>(
  fetch("/api/v1/connections/runtime/bind",{method:"POST",headers:runtimeHeaders(token,adminToken),
    body:JSON.stringify({stream_id,source_connection_id,target_connection_id})}))
};
