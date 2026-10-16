import { useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import type { MenderClient } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import { Input } from "@mender/ui/components/input";
import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
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
  const [isolation,setIsolation]=useState<"restricted_host"|"host">("restricted_host");
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState("");
  const valid=backend!==null&&name.trim().length>0&&name.trim().length<=120&&root.startsWith("/");

  async function create(){
    if(!valid || backend===null || busy || step!=="review")return;
    setBusy(true);setError("");
    try{
      const item=await client.createAgent({
        name:name.trim(),backend,workspaceRoot:root.trim(),isolation,defaultModel:defaultModel.trim(),
      });
      void navigate("/agents/"+item.agentId);
    }catch(reason){setError(errorMessage(reason));setBusy(false);}
  }
  async function choose(){
    if(!chooseLocalDirectory)return;
    try{const path=await chooseLocalDirectory();if(path)setRoot(path);}catch(reason){setError(errorMessage(reason));}
  }

  return <>
    <PageToolbar actions={<Button variant="outline" onClick={()=>void navigate(pinnedBackend?"/plugins":"/agents")}>{t(pinnedBackend?"Back":"Back to agents")}</Button>}/>
    {error?<ErrorNotice message={error}/>:null}
    <DataPanel title="Add agent">
      <div className="mx-auto flex max-w-2xl flex-col gap-5 p-5 sm:p-8">
        <div className="text-xs text-muted-foreground">{t(step==="select"?"Select backend":step==="configure"?"Configure agent":"Review agent")}</div>
        {step==="select"?<>
          <p className="text-sm text-muted-foreground">{t("Choose an installed Agent CLI backend. No Agent process starts during setup.")}</p>
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
            <Input id="agent-name" maxLength={120} placeholder={t("Code Reviewer")} value={name} onChange={e=>setName(e.target.value)}/>
          </div>
          <div className="grid gap-2">
            <label htmlFor="agent-root" className="text-sm font-medium">{t("Workspace directory")}</label>
            <div className="flex gap-2"><Input id="agent-root" value={root} onChange={e=>setRoot(e.target.value)}
              placeholder="/Users/example/project" className="min-w-0 flex-1"/>
              {chooseLocalDirectory?<Button variant="outline" onClick={()=>void choose()}>{t("Choose local directory")}</Button>:null}
            </div>
          </div>
          <div className="grid gap-2">
            <label htmlFor="agent-default-model" className="text-sm font-medium">{t("Default model (optional)")}</label>
            <Input id="agent-default-model" maxLength={128} value={defaultModel} onChange={e=>setDefaultModel(e.target.value)} placeholder={t("Use Agent default")}/>
            <p className="text-xs text-muted-foreground">{t("The Agent must advertise the selected model when a task runs.")}</p>
          </div>
          <div className="grid gap-2">
            <label htmlFor="agent-isolation" className="text-sm font-medium">{t("Process isolation")}</label>
            <select id="agent-isolation" value={isolation} onChange={e=>setIsolation(e.target.value as typeof isolation)}
              className="h-10 rounded-xl border border-input bg-background px-3 text-sm">
              <option value="restricted_host">{t("Restricted host")}</option>
              <option value="host">{t("Host (not isolated)")}</option>
            </select>
            <p className="text-xs text-muted-foreground">{t("Host mode can access files and credentials with the Runtime user's permissions.")}</p>
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
            <div><span className="text-muted-foreground">{t("Name")}: </span>{name}</div>
            <div className="break-all"><span className="text-muted-foreground">{t("Workspace")}: </span>{root}</div>
            <div><span className="text-muted-foreground">{t("Default model (optional)")}: </span>{defaultModel||t("Use Agent default")}</div>
            <div><span className="text-muted-foreground">{t("Isolation")}: </span>{isolation==="host"?t("Host (not isolated)"):t("Restricted host")}</div>
          </div>
          <p className="text-xs leading-5 text-muted-foreground">{t("Creating an Agent prepares its governed tools without starting a model, approving tool requests, or granting Profile access.")}</p>
          <div className="flex justify-between gap-2">
            <Button variant="outline" disabled={busy} onClick={()=>setStep("configure")}>{t("Back")}</Button>
            <Button disabled={busy||!valid} onClick={()=>void create()}>{busy?t("Creating…"):t("Create agent")}</Button>
          </div>
        </>}
      </div>
    </DataPanel>
  </>;
}
