import {useState} from "react";
import type {AgentTask,MenderClient} from "@mender/sdk";
import {Button} from "@mender/ui/components/button";
import {Textarea} from "@mender/ui/components/textarea";
import {t} from "../../i18n/i18n";
import {errorMessage} from "../../shared/errors";

type ReviewMode = "none" | "acknowledge" | "reopen";

function unavailabilityText(reason:string):string{
  switch(reason){
    case "unknown_outcome":return t("An unknown task cannot be resumed. Its work may already have been handled by later tasks.");
    case "original_profile_required":return t("This task belongs to its original Profile. Only that Profile can explicitly invoke its authorized resume tool.");
    case "backend_unavailable":return t("Native continuation is unavailable for this Agent backend.");
    case "restricted_session":return t("The isolated Agent session cannot be safely restored across processes.");
    case "workspace_changed":return t("The original workspace identity has changed; continuation is blocked.");
    case "native_session_unavailable":return t("No verified completed native session is available for continuation.");
    default:return t("This task is not eligible for a new session turn.");
  }
}

// Owner-facing, progressively disclosed operations. This component never
// supplies a prompt for an unknown Task or treats an owner disposition as
// authoritative evidence that the original Agent completed.
export function AgentTaskLifecycleActions({client,task,onTaskUpdated,onTaskCreated}:{
  client:MenderClient;
  task:AgentTask;
  onTaskUpdated:(task:AgentTask)=>void;
  onTaskCreated:(task:AgentTask)=>void;
}){
  const [reviewMode,setReviewMode]=useState<ReviewMode>("none");
  const [composing,setComposing]=useState(false);
  const [prompt,setPrompt]=useState("");
  const [confirmed,setConfirmed]=useState(false);
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState("");

  async function review(){
    if(busy || task.status!=="unknown" || reviewMode==="none")return;
    const handling=reviewMode==="acknowledge"?"handled_elsewhere":"needs_review";
    setBusy(true);setError("");
    try{
      const updated=await client.reviewUnknownAgentTask(task.taskId,handling);
      onTaskUpdated(updated);
      setReviewMode("none");
    }catch(reason){setError(errorMessage(reason));}
    finally{setBusy(false);}
  }
  async function resume(){
    if(busy || !task.resumeAvailability?.eligible || task.status!=="completed" ||
      !confirmed || prompt.trim()==="" || prompt.length>65536)return;
    setBusy(true);setError("");
    try{
      const next=await client.resumeAgentTask(task.taskId,prompt);
      setComposing(false);setPrompt("");setConfirmed(false);
      onTaskCreated(next);
    }catch(reason){setError(errorMessage(reason));}
    finally{setBusy(false);}
  }

  if(task.status==="unknown"){
    const handled=task.disposition?.handling==="handled_elsewhere";
    return <section className="space-y-3 rounded-lg border border-border p-3" aria-label={t("Unknown task handling")}>
      <p className="text-sm font-medium">{t("Unknown task: do not repeat execution")}</p>
      <p className="text-xs leading-5 text-muted-foreground">
        {t("The original Agent result remains unknown. Later tasks may already have accomplished the goal. Do not retry or resume this task; inspect the current workspace instead.")}
      </p>
      {task.disposition?<p className="text-xs">
        {handled?t("Owner reported this work was handled elsewhere; this is not proof that the original Agent task completed."):
          t("Owner marked this unknown task as needing review; no Agent action was started.")}
        {" · "}{t("Recorded")}: {new Date(task.disposition.recordedAt).toLocaleString()}
      </p>:null}
      {reviewMode==="none"?<Button size="sm" variant="outline" disabled={busy}
          onClick={()=>setReviewMode(handled?"reopen":"acknowledge")}>
          {t(handled?"Reopen handling review":"Mark handled elsewhere")}
        </Button>:
        <div className="space-y-2">
          <p className="text-xs leading-5 text-muted-foreground">
            {t(reviewMode==="acknowledge"?
              "Confirm the work has already been addressed independently. This records only your review and never changes the original task outcome.":
              "Reopen handling review without changing the original unknown outcome or executing an Agent.")}
          </p>
          <div className="flex flex-wrap gap-2">
            <Button size="sm" disabled={busy} onClick={()=>void review()}>{t("Confirm handling review")}</Button>
            <Button size="sm" variant="outline" disabled={busy} onClick={()=>setReviewMode("none")}>{t("Cancel")}</Button>
          </div>
        </div>}
      {error?<p role="alert" className="text-xs text-destructive">{error}</p>:null}
    </section>;
  }

  if(task.status!=="completed")return null;
  if(!task.resumeAvailability?.eligible){
    return <p className="text-xs leading-5 text-muted-foreground">
      {unavailabilityText(task.resumeAvailability?.reason??"not_completed")}
    </p>;
  }
  return <section className="space-y-3 rounded-lg border border-border p-3" aria-label={t("Continue native session")}>
    <p className="text-sm font-medium">{t("Continue native session")}</p>
    <p className="text-xs leading-5 text-muted-foreground">
      {t("This creates a NEW task in the same native session, not a retry. Review current files and prior results first to prevent duplicate changes. Backend availability and authorization are rechecked on submit.")}
    </p>
    {!composing?<Button size="sm" variant="outline" disabled={busy}
      onClick={()=>setComposing(true)}>{t("Compose new instruction")}</Button>:
      <div className="space-y-3">
        <label className="block space-y-1 text-xs font-medium">
          <span>{t("New instruction")}</span>
          <Textarea aria-label={t("New instruction")} value={prompt} maxLength={65536}
            onChange={e=>setPrompt(e.target.value)} rows={4}
            placeholder={t("Describe only the new work that remains after reviewing existing changes.")}/>
        </label>
        <label className="flex items-start gap-2 text-xs leading-5">
          <input type="checkbox" checked={confirmed} onChange={e=>setConfirmed(e.target.checked)}
            className="mt-1 size-4" />
          {t("I reviewed the current work and confirm this new instruction will not repeat already completed changes.")}
        </label>
        <div className="flex flex-wrap gap-2">
          <Button size="sm" disabled={busy || !confirmed || prompt.trim()==="" || prompt.length>65536}
            onClick={()=>void resume()}>{t("Submit new continuation task")}</Button>
          <Button size="sm" variant="outline" disabled={busy} onClick={()=>{setComposing(false);setPrompt("");setConfirmed(false);}}>
            {t("Cancel")}
          </Button>
        </div>
      </div>}
    {error?<p role="alert" className="text-xs text-destructive">{error}</p>:null}
  </section>;
}
