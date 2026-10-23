import {useState,type FormEvent} from "react";
import {LockKeyhole,ShieldCheck} from "lucide-react";
import {Button} from "./ui/button";

export type ConsoleRole="operator"|"admin";
export async function currentConsoleSession():Promise<{mode:"local"|"legacy";role:ConsoleRole|null}>{
 const response=await fetch("/api/v1/auth/session",{cache:"no-store",credentials:"same-origin"});
 if(response.status===404)return {mode:"legacy",role:null};
 if(!response.ok)throw Error("Unable to check sign-in availability");
 const data=await response.json() as {authenticated:boolean;role:ConsoleRole};
 return {mode:"local",role:data.authenticated?data.role:null};
}
export async function signOutConsole():Promise<void>{
 const response=await fetch("/api/v1/auth/session",{method:"DELETE",credentials:"same-origin"});
 if(!response.ok)throw Error("Unable to terminate session");
}
export function SessionLogin({onSignedIn}:{onSignedIn:(role:ConsoleRole)=>void}){
 const[username,setUsername]=useState("");
 const[password,setPassword]=useState("");
 const[busy,setBusy]=useState(false);
 const[error,setError]=useState("");
 const submit=async(event:FormEvent)=>{
  event.preventDefault();setBusy(true);setError("");
  try{
   const response=await fetch("/api/v1/auth/session",{
    method:"POST",credentials:"same-origin",
    headers:{"Content-Type":"application/json"},
    body:JSON.stringify({username:username.trim(),password})
   });
   if(!response.ok)throw Error("Incorrect login or local sessions are unavailable");
   const result=await response.json() as {authenticated:boolean;role:ConsoleRole};
   if(!result.authenticated||!["operator","admin"].includes(result.role))throw Error("Invalid session");
   setPassword("");onSignedIn(result.role);
  }catch(e){setError(e instanceof Error?e.message:"Could not sign in")}
  finally{setBusy(false)}
 };
 return <main className="min-h-screen flex items-center justify-center px-4 py-12">
  <form onSubmit={submit} className="df-studio-panel rounded-xl border p-6 sm:p-8 w-full max-w-md space-y-5" aria-label="DataFlow sign in">
   <div className="flex items-center gap-3"><div className="brand-mark p-3 rounded-lg"><LockKeyhole size={22}/></div><div><h1 className="text-xl font-bold">Sign in to DataFlow</h1><p className="muted text-xs">Use your QA account credentials</p></div></div>
   <label className="block text-sm space-y-1.5">Username<input aria-label="Username" className="field" type="text" autoComplete="username" required value={username} onChange={e=>setUsername(e.target.value)}/></label>
   <label className="block text-sm space-y-1.5">Password<input aria-label="Sign in password" className="field" type="password" autoComplete="current-password" required value={password} onChange={e=>setPassword(e.target.value)}/></label>
   {error&&<p role="alert" className="df-text-error text-xs">{error}</p>}
   <Button type="submit" disabled={busy||!username.trim()||!password} className="w-full"><ShieldCheck size={17}/>{busy?"Signing in…":"Sign in"}</Button>
  </form>
 </main>
}
