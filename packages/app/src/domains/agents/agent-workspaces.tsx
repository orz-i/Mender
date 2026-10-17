import {useEffect,useState} from "react";
import {MenderApiError} from "@mender/sdk";
import type {Agent,AgentWorkspace,MenderClient,Profile} from "@mender/sdk";
import {Button} from "@mender/ui/components/button";
import {Input} from "@mender/ui/components/input";
import {Dialog,DialogContent,DialogDescription,DialogFooter,DialogHeader,DialogTitle} from "@mender/ui/components/dialog";
import {t} from "../../i18n/i18n";
import {errorMessage} from "../../shared/errors";
import {ErrorNotice,EmptyState} from "../../shared/page";
import {ResultPanel} from "../../shared/page-toolbar";
import {EmptyPanelState} from "../../shared/workbench";

type Action={kind:"add"}|{kind:"grant";key:string}|{kind:"remove";key:string}|{kind:"revoke";key:string;profileId:string}|null;

// Client-side comparison is only for immediate feedback. The owner service
// resolves symlinks/canonical paths and SQLite rejects races authoritatively.
function comparablePath(value:string):string{
 return value.trim().replace(/\/+$/u,"")||"/";
}
function registrationErrorMessage(reason:unknown):string{
 const code=reason instanceof MenderApiError?reason.code:
  reason instanceof Error?reason.message:
  typeof reason==="string"?reason:
  reason!==null&&typeof reason==="object"&&"message" in reason&&typeof reason.message==="string"?reason.message:"";
 if(code.includes("agent_workspace_default_directory"))
  return t("This directory is already the Agent's default workspace.");
 if(code.includes("agent_workspace_alias_exists"))
  return t("This workspace alias already exists.");
 if(code.includes("agent_workspace_directory_exists"))
  return t("This directory is already registered for this Agent. Refresh the workspace list.");
 if(code.includes("agent_workspace_governed_tools_enabled"))
  return t("Disable governed tools before removing this workspace.");
 if((reason instanceof MenderApiError&&reason.status===409)||
    code.includes("agent resource conflict"))
  return t("Workspace registration conflicts with existing data. Refresh the workspace list.");
 return errorMessage(reason);
}

export function AgentWorkspaceSettings({
 client,agent,chooseLocalDirectory,
}:{client:MenderClient;agent:Agent;chooseLocalDirectory?:()=>Promise<string|null>}){
 const [items,setItems]=useState<AgentWorkspace[]>([]);
 const [profiles,setProfiles]=useState<Profile[]>([]);
 const [action,setAction]=useState<Action>(null);
 const [key,setKey]=useState("");
 const [root,setRoot]=useState("");
 const [profile,setProfile]=useState("");
 const [busy,setBusy]=useState(false);
 const [error,setError]=useState("");
 const [revision,setRevision]=useState(0);
 useEffect(()=>{
  let active=true;
  void Promise.all([client.listAgentWorkspaces(agent.agentId),client.listProfiles()])
   .then(([w,p])=>{
    if(active){setItems(w.workspaces);setProfiles(p.profiles);}
   }).catch((e:unknown)=>{if(active)setError(errorMessage(e));});
  return ()=>{active=false;};
 },[client,agent.agentId,revision]);

 async function submit(){
  if(!action||busy||!valid)return;
  setBusy(true);setError("");
  try{
   switch(action.kind){
    case "add":await client.bindAgentWorkspace(agent.agentId,key,root.trim());break;
    case "grant":await client.grantAgentWorkspace(agent.agentId,action.key,profile);break;
    case "remove":await client.unbindAgentWorkspace(agent.agentId,action.key);break;
    case "revoke":await client.revokeAgentWorkspace(agent.agentId,action.key,action.profileId);break;
   }
   setAction(null);setRevision(v=>v+1);
  }catch(e){setError(action.kind==="add"||action.kind==="remove"?registrationErrorMessage(e):errorMessage(e));}finally{setBusy(false);}
 }
 function openAdd(){setError("");setKey("");setRoot("");setAction({kind:"add"});}
 function openGrant(key:string,granted:string[]){
  setError("");setProfile(profiles.find(p=>!granted.includes(p.profileId))?.profileId??"");
  setAction({kind:"grant",key});
 }
 async function choose(){
  if(!chooseLocalDirectory)return;
  try{
   const selected=await chooseLocalDirectory();
   if(selected){setRoot(selected);setError("");}
  }catch(e){setError(errorMessage(e));}
 }
 const validAlias=/^[a-z0-9_-]{1,64}$/.test(key)&&key!=="default";
 const validRoot=root.trim().startsWith("/")&&root.trim().length>1;
 const aliasDuplicate=validAlias&&items.some(w=>w.key===key);
 const comparableRoot=comparablePath(root);
 const defaultDuplicate=validRoot&&comparableRoot===comparablePath(agent.workspaceRoot);
 const registeredDirectory=validRoot?items.find(w=>comparablePath(w.workspaceRoot)===comparableRoot):undefined;
 const valid=action?.kind==="add"?validAlias&&validRoot&&!aliasDuplicate&&!defaultDuplicate&&!registeredDirectory:
  action?.kind==="grant"?!!profile:true;
 return <>
  {error?<ErrorNotice message={error}/>:null}
  <ResultPanel title="Agent workspaces">
   <div className="flex items-center justify-between gap-3 border-b border-border p-4">
    <p className="text-xs text-muted-foreground">{t("An Agent can use additional owner-approved local directories without creating Sandboxes.")}</p>
    <Button size="sm" onClick={openAdd}>{t("Add workspace")}</Button>
   </div>
   <div className="divide-y divide-border">
    <div className="flex flex-wrap items-center justify-between gap-2 p-4">
     <div><div className="font-medium">{t("Default workspace")}</div>
      <div className="break-all text-xs text-muted-foreground">{agent.workspaceRoot}</div></div>
    </div>
    {items.map(w=><div key={w.key} className="space-y-3 p-4">
     <div className="flex flex-wrap items-center justify-between gap-2">
      <div><div className="font-medium">{w.key}</div>
       <div className="break-all text-xs text-muted-foreground">{w.workspaceRoot}</div></div>
      <div className="flex flex-wrap gap-2">
       <Button size="sm" variant="outline" onClick={()=>openGrant(w.key,w.profileIds)}>{t("Grant Profile")}</Button>
       <Button size="sm" variant="outline" onClick={()=>setAction({kind:"remove",key:w.key})}>{t("Remove")}</Button>
      </div>
     </div>
     {w.profileIds.length?<div className="space-y-1">
      {w.profileIds.map(id=><div key={id} className="flex items-center justify-between gap-2 text-xs">
       <span>{profiles.find(p=>p.profileId===id)?.displayName??id}</span>
       <Button size="sm" variant="ghost" onClick={()=>setAction({kind:"revoke",key:w.key,profileId:id})}>{t("Revoke")}</Button>
      </div>)}</div>:<p className="text-xs text-muted-foreground">{t("No Profile workspace access")}</p>}
    </div>)}
   </div>
   {!items.length?<EmptyPanelState><EmptyState title="No additional workspaces"
    description="Register another local directory to reuse this Agent."/></EmptyPanelState>:null}
  </ResultPanel>
  <Dialog open={action!==null} onOpenChange={open=>{if(!open)setAction(null)}}>
   <DialogContent>
    <DialogHeader><DialogTitle>{t(action?.kind==="add"?"Add workspace":action?.kind==="grant"?"Grant Profile":action?.kind==="remove"?"Remove workspace":"Revoke workspace access")}</DialogTitle>
     <DialogDescription>{t("Workspace selections are governed separately from Agent delegation grants.")}</DialogDescription>
    </DialogHeader>
    {action?.kind==="add"?<>
     <label className="text-sm font-medium" htmlFor="agent-workspace-key">{t("Workspace key")}</label>
     <Input id="agent-workspace-key" value={key} maxLength={64}
      onChange={e=>{setKey(e.target.value.toLowerCase());setError("");}} placeholder="my-project"/>
     <p className="text-xs text-muted-foreground">{t("Use a-z, 0-9, - or _. Uppercase letters are converted to lowercase.")}</p>
     {key&&!validAlias?<p role="alert" className="text-xs text-destructive">{t("Enter a valid workspace alias.")}</p>:null}
     {aliasDuplicate?<p role="alert" className="text-xs text-destructive">{t("This workspace alias already exists.")}</p>:null}
     <label className="text-sm font-medium" htmlFor="agent-workspace-root">{t("Workspace directory")}</label>
     <div className="flex gap-2">
      <Input id="agent-workspace-root" className="min-w-0 flex-1" value={root}
       onChange={e=>{setRoot(e.target.value);setError("");}} placeholder="/Users/example/project"/>
      {chooseLocalDirectory?<Button variant="outline" onClick={()=>void choose()}>{t("Choose local directory")}</Button>:null}
     </div>
     {root&&!validRoot?<p role="alert" className="text-xs text-destructive">{t("Select an existing absolute local directory.")}</p>:null}
     {defaultDuplicate?<p role="alert" className="text-xs text-destructive">{t("This directory is already the Agent's default workspace.")}</p>:null}
     {registeredDirectory?<p role="alert" className="text-xs text-destructive">{t("This directory is already registered as workspace {{key}}.",{key:registeredDirectory.key})}</p>:null}
    </>:action?.kind==="grant"?<>
     <label className="text-sm font-medium" htmlFor="workspace-grant-profile">{t("Profile")}</label>
     <select id="workspace-grant-profile" value={profile} onChange={e=>setProfile(e.target.value)}
      className="h-10 rounded-lg border bg-background px-3">
      {profiles.filter(p=>!items.find(w=>w.key===action.key)?.profileIds.includes(p.profileId))
       .map(p=><option key={p.profileId} value={p.profileId}>{p.displayName}</option>)}
     </select>
    </>:null}
    {error?<p role="alert" className="text-xs text-destructive">{error}</p>:null}
    <DialogFooter>
     <Button variant="outline" disabled={busy} onClick={()=>setAction(null)}>{t("Cancel")}</Button>
     <Button disabled={busy||!valid} onClick={()=>void submit()}>{t("Confirm")}</Button>
    </DialogFooter>
   </DialogContent>
  </Dialog>
 </>;
}
