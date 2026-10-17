import {useEffect,useMemo,useState} from "react";
import type {Agent,AgentToolPlacement,AgentWorkspace,MenderClient,Sandbox,SandboxToolSuite} from "@mender/sdk";
import {Badge} from "@mender/ui/components/badge";
import {Button} from "@mender/ui/components/button";
import {Dialog,DialogContent,DialogDescription,DialogFooter,DialogHeader,DialogTitle} from "@mender/ui/components/dialog";
import {errorMessage} from "../../shared/errors";
import {t} from "../../i18n/i18n";
import {ErrorNotice} from "../../shared/page";
import {ResultPanel} from "../../shared/page-toolbar";

type WorkspaceOption={key:string;root:string};
const eligibleKeys=["workspace.read","workspace.search","workspace.edit"];
const keyLabel=(key:string)=>key===""?t("Default workspace"):key;
const sourceLabel=(key:string)=>key==="workspace.read"?t("Read"):
 key==="workspace.search"?t("Search"):key==="workspace.edit"?t("Edit"):key;

export function AgentToolExecutionSettings({client,agent}:{client:MenderClient;agent:Agent}){
 const [workspaces,setWorkspaces]=useState<AgentWorkspace[]>([]);
 const [placements,setPlacements]=useState<AgentToolPlacement[]>([]);
 const [loaded,setLoaded]=useState(false);
 const [error,setError]=useState("");
 const [revision,setRevision]=useState(0);
 const [editing,setEditing]=useState<WorkspaceOption|null>(null);
 const [sandboxOptions,setSandboxOptions]=useState<Sandbox[]>([]);
 const [selectedSandbox,setSelectedSandbox]=useState("");
 const [suite,setSuite]=useState<SandboxToolSuite|null>(null);
 const [selectedTools,setSelectedTools]=useState<string[]>([]);
 const [loading,setLoading]=useState(false);
 const [busy,setBusy]=useState(false);
 useEffect(()=>{
  let live=true;
  void Promise.all([client.listAgentWorkspaces(agent.agentId),client.listAgentToolPlacements(agent.agentId)])
   .then(([w,p])=>{if(live){setWorkspaces(w.workspaces);setPlacements(p.placements);setLoaded(true);}})
   .catch(e=>{if(live){setError(errorMessage(e));setLoaded(true);}});
  return()=>{live=false;};
 },[client,agent.agentId,revision]);
 const options:WorkspaceOption[]=useMemo(()=>[
   {key:"",root:agent.workspaceRoot},
   ...workspaces.map(w=>({key:w.key,root:w.workspaceRoot})),
 ],[agent.workspaceRoot,workspaces]);
 const candidates=useMemo(()=>sandboxOptions.filter(s=>
  s.backend==="local_workspace"&&s.isolation==="restricted_host"&&s.state==="ready"&&s.backendAvailable&&
  editing!==null&&s.localRoot===editing.root
 ),[sandboxOptions,editing]);
 const current=editing?placements.find(p=>p.workspaceKey===editing.key):undefined;
 const tools=useMemo(()=>suite?.status==="ready"?suite.tools.filter(tool=>eligibleKeys.includes(tool.sourceKey)):[],[suite]);
 function open(option:WorkspaceOption){
  setError("");setEditing(option);setSuite(null);setSandboxOptions([]);setSelectedTools([]);
  const existing=placements.find(p=>p.workspaceKey===option.key);
  setSelectedSandbox(existing?.sandboxId??"");setSelectedTools(existing?.toolIds??[]);
  setLoading(true);
  void client.listSandboxes().then(({sandboxes})=>{
    setSandboxOptions(sandboxes);
  }).catch(e=>setError(errorMessage(e))).finally(()=>setLoading(false));
 }
 useEffect(()=>{
  if(!editing||!selectedSandbox)return;
  let live=true;
  void client.getSandboxTools(selectedSandbox).then(value=>{if(live)setSuite(value);})
   .catch(e=>{if(live){setSuite(null);setError(errorMessage(e));}})
   .finally(()=>{if(live)setLoading(false);});
  return()=>{live=false};
 },[client,editing,selectedSandbox]);
 async function save(){
  if(!editing||!selectedSandbox||selectedTools.length===0||busy||loading||suite?.status!=="ready"||
   !candidates.some(s=>s.sandboxId===selectedSandbox)||
   selectedTools.some(id=>!tools.some(t=>t.toolId===id))||
   agent.isolation!=="restricted_host")return;
  setBusy(true);setError("");
  try{
   await client.configureAgentToolPlacement(agent.agentId,editing.key,selectedSandbox,selectedTools);
   setEditing(null);setRevision(x=>x+1);
  }catch(e){setError(errorMessage(e));}finally{setBusy(false);}
 }
 async function disable(){
  if(!editing||!current||busy)return;
  setBusy(true);setError("");
  try{
   await client.disableAgentToolPlacement(agent.agentId,editing.key);
   setEditing(null);setRevision(x=>x+1);
  }catch(e){setError(errorMessage(e));}finally{setBusy(false);}
 }
 const ready=agent.isolation==="restricted_host";
 return <>
  {error&&!editing?<ErrorNotice message={error}/>:null}
  <ResultPanel title="Governed tool execution">
   <div className="space-y-1 border-b border-border p-4">
    <p className="text-xs text-muted-foreground">{t("Governed tools are optional per workspace. Normal Agent tasks do not require a Sandbox.")}</p>
    {!ready?<p role="alert" className="text-xs text-destructive">{t("This Host Agent cannot enable governed execution. Create a separate restricted-host Agent; existing settings remain unchanged.")}</p>:null}
   </div>
   {loaded?<div className="divide-y divide-border">{options.map(option=>{
    const selected=placements.find(p=>p.workspaceKey===option.key);
    const valid=selected&&selected.workspaceRoot===option.root;
    return <div key={option.key} className="flex flex-wrap items-center justify-between gap-4 p-4">
      <div className="min-w-0 space-y-1">
       <div className="flex items-center gap-2"><span className="font-medium">{keyLabel(option.key)}</span>
        <Badge variant="outline">{selected?valid?t("Configured"):t("Needs attention"):t("Disabled")}</Badge></div>
       <p className="break-all font-mono text-xs text-muted-foreground">{option.root}</p>
       {selected?<p className="text-xs text-muted-foreground">{t("Existing tools selected")}: {selected.toolIds.length} · {t("Execution environment")}: {selected.sandboxId}</p>:null}
      </div>
      <Button size="sm" variant="outline" disabled={!ready} onClick={()=>open(option)}>{selected?t("Edit configuration"):t("Configure")}</Button>
    </div>;
   })}</div>:<p className="p-4 text-sm text-muted-foreground">{t("Loading agents…")}</p>}
  </ResultPanel>
  <Dialog open={editing!==null} onOpenChange={value=>{if(!value&&!busy){setEditing(null);setError("");}}}>
   <DialogContent className="max-h-[85vh] overflow-y-auto">
    <DialogHeader>
     <DialogTitle>{t("Configure governed tools")}</DialogTitle>
     <DialogDescription>{t("Select an existing ready restricted local Sandbox for this exact workspace, then choose prepared Workspace Tools. Profile bindings, Sandbox grants and Policy remain required.")}</DialogDescription>
    </DialogHeader>
    <div className="space-y-3">
     <p className="text-sm font-medium">{editing?keyLabel(editing.key):""}</p>
     <p className="break-all font-mono text-xs text-muted-foreground">{editing?.root}</p>
     <label htmlFor="agent-governed-sandbox" className="text-sm font-medium">{t("Execution environment")}</label>
     <select id="agent-governed-sandbox" className="h-10 w-full rounded-lg border border-input bg-background px-3 text-sm"
       value={candidates.some(s=>s.sandboxId===selectedSandbox)?selectedSandbox:""}
       disabled={busy||loading}
       onChange={e=>{setSelectedSandbox(e.target.value);setSelectedTools([]);setSuite(null);setLoading(true);setError("");}}>
      <option value="">{t("Choose an eligible Sandbox")}</option>
      {candidates.map(s=><option key={s.sandboxId} value={s.sandboxId}>{s.name}</option>)}
     </select>
     {!loading&&candidates.length===0?<p role="alert" className="text-xs text-muted-foreground">{t("No matching ready restricted Sandbox. Create or start one separately in Sandboxes; this Agent workspace stays independent.")}</p>:null}
     {selectedSandbox&&suite?.status!=="ready"&&!loading?<p role="alert" className="text-xs text-destructive">{t("Workspace Tools must already be prepared for this Sandbox.")}</p>:null}
     {tools.length>0&&candidates.some(s=>s.sandboxId===selectedSandbox)?<fieldset className="space-y-2">
      <legend className="text-sm font-medium">{t("Allowed Workspace Tools")}</legend>
      {tools.map(tool=><label key={tool.toolId} className="flex items-center gap-2 text-sm">
       <input type="checkbox" checked={selectedTools.includes(tool.toolId)}
        onChange={e=>setSelectedTools(old=>e.target.checked?[...old,tool.toolId]:old.filter(id=>id!==tool.toolId))}
        disabled={busy}/>
       <span>{sourceLabel(tool.sourceKey)}</span>
      </label>)}
      <p className="text-xs text-muted-foreground">{t("Shell and Agent delegation are never supplied in this mode.")}</p>
     </fieldset>:null}
     {error?<p role="alert" className="text-xs text-destructive">{error}</p>:null}
    </div>
    <DialogFooter>
     {current?<Button size="sm" variant="destructive" disabled={busy} onClick={()=>void disable()}>{t("Disable governed tools")}</Button>:null}
     <Button variant="outline" disabled={busy} onClick={()=>{setEditing(null);setError("");}}>{t("Cancel")}</Button>
     <Button disabled={busy||loading||!selectedSandbox||!candidates.some(s=>s.sandboxId===selectedSandbox)||
        suite?.status!=="ready"||selectedTools.length===0||
        selectedTools.some(id=>!tools.some(t=>t.toolId===id))}
       onClick={()=>void save()}>{t("Save")}</Button>
    </DialogFooter>
   </DialogContent>
  </Dialog>
 </>;
}
