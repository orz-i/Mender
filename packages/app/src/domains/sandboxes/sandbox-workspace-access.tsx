import { useEffect, useMemo, useState } from "react";
import type {
  ManagementClient, Profile, ProfileToolTarget, Sandbox,
  SandboxProfileGrant, ToolSummary, WorkspaceRecord,
} from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { ErrorNotice } from "../../shared/page";
import { DataPanel } from "../../shared/workbench";

const workspaceCapability = new Map([
  ["workspace.read", "file.read"], ["workspace.search", "file.search"],
  ["workspace.edit", "file.edit"], ["workspace.shell", "process.shell"],
]);

/** Native Workspace setup is a resource/target binding, not an internal
 * Provider or Tool installation. Each action changes exactly one permission;
 * no call auto-creates a Profile ToolBinding or Sandbox grant. */
export function SandboxWorkspaceAccess({client,sandbox}: {client:ManagementClient;sandbox:Sandbox}) {
  const [expanded,setExpanded]=useState(false);
  const [workspaces,setWorkspaces]=useState<WorkspaceRecord[]>([]);
  const [profiles,setProfiles]=useState<Profile[]>([]);
  const [grants,setGrants]=useState<SandboxProfileGrant[]>([]);
  const [tools,setTools]=useState<ToolSummary[]>([]);
  const [workspaceId,setWorkspaceId]=useState("");
  const [profileId,setProfileId]=useState("");
  const [placed,setPlaced]=useState(false);
  const [workspaceGrants,setWorkspaceGrants]=useState<string[]>([]);
  const [targets,setTargets]=useState<ProfileToolTarget[]>([]);
  const [selectedTool,setSelectedTool]=useState("");
  const [revision,setRevision]=useState(0);
  const [loaded,setLoaded]=useState(false);
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState("");

  useEffect(()=>{
    if(!expanded)return;
    let active=true;
    void Promise.all([
      client.listWorkspaces(),client.listProfiles(),client.getSandboxProfileGrants(sandbox.sandboxId),
      client.browseTools({sourceId:"native:workspace",limit:100}),
    ]).then(([ws,ps,gs,catalog])=>{
      if(!active)return;
      setWorkspaces(ws.workspaces);setProfiles(ps.profiles);setGrants(gs.grants);
      setTools(catalog.tools.filter(tool=>tool.sourceId==="native:workspace"));
      setWorkspaceId(previous=>ws.workspaces.some(w=>w.workspaceId===previous)?previous:
        ws.workspaces.find(w=>w.verifiedRoot===sandbox.localRoot)?.workspaceId??"");
      setProfileId(previous=>ps.profiles.some(p=>p.profileId===previous)?previous:ps.profiles[0]?.profileId??"");
      setLoaded(true);setError("");
    }).catch((reason:unknown)=>{if(active){setLoaded(true);setError(errorMessage(reason));}});
    return()=>{active=false;};
  },[expanded,client,sandbox.sandboxId,sandbox.localRoot,revision]);

  useEffect(()=>{
    if(!expanded||!workspaceId)return;
    let active=true;
    void Promise.all([client.listWorkspacePlacements(workspaceId),client.listWorkspaceProfileGrants(workspaceId)]).then(([links,allowed])=>{
      if(!active)return;
      setPlaced(links.placements.some(p=>p.sandboxId===sandbox.sandboxId));
      setWorkspaceGrants(allowed.profileIds);
    }).catch((reason:unknown)=>{if(active)setError(errorMessage(reason));});
    return()=>{active=false;};
  },[expanded,workspaceId,client,sandbox.sandboxId,revision]);

  const selectedProfile=profiles.find(p=>p.profileId===profileId);
  const selectedProfileSlug=selectedProfile?.slug;
  useEffect(()=>{
    if(!expanded||!selectedProfileSlug)return;
    let active=true;
    void client.listProfileToolTargets(selectedProfileSlug).then(({targets})=>{
      if(active)setTargets(targets.filter(item=>item.sandboxId===sandbox.sandboxId));
    }).catch((reason:unknown)=>{if(active)setError(errorMessage(reason));});
    return()=>{active=false;};
  },[expanded,client,selectedProfileSlug,sandbox.sandboxId,revision]);
  const selectedWorkspace=workspaces.find(w=>w.workspaceId===workspaceId);
  const matchingRoot=selectedWorkspace?.verifiedRoot===sandbox.localRoot;
  const matchingTools=useMemo(()=>tools.filter(tool=>
    selectedProfile?.tools.some(binding=>binding.toolId===tool.toolId)),[tools,selectedProfile]);
  const effectiveTool=matchingTools.find(tool=>tool.toolId===selectedTool)??matchingTools[0];
  const allowedCaps=grants.find(g=>g.profileId===profileId)?.capabilities??[];
  const capability=effectiveTool?workspaceCapability.get(`workspace.${effectiveTool.name}`):undefined;
  const hasSandboxGrant=Boolean(capability&&allowedCaps.includes(capability as typeof allowedCaps[number]));
  const alreadyTargeted=targets.some(item=>item.profileId===profileId&&item.workspaceId===workspaceId&&item.toolId===effectiveTool?.toolId);
  const available=Boolean(placed&&matchingRoot&&workspaceGrants.includes(profileId)&&hasSandboxGrant);

  async function mutate(op:()=>Promise<unknown>){
    if(busy)return;
    setBusy(true);setError("");
    try {await op();setRevision(value=>value+1);}
    catch(reason){setError(errorMessage(reason));}
    finally{setBusy(false);}
  }

  if(sandbox.backend!=="local_workspace")return null;
  return <DataPanel title="Workspace execution" description={sandbox.name}>
    <div className="px-4 py-3">
      <Button variant="outline" size="sm" aria-expanded={expanded} onClick={()=>setExpanded(v=>!v)}>
        {t(expanded?"Hide Workspace setup":"Configure Workspace execution")}
      </Button>
    </div>
    {expanded?<div className="space-y-4 border-t border-border p-4">
      <p className="text-xs leading-5 text-muted-foreground">{t("A Workspace defines the project. A Sandbox defines where a selected tool runs. Selecting either never grants tool access automatically.")}</p>
      {error?<ErrorNotice message={error}/>:null}
      {!loaded?<p className="text-sm text-muted-foreground">{t("Loading Workspace access…")}</p>:<>
        <div className="space-y-2">
          <label className="block text-sm font-medium" htmlFor="sandbox-workspace-selection">{t("Project Workspace")}</label>
          <div className="flex flex-wrap gap-2">
            <select id="sandbox-workspace-selection" className="h-9 min-w-0 flex-1 rounded-lg border border-input bg-background px-2 text-sm"
              value={workspaceId} onChange={event=>{setWorkspaceId(event.target.value);setPlaced(false);setWorkspaceGrants([]);}}>
              <option value="">{t("Choose a registered Workspace")}</option>
              {workspaces.map(item=><option key={item.workspaceId} value={item.workspaceId}>{item.displayName}</option>)}
            </select>
            {!workspaces.some(w=>w.verifiedRoot===sandbox.localRoot)?
              <Button disabled={busy||!sandbox.localRoot} size="sm" variant="outline" onClick={()=>void mutate(async()=>{
                const created=await client.createWorkspace({displayName:sandbox.name,verifiedRoot:sandbox.localRoot});
                setWorkspaceId(created.workspaceId);
              })}>{t("Register this Workspace")}</Button>:null}
            {workspaceId&&matchingRoot&&!placed?<Button disabled={busy} size="sm" variant="outline"
              onClick={()=>void mutate(()=>client.linkWorkspaceSandbox(workspaceId,sandbox.sandboxId))}>{t("Attach Sandbox")}</Button>:null}
            {workspaceId&&placed?<Button disabled={busy} size="sm" variant="outline"
              onClick={()=>void mutate(()=>client.unlinkWorkspaceSandbox(workspaceId,sandbox.sandboxId))}>{t("Detach Sandbox")}</Button>:null}
          </div>
          {workspaceId&&!matchingRoot?<p className="text-xs text-muted-foreground">{t("This Workspace is on a different root; select a matching Workspace or Sandbox.")}</p>:null}
          {placed?<p className="text-xs text-muted-foreground">{t("Exact local root verified. No Profile permissions were created.")}</p>:null}
        </div>
        {workspaceId&&placed&&matchingRoot?<div className="space-y-3 border-t border-border pt-4">
          <label htmlFor="sandbox-profile-selection" className="block text-sm font-medium">{t("Profile to authorize")}</label>
          <select id="sandbox-profile-selection" className="h-9 w-full rounded-lg border border-input bg-background px-2 text-sm"
            value={profileId} onChange={event=>{setProfileId(event.target.value);setTargets([]);}}>
            {profiles.length===0?<option value="">{t("Create a Profile first")}</option>:null}
            {profiles.map(profile=><option key={profile.profileId} value={profile.profileId}>{profile.displayName}</option>)}
          </select>
          {selectedProfile?<>
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-xs text-muted-foreground">{t(workspaceGrants.includes(profileId)?"Workspace access granted":"Workspace access not granted")}</span>
              <Button size="sm" variant="outline" disabled={busy} onClick={()=>void mutate(()=>workspaceGrants.includes(profileId)
                ?client.revokeWorkspaceProfile(workspaceId,profileId)
                :client.grantWorkspaceProfile(workspaceId,profileId))}>
                {t(workspaceGrants.includes(profileId)?"Revoke Workspace access":"Grant Workspace access")}
              </Button>
            </div>
            <label htmlFor="sandbox-tool-selection" className="block text-sm font-medium">{t("Selected Profile tool")}</label>
            <select id="sandbox-tool-selection" className="h-9 w-full rounded-lg border border-input bg-background px-2 text-sm"
              value={effectiveTool?.toolId??""} onChange={event=>setSelectedTool(event.target.value)}>
              {matchingTools.length===0?<option value="">{t("Select Workspace tools in Profile first")}</option>:null}
              {matchingTools.map(tool=><option key={tool.toolId} value={tool.toolId}>{tool.title}</option>)}
            </select>
            {!hasSandboxGrant&&effectiveTool?<p className="text-xs text-muted-foreground">{t("Grant this capability in the Sandbox Profile access tab first.")}</p>:null}
            {effectiveTool?<div className="flex flex-wrap items-center gap-2">
              <Button size="sm" disabled={busy||!available||alreadyTargeted} onClick={()=>void mutate(()=>client.addProfileToolTarget(selectedProfile.slug,{
                toolId:effectiveTool.toolId,workspaceId,sandboxId:sandbox.sandboxId,
              }))}>{t("Authorize exact tool target")}</Button>
              {alreadyTargeted?<Button size="sm" variant="outline" disabled={busy} onClick={()=>void mutate(()=>client.removeProfileToolTarget(selectedProfile.slug,effectiveTool.toolId,sandbox.sandboxId,workspaceId))}>
                {t("Remove tool target")}</Button>:null}
              <span className="text-xs text-muted-foreground">{t(alreadyTargeted?"Target authorized":"Target not authorized")}</span>
            </div>:null}
          </>:null}
        </div>:null}
      </>}
    </div>:null}
  </DataPanel>;
}
