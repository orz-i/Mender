import { useEffect, useState } from "react";
import type { ManagementClient, Sandbox, SandboxToolSuite } from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@mender/ui/components/table";
import { SandboxJobsPanel } from "./sandbox-jobs-panel";
import { SandboxJobRecoveryPanel } from "./sandbox-job-recovery-panel";
import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { ErrorNotice } from "../../shared/page";
import { DataPanel, EmptyPanelState } from "../../shared/workbench";

// Capability inspection only. Tools are canonical and are not installed on or retired from individual Sandboxes.
function WorkspaceCapabilities({client,sandbox}:{client:ManagementClient;sandbox:Sandbox}) {
  const [suite,setSuite]=useState<SandboxToolSuite|null>(null);
  const [loaded,setLoaded]=useState(false);
  const [error,setError]=useState("");
  useEffect(()=>{
    let active=true;
    void client.getSandboxTools(sandbox.sandboxId).then(item=>{
      if(active){setSuite(item);setLoaded(true);setError("");}
    }).catch((reason:unknown)=>{if(active){setLoaded(true);setError(errorMessage(reason));}});
    return()=>{active=false;};
  },[client,sandbox.sandboxId]);
  return <DataPanel title="Workspace capabilities" description={sandbox.name}>
    <div className="space-y-2 border-b border-border p-4">
      {suite?<Badge variant="outline">{t(suite.available?"Ready":"Incompatible")}</Badge>:null}
      <p className="text-xs leading-5 text-muted-foreground">{suite?.issue || t("These are shared native tools. Execution requires Profile ToolBinding, Workspace and Sandbox grants, and Policy approval.")}</p>
    </div>
    {error?<div className="p-4"><ErrorNotice message={error}/></div>:null}
    {!loaded?<p className="p-5 text-sm text-muted-foreground">{t("Loading Sandbox tools…")}</p>
      :suite?.tools.length?<Table><TableHeader><TableRow>
        <TableHead>{t("Tool")}</TableHead><TableHead>{t("Source key")}</TableHead>
      </TableRow></TableHeader><TableBody>{suite.tools.map(tool=><TableRow key={tool.toolId}>
        <TableCell className="font-medium" title={tool.toolId}>{tool.title}</TableCell>
        <TableCell className="font-mono text-xs">{tool.sourceKey}</TableCell>
      </TableRow>)}</TableBody></Table>
      :<EmptyPanelState><p className="text-sm text-muted-foreground">{t("No compatible Workspace capabilities found.")}</p></EmptyPanelState>}
  </DataPanel>;
}

export function SandboxToolsPanel({client,sandbox}:{client:ManagementClient;sandbox:Sandbox}) {
  const [category,setCategory]=useState<"workspace"|"jobs"|"recovery">("workspace");
  if(sandbox.backend!=="ssh_remote")return <WorkspaceCapabilities client={client} sandbox={sandbox}/>;
  return <div className="space-y-4">
    <div className="flex items-center gap-2" role="tablist" aria-label={t("Sandbox capabilities")}>
      <Button size="sm" role="tab" aria-selected={category==="workspace"} variant={category==="workspace"?"default":"outline"}
        onClick={()=>setCategory("workspace")}>{t("Workspace capabilities")}</Button>
      <Button size="sm" role="tab" aria-selected={category==="jobs"} variant={category==="jobs"?"default":"outline"}
        onClick={()=>setCategory("jobs")}>{t("SSH Jobs capabilities")}</Button>
      <Button size="sm" role="tab" aria-selected={category==="recovery"} variant={category==="recovery"?"default":"outline"}
        onClick={()=>setCategory("recovery")}>{t("SSH Job Recovery")}</Button>
    </div>
    {category==="workspace"?<WorkspaceCapabilities client={client} sandbox={sandbox}/>
      :category==="jobs"?<SandboxJobsPanel client={client} sandbox={sandbox}/>
      :<SandboxJobRecoveryPanel client={client} sandbox={sandbox}/>}
  </div>;
}
