import { SandboxJobsAccess } from "./sandbox-jobs-access";
import { useEffect, useState } from "react";
import type { ManagementClient, Sandbox, SandboxToolSuite } from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@mender/ui/components/table";
import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { ErrorNotice } from "../../shared/page";
import { DataPanel, EmptyPanelState } from "../../shared/workbench";

// Read-only Backend capabilities; canonical Jobs Tools cannot be installed or removed per Sandbox.
export function SandboxJobsPanel({client,sandbox}:{client:ManagementClient;sandbox:Sandbox}) {
  const [suite,setSuite]=useState<SandboxToolSuite|null>(null);
  const [loaded,setLoaded]=useState(false);
  const [error,setError]=useState("");
  useEffect(()=>{
    let active=true;
    void client.getSandboxJobTools(sandbox.sandboxId).then(item=>{
      if(active){setSuite(item);setLoaded(true);setError("");}
    }).catch((reason:unknown)=>{if(active){setLoaded(true);setError(errorMessage(reason));}});
    return()=>{active=false;};
  },[client,sandbox.sandboxId]);
  return <div className="space-y-4"><DataPanel title={t("SSH Durable Jobs capabilities")} description={sandbox.name}>
    <div className="space-y-2 border-b border-border px-5 py-4">
      {suite?<Badge variant="outline">{t(suite.available?"Ready":"Incompatible")}</Badge>:null}
      <p className="text-xs leading-5 text-muted-foreground">{suite?.issue || t("Shared native Jobs tools require explicit Profile ToolBinding, SSH Sandbox capability and Policy authorization.")}</p>
    </div>
    {error?<div className="p-4"><ErrorNotice message={error}/></div>:null}
    {!loaded?<p className="p-5 text-sm text-muted-foreground">{t("Loading Sandbox tools…")}</p>
      :suite?.tools.length?<Table><TableHeader><TableRow>
        <TableHead>{t("Tool")}</TableHead><TableHead>{t("Source key")}</TableHead>
      </TableRow></TableHeader><TableBody>{suite.tools.map(tool=><TableRow key={tool.toolId}>
        <TableCell className="font-medium" title={tool.toolId}>{tool.title}</TableCell>
        <TableCell className="font-mono text-xs">{tool.sourceKey}</TableCell>
      </TableRow>)}</TableBody></Table>
      :<EmptyPanelState><p className="text-sm text-muted-foreground">{t("No compatible Jobs capabilities found.")}</p></EmptyPanelState>}
  </DataPanel><SandboxJobsAccess client={client} sandbox={sandbox}/></div>;
}
