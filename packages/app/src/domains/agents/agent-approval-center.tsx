import {useEffect,useState} from "react";
import type {Agent,AgentApprovalMode,AgentApprovalPolicy,AgentWorkspace,MenderClient,PendingAgentApproval} from "@mender/sdk";
import {Button} from "@mender/ui/components/button";
import {Badge} from "@mender/ui/components/badge";
import {Dialog,DialogContent,DialogDescription,DialogFooter,DialogHeader,DialogTitle} from "@mender/ui/components/dialog";
import {Table,TableBody,TableCell,TableHead,TableHeader,TableRow} from "@mender/ui/components/table";
import {formatDateTime,t} from "../../i18n/i18n";
import {errorMessage} from "../../shared/errors";
import {ErrorNotice,EmptyState} from "../../shared/page";
import {PageTab,PageTabs,PageToolbar,ResultPanel} from "../../shared/page-toolbar";
import {EmptyPanelState} from "../../shared/workbench";

type Workspace={key:string;root:string};
type Editor={agent:Agent;workspaces:Workspace[];policies:AgentApprovalPolicy[];workspaceKey:string;mode:AgentApprovalMode};
const kindLabel=(kind:string)=>t(kind==="read"?"Read":kind==="edit"?"Edit":kind==="execute"?"Execute":"Other");
const modeLabel=(mode:AgentApprovalMode)=>t(mode==="manual"?"Ask every time":mode==="workspace_read"?"Allow workspace reads": "Trust native Agent actions (24 hours)");
const workspaceLabel=(key:string)=>key||t("Default workspace");

export function AgentApprovalCenter({client}:{client:MenderClient}){
 const [tab,setTab]=useState<"pending"|"authorization">("pending");
 const [pending,setPending]=useState<PendingAgentApproval[]>([]);
 const [agents,setAgents]=useState<Agent[]>([]);
 const [loaded,setLoaded]=useState(false);
 const [agentsLoaded,setAgentsLoaded]=useState(false);
 const [revision,setRevision]=useState(0);
 const [error,setError]=useState("");
 const [busy,setBusy]=useState(false);
 const [editor,setEditor]=useState<Editor|null>(null);
 const [trustConfirmed,setTrustConfirmed]=useState(false);
 const local=client.supportsLocalAgentApprovals===true;
 useEffect(()=>{
  if(!local)return;
  let active=true;
  const refresh=()=>{
   void client.listPendingAgentApprovals().then(({approvals})=>{
    if(active){setPending(approvals);setLoaded(true);}
   }).catch(reason=>{if(active){setError(errorMessage(reason));setLoaded(true);}});
  };
  refresh();
  const timer=setInterval(refresh,2500);
  return()=>{active=false;clearInterval(timer);};
 },[client,revision,local]);
 useEffect(()=>{
  if(!local||tab!=="authorization")return;
  let active=true;
  void client.listAgents().then(({agents})=>{
   if(active){setAgents(agents);setAgentsLoaded(true);}
  }).catch(reason=>{if(active){setError(errorMessage(reason));setAgentsLoaded(true);}});
  return()=>{active=false;};
 },[client,tab,revision,local]);
 async function decide(item:PendingAgentApproval,choice:"allow_once"|"reject"){
  if(busy)return;
  setBusy(true);setError("");
  try{
   await client.decideAgentApproval(item.approval.approvalId,choice);
   setPending(current=>current.filter(x=>x.approval.approvalId!==item.approval.approvalId));
   setRevision(n=>n+1);
  }catch(reason){setError(errorMessage(reason));}finally{setBusy(false);}
 }
 async function edit(agentId:string,initialKey=""){
  if(busy)return;
  setBusy(true);setError("");
  try{
   const [agent,workspaces,configured]=await Promise.all([
    client.getAgent(agentId),client.listAgentWorkspaces(agentId),client.listAgentApprovalPolicies(agentId),
   ]);
   const choices:Workspace[]=[{key:"",root:agent.workspaceRoot},
    ...(workspaces.workspaces??[]).map((item:AgentWorkspace)=>({key:item.key,root:item.workspaceRoot}))];
   const workspaceKey=choices.some(x=>x.key===initialKey)?initialKey:"";
   const current=configured.policies.find(p=>p.workspaceKey===workspaceKey);
   const mode=current?.mode??"manual";
   setTrustConfirmed(false);
   setEditor({agent,workspaces:choices,policies:configured.policies,workspaceKey,mode});
  }catch(reason){setError(errorMessage(reason));}finally{setBusy(false);}
 }
 function selectWorkspace(key:string){
  setEditor(old=>{
   if(!old)return old;
   const p=old.policies.find(item=>item.workspaceKey===key);
   const mode=p?.mode??"manual";
   return {...old,workspaceKey:key,mode};
  });
  setTrustConfirmed(false);
 }
 async function save(){
  if(!editor||busy||(editor.mode==="trusted"&&!trustConfirmed))return;
  setBusy(true);setError("");
  try{
   await client.setAgentApprovalPolicy(editor.agent.agentId,editor.workspaceKey,editor.mode);
   if(editor.mode!=="manual"){
    // A mode selected with an existing pending request must release that
    // request too, otherwise the user would still have to watch Task details.
    await client.releasePendingAgentApprovals(editor.agent.agentId,editor.workspaceKey);
   }
   setEditor(null);setRevision(n=>n+1);
  }catch(reason){setError(errorMessage(reason));}finally{setBusy(false);}
 }
 const activeWorkspace=editor?.workspaces.find(x=>x.key===editor.workspaceKey);
 if(!local)return <EmptyPanelState><EmptyState title="Local approval access required"
   description="Approval decisions and unattended modes are available only to the authenticated desktop owner."/></EmptyPanelState>;
 return <>
  <PageToolbar tabs={<PageTabs label="Agent approvals">
    <PageTab label="Pending approvals" active={tab==="pending"} count={pending.length} onClick={()=>setTab("pending")}/>
    <PageTab label="Unattended authorization" active={tab==="authorization"} onClick={()=>setTab("authorization")}/>
   </PageTabs>}
   actions={<Button variant="outline" size="sm" onClick={()=>setRevision(n=>n+1)}>{t("Refresh")}</Button>}/>
  {error&&!editor?<ErrorNotice message={error}/>:null}
  {tab==="pending"?<ResultPanel title="Pending Agent approvals">
   {!loaded?<p className="p-5 text-sm text-muted-foreground">{t("Loading agents…")}</p>:
    pending.length?<Table><TableHeader><TableRow>
     <TableHead>{t("Agent")}</TableHead><TableHead>{t("Workspace")}</TableHead>
     <TableHead>{t("Requested tool category")}</TableHead><TableHead>{t("Proposed file")}</TableHead>
     <TableHead>{t("Expires at")}</TableHead><TableHead className="text-right">{t("Actions")}</TableHead>
    </TableRow></TableHeader><TableBody>{pending.map(item=><TableRow key={item.approval.approvalId}>
      <TableCell><div className="font-medium">{item.agentName}</div><div className="font-mono text-xs text-muted-foreground" title={item.approval.taskId}>{item.approval.taskId.slice(0,17)}…</div></TableCell>
      <TableCell>{workspaceLabel(item.workspaceKey)}</TableCell>
      <TableCell><Badge variant="outline">{kindLabel(item.approval.kind)}</Badge></TableCell>
      <TableCell className="max-w-60 break-all font-mono text-xs">{item.approval.target||t("No reviewable path")}</TableCell>
      <TableCell className="text-xs">{formatDateTime(item.approval.expiresAt)}</TableCell>
      <TableCell><div className="flex flex-wrap justify-end gap-2">
        {item.approval.canAllow?<Button size="sm" disabled={busy} onClick={()=>void decide(item,"allow_once")}>{t("Allow once")}</Button>:null}
        <Button size="sm" variant="outline" disabled={busy} onClick={()=>void decide(item,"reject")}>{t("Reject")}</Button>
        <Button size="sm" variant="outline" disabled={busy} onClick={()=>void edit(item.approval.agentId,item.workspaceKey)}>{t("Set authorization mode")}</Button>
      </div></TableCell>
    </TableRow>)}</TableBody></Table>:
    <EmptyPanelState><EmptyState title="No pending approvals" description="New Agent permission requests appear here automatically."/></EmptyPanelState>}
   </ResultPanel>:
   <ResultPanel title="Agent authorization modes">
    <div className="border-b border-border p-4 text-xs text-muted-foreground">{t("Approval modes are independent for each Agent and workspace. All existing Agents remain in Ask every time mode until you change them.")}</div>
    {!agentsLoaded?<p className="p-5 text-sm text-muted-foreground">{t("Loading agents…")}</p>:
     agents.length?<div className="divide-y divide-border">{agents.map(a=><div key={a.agentId} className="flex flex-wrap items-center justify-between gap-3 p-4">
      <div className="min-w-0 space-y-1">
       <div className="font-medium">{a.name}</div>
       <div className="text-xs text-muted-foreground">{a.isolation==="host"?t("Host (not isolated)"):t("Restricted host")}</div>
       <div className="break-all font-mono text-xs text-muted-foreground">{a.workspaceRoot}</div>
      </div>
      <Button variant="outline" size="sm" disabled={busy} onClick={()=>void edit(a.agentId)}>{t("Configure authorization")}</Button>
     </div>)}</div>:
     <EmptyPanelState><EmptyState title="No agents" description="Add an Agent before configuring authorization."/></EmptyPanelState>}
   </ResultPanel>}
  <Dialog open={editor!==null} onOpenChange={open=>{if(!open&&!busy){setEditor(null);setError("");}}}>
   <DialogContent className="max-h-[85vh] overflow-y-auto">
    <DialogHeader><DialogTitle>{t("Configure Agent authorization")}</DialogTitle>
     <DialogDescription>{t("Choose how this Agent handles native ACP permission requests for a specific workspace.")}</DialogDescription>
    </DialogHeader>
    <div className="space-y-4">
     <div className="text-sm font-medium">{editor?.agent.name}</div>
     <label className="block space-y-1 text-sm"><span className="font-medium">{t("Workspace")}</span>
      <select className="h-10 w-full rounded-xl border border-input bg-background px-3 text-sm" aria-label={t("Workspace")}
       value={editor?.workspaceKey??""} disabled={busy} onChange={e=>selectWorkspace(e.target.value)}>
       {editor?.workspaces.map(w=><option key={w.key} value={w.key}>{workspaceLabel(w.key)}</option>)}
      </select>
     </label>
     {activeWorkspace?<p className="break-all font-mono text-xs text-muted-foreground">{activeWorkspace.root}</p>:null}
     <fieldset className="space-y-2">
      <legend className="text-sm font-medium">{t("Authorization mode")}</legend>
      {(["manual","workspace_read","trusted"] as const).map(mode=>
       <label key={mode} className="flex items-start gap-3 rounded-lg border border-border px-3 py-3 text-sm">
        <input type="radio" name="agent-approval-mode" className="mt-1" checked={editor?.mode===mode}
         onChange={()=>{setEditor(prev=>prev?{...prev,mode}:prev);setTrustConfirmed(false);}} disabled={busy}/>
        <span>{modeLabel(mode)}</span>
       </label>)}
     </fieldset>
     {editor?.mode==="workspace_read"?<p className="text-xs text-muted-foreground">{t("Only verified single-file reads inside this workspace are automatically allowed. All other requests still need review.")}</p>:null}
     {editor?.mode==="trusted"?<div className="space-y-3">
      <p role="alert" className="text-xs text-destructive">{t("Trusted mode permits this Agent's native ACP read, edit, and execute requests for up to 24 hours. In Host mode the process has your OS user's authority, may affect files outside this workspace, and is NOT sandboxed by Mender.")}</p>
      <label className="flex items-start gap-2 text-sm">
       <input type="checkbox" className="mt-1" checked={trustConfirmed} disabled={busy} onChange={e=>setTrustConfirmed(e.target.checked)}/>
       <span>{t("I understand the Host-mode risk and authorize unattended native operations for 24 hours.")}</span>
      </label>
     </div>:null}
     <p className="text-xs text-muted-foreground">{t("Changing this mode applies to subsequent requests; pending matching requests can be released automatically after saving.")}</p>
     {error?<p role="alert" className="text-xs text-destructive">{error}</p>:null}
    </div>
    <DialogFooter>
     <Button variant="outline" size="sm" disabled={busy} onClick={()=>{setEditor(null);setError("");}}>{t("Cancel")}</Button>
     <Button size="sm" disabled={busy||!editor||(editor.mode==="trusted"&&!trustConfirmed)} onClick={()=>void save()}>{t("Save")}</Button>
    </DialogFooter>
   </DialogContent>
  </Dialog>
 </>;
}
