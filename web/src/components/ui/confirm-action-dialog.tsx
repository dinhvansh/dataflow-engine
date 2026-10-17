import {useEffect,useRef} from "react";
import {AlertTriangle,ShieldCheck} from "lucide-react";
import {Button} from "./button";

/** Native modal focus management + Signalroom-owned presentation. No dependency on browser confirm(). */
type Props={title:string;message:string;target:string;targetLabel?:string;confirmLabel:string;destructive?:boolean;onCancel:()=>void;onConfirm:()=>void};
export function ConfirmActionDialog({title,message,target,targetLabel="Scratch target · verify before proceeding",confirmLabel,destructive=false,onCancel,onConfirm}:Props){
 const dialog=useRef<HTMLDialogElement>(null);
 useEffect(()=>{
  const el=dialog.current;
  if(!el)return;
  if(!el.open)el.showModal();
  return()=>{if(el.open)el.close()};
 },[]);
 return <dialog ref={dialog} onCancel={e=>{e.preventDefault();onCancel()}} aria-label={title} className="df-action-dialog w-[min(92vw,480px)] rounded-md p-0">
  <div className="p-5 sm:p-6">
   <div className="flex items-start gap-3"><div className={`df-action-dialog-symbol flex h-10 w-10 shrink-0 items-center justify-center rounded-md ${destructive?"is-danger":""}`}>{destructive?<AlertTriangle size={20}/>:<ShieldCheck size={20}/>}</div>
    <div><h3 className="text-lg font-bold">{title}</h3><p className="muted mt-2 text-sm leading-relaxed">{message}</p></div></div>
   <div className="df-action-dialog-target mt-5 rounded-md border p-3"><span className="muted block text-[10px] font-semibold uppercase tracking-widest">{targetLabel}</span><code className="mt-1 block break-all font-mono text-xs">{target}</code></div>
   <div className="mt-6 flex flex-wrap justify-end gap-2"><Button type="button" variant="outline" autoFocus onClick={onCancel}>Cancel</Button>
    <Button type="button" variant={destructive?"destructive":"default"} onClick={onConfirm}>{confirmLabel}</Button></div>
  </div>
 </dialog>;
}
