import { useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router";

import type { Agent, AgentTools, AgentTask, AgentAdmissionEvent, AgentModelChoices, MenderClient, Profile } from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@mender/ui/components/dialog";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@mender/ui/components/table";

import { formatDateTime, t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { ErrorNotice, EmptyState } from "../../shared/page";
import { PageTab, PageTabs, PageToolbar, ResultPanel } from "../../shared/page-toolbar";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import { EmptyPanelState } from "../../shared/workbench";
import { Input } from "@mender/ui/components/input";
import { AgentWorkspaceSettings } from "./agent-workspaces";
import { agentBackendLabel } from "./agent-backends";

function taskStatusText(value:AgentTask["status"]):string{
  switch(value){
    case "queued":return t("Queued");
    case "running":return t("Running");
    case "completed":return t("Completed");
    case "failed":return t("Failed");
    case "cancelled":return t("Cancelled");
    case "unknown":return t("Unknown");
  }
}
type Topic = "overview" | "workspaces" | "tools" | "tasks" | "access" | "activity";
type ConfirmAction = {kind:"cancel"|"revoke"|"delete";id:string} | null;

export function AgentDetailPage({client}:{client:MenderClient}){
  const {agentId}=useParams();
  const navigate=useNavigate();
  const [agent,setAgent]=useState<Agent|null>(null);
  const [tasks,setTasks]=useState<AgentTask[]>([]);
  const [tools,setTools]=useState<AgentTools|null>(null);
  const [activity,setActivity]=useState<AgentAdmissionEvent[]>([]);
  const [grants,setGrants]=useState<string[]>([]);
  const [profiles,setProfiles]=useState<Profile[]>([]);
  const [topic,setTopic]=useState<Topic>("overview");
  const [loaded,setLoaded]=useState(false);
  const [error,setError]=useState("");
  const [revision,setRevision]=useState(0);
  const [busy,setBusy]=useState(false);
  const [grantOpen,setGrantOpen]=useState(false);
  const [selectedProfile,setSelectedProfile]=useState("");
  const [confirm,setConfirm]=useState<ConfirmAction>(null);
  const [viewTask,setViewTask]=useState<AgentTask|null>(null);
  const [settingsOpen,setSettingsOpen]=useState(false);
  const [modelDraft,setModelDraft]=useState("");
  const [effortDraft,setEffortDraft]=useState("");
  const [modelChoices,setModelChoices]=useState<AgentModelChoices|null>(null);
  const [modelLoading,setModelLoading]=useState(false);
  const [modelInspectionError,setModelInspectionError]=useState("");

  useEffect(()=>{
    if(!agentId)return;
    let active=true;
    void Promise.all([
      client.getAgent(agentId),
      client.listAgentTasks(agentId),
      client.listAgentGrants(agentId),
      client.getAgentTools(agentId),
    ]).then(([a,taskResult,grantResult,toolResult])=>{
      if(!active)return;
      setAgent(a);setTasks(taskResult.tasks);setGrants(grantResult.profileIds);setTools(toolResult);
      setLoaded(true);setError("");
    }).catch((reason:unknown)=>{
      if(active){setError(errorMessage(reason));setLoaded(true);}
    });
    return ()=>{active=false};
  },[client,agentId,revision]);

  useEffect(()=>{
    if(!agentId||topic!=="activity")return;
    let live=true;
    void client.listAgentAdmissions(agentId).then(({events})=>{
      if(live){setActivity(events);setError("");}
    }).catch((reason:unknown)=>{if(live)setError(errorMessage(reason))});
    return ()=>{live=false;};
  },[client,agentId,topic,revision]);
  const pagination=usePagination(tasks);
  const options=useMemo(()=>profiles.filter(p=>!grants.includes(p.profileId)),[profiles,grants]);

  async function repairTools(){
    if(!agentId||busy)return;
    setBusy(true);setError("");
    try{setTools(await client.ensureAgentTools(agentId));setRevision(n=>n+1);}
    catch(reason){setError(errorMessage(reason));}
    finally{setBusy(false);}
  }
  async function beginGrant(){
    setError("");
    try{
      const result=await client.listProfiles();
      const remaining=result.profiles.filter(p=>!grants.includes(p.profileId));
      setProfiles(result.profiles);
      setSelectedProfile(remaining[0]?.profileId??"");
      setGrantOpen(true);
    }catch(reason){setError(errorMessage(reason));}
  }
  async function addGrant(){
    if(!agentId||!selectedProfile||busy)return;
    setBusy(true);setError("");
    try{
      await client.grantAgentProfile(agentId,selectedProfile);
      setGrantOpen(false);setRevision(n=>n+1);
    }catch(reason){setError(errorMessage(reason));}
    finally{setBusy(false)}
  }
  async function confirmOperation(){
    if(!agentId||!confirm||busy)return;
    setBusy(true);setError("");
    try{
      switch(confirm.kind){
        case "cancel":await client.cancelAgentTask(confirm.id);break;
        case "revoke":await client.revokeAgentProfile(agentId,confirm.id);break;
        case "delete":await client.deleteAgent(agentId);break;
      }
      const wasDelete=confirm.kind==="delete";
      setConfirm(null);
      if(wasDelete)void navigate("/agents");
      else setRevision(n=>n+1);
    }catch(reason){setError(errorMessage(reason));}
    finally{setBusy(false)}
  }
  async function inspectModels(){
    if(!agentId)return;
    setModelLoading(true);setModelInspectionError("");
    try {setModelChoices(await client.inspectAgentModels(agentId));}
    catch(reason){setModelInspectionError(errorMessage(reason));setModelChoices(null);}
    finally{setModelLoading(false);}
  }
  function editDefaultModel(){
    setModelDraft(agent?.defaultModel??"");
    setEffortDraft(agent?.defaultReasoningEffort??"");
    setModelChoices(null);setModelInspectionError("");
    setSettingsOpen(true);
    // Explicit owner intent: inspection launches a bounded ACP session, no prompt.
    void inspectModels();
  }
  async function saveSettings(){
    if(!agentId||busy)return;
    setBusy(true);setError("");
    try{setAgent(await client.updateAgentSettings(agentId,modelDraft.trim(),effortDraft));setSettingsOpen(false);setRevision(v=>v+1);}
    catch(reason){setError(errorMessage(reason));}finally{setBusy(false);}
  }
  async function openTask(id:string){
    try{
      const value=await client.getAgentTask(id);
      setViewTask(value);
    }catch(reason){setError(errorMessage(reason));}
  }
  return <>
    <PageToolbar
      tabs={agent?<PageTabs label="Agent details">
        <PageTab label="Overview" active={topic==="overview"} onClick={()=>setTopic("overview")}/>
        <PageTab label="Workspaces" active={topic==="workspaces"} onClick={()=>setTopic("workspaces")}/>
        <PageTab label="Tools" count={tools?.tools.length??0} active={topic==="tools"} onClick={()=>setTopic("tools")}/>
        <PageTab label="Tasks" count={tasks.length} active={topic==="tasks"} onClick={()=>setTopic("tasks")}/>
        <PageTab label="Access" count={grants.length} active={topic==="access"} onClick={()=>setTopic("access")}/>
        <PageTab label="Activity" active={topic==="activity"} onClick={()=>setTopic("activity")}/>
      </PageTabs>:undefined}
      actions={<>
        <Button variant="outline" size="sm" onClick={()=>void navigate("/agents")}>{t("Back to agents")}</Button>
        <Button variant="outline" size="sm" onClick={()=>setRevision(n=>n+1)}>{t("Refresh")}</Button>
      </>}
    />
    {error?<ErrorNotice message={error}/>:null}
    {!loaded?<ResultPanel title="Agent details"><p className="p-6 text-sm text-muted-foreground">{t("Loading agents…")}</p></ResultPanel>:
    !agent?<ResultPanel title="Agent details"><EmptyPanelState><EmptyState title="No agents" description="This Agent could not be found."/></EmptyPanelState></ResultPanel>:
    topic==="overview"?<ResultPanel title="Agent overview">
      <dl className="grid grid-cols-1 gap-5 p-5 sm:grid-cols-2">
        <div><dt className="text-xs text-muted-foreground">{t("Name")}</dt><dd className="mt-1 font-medium">{agent.name}</dd></div>
        <div><dt className="text-xs text-muted-foreground">{t("Backend")}</dt><dd className="mt-1">{agentBackendLabel(agent.backend)}</dd></div>
        <div><dt className="text-xs text-muted-foreground">{t("Status")}</dt>
          <dd className="mt-1"><Badge variant="outline">{agent.backendAvailable?t("CLI detected"):t("CLI missing")}</Badge></dd></div>
        <div><dt className="text-xs text-muted-foreground">{t("Isolation")}</dt>
          <dd className="mt-1">{agent.isolation==="host"?t("Host (not isolated)"):t("Restricted host")}</dd></div>
        <div><dt className="text-xs text-muted-foreground">{t("Default model (optional)")}</dt>
          <dd className="mt-1 text-sm">{agent.defaultModel||t("Use Agent default")}</dd></div>
        <div><dt className="text-xs text-muted-foreground">{t("Default reasoning effort")}</dt>
          <dd className="mt-1 text-sm">{agent.defaultReasoningEffort||t("Use Agent default")}</dd></div>
        <div className="sm:col-span-2"><dt className="text-xs text-muted-foreground">{t("Workspace directory")}</dt>
          <dd className="mt-1 break-all font-mono text-xs">{agent.workspaceRoot}</dd></div>
        <div><dt className="text-xs text-muted-foreground">{t("Created at")}</dt><dd className="mt-1 text-sm">{formatDateTime(agent.createdAt)}</dd></div>
        {agent.issue?<div className="sm:col-span-2 text-sm text-destructive">{agent.issue}</div>:null}
      </dl>
      <div className="flex flex-wrap justify-between gap-2 border-t border-border p-4">
        <p className="max-w-xl text-xs leading-5 text-muted-foreground">{t("Grant Profile access explicitly; this Agent will not run until invoked through an authorized Tool.")}</p>
        <div className="flex flex-wrap gap-2">
          <Button size="sm" variant="outline" onClick={editDefaultModel}>{t("Edit runtime defaults")}</Button>
          <Button size="sm" variant="outline" onClick={()=>setTopic("workspaces")}>{t("Manage workspaces")}</Button>
          <Button size="sm" onClick={()=>setTopic("tools")}>{t("View Agent tools")}</Button>
          <Button size="sm" variant="destructive" onClick={()=>setConfirm({kind:"delete",id:agent.agentId})}>{t("Delete agent")}</Button>
        </div>
      </div>
    </ResultPanel>:
    topic==="workspaces"?<AgentWorkspaceSettings client={client} agent={agent}/>:
    topic==="tools"?<ResultPanel title="Agent tools">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border p-4">
        <div className="min-w-0 space-y-1">
          <p className="font-medium">{t("Agent Delegation")}</p>
          <p className="text-xs text-muted-foreground">{tools?.status==="ready"?t("Tools prepared; Profile bindings and Agent grants are still required."):
            tools?.status==="legacy"?t("Existing Agent tools can be adopted without changing their identities."):
            tools?.status==="conflict"?t("Multiple or mismatched Agent Providers require explicit resolution in advanced Provider management."):
            t("Agent tools are not ready; review and repair the binding.")}</p>
        </div>
        {tools?.status==="conflict"?
          <Button size="sm" variant="outline" onClick={()=>void navigate("/tools")}>{t("View tool services")}</Button>:
          <Button size="sm" disabled={busy} onClick={()=>void repairTools()}>{tools?.status==="ready"?t("Refresh Agent tools"):t("Prepare Agent tools")}</Button>}
      </div>
      {tools?.tools.length?<Table><TableHeader><TableRow>
        <TableHead>{t("Tool")}</TableHead><TableHead>{t("Source key")}</TableHead>
      </TableRow></TableHeader><TableBody>{tools.tools.map(tool=><TableRow key={tool.toolId}>
        <TableCell className="font-medium" title={tool.toolId}>{agent.name} · {tool.title}</TableCell>
        <TableCell className="font-mono text-xs">{tool.sourceKey}</TableCell>
      </TableRow>)}</TableBody></Table>:
      <EmptyPanelState><EmptyState title="No Agent tools" description="Prepare Agent tools to make them available for explicit Profile selection."/></EmptyPanelState>}
    </ResultPanel>:
    topic==="tasks"?<><ResultPanel title="Agent tasks">
      {tasks.length?<><Table><TableHeader><TableRow>
        <TableHead>{t("Task")}</TableHead><TableHead>{t("Status")}</TableHead>
        <TableHead>{t("Created at")}</TableHead><TableHead className="text-right">{t("Actions")}</TableHead>
      </TableRow></TableHeader><TableBody>{pagination.items.map(task=><TableRow key={task.taskId}>
        <TableCell className="max-w-[230px] truncate font-mono text-xs" title={task.taskId}>{task.taskId}</TableCell>
        <TableCell><Badge variant="outline">{taskStatusText(task.status)}</Badge></TableCell>
        <TableCell className="text-xs">{formatDateTime(task.createdAt)}</TableCell>
        <TableCell className="space-x-2 text-right">
          <Button size="sm" variant="outline" onClick={()=>void openTask(task.taskId)}>{t("View")}</Button>
          {task.status==="queued"||task.status==="running"?
            <Button size="sm" variant="outline" disabled={task.cancelRequested}
              onClick={()=>setConfirm({kind:"cancel",id:task.taskId})}>{t("Cancel task")}</Button>:null}
        </TableCell>
      </TableRow>)}</TableBody></Table></>
      :<EmptyPanelState><EmptyState title="No Agent tasks" description="Delegated task history will appear here."/></EmptyPanelState>}
    </ResultPanel>
    {pagination.total > 0 ? <TablePagination label="Tasks" pagination={pagination}/> : null}</>:
    topic==="activity"?<ResultPanel title="Agent activity">
      {activity.length?<Table><TableHeader><TableRow>
        <TableHead>{t("Time")}</TableHead>
        <TableHead>{t("Action")}</TableHead>
        <TableHead>{t("Origin")}</TableHead>
        <TableHead>{t("Policy decision")}</TableHead>
        <TableHead>{t("Status")}</TableHead>
      </TableRow></TableHeader><TableBody>{activity.map((ev,i)=>
        <TableRow key={ev.agentId+"-"+ev.occurredAt+"-"+i}>
          <TableCell className="text-xs">{formatDateTime(ev.occurredAt)}</TableCell>
          <TableCell className="font-mono text-xs">{ev.action}</TableCell>
          <TableCell className="text-xs">{ev.origin}</TableCell>
          <TableCell className="text-xs">{ev.policyKind}</TableCell>
          <TableCell><Badge variant="outline">{ev.status}</Badge></TableCell>
        </TableRow>
      )}</TableBody></Table>:
      <EmptyPanelState><EmptyState title="No Agent activity"
        description="Agent admission decisions will appear here."/></EmptyPanelState>}
    </ResultPanel>:
    <ResultPanel title="Agent access">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border p-4">
        <p className="text-xs text-muted-foreground">{t("Only explicitly granted Profiles may delegate tasks to this Agent.")}</p>
        <Button size="sm" onClick={()=>void beginGrant()}>{t("Grant Profile")}</Button>
      </div>
      {grants.length?<div className="divide-y divide-border">{grants.map(id=>
        <div className="flex items-center justify-between gap-3 px-5 py-4" key={id}>
          <span className="min-w-0 truncate font-mono text-xs" title={id}>{id}</span>
          <Button size="sm" variant="outline" onClick={()=>setConfirm({kind:"revoke",id})}>{t("Revoke")}</Button>
        </div>)}</div>:
        <EmptyPanelState><EmptyState title="No Agent grants" description="Profiles have no access to this Agent until explicitly granted."/></EmptyPanelState>}
    </ResultPanel>}

    <Dialog open={settingsOpen} onOpenChange={setSettingsOpen}>
      <DialogContent><DialogHeader><DialogTitle>{t("Edit runtime defaults")}</DialogTitle>
        <DialogDescription>{t("Model choices come from this Agent's authenticated ACP session; no prompt is sent.")}</DialogDescription>
      </DialogHeader>
      <div className="space-y-3">
        {modelLoading?<p className="text-sm text-muted-foreground">{t("Inspecting available models…")}</p>:null}
        {modelChoices?
          <select aria-label={t("Agent model")} className="h-10 w-full rounded-xl border border-input bg-background px-3 text-sm"
            value={modelDraft} onChange={e=>setModelDraft(e.target.value)}>
            <option value="">{t("Use Agent default")} ({modelChoices.current})</option>
            {modelChoices.models.map(model=><option key={model.id} value={model.id}>{model.name} ({model.id})</option>)}
            {modelDraft!==""&&!modelChoices.models.some(model=>model.id===modelDraft)?
              <option value={modelDraft} disabled>{modelDraft} — {t("Not currently advertised")}</option>:null}
          </select>:
          !modelLoading?<Input aria-label={t("Agent model")} maxLength={128} value={modelDraft} onChange={e=>setModelDraft(e.target.value)}
            placeholder={t("Use Agent default")}/>:null}
        {modelChoices?.reasoningEffort?
          <div className="space-y-2">
            <label htmlFor="agent-reasoning-effort" className="text-sm font-medium">{t("Reasoning effort")}</label>
            <select id="agent-reasoning-effort" className="h-10 w-full rounded-xl border border-input bg-background px-3 text-sm"
              value={effortDraft} onChange={e=>setEffortDraft(e.target.value)}>
              <option value="">{t("Use Agent default")} ({modelChoices.reasoningEffort.current})</option>
              {modelChoices.reasoningEffort.choices.map(choice=><option key={choice.id} value={choice.id}>{choice.name} ({choice.id})</option>)}
              {effortDraft!==""&&!modelChoices.reasoningEffort.choices.some(choice=>choice.id===effortDraft)?
                <option value={effortDraft} disabled>{effortDraft} — {t("Not currently advertised")}</option>:null}
            </select>
            <p className="text-xs text-muted-foreground">{t("Available reasoning levels may change when the model changes; execution revalidates them.")}</p>
          </div>:
          modelChoices?<p className="text-xs text-muted-foreground">{t("This Agent does not advertise a reasoning effort selector.")}</p>:null}
        {modelInspectionError?<p role="alert" className="text-xs text-destructive">{t("Model inspection unavailable")}: {modelInspectionError}</p>:null}
        <Button variant="outline" size="sm" disabled={modelLoading} onClick={()=>void inspectModels()}>{t("Refresh model choices")}</Button>
      </div>
      <DialogFooter><Button variant="outline" onClick={()=>setSettingsOpen(false)}>{t("Cancel")}</Button>
        <Button disabled={busy||modelLoading||(modelChoices!==null&&modelDraft!==""&&!modelChoices.models.some(model=>model.id===modelDraft)) ||
          (modelChoices?.reasoningEffort!==undefined&&modelChoices.reasoningEffort!==null&&effortDraft!==""&&
           !modelChoices.reasoningEffort.choices.some(choice=>choice.id===effortDraft))}
          onClick={()=>void saveSettings()}>{t("Save")}</Button></DialogFooter>
      </DialogContent>
    </Dialog>
    <Dialog open={grantOpen} onOpenChange={setGrantOpen}>
      <DialogContent><DialogHeader><DialogTitle>{t("Grant Profile")}</DialogTitle>
        <DialogDescription>{t("Choose a Profile to authorize Agent delegation. Tool membership and Policy still apply.")}</DialogDescription>
      </DialogHeader>
      <label htmlFor="agent-profile-selection" className="text-sm font-medium">{t("Profile")}</label>
      <select id="agent-profile-selection" value={selectedProfile} onChange={e=>setSelectedProfile(e.target.value)}
        className="h-10 w-full rounded-xl border border-input bg-background px-3 text-sm">
        {options.length?options.map(p=><option key={p.profileId} value={p.profileId}>{p.displayName}</option>):
          <option value="">{t("No available Profiles")}</option>}
      </select>
      <DialogFooter><Button variant="outline" onClick={()=>setGrantOpen(false)}>{t("Cancel")}</Button>
        <Button disabled={!selectedProfile||busy} onClick={()=>void addGrant()}>{t("Grant")}</Button></DialogFooter>
      </DialogContent>
    </Dialog>

    <Dialog open={confirm!==null} onOpenChange={open=>{if(!open)setConfirm(null)}}>
      <DialogContent><DialogHeader><DialogTitle>{t(confirm?.kind==="revoke"?"Revoke Agent access":confirm?.kind==="cancel"?"Cancel Agent task":"Delete agent")}</DialogTitle>
        <DialogDescription>{t(confirm?.kind==="revoke"?"Revocation blocks new delegations; existing tasks may finish.":confirm?.kind==="cancel"?"Cancellation requests a stop but does not prove the Agent stopped.":"Agent removal is blocked when task history still references it.")}</DialogDescription>
      </DialogHeader><DialogFooter>
        <Button variant="outline" disabled={busy} onClick={()=>setConfirm(null)}>{t("Cancel")}</Button>
        <Button variant="destructive" disabled={busy} onClick={()=>void confirmOperation()}>{t("Confirm")}</Button>
      </DialogFooter></DialogContent>
    </Dialog>

    <Dialog open={viewTask!==null} onOpenChange={open=>{if(!open)setViewTask(null)}}>
      <DialogContent className="max-h-[85vh] overflow-y-auto">
        <DialogHeader><DialogTitle>{t("Agent task result")}</DialogTitle>
          <DialogDescription>{viewTask?.taskId} · {viewTask?.status}</DialogDescription>
        </DialogHeader>
        {viewTask?.model||viewTask?.reasoningEffort||viewTask?.workspace?<dl className="grid grid-cols-2 gap-3 text-sm">
          {viewTask.model?<div><dt className="text-xs text-muted-foreground">{t("Agent model")}</dt><dd>{viewTask.model}</dd></div>:null}
          {viewTask.reasoningEffort?<div><dt className="text-xs text-muted-foreground">{t("Reasoning effort")}</dt><dd>{viewTask.reasoningEffort}</dd></div>:null}
          {viewTask.workspace?<div><dt className="text-xs text-muted-foreground">{t("Workspace")}</dt><dd>{viewTask.workspace}</dd></div>:null}
        </dl>:null}
        {viewTask?.result?<pre className="max-h-[45vh] overflow-auto whitespace-pre-wrap break-words rounded-lg bg-muted p-4 text-xs">{viewTask.result}</pre>:
          <p className="text-sm text-muted-foreground">{viewTask?.diagnostic||t("No result available.")}</p>}
        <DialogFooter><Button variant="outline" onClick={()=>setViewTask(null)}>{t("Close")}</Button></DialogFooter>
      </DialogContent>
    </Dialog>
  </>;
}
