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
 save:(model:TransformModel,token:string)=>request<SavedTransform>("/api/v1/transform/models",token,"POST",{model})
};
