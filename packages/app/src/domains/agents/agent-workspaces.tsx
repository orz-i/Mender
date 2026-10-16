import {useEffect,useMemo,useState} from "react";
import type {Agent,AgentWorkspace,MenderClient,Profile,Sandbox} from "@mender/sdk";
import {Button} from "@mender/ui/components/button";
import {Input} from "@mender/ui/components/input";
import {Dialog,DialogContent,DialogDescription,DialogFooter,DialogHeader,DialogTitle} from "@mender/ui/components/dialog";
import {t} from "../../i18n/i18n";
import {errorMessage} from "../../shared/errors";
import {ErrorNotice,EmptyState} from "../../shared/page";
import {ResultPanel} from "../../shared/page-toolbar";
import {EmptyPanelState} from "../../shared/workbench";

type Action = {kind:"add"}|{kind:"grant";key:string}|{kind:"remove";key:string}|{kind:"revoke";key:string;profileId:string}|null;
export function AgentWorkspaceSettings({client,agent}:{client:MenderClient;agent:Agent}){
 const [items,setItems]=useState<AgentWorkspace[]>([]);
 const [sandboxes,setSandboxes]=useState<Sandbox[]>([]);
 const [profiles,setProfiles]=useState<Profile[]>([]);
 const [action,setAction]=useState<Action>(null);
 const [key,setKey]=useState("");
 const [sandbox,setSandbox]=useState("");
 const [profile,setProfile]=useState("");
 const [busy,setBusy]=useState(false);
 const [error,setError]=useState("");
 const [revision,setRevision]=useState(0);
 const local=useMemo(()=>sandboxes.filter(s=>s.backend==="local_workspace"&&s.state==="ready"&&
   !(agent.isolation==="host"&&s.isolation==="restricted_host")), [sandboxes,agent.isolation]);
 useEffect(()=>{
   let active=true;
   void Promise.all([client.listAgentWorkspaces(agent.agentId),client.listSandboxes(),client.listProfiles()])
    .then(([w,s,p])=>{if(active){setItems(w.workspaces);setSandboxes(s.sandboxes);setProfiles(p.profiles);setError("");}})
    .catch((e:unknown)=>{if(active)setError(errorMessage(e));});
   return ()=>{active=false;};
 },[client,agent.agentId,revision]);
 async function submit(){
   if(!action||busy)return;
   setBusy(true);setError("");
   try{
     switch(action.kind){
       case "add":await client.bindAgentWorkspace(agent.agentId,key,sandbox);break;
       case "grant":await client.grantAgentWorkspace(agent.agentId,action.key,profile);break;
       case "remove":await client.unbindAgentWorkspace(agent.agentId,action.key);break;
       case "revoke":await client.revokeAgentWorkspace(agent.agentId,action.key,action.profileId);break;
     }
     setAction(null);setRevision(v=>v+1);
   }catch(e){setError(errorMessage(e));}finally{setBusy(false);}
 }
 function openAdd(){setKey("");setSandbox(local[0]?.sandboxId??"");setAction({kind:"add"});}
 function openGrant(key:string,granted:string[]){
   setProfile(profiles.find(p=>!granted.includes(p.profileId))?.profileId??"");
   setAction({kind:"grant",key});
 }
 const valid=action?.kind==="add"?/^[a-z0-9_-]{1,64}$/.test(key)&&key!=="default"&&!!sandbox:
   action?.kind==="grant"?!!profile:true;
 return <>
   {error?<ErrorNotice message={error}/>:null}
   <ResultPanel title="Agent workspaces">
    <div className="flex items-center justify-between gap-3 border-b border-border p-4">
     <p className="text-xs text-muted-foreground">{t("Only owner-approved local Sandbox workspaces can be selected for Agent tasks.")}</p>
     <Button size="sm" disabled={!local.length} onClick={openAdd}>{t("Add workspace")}</Button>
    </div>
    <div className="divide-y divide-border">
     <div className="flex flex-wrap items-center justify-between gap-2 p-4">
      <div><div className="font-medium">{t("Default workspace")}</div><div className="text-xs text-muted-foreground break-all">{agent.workspaceRoot}</div></div>
     </div>
     {items.map(w=><div key={w.key} className="space-y-3 p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
       <div><div className="font-medium">{w.key}</div><div className="text-xs text-muted-foreground">{sandboxes.find(s=>s.sandboxId===w.sandboxId)?.name??w.sandboxId}</div></div>
       <div className="flex flex-wrap gap-2"><Button size="sm" variant="outline" onClick={()=>openGrant(w.key,w.profileIds)}>{t("Grant Profile")}</Button>
       <Button size="sm" variant="outline" onClick={()=>setAction({kind:"remove",key:w.key})}>{t("Remove")}</Button></div>
      </div>
      {w.profileIds.length?<div className="space-y-1">{w.profileIds.map(id=><div key={id} className="flex items-center justify-between gap-2 text-xs">
       <span>{profiles.find(p=>p.profileId===id)?.displayName??id}</span>
       <Button size="sm" variant="ghost" onClick={()=>setAction({kind:"revoke",key:w.key,profileId:id})}>{t("Revoke")}</Button>
      </div>)}</div>:<p className="text-xs text-muted-foreground">{t("No Profile workspace access")}</p>}
     </div>)}
    </div>
    {!items.length?<EmptyPanelState><EmptyState title="No additional workspaces" description="Register an existing local Sandbox to reuse this Agent."/></EmptyPanelState>:null}
   </ResultPanel>
   <Dialog open={action!==null} onOpenChange={open=>{if(!open)setAction(null)}}>
    <DialogContent><DialogHeader><DialogTitle>{t(action?.kind==="add"?"Add workspace":action?.kind==="grant"?"Grant Profile":action?.kind==="remove"?"Remove workspace":"Revoke workspace access")}</DialogTitle>
     <DialogDescription>{t("Workspace selections are governed separately from Agent delegation grants.")}</DialogDescription>
    </DialogHeader>
    {action?.kind==="add"?<>
      <label className="text-sm font-medium" htmlFor="workspace-key">{t("Workspace key")}</label>
      <Input id="workspace-key" value={key} maxLength={64} onChange={e=>setKey(e.target.value)} placeholder="my-project"/>
      <label className="text-sm font-medium" htmlFor="workspace-sandbox">{t("Local Sandbox")}</label>
      <select id="workspace-sandbox" value={sandbox} onChange={e=>setSandbox(e.target.value)} className="h-10 rounded-lg border bg-background px-3">
        {local.map(s=><option key={s.sandboxId} value={s.sandboxId}>{s.name} · {s.localRoot}</option>)}
      </select>
    </>:action?.kind==="grant"?<>
      <label className="text-sm font-medium" htmlFor="workspace-grant-profile">{t("Profile")}</label>
      <select id="workspace-grant-profile" value={profile} onChange={e=>setProfile(e.target.value)} className="h-10 rounded-lg border bg-background px-3">
        {profiles.filter(p=>!items.find(w=>w.key===action.key)?.profileIds.includes(p.profileId)).map(p=><option key={p.profileId} value={p.profileId}>{p.displayName}</option>)}
      </select>
    </>:null}
    <DialogFooter><Button variant="outline" disabled={busy} onClick={()=>setAction(null)}>{t("Cancel")}</Button>
     <Button disabled={busy||!valid} onClick={()=>void submit()}>{t("Confirm")}</Button></DialogFooter>
    </DialogContent>
   </Dialog>
 </>;
}
