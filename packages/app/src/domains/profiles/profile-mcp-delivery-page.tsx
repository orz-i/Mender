import { ChevronLeftIcon } from "lucide-react";
import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router";

import { MenderClientInvariantError, type ManagementClient, type Profile, type ProfileMCPDelivery, type Tunnel } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import { t } from "../../i18n/i18n";
import { ServersPage } from "../servers/servers-page";
import { errorMessage } from "../../shared/errors";
import { ErrorNotice } from "../../shared/page";

/** MCP owns projection settings; Profile continues to own membership. */
export function ProfileMCPDeliveryPage({
  client,
  copyText,
}: {
  client: ManagementClient;
  copyText?: (value: string) => Promise<void>;
}) {
  const { slug } = useParams();
  if (!slug) return null;
  return <MCPDeliveryEditor key={slug} slug={slug} client={client} {...(copyText===undefined?{}:{copyText})} />;
}

type Snapshot = {profile:Profile;config:ProfileMCPDelivery|null;tunnels:Tunnel[];client:ManagementClient;version:number;names:Map<string,string>};

function MCPDeliveryEditor({slug,client,copyText}:{slug:string;client:ManagementClient;copyText?: (value:string)=>Promise<void>}){
  const navigate=useNavigate();
  const [loadedSnapshot,setSnapshot]=useState<Snapshot|null>(null);
  const [mode,setMode]=useState<ProfileMCPDelivery["mode"]>("hybrid");
  const [tunnelId,setTunnelId]=useState("");
  const [selected,setSelected]=useState<Set<string>>(new Set());
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState("");
  const [version,setVersion]=useState(0);
  const snapshot=loadedSnapshot?.client===client&&loadedSnapshot.version===version?loadedSnapshot:null;

  useEffect(()=>{
    let active=true;
    void Promise.all([client.getProfile(slug),client.getProfileMCPDelivery(slug),client.listTunnels()])
      .then(async([profile,config,tunnels])=>{
        if(!active)return;
        if(config&&(config.profileId!==profile.profileId||config.profileRevision!==profile.revision)){
          throw new MenderClientInvariantError("Profile changed while loading MCP delivery. Reload before saving.");
        }
        setError("");setSnapshot({profile,config,tunnels:tunnels.tunnels,client,version,names:new Map()});
        setMode(config?.mode??"hybrid");setTunnelId(config?.tunnelId??"");setSelected(new Set(config?.directToolIds??[]));
        const titles=new Map<string,string>();
        for(let offset=0;offset<profile.tools.length&&active;offset+=8){
          const bindings=profile.tools.slice(offset,offset+8);
          const tools=await Promise.allSettled(bindings.map(binding=>client.getTool(binding.toolId)));
          tools.forEach((result,index)=>{if(result.status==="fulfilled")titles.set(bindings[index]!.toolId,result.value.title||result.value.name)});
        }
        if(active)setSnapshot(current=>current?.client===client&&current.version===version?{...current,names:titles}:current);
      }).catch((reason:unknown)=>{if(active)setError(errorMessage(reason))});
    return()=>{active=false};
  },[client,slug,version]);

  const directToolIds=mode==="hybrid"?[...selected].sort():[];
  const current=snapshot?.config;
  const currentIds=[...(current?.directToolIds??[])].sort();
  const dirty=snapshot!==null&&(!current||current.mode!==mode||(current.tunnelId??"")!==tunnelId||currentIds.length!==directToolIds.length||currentIds.some((id,index)=>id!==directToolIds[index]));

  async function save(){
    if(!snapshot||busy||!dirty)return;
    setBusy(true);setError("");
    try{
      const config=await client.configureProfileMCPDelivery(slug,{
        mode,tunnelId,directToolIds,expectedProfileRevision:snapshot.profile.revision,
        ...(snapshot.config===null?{}:{expectedRevision:snapshot.config.revision}),
      });
      if(config.profileId!==snapshot.profile.profileId||config.profileRevision!==snapshot.profile.revision){
        throw new MenderClientInvariantError("Profile changed while saving MCP delivery. Reload before saving again.");
      }
      setSnapshot({...snapshot,config});
      setMode(config.mode);setTunnelId(config.tunnelId??"");setSelected(new Set(config.directToolIds));
    }catch(reason){setError(errorMessage(reason))}finally{setBusy(false)}
  }
  return (
    <div className="flex flex-col gap-4">
      <div className="flex min-w-0 flex-wrap items-center gap-3">
        <Button variant="outline" size="sm" onClick={() =>
          void navigate("/profiles/" + encodeURIComponent(slug), { state: { initialSection: "deliveries" } })}>
          <ChevronLeftIcon data-icon="inline-start" />
          {t("Back to profile")}
        </Button>
        <div className="min-w-0">
          <h1 className="text-base font-semibold">{t("Profile MCP delivery")}</h1>
          <p className="text-xs text-muted-foreground">
            {t("Manage this Profile's running loopback MCP servers and their linked Tunnels.")}
          </p>
        </div>
      </div>
      {error?<ErrorNotice message={error} onRetry={()=>{setError("");setVersion(value=>value+1)}}/>:null}
      {!snapshot&&!error?<p className="text-sm text-muted-foreground">{t("Loading…")}</p>:null}
      {snapshot?<form aria-label={t("MCP delivery settings")} onSubmit={event=>{event.preventDefault();void save()}}
        className="flex flex-col gap-5 rounded-xl border border-border bg-card p-5">
        <div>
          <h2 className="text-sm font-semibold">{t("MCP delivery settings")}</h2>
          <p className="mt-1 text-xs text-muted-foreground">{t("Saving MCP settings does not start a server or Tunnel and does not change Profile membership.")}</p>
        </div>
        <label className="flex flex-col gap-2 text-sm">{t("Exposure mode")}
          <select aria-label={t("Exposure mode")} value={mode} disabled={busy}
            onChange={event=>setMode(event.target.value as ProfileMCPDelivery["mode"])}
            className="h-10 rounded-lg border border-input bg-background px-3">
            <option value="direct">{t("direct")}</option><option value="dynamic">{t("dynamic")}</option><option value="hybrid">{t("hybrid")}</option>
          </select>
          <span className="text-xs leading-5 text-muted-foreground">{t("Direct exports selected Tools. Dynamic exposes discovery meta-tools. Hybrid supports both and lets selected Tools opt into direct export.")}</span>
        </label>
        <label className="flex flex-col gap-2 text-sm">{t("Tunnel")}
          <select aria-label={t("MCP delivery tunnel")} value={tunnelId} disabled={busy} onChange={event=>setTunnelId(event.target.value)}
            className="h-10 rounded-lg border border-input bg-background px-3">
            <option value="">{t("Local only (no Tunnel)")}</option>
            {snapshot.tunnels.map(tunnel=><option key={tunnel.tunnelId} value={tunnel.tunnelId}>{tunnel.displayName}</option>)}
          </select>
        </label>
        {mode==="hybrid"?<fieldset className="flex flex-col gap-3">
          <legend className="mb-2 text-sm font-medium">{t("Direct export")}</legend>
          {snapshot.profile.tools.map((binding,index)=>{
            const name=binding.titleOverride||binding.alias||snapshot.names.get(binding.toolId)||t("Tool {{number}}",{number:index+1});
            return <label key={binding.toolId} className="flex items-center gap-2 text-sm">
              <input type="checkbox" disabled={busy} checked={selected.has(binding.toolId)} aria-label={t("Direct export")+" "+name}
                onChange={event=>{const next=new Set(selected);if(event.target.checked)next.add(binding.toolId);else next.delete(binding.toolId);setSelected(next)}}/>
              {name}
            </label>;
          })}
          {snapshot.profile.tools.length===0?<p className="text-xs text-muted-foreground">{t("No tools selected")}</p>:null}
        </fieldset>:null}
        <div className="flex flex-wrap items-center justify-end gap-2 border-t border-border pt-4">
          <Button type="button" variant="outline" disabled={busy} onClick={()=>{setError("");setVersion(value=>value+1)}}>{t("Reload settings")}</Button>
          <Button type="submit" disabled={busy||!dirty}>{t(busy?"Saving…":"Save MCP settings")}</Button>
        </div>
      </form>:null}
      {snapshot?.config?<ServersPage client={client} profileSlug={slug}
        {...(copyText === undefined ? {} : { copyText })} />:null}
    </div>
  );
}
