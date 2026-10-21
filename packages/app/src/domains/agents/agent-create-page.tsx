import { useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import type { MenderClient } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import { Input } from "@mender/ui/components/input";
import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { FormHeading, FormInfo } from "../../shared/form-info";
import { ErrorNotice } from "../../shared/page";
import { PageToolbar } from "../../shared/page-toolbar";
import { DataPanel } from "../../shared/workbench";
import {
  agentBackendDescription, agentBackendForPluginId, agentBackendLabel, agentBackendOptions,
  type AgentBackend,
} from "./agent-backends";

type Step = "select" | "configure" | "review";

export function AgentCreatePage({
  client, chooseLocalDirectory,
}:{
  client:MenderClient;
  chooseLocalDirectory?:()=>Promise<string|null>;
}){
  const [params]=useSearchParams();
  const navigate=useNavigate();
  const pluginId=params.get("pluginId");
  const pinnedBackend=pluginId===null?null:agentBackendForPluginId(pluginId);
  if(pluginId!==null&&pinnedBackend===null){
    return <>
      <PageToolbar actions={<Button variant="outline" onClick={()=>void navigate("/plugins")}>{t("Back")}</Button>}/>
      <ErrorNotice message={t("Unknown Agent backend plugin.")}/>
    </>;
  }
  // React Router may keep this route mounted while pluginId changes. Remount
  // the focused form, so a previous plugin's backend/draft cannot carry over.
  return <AgentCreateForm key={pluginId??"generic"} client={client}
    {...(chooseLocalDirectory===undefined?{}:{chooseLocalDirectory})} pinnedBackend={pinnedBackend}/>;
}

function AgentCreateForm({
  client, chooseLocalDirectory, pinnedBackend,
}:{
  client:MenderClient;
  chooseLocalDirectory?:()=>Promise<string|null>;
  pinnedBackend:AgentBackend|null;
}){
  const navigate=useNavigate();
  const [step,setStep]=useState<Step>(pinnedBackend?"configure":"select");
  const [selectedBackend,setSelectedBackend]=useState<AgentBackend|null>(pinnedBackend);
  const backend=pinnedBackend??selectedBackend;
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
  const invalidRoot=root.length>0&&!trimmedRoot.startsWith("/");
  const valid=backend!==null&&trimmedName.length>0&&trimmedName.length<=120&&trimmedRoot.startsWith("/");
  const stepNumber=step==="select"?1:step==="configure"?(pinnedBackend?1:2):(pinnedBackend?2:3);
  const totalSteps=pinnedBackend?2:3;

  async function create(){
    if(!valid || backend===null || busy || step!=="review")return;
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
    <PageToolbar actions={<Button variant="outline" onClick={()=>void navigate(pinnedBackend?"/plugins":"/agents")}>{t(pinnedBackend?"Back":"Back to agents")}</Button>}/>
    <DataPanel title="Add agent">
      <div className="mx-auto flex max-w-2xl flex-col gap-5 p-5 sm:p-8">
        <div className="text-xs text-muted-foreground">{t("Step {{step}} of {{total}}",{step:stepNumber,total:totalSteps})}</div>
        {step!=="select"?<h2 className="sr-only">{t(step==="configure"?"Configure agent":"Review agent")}</h2>:null}
        {step==="select"?<>
          <FormHeading label={t("Select backend")} help={t("Choose an installed Agent CLI backend. No Agent process starts during setup.")} />
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">{agentBackendOptions.map(choice=>
            <button key={choice} type="button" aria-pressed={backend===choice}
              onClick={()=>{if(selectedBackend!==choice)setDefaultModel("");setSelectedBackend(choice);}}
              className={`rounded-xl border p-5 text-left transition-colors focus-visible:ring-2 focus-visible:ring-ring ${backend===choice?"border-foreground bg-secondary/40":"border-border hover:border-foreground/50"}`}>
              <div className="font-medium">{agentBackendLabel(choice)}</div>
              <p className="mt-2 text-xs leading-5 text-muted-foreground">{agentBackendDescription(choice)}</p>
            </button>)}</div>
          <div className="flex justify-end"><Button disabled={backend===null} onClick={()=>setStep("configure")}>{t("Continue")}</Button></div>
        </>:step==="configure"?<>
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
              placeholder="/Users/example/project" className="min-w-[12rem] flex-1"/>
              {chooseLocalDirectory?<Button variant="outline" onClick={()=>void choose()}>{t("Choose local directory")}</Button>:null}
            </div>
            {invalidRoot?<p id="agent-root-error" role="alert" className="text-xs text-destructive">{t("Enter an absolute workspace directory starting with /.")}</p>:null}
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
            <select id="agent-isolation" value={isolation} onChange={e=>{setIsolation(e.target.value as typeof isolation);setError("");}}
              className="h-10 rounded-xl border border-input bg-background px-3 text-sm">
              <option value="restricted_host">{t("Restricted host")}</option>
              <option value="host">{t("Host (not isolated)")}</option>
            </select>
            {isolation==="host" ? <p role="note" className="text-xs font-medium">{t("Host mode can access files and credentials with the Runtime user's permissions.")}</p> : null}
            {backend==="antigravity"?
              <FormInfo label={t("Antigravity permissions")}>{t("Antigravity uses its native tool permissions. Configure workspace authorization after creation to enable 24-hour YOLO; it is off by default.")}</FormInfo>:null}
            {backend==="antigravity"&&isolation==="restricted_host"?
              <p className="text-xs text-muted-foreground">{t("Antigravity may need local account and keychain access that Restricted host prevents. Choose Host explicitly only if you accept its filesystem and credential access.")}</p>:null}
            {backend==="opencode"&&isolation==="restricted_host"?
              <p className="text-xs text-muted-foreground">{t("OpenCode may need local account and config access that Restricted host prevents. Choose Host explicitly only if you accept its filesystem and credential access.")}</p>:null}
          </div>
          <div className="flex justify-between gap-2">
            <Button variant="outline" onClick={()=>pinnedBackend?void navigate("/plugins"):setStep("select")}>{t("Back")}</Button>
            <Button disabled={!valid} onClick={()=>setStep("review")}>{t("Review")}</Button>
          </div>
        </>:<>
          <div className="space-y-3 rounded-xl border border-border p-5 text-sm">
            <div><span className="text-muted-foreground">{t("Backend")}: </span>{backend===null?t("Select backend"):agentBackendLabel(backend)}</div>
            <div><span className="text-muted-foreground">{t("Name")}: </span>{trimmedName}</div>
            <div className="break-all"><span className="text-muted-foreground">{t("Workspace")}: </span>{trimmedRoot}</div>
            <div><span className="text-muted-foreground">{t("Default model (optional)")}: </span>{defaultModel.trim()||t("Use Agent default")}</div>
            <div><span className="text-muted-foreground">{t("Isolation")}: </span>{isolation==="host"?t("Host (not isolated)"):t("Restricted host")}</div>
          </div>
          {error?<p role="alert" className="text-sm text-destructive">{error}</p>:null}
          <div className="flex items-center gap-1 text-sm">
            <span>{t("Creating does not grant Profile access.")}</span>
            <FormInfo label={t("Create agent")}>{t("Creating an Agent prepares its governed tools without starting a model, approving tool requests, or granting Profile access.")}</FormInfo>
          </div>
          <div className="flex justify-between gap-2">
            <Button variant="outline" disabled={busy} onClick={()=>{setError("");setStep("configure");}}>{t("Back")}</Button>
            <Button disabled={busy||!valid} onClick={()=>void create()}>{busy?t("Creating…"):t("Create agent")}</Button>
          </div>
        </>}
      </div>
    </DataPanel>
  </>;
}
