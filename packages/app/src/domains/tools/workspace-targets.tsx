import {useEffect,useMemo,useState} from "react";
import {useNavigate} from "react-router";
import type {MenderClient,WorkspaceToolTarget,Sandbox} from "@mender/sdk";
import {Button} from "@mender/ui/components/button";
import {Input} from "@mender/ui/components/input";
import {Dialog,DialogContent,DialogDescription,DialogFooter,DialogHeader,DialogTitle} from "@mender/ui/components/dialog";
import {t} from "../../i18n/i18n";
import {errorMessage} from "../../shared/errors";
import {ErrorNotice,EmptyState} from "../../shared/page";
import {ResultPanel} from "../../shared/page-toolbar";
import {EmptyPanelState} from "../../shared/workbench";

export function WorkspaceTargetSettings({client,providerId,defaultSandboxId}:{client:MenderClient;providerId:string;defaultSandboxId:string}){
 const navigate=useNavigate();
 const [items,setItems]=useState<WorkspaceToolTarget[]>([]);
 const [sandboxes,setSandboxes]=useState<Sandbox[]>([]);
 const [dialog,setDialog]=useState<"add"|"remove"|null>(null);
 const [key,setKey]=useState("");
 const [sandboxId,setSandboxId]=useState("");
 const [busy,setBusy]=useState(false);
 const [error,setError]=useState("");
 const [revision,setRevision]=useState(0);
 const candidates=useMemo(()=>sandboxes.filter(s=>s.state==="ready"&&s.backendAvailable&&s.sandboxId!==defaultSandboxId&&
  !items.some(item=>item.sandboxId===s.sandboxId)),[sandboxes,items,defaultSandboxId]);
 useEffect(()=>{
  let live=true;
  void Promise.all([client.listWorkspaceToolTargets(providerId),client.listSandboxes()]).then(([w,s])=>{
   if(live){setItems(w.workspaces);setSandboxes(s.sandboxes);setError("");}
  }).catch((e:unknown)=>{if(live)setError(errorMessage(e));});
  return ()=>{live=false;};
 },[client,providerId,revision]);
 function showAdd(){setKey("");setSandboxId(candidates[0]?.sandboxId??"");setDialog("add");}
 async function refreshTools(){
  if(busy)return;
  setBusy(true);setError("");
  try{await client.ensureSandboxTools(defaultSandboxId);setRevision(v=>v+1);}
  catch(e){setError(errorMessage(e));}finally{setBusy(false);}
 }
 async function save(){
  if(!dialog||busy)return;
  setBusy(true);setError("");
  try{
   if(dialog==="add"){await client.bindWorkspaceToolTarget(providerId,key,sandboxId);}
   else {await client.unbindWorkspaceToolTarget(providerId,key);}
   setDialog(null);setRevision(v=>v+1);
  }catch(e){setError(errorMessage(e));}finally{setBusy(false);}
 }
 return <>
  {error?<ErrorNotice message={error}/>:null}
  <ResultPanel title="Workspace targets">
   <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border p-4">
    <p className="max-w-xl text-xs text-muted-foreground">{t("One set of tools can use multiple explicitly permitted Sandboxes. Permissions are managed on each Sandbox.")}</p>
    <div className="flex flex-wrap gap-2">
     <Button size="sm" variant="outline" disabled={busy} onClick={()=>void refreshTools()}>{t("Refresh tools")}</Button>
     <Button size="sm" disabled={busy||!candidates.length} onClick={showAdd}>{t("Add workspace")}</Button>
    </div>
   </div>
   <div className="divide-y divide-border">
    <div className="flex flex-wrap items-center justify-between gap-2 p-4">
     <div><div className="font-medium">{t("Default workspace")}</div>
      <p className="text-xs text-muted-foreground">{sandboxes.find(s=>s.sandboxId===defaultSandboxId)?.name??defaultSandboxId}</p></div>
     <Button size="sm" variant="outline" onClick={()=>void navigate("/sandboxes/"+defaultSandboxId)}>{t("Manage permissions")}</Button>
    </div>
    {items.map(item=><div key={item.key} className="flex flex-wrap items-center justify-between gap-2 p-4">
     <div><div className="font-medium">{item.key}</div><p className="text-xs text-muted-foreground">{sandboxes.find(s=>s.sandboxId===item.sandboxId)?.name??item.sandboxId}</p></div>
     <div className="flex gap-2">
      <Button size="sm" variant="outline" onClick={()=>void navigate("/sandboxes/"+item.sandboxId)}>{t("Manage permissions")}</Button>
      <Button size="sm" variant="outline" onClick={()=>{setKey(item.key);setDialog("remove");}}>{t("Remove")}</Button>
     </div>
    </div>)}
   </div>
   {items.length===0?<EmptyPanelState><EmptyState title="No additional workspaces" description="Add an existing Sandbox to reuse these four tools."/></EmptyPanelState>:null}
   <div className="border-t border-border p-4 text-xs text-muted-foreground">{t("New workspace selection becomes available in tool schemas after the owner refreshes this service's tools.")}</div>
  </ResultPanel>
  <Dialog open={dialog!==null} onOpenChange={o=>{if(!o)setDialog(null);}}>
   <DialogContent><DialogHeader><DialogTitle>{t(dialog==="add"?"Add workspace":"Remove workspace")}</DialogTitle>
    <DialogDescription>{t("Workspace names select owner-approved Sandboxes and do not replace Sandbox access grants.")}</DialogDescription>
   </DialogHeader>
   {dialog==="add"?<>
    <label htmlFor="tool-workspace-key" className="text-sm font-medium">{t("Workspace key")}</label>
    <Input id="tool-workspace-key" maxLength={64} placeholder="project-b" value={key} onChange={e=>setKey(e.target.value)}/>
    <label htmlFor="tool-workspace-target" className="text-sm font-medium">{t("Sandbox")}</label>
    <select id="tool-workspace-target" value={sandboxId} onChange={e=>setSandboxId(e.target.value)} className="h-10 rounded-lg border bg-background px-3">
     {candidates.map(s=><option key={s.sandboxId} value={s.sandboxId}>{s.name} ({s.backend})</option>)}
    </select>
   </>:<p className="text-sm">{t("This stops future selection of the target. Existing Sandbox permissions are unchanged.")}</p>}
   <DialogFooter><Button variant="outline" onClick={()=>setDialog(null)}>{t("Cancel")}</Button>
    <Button disabled={busy||(dialog==="add"&&(!/^[a-z0-9_-]{1,64}$/.test(key)||key==="default"||!sandboxId))} onClick={()=>void save()}>{t("Confirm")}</Button>
   </DialogFooter>
   </DialogContent>
  </Dialog>
 </>;
}
