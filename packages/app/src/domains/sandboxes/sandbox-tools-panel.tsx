import { useEffect, useState } from "react";
import { useNavigate } from "react-router";
import { SandboxJobsPanel } from "./sandbox-jobs-panel";
import { SandboxJobRecoveryPanel } from "./sandbox-job-recovery-panel";
import type { MenderClient, Sandbox, SandboxToolSuite } from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@mender/ui/components/dialog";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@mender/ui/components/table";

import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { ErrorNotice } from "../../shared/page";
import { DataPanel, EmptyPanelState } from "../../shared/workbench";
import { EmptyState } from "../../shared/page";

// One resource-bound category of information; GET cannot provision a
// Provider, open a Sandbox, or expose any Tool to a Profile.
function WorkspaceToolsPanel({client,sandbox}:{client:MenderClient;sandbox:Sandbox}){
  const navigate=useNavigate();
  const [suite,setSuite]=useState<SandboxToolSuite|null>(null);
  const [loaded,setLoaded]=useState(false);
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState("");
  const [confirmRemove,setConfirmRemove]=useState(false);
  useEffect(()=>{
    let live=true;
    void client.getSandboxTools(sandbox.sandboxId).then(value=>{
      if(live){setSuite(value);setLoaded(true);setError("");}
    }).catch((reason:unknown)=>{if(live){setError(errorMessage(reason));setLoaded(true);}});
    return()=>{live=false;};
  },[client,sandbox.sandboxId]);

  async function prepare(){
    if(busy)return;
    setBusy(true);setError("");
    try{setSuite(await client.ensureSandboxTools(sandbox.sandboxId));}
    catch(reason){setError(errorMessage(reason));}
    finally{setBusy(false);}
  }
  async function remove(){
    if(busy||!confirmRemove)return;
    setBusy(true);setError("");
    try{
      await client.removeSandboxTools(sandbox.sandboxId);
      setSuite(await client.getSandboxTools(sandbox.sandboxId));
      setConfirmRemove(false);
    }catch(reason){setError(errorMessage(reason));}
    finally{setBusy(false);}
  }
  const status=suite?.status;
  return <>
    <DataPanel title="Sandbox Workspace Tools" description={sandbox.name}>
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border p-4">
        <div className="min-w-0 space-y-2">
          <p className="text-sm font-medium">{t("Workspace Tool Suite")}</p>
          {suite?<Badge variant="outline">{t(status==="ready"?"Ready":
            status==="legacy"?"Existing tools":status==="incomplete"?"Incomplete":
            status==="conflict"?"Conflict":status==="incompatible"?"Incompatible":"Not prepared")}</Badge>:null}
          <p className="text-xs leading-5 text-muted-foreground">
            {suite?.status==="conflict"
              ?t("Multiple Workspace Providers target this Sandbox. Resolve the conflict in advanced Provider management.")
              :suite?.status==="legacy"
                ?t("Existing Workspace Tools can be adopted without changing ProviderID or ToolIDs.")
                :suite?.status==="ready"
                  ?t("Tools are prepared. Profile ToolBindings, Sandbox grants and Policy are still required.")
                  :suite?.issue
                    ?suite.issue
                    :t("Prepare Workspace Tools explicitly; this does not grant Profile access.")}
          </p>
        </div>
        {suite?.status==="conflict"?
          <Button variant="outline" size="sm" onClick={()=>void navigate("/providers?showInternal=1")}>{t("Review internal providers")}</Button>
          :<div className="flex flex-wrap gap-2">
            {suite?.available && suite.status!=="ready"?
              <Button size="sm" disabled={busy} onClick={()=>void prepare()}>
                {t(suite.status==="legacy"?"Adopt Workspace Tools":"Prepare Workspace Tools")}
              </Button>:null}
            {suite?.providerId && suite.status!=="legacy"?
              <Button size="sm" variant="outline" disabled={busy}
                onClick={()=>setConfirmRemove(true)}>{t("Retire Workspace Tools")}</Button>:null}
          </div>}
      </div>
      {error?<div className="p-4"><ErrorNotice message={error}/></div>:null}
      {!loaded?<p className="p-5 text-sm text-muted-foreground">{t("Loading Sandbox tools…")}</p>
        :suite?.tools.length?<><Table><TableHeader><TableRow>
          <TableHead>{t("Tool")}</TableHead><TableHead>{t("Source key")}</TableHead>
        </TableRow></TableHeader><TableBody>{suite.tools.map(tool=><TableRow key={tool.toolId}>
          <TableCell className="font-medium" title={tool.toolId}>{sandbox.name} · {tool.title}</TableCell>
          <TableCell className="font-mono text-xs">{tool.sourceKey}</TableCell>
        </TableRow>)}</TableBody></Table></>:
        <EmptyPanelState><EmptyState title="No Sandbox tools"
          description="No Workspace Tool Suite is currently bound to this Sandbox."/></EmptyPanelState>}
    </DataPanel>

    <Dialog open={confirmRemove} onOpenChange={open=>{if(!busy)setConfirmRemove(open);}}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("Retire Workspace Tools")}</DialogTitle>
          <DialogDescription>{t("Remove the owned Tool Suite only when no Profile or Policy references remain. Sandbox files are not deleted.")}</DialogDescription>
        </DialogHeader>
        {error?<ErrorNotice message={error}/>:null}
        <DialogFooter>
          <Button variant="outline" disabled={busy} onClick={()=>setConfirmRemove(false)}>{t("Cancel")}</Button>
          <Button variant="destructive" disabled={busy} onClick={()=>void remove()}>{t("Confirm")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </>;
}

// Keep one category visible at a time. Preparing Workspace does not prepare
// Jobs, and selecting Jobs never implicitly changes Profile access.
export function SandboxToolsPanel({client,sandbox}:{client:MenderClient;sandbox:Sandbox}){
  const [category,setCategory]=useState<"workspace"|"jobs"|"recovery">("workspace");
  if(sandbox.backend!=="ssh_remote")return <WorkspaceToolsPanel client={client} sandbox={sandbox}/>;
  return <div className="space-y-4">
    <div className="flex items-center gap-2" role="tablist" aria-label={t("Sandbox tool suite")}>
      <Button size="sm" role="tab" aria-selected={category==="workspace"} variant={category==="workspace"?"default":"outline"}
        onClick={()=>setCategory("workspace")}>{t("Workspace Tool Suite")}</Button>
      <Button size="sm" role="tab" aria-selected={category==="jobs"} variant={category==="jobs"?"default":"outline"}
        onClick={()=>setCategory("jobs")}>{t("Jobs Tool Suite")}</Button>
      <Button size="sm" role="tab" aria-selected={category==="recovery"} variant={category==="recovery"?"default":"outline"}
        onClick={()=>setCategory("recovery")}>{t("SSH Job Recovery")}</Button>
    </div>
    {category==="workspace"?<WorkspaceToolsPanel client={client} sandbox={sandbox}/>:
      category==="jobs"?<SandboxJobsPanel client={client} sandbox={sandbox}/>:
      <SandboxJobRecoveryPanel client={client} sandbox={sandbox}/>}
  </div>;
}
