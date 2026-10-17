import {useState} from "react";
import {CalendarClock,RefreshCw,Save} from "lucide-react";
import {Button} from "./ui/button";
import {scheduleApi,type ManagedStream,type ManagedSchedule} from "../lib/api";

export function SchedulePanel({token,streams}:{token:string;streams:ManagedStream[]}){
 const[streamID,setStreamID]=useState("");
 const[interval,setIntervalSeconds]=useState(300);
 const[enabled,setEnabled]=useState(false);
 const[schedules,setSchedules]=useState<ManagedSchedule[]>([]);
 const[busy,setBusy]=useState(false);
 const[error,setError]=useState("");
 const[notice,setNotice]=useState("");
 const fetchSchedules=async()=>{setBusy(true);setError("");try{setSchedules((await scheduleApi.list(token)).schedules)}catch(e){setError(String(e))}finally{setBusy(false)}};
 const save=async()=>{setBusy(true);setError("");setNotice("");
  try{
   if(!streamID)throw new Error("Choose a registered stream");
   const result=await scheduleApi.save({stream_id:streamID,interval_seconds:interval,enabled},token);
   setNotice(`Schedule saved: ${result.enabled?"Enabled":"Disabled"}`);
   setSchedules((await scheduleApi.list(token)).schedules);
  }catch(e){setError(String(e))}finally{setBusy(false)}
 };
 return <div className="df-studio-workspace df-studio-panel mt-5 rounded-lg border border-slate-700 bg-slate-800/40 p-4">
  <div className="mb-2 flex items-center gap-2 font-semibold"><CalendarClock size={18}/>Interval Scheduler (Opt-in)</div>
  <p className="muted mb-4 text-xs">Requires DATAFLOW_ENABLE_MANAGED_SCHEDULER=true on Go backend. No cron/DAG scheduling yet. Default: disabled.</p>
  <div className="grid gap-3 sm:grid-cols-3">
   <label className="space-y-1 sm:col-span-2"><span className="muted text-xs">Registered Stream</span>
    <select className="field" value={streamID} onChange={e=>setStreamID(e.target.value)}>
     <option value="">Select stream</option>{streams.map(s=><option key={s.id} value={s.id}>{s.source_schema}.{s.source_table} ({s.id.slice(0,8)})</option>)}
    </select>
   </label>
   <label className="space-y-1"><span className="muted text-xs">Every N seconds (60–86400)</span><input className="field" type="number" min={60} max={86400} value={interval} onChange={e=>setIntervalSeconds(Number(e.target.value))}/></label>
  </div>
  <label className="mt-3 flex items-center gap-2 text-sm"><input type="checkbox" checked={enabled} onChange={e=>setEnabled(e.target.checked)}/> Enable automatic trigger for this stream</label>
  <div className="mt-4 flex flex-wrap gap-2"><Button size="sm" disabled={busy||!token||!streamID||interval<60||interval>86400} onClick={save}><Save size={15}/>Save Schedule</Button><Button size="sm" variant="outline" disabled={busy||!token} onClick={fetchSchedules}><RefreshCw size={15}/>Refresh</Button></div>
  {notice&&<p className="mt-3 text-xs text-emerald-300">{notice}</p>}
  {error&&<p className="mt-3 text-xs text-red-300">{error}</p>}
  {schedules.length>0&&<div className="mt-4 overflow-x-auto"><table className="w-full min-w-[480px] text-left text-xs"><thead><tr className="border-b border-slate-700 text-slate-400"><th className="py-2">Stream ID</th><th>Frequency</th><th>Status</th><th>Next</th></tr></thead><tbody>{schedules.map(s=><tr className="border-b border-slate-800" key={s.id}><td className="py-2 font-mono">{s.stream_id.slice(0,8)}</td><td>{s.interval_seconds}s</td><td>{s.enabled?"Enabled":"Disabled"}</td><td>{new Date(s.next_run_at).toLocaleString()}</td></tr>)}</tbody></table></div>}
 </div>;
}
