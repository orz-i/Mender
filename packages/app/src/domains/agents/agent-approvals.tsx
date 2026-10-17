import {useEffect,useState} from "react";
import type {AgentApproval,AgentTask,MenderClient} from "@mender/sdk";
import {Button} from "@mender/ui/components/button";
import {t} from "../../i18n/i18n";
import {errorMessage} from "../../shared/errors";

// Focused Task detail only; a remote Profile sees no approval endpoint.
// Permission titles, raw tool arguments and diff contents are deliberately
// absent until an independent trustworthy preview/execution boundary exists.
export function AgentTaskApprovals({
 client,task,onTaskUpdated,
}:{client:MenderClient;task:AgentTask;onTaskUpdated?:(task:AgentTask)=>void}){
 const [records,setRecords]=useState<AgentApproval[]>([]);
 const [busy,setBusy]=useState(false);
 const [error,setError]=useState("");
 const [revision,setRevision]=useState(0);
 useEffect(()=>{
  let active=true;
  const refresh=()=>{
   void Promise.all([client.listAgentApprovals(task.taskId),client.getAgentTask(task.taskId)])
    .then(([result,current])=>{
     if(!active)return;
     setRecords(result.approvals);
     onTaskUpdated?.(current);
    }).catch(reason=>{if(active)setError(errorMessage(reason));});
  };
  refresh();
  if(task.status!=="queued"&&task.status!=="running")return()=>{active=false;};
  const timer=setInterval(refresh,1500);
  return()=>{active=false;clearInterval(timer);};
 },[client,task.taskId,task.status,revision,onTaskUpdated]);
 async function decide(item:AgentApproval,choice:"allow_once"|"reject"){
  if(busy)return;
  setBusy(true);setError("");
  try{
   await client.decideAgentApproval(item.approvalId,choice);
   setRevision(v=>v+1);
  }catch(reason){setError(errorMessage(reason));}finally{setBusy(false);}
 }
 const pending=records.filter(a=>a.status==="pending");
 const first=pending[0];
 if(!first && !error)return null;
 return <section className="space-y-3 rounded-xl border border-border p-3" aria-label={t("ACP tool approvals")}>
   <div className="text-sm font-semibold">{t("ACP tool approvals")}</div>
   {first?<div className="space-y-2">
     <p className="text-sm">{t("Awaiting owner approval")} ({pending.length})</p>
     <p className="text-sm">{t("Requested tool category")}: <strong>{t(first.kind==="read"?"Read":first.kind==="edit"?"Edit":first.kind==="execute"?"Execute":"Other")}</strong></p>
     <p className="text-xs text-muted-foreground">{t("This is a cooperative OpenCode permission request, not proof that all native tools are sandboxed or governed by Mender.")}</p>
     <p className="text-xs text-muted-foreground">{t("Expires at")}: {new Date(first.expiresAt).toLocaleString()}</p>
     <div className="flex flex-wrap gap-2">
      <Button size="sm" disabled={busy} onClick={()=>void decide(first,"allow_once")}>{t("Allow once")}</Button>
      <Button size="sm" variant="outline" disabled={busy} onClick={()=>void decide(first,"reject")}>{t("Reject")}</Button>
     </div>
   </div>:null}
   {error?<p role="alert" className="text-xs text-destructive">{error}</p>:null}
 </section>;
}
