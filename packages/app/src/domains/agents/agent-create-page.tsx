import { useState } from "react";
import { Navigate, useNavigate, useSearchParams } from "react-router";
import type { ManagementClient } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import { Input } from "@mender/ui/components/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@mender/ui/components/select";
import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { FormInfo } from "../../shared/form-info";
import { ErrorNotice } from "../../shared/page";
import { PageToolbar } from "../../shared/page-toolbar";
import { DataPanel } from "../../shared/workbench";
import {
  defaultWorkspacePlaceholder, isAbsoluteWorkspacePath,
} from "../../shared/workspace-paths";
import {
  agentBackendForPluginId, agentBackendLabel, type AgentBackend,
} from "./agent-backends";

type Step = "configure" | "review";

export function AgentCreatePage({
  client, chooseLocalDirectory,
}:{
  client:ManagementClient;
  chooseLocalDirectory?:()=>Promise<string|null>;
}){
  const [params]=useSearchParams();
  const navigate=useNavigate();
  const pluginId=params.get("pluginId");
  const fromPreset=params.get("from")==="agents";
  // Every create route is backend-pinned. The registry is the only selector.
  if (pluginId === null) return <Navigate to="/agents" replace />;
  const pinnedBackend=agentBackendForPluginId(pluginId);
  if(pinnedBackend===null){
    return <>
      <PageToolbar actions={<Button variant="outline" onClick={()=>void navigate(fromPreset?"/agents":"/plugins")}>{t("Back")}</Button>}/>
      <ErrorNotice message={t("Unknown Agent backend plugin.")}/>
    </>;
  }
  // React Router may keep this route mounted while pluginId changes. Remount
  // the focused form, so a previous plugin's backend/draft cannot carry over.
  return <AgentCreateForm key={pluginId} client={client}
    {...(chooseLocalDirectory===undefined?{}:{chooseLocalDirectory})} backend={pinnedBackend}
    returnTo={fromPreset?"/agents":"/plugins"}/>;
}

function AgentCreateForm({
  client, chooseLocalDirectory, backend, returnTo,
}:{
  client:ManagementClient;
  chooseLocalDirectory?:()=>Promise<string|null>;
  backend:AgentBackend;
  returnTo:string;
}){
  const navigate=useNavigate();
  const [step,setStep]=useState<Step>("configure");
  const [name,setName]=useState("");
  const [root,setRoot]=useState("");
  const [defaultModel,setDefaultModel]=useState("");
  const [modelSettingsOpen,setModelSettingsOpen]=useState(false);
  const [isolation,setIsolation]=useState<"restricted_host"|"host">("restricted_host");
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState("");
  const trimmedName=name.trim();
  const trimmedRoot=root.trim();
  const invalidName=name.length>0&&trimmedName.length===0;
  const invalidRoot=root.length>0&&!isAbsoluteWorkspacePath(trimmedRoot);
  const valid=trimmedName.length>0&&trimmedName.length<=120&&isAbsoluteWorkspacePath(trimmedRoot);
  const stepNumber=step==="configure"?1:2;
  const totalSteps=2;

  async function create(){
    if(!valid || busy || step!=="review")return;
    setBusy(true);setError("");
    try{
      const item=await client.createAgent({
        name:trimmedName,backend,workspaceRoot:trimmedRoot,isolation,defaultModel:defaultModel.trim(),
      });
      void navigate("/agents/"+item.agentId);
    }catch(reason){setError(errorMessage(reason));setBusy(false);}
  }
  async function choose(){
    if(!chooseLocalDirectory)return;
    try{const path=await chooseLocalDirectory();if(path){setRoot(path);setError("");}}catch(reason){setError(errorMessage(reason));}
  }

  return <>
    <PageToolbar actions={<Button variant="outline" onClick={()=>void navigate(returnTo)}>{t("Back")}</Button>}/>
    <DataPanel title="Configure agent">
      <div className="mx-auto flex max-w-2xl flex-col gap-5 p-5 sm:p-8">
        <div className="text-xs text-muted-foreground">{t("Step {{step}} of {{total}}",{step:stepNumber,total:totalSteps})}</div>
        <h2 className="sr-only">{t(step==="configure"?"Configure agent":"Review configuration")}</h2>
        {step==="configure"?<>

          <div className="grid gap-2">
            <label htmlFor="agent-name" className="text-sm font-medium">{t("Agent name")}</label>
            <Input id="agent-name" maxLength={120} placeholder={t("Code Reviewer")} value={name}
              aria-invalid={invalidName} aria-describedby={invalidName?"agent-name-error":undefined}
              onChange={e=>{setName(e.target.value);setError("");}}/>
            {invalidName?<p id="agent-name-error" role="alert" className="text-xs text-destructive">{t("Enter an agent name, not just spaces.")}</p>:null}
          </div>
          <div className="grid gap-2">
            <label htmlFor="agent-root" className="text-sm font-medium">{t("Workspace directory")}</label>
            <div className="flex flex-wrap gap-2"><Input id="agent-root" value={root}
              aria-invalid={invalidRoot} aria-describedby={invalidRoot?"agent-root-error":undefined}
              onChange={e=>{setRoot(e.target.value);setError("");}}
              placeholder={defaultWorkspacePlaceholder()} className="min-w-[12rem] flex-1"/>
              {chooseLocalDirectory?<Button variant="outline" onClick={()=>void choose()}>{t("Choose local directory")}</Button>:null}
            </div>
            {invalidRoot?<p id="agent-root-error" role="alert" className="text-xs text-destructive">{t("Enter an absolute workspace directory.")}</p>:null}
            {error?<p role="alert" className="text-xs text-destructive">{error}</p>:null}
          </div>
          <details open={modelSettingsOpen} className="rounded-xl border border-border/70">
            <summary onClick={event=>{event.preventDefault();setModelSettingsOpen(open=>!open);}}
              className="cursor-pointer px-3 py-2.5 text-sm font-medium">{t("Advanced settings")}</summary>
            <div className="grid gap-2 border-t border-border/70 p-3">
              <div className="flex items-center gap-1">
                <label htmlFor="agent-default-model" className="text-sm font-medium">{t("Default model (optional)")}</label>
                <FormInfo label={t("Default model (optional)")}>{t("The Agent must advertise the selected model when a task runs.")}</FormInfo>
              </div>
              <Input id="agent-default-model" maxLength={128} value={defaultModel} onChange={e=>{setDefaultModel(e.target.value);setError("");}} placeholder={t("Use Agent default")}/>
            </div>
          </details>
          <div className="grid gap-2">
            <div className="flex items-center gap-1">
              <label htmlFor="agent-isolation" className="text-sm font-medium">{t("Process isolation")}</label>
              <FormInfo label={t("Process isolation")}>{t("The isolation setting controls which host files and credentials the Agent process can access.")}</FormInfo>
            </div>
            <Select
              value={isolation}
              onValueChange={(next) => {
                if (next === "restricted_host" || next === "host") {
                  setIsolation(next);
                  setError("");
                }
              }}
            >
              <SelectTrigger
                id="agent-isolation"
                aria-label={t("Process isolation")}
                className="h-10 w-full rounded-xl bg-background px-3"
              >
                <SelectValue>
                  {isolation === "host" ? t("Host (not isolated)") : t("Restricted host")}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="restricted_host">{t("Restricted host")}</SelectItem>
                <SelectItem value="host">{t("Host (not isolated)")}</SelectItem>
              </SelectContent>
            </Select>
            {isolation==="host" ? <p role="note" className="text-xs font-medium">{t("Host mode can access files and credentials with the Runtime user's permissions.")}</p> : null}
            {backend==="antigravity"?
              <FormInfo label={t("Antigravity permissions")}>{t("Antigravity uses its native tool permissions. Configure workspace authorization after creation to enable 24-hour YOLO; it is off by default.")}</FormInfo>:null}
            {backend==="antigravity"&&isolation==="restricted_host"?
              <p className="text-xs text-muted-foreground">{t("Antigravity may need local account and keychain access that Restricted host prevents. Choose Host explicitly only if you accept its filesystem and credential access.")}</p>:null}
            {backend==="opencode"&&isolation==="restricted_host"?
              <p className="text-xs text-muted-foreground">{t("OpenCode may need local account and config access that Restricted host prevents. Choose Host explicitly only if you accept its filesystem and credential access.")}</p>:null}
          </div>
          <div className="flex justify-between gap-2">
            <Button variant="outline" onClick={()=>void navigate(returnTo)}>{t("Back")}</Button>
            <Button disabled={!valid} onClick={()=>setStep("review")}>{t("Review")}</Button>
          </div>
        </>:<>
          <div className="space-y-3 rounded-xl border border-border p-5 text-sm">
            <div><span className="text-muted-foreground">{t("Backend")}: </span>{agentBackendLabel(backend)}</div>
            <div><span className="text-muted-foreground">{t("Name")}: </span>{trimmedName}</div>
            <div className="break-all"><span className="text-muted-foreground">{t("Workspace")}: </span>{trimmedRoot}</div>
            <div><span className="text-muted-foreground">{t("Default model (optional)")}: </span>{defaultModel.trim()||t("Use Agent default")}</div>
            <div><span className="text-muted-foreground">{t("Isolation")}: </span>{isolation==="host"?t("Host (not isolated)"):t("Restricted host")}</div>
          </div>
          {error?<p role="alert" className="text-sm text-destructive">{error}</p>:null}
          <div className="flex items-center gap-1 text-sm">
            <span>{t("Creating does not grant Profile access.")}</span>
            <FormInfo label={t("Save configuration")}>{t("Saving an Agent workspace prepares governed tools without starting a model, approving tool requests, or granting Profile access.")}</FormInfo>
          </div>
          <div className="flex justify-between gap-2">
            <Button variant="outline" disabled={busy} onClick={()=>{setError("");setStep("configure");}}>{t("Back")}</Button>
            <Button disabled={busy||!valid} onClick={()=>void create()}>{busy?t("Saving…"):t("Save configuration")}</Button>
          </div>
        </>}
      </div>
    </DataPanel>
  </>;
}
