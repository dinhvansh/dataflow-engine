export type TransformKind="filter"|"select"|"rename"|"calculate"|"aggregate"|"sort"|"limit";
export type TransformMode="visual"|"sql";
export type TransformColumn={name:string;type:string};
export type TransformStep={
 id:string;type:TransformKind;enabled?:boolean;
 column?:string;columns?:string[];operator?:string;value?:string;value_type?:string;
 alias?:string;formula_operator?:string;group_by?:string[];
 function?:string;direction?:string;limit?:number;
};
export type TransformModel={
 name:string;mode:TransformMode;
 source:{database:string;table:string};
 steps:TransformStep[];sql?:string;
};
export type CompileResult={sql:string;steps_sql:string[]};
export type PreviewResult={sql:string;rows:number;data:Record<string,unknown>[];meta:TransformColumn[]};
export type LineageNode={id:string;label:string;kind:"source"|"model"|"missing";status:string;target?:string};
export type LineageEdge={from:string;to:string};
export type LineageProblem={model:string;code:string;detail:string};
export type LineageGraph={nodes:LineageNode[];edges:LineageEdge[];model_order:string[];problems:LineageProblem[];impact?:string[]};
export type AutoBuildStep={index:number;model_name:string;status:string;dbt_run_id:string;rows_verified:number;error:string};
export type AutoBuildRun={run_id:string;status:string;target_model:string;current_step?:number;step_count?:number;order?:string[];error?:string;steps?:AutoBuildStep[]};
export type TableRun={run_id:string;state:string;model_name?:string;stage?:string;target?:string;rows?:number;error?:string;has_backup?:boolean;attempts?:number;log_tail?:string};
export type DbtRun={run_id:string;status:string;model_name?:string;target?:string;materialization?:string;attempts?:number;error?:string;log_tail?:string;run_results?:unknown};
export type SavedTransform={id:string;name:string;model:TransformModel;sql:string;updated_at:string};

async function request<T>(url:string,token:string,method:"GET"|"POST",body?:unknown):Promise<T>{
 const response=await fetch(url,{method,headers:{"Authorization":"Bearer "+token,...(body!==undefined?{"Content-Type":"application/json"}:{})},body:body===undefined?undefined:JSON.stringify(body)});
 if(!response.ok){throw new Error((await response.text()).slice(0,600)||"HTTP "+response.status)}
 return response.json() as Promise<T>;
}
export const transformApi={
 compile:(model:TransformModel,token:string)=>request<CompileResult>("/api/v1/transform/compile",token,"POST",{model}),
 preview:(model:TransformModel,token:string,stepIndex?:number,limit=50)=>request<PreviewResult>("/api/v1/transform/preview",token,"POST",{model,limit,...(stepIndex!==undefined?{step_index:stepIndex}:{})}),
 columns:(source:TransformModel["source"],token:string)=>request<{columns:TransformColumn[]}>("/api/v1/transform/columns",token,"POST",source),
 models:(token:string)=>request<{models:SavedTransform[]}>("/api/v1/transform/models",token,"GET"),
 save:(model:TransformModel,token:string)=>request<SavedTransform>("/api/v1/transform/models",token,"POST",{model}),
 dbtRun:(model_id:string,token:string)=>request<DbtRun>("/api/v1/transform/dbt/runs",token,"POST",{model_id,materialization:"view"}),
 dbtStatus:(id:string,token:string)=>request<DbtRun>("/api/v1/transform/dbt/runs?id="+encodeURIComponent(id),token,"GET"),
 dbtHistory:(token:string)=>request<{runs:DbtRun[]}>("/api/v1/transform/dbt/runs",token,"GET"),
 lineage:(token:string,impact?:string)=>request<LineageGraph>("/api/v1/transform/lineage"+(impact?"?impact="+encodeURIComponent(impact):""),token,"GET"),
 tableStage:(model_id:string,token:string,unique_column:string)=>request<TableRun>("/api/v1/transform/table/runs",token,"POST",{model_id,unique_column}),
 tableStatus:(id:string,token:string)=>request<{runs:TableRun[]}>("/api/v1/transform/table/runs?id="+encodeURIComponent(id),token,"GET"),
 tableAction:(run_id:string,action:"publish"|"rollback",token:string)=>request<TableRun>("/api/v1/transform/table/action",token,"POST",{run_id,action,confirm:action.toUpperCase()}),
 autoBuild:(model_id:string,token:string)=>request<AutoBuildRun>("/api/v1/transform/auto-build",token,"POST",{model_id}),
 autoBuildStatus:(run_id:string,token:string)=>request<AutoBuildRun>("/api/v1/transform/auto-build?id="+encodeURIComponent(run_id),token,"GET")
};
