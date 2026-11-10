import { useEffect,useMemo,useState } from "react";
import type { ManagementClient,Profile,Sandbox,SandboxProfileGrant,ToolSummary,ProfileToolTarget } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import { DataPanel } from "../../shared/workbench";
import { ErrorNotice } from "../../shared/page";
import { errorMessage } from "../../shared/errors";
import { t } from "../../i18n/i18n";

const requiredJobCapabilities=new Map([
  ["jobs.submit","job.submit"],["jobs.status","job.observe"],
  ["jobs.logs","job.observe"],["jobs.result","job.observe"],["jobs.cancel","job.cancel"],
]);

// Explicitly binds canonical native:jobs tools to an SSH Sandbox. A Profile
// ToolBinding and an independent Sandbox grant are prerequisites, not effects.
export function SandboxJobsAccess({client,sandbox}:{client:ManagementClient;sandbox:Sandbox}){
 const [expanded,setExpanded]=useState(false);
 const [profiles,setProfiles]=useState<Profile[]>([]);
 const [tools,setTools]=useState<ToolSummary[]>([]);
 const [grants,setGrants]=useState<SandboxProfileGrant[]>([]);
 const [profileId,setProfileId]=useState("");
 const [toolId,setToolId]=useState("");
 const [targets,setTargets]=useState<ProfileToolTarget[]>([]);
 const [busy,setBusy]=useState(false);
 const [loaded,setLoaded]=useState(false);
 const [error,setError]=useState("");
 const [revision,setRevision]=useState(0);
 useEffect(()=>{
   if(!expanded)return;
   let active=true;
   void Promise.all([client.listProfiles(),client.browseTools({sourceId:"native:jobs",limit:100}),client.getSandboxProfileGrants(sandbox.sandboxId)])
     .then(([p,c,g])=>{
       if(!active)return;
       setProfiles(p.profiles);setTools(c.tools.filter(t=>t.sourceId==="native:jobs"));setGrants(g.grants);
       setProfileId(prev=>p.profiles.some(x=>x.profileId===prev)?prev:p.profiles[0]?.profileId??"");
       setLoaded(true);setError("");
     }).catch((e:unknown)=>{if(active){setError(errorMessage(e));setLoaded(true);}});
   return()=>{active=false;};
 },[expanded,client,sandbox.sandboxId,revision]);
 const profile=profiles.find(p=>p.profileId===profileId);
 const profileSlug=profile?.slug;
 useEffect(()=>{
  if(!expanded||!profileSlug)return;
  let active=true;
  void client.listProfileToolTargets(profileSlug).then(({targets})=>{
   if(active)setTargets(targets.filter(t=>t.sandboxId===sandbox.sandboxId&&t.workspaceId===""));
  }).catch((e:unknown)=>{if(active)setError(errorMessage(e));});
  return()=>{active=false;};
 },[expanded,profileSlug,client,sandbox.sandboxId,revision]);
 const selected=useMemo(()=>tools.filter(t=>profile?.tools.some(b=>b.toolId===t.toolId)),[tools,profile]);
 const active=selected.find(t=>t.toolId===toolId)??selected[0];
 const required=active?requiredJobCapabilities.get(`jobs.${active.name}`):undefined;
 const caps=grants.find(g=>g.profileId===profileId)?.capabilities??[];
 const granted=Boolean(required&&caps.includes(required as typeof caps[number]));
 const targeted=targets.some(t=>t.profileId===profileId&&t.toolId===active?.toolId);
 async function mutate(fn:()=>Promise<unknown>){if(busy)return;setBusy(true);setError("");try{await fn();setRevision(v=>v+1);}catch(e){setError(errorMessage(e));}finally{setBusy(false);}}
 return <DataPanel title="SSH Jobs execution" description={sandbox.name}>
  <div className="p-4"><Button size="sm" variant="outline" aria-expanded={expanded} onClick={()=>setExpanded(v=>!v)}>{t(expanded?"Hide SSH Jobs setup":"Configure SSH Jobs targets")}</Button></div>
  {expanded?<div className="space-y-3 border-t border-border p-4">
    <p className="text-xs text-muted-foreground">{t("SSH Jobs targets require a selected Profile tool and a separate Sandbox Job capability grant. Selecting a target does not submit a job.")}</p>
    {error?<ErrorNotice message={error}/>:null}
    {!loaded?<p className="text-sm text-muted-foreground">{t("Loading SSH Jobs access…")}</p>:<>
      <label className="block text-sm font-medium" htmlFor="jobs-target-profile">{t("Profile to authorize")}</label>
      <select id="jobs-target-profile" className="h-9 w-full rounded-lg border border-input bg-background px-2 text-sm" value={profileId}
        onChange={e=>{setProfileId(e.target.value);setTargets([]);}}>
        {profiles.length===0?<option value="">{t("Create a Profile first")}</option>:null}
        {profiles.map(p=><option key={p.profileId} value={p.profileId}>{p.displayName}</option>)}
      </select>
      {profile?<>
        <label className="block text-sm font-medium" htmlFor="jobs-target-tool">{t("Selected Profile tool")}</label>
        <select id="jobs-target-tool" className="h-9 w-full rounded-lg border border-input bg-background px-2 text-sm" value={active?.toolId??""}
          onChange={e=>setToolId(e.target.value)}>
          {selected.length===0?<option value="">{t("Select SSH Jobs tools in Profile first")}</option>:null}
          {selected.map(t=><option key={t.toolId} value={t.toolId}>{t.title}</option>)}
        </select>
        {active&&!granted?<p className="text-xs text-muted-foreground">{t("Grant this Job capability in the Sandbox Profile access tab first.")}</p>:null}
        {active?<div className="flex flex-wrap items-center gap-2">
          <Button size="sm" disabled={busy||!granted||targeted} onClick={()=>void mutate(()=>client.addProfileToolTarget(profile.slug,{
            toolId:active.toolId,sandboxId:sandbox.sandboxId,workspaceId:"",
          }))}>{t("Authorize exact Job target")}</Button>
          {targeted?<Button size="sm" variant="outline" disabled={busy} onClick={()=>void mutate(()=>client.removeProfileToolTarget(profile.slug,active.toolId,sandbox.sandboxId,""))}>{t("Remove Job target")}</Button>:null}
          <span className="text-xs text-muted-foreground">{t(targeted?"Target authorized":"Target not authorized")}</span>
        </div>:null}
      </>:null}
    </>}
  </div>:null}
 </DataPanel>;
}
