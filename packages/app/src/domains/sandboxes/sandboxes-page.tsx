import { useEffect, useMemo, useState } from "react";
import { BoxIcon, HardDriveIcon, ShieldAlertIcon, SquareTerminalIcon } from "lucide-react";
import { Link, useNavigate, useParams } from "react-router";

import type { Credential, MenderClient, Sandbox } from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import { Input } from "@mender/ui/components/input";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@mender/ui/components/table";

import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { EmptyState, ErrorNotice, PageHeader } from "../../shared/page";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import { DataPanel, EmptyPanelState, MetricStrip, MetricTile, SearchField } from "../../shared/workbench";

export function SandboxesPage({ client }: { client: MenderClient }) {
  const navigate = useNavigate();
  const [items, setItems] = useState<Sandbox[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const [backend, setBackend] = useState("all");

  useEffect(() => {
    let active = true;
    void client.listSandboxes().then(({sandboxes}) => {
      if (!active) return;
      setItems(sandboxes);setError("");setLoaded(true);
    }).catch((reason: unknown) => {
      if (active) {setError(errorMessage(reason));setLoaded(true);}
    });
    return () => {active=false;};
  }, [client]);

  const filtered = useMemo(() => items.filter((item) =>
    (backend === "all" || item.backend === backend) &&
    `${item.name} ${item.sandboxId} ${item.localRoot}`.toLowerCase().includes(query.toLowerCase()),
  ),[items,backend,query]);
  const pagination = usePagination(filtered);

  return <>
    <PageHeader actions={<Button onClick={() => {void navigate("/sandboxes/new");}}>{t("Create sandbox")}</Button>} />
    {error ? <ErrorNotice message={error} /> : null}
    <MetricStrip>
      <MetricTile icon={BoxIcon} label="Sandboxes" value={items.length} />
      <MetricTile icon={SquareTerminalIcon} label="Ready sandboxes" value={items.filter((s)=>s.state==="ready").length} />
      <MetricTile icon={HardDriveIcon} label="Local workspaces" value={items.filter((s)=>s.backend==="local_workspace").length} />
      <MetricTile icon={ShieldAlertIcon} label="Unavailable sandboxes" value={items.filter((s)=>s.state!=="ready").length} />
    </MetricStrip>
    <div className="mt-4">
      <DataPanel title="Sandbox inventory" description="Manage where Sandbox Workspace Tools run. Execution is subject to Principal and Policy admission." toolbar={
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
          <SearchField label="Search sandboxes" placeholder="Search sandboxes" value={query} onChange={setQuery}/>
          <select aria-label={t("Backend filter")} value={backend} onChange={(e)=>setBackend(e.target.value)} className="h-9 rounded-lg border border-input bg-background px-3 text-sm">
            <option value="all">{t("All backends")}</option>
            <option value="local_workspace">{t("Local workspace")}</option>
            <option value="docker">{t("Docker")}</option>
            <option value="daytona">{t("Daytona")}</option>
            <option value="e2b">{t("E2B")}</option>
          </select>
        </div>
      }>
        {!loaded ? <div className="p-8 text-sm text-muted-foreground">{t("Loading Sandboxes…")}</div> : filtered.length ? <>
          <Table><TableHeader><TableRow>
            <TableHead>{t("Name")}</TableHead><TableHead>{t("Backend")}</TableHead><TableHead>{t("Isolation")}</TableHead><TableHead>{t("State")}</TableHead><TableHead>{t("Workspace")}</TableHead><TableHead className="text-right">{t("Actions")}</TableHead>
          </TableRow></TableHeader><TableBody>{pagination.items.map((item) =>
            <TableRow key={item.sandboxId}>
              <TableCell className="font-medium">{item.name}</TableCell>
              <TableCell>{sandboxBackend(item)}</TableCell>
              <TableCell>{isolationLabel(item.isolation)}</TableCell>
              <TableCell><Badge variant="outline">{item.backendAvailable ? (item.state === "ready" ? t("Ready") : item.state) : t("Backend unavailable")}</Badge></TableCell>
              <TableCell className="max-w-[240px] truncate font-mono text-xs text-muted-foreground" title={item.localRoot}>{item.localRoot || "—"}</TableCell>
              <TableCell className="text-right"><Button size="sm" variant="outline" onClick={()=>{void navigate(`/sandboxes/${item.sandboxId}`);}}>{t("View")}</Button></TableCell>
            </TableRow>)}
          </TableBody></Table>
          <div className="px-4 pb-3"><TablePagination label="Sandboxes" pagination={pagination}/></div>
        </> : <EmptyPanelState><EmptyState title="No sandboxes" description="Create a local Sandbox before binding Workspace tools to it." /></EmptyPanelState>}
      </DataPanel>
    </div>
  </>;
}

export function CreateSandboxPage({client}:{client:MenderClient}) {
 const navigate=useNavigate();
 const [step,setStep]=useState<"backend"|"configure"|"review">("backend");
 const [selected,setSelected]=useState<"local_workspace"|"docker"|"daytona">("local_workspace");
 const [name,setName]=useState("");
 const [root,setRoot]=useState("");
 const [isolation,setIsolation]=useState<""|"host"|"restricted_host">("");
 const [restrictedAvailable,setRestrictedAvailable]=useState(false);
 const [dockerAvailable,setDockerAvailable]=useState(false);
 const [dockerStatus,setDockerStatus]=useState("");
 const [daytonaConfigured,setDaytonaConfigured]=useState(false);
 const [daytonaStatus,setDaytonaStatus]=useState("");
 const [credentials,setCredentials]=useState<Credential[]>([]);
 const [daytonaCredential,setDaytonaCredential]=useState("");
 const [daytonaCPU,setDaytonaCPU]=useState(1);
 const [daytonaMemory,setDaytonaMemory]=useState(2);
 const [daytonaDisk,setDaytonaDisk]=useState(3);
 const [daytonaTTL,setDaytonaTTL]=useState(30);
 const [imageDigest,setImageDigest]=useState("");
 const [userId,setUserId]=useState(10001);
 const [groupId,setGroupId]=useState(10001);
 const [cpuMilli,setCpuMilli]=useState(1000);
 const [memoryMiB,setMemoryMiB]=useState(512);
 const [workspaceMiB,setWorkspaceMiB]=useState(64);
 const [pids,setPids]=useState(64);
 const [error,setError]=useState("");
 const [busy,setBusy]=useState(false);
 useEffect(()=>{
  let active=true;
  void client.getRuntimeGovernance().then(info=>{if(active)setRestrictedAvailable(info.backend.restrictedHostAvailable);}).catch((reason:unknown)=>{if(active)setError(errorMessage(reason));});
  void client.getSandboxBackends().then(info=>{if(active){setDockerAvailable(info.dockerAvailable);setDockerStatus(info.dockerStatus);setDaytonaConfigured(info.daytonaConfigured);setDaytonaStatus(info.daytonaStatus);}}).catch((reason:unknown)=>{if(active)setDockerStatus(errorMessage(reason));});
  void client.listCredentials().then(result=>{if(active)setCredentials(result.credentials);}).catch((reason:unknown)=>{if(active)setError(errorMessage(reason));});
  return ()=>{active=false;};
 },[client]);
 const digestOK=/^[a-z0-9]+(?:[.-][a-z0-9]+)+(?:\/[a-z0-9]+(?:[._-][a-z0-9]+)*){2,}@sha256:[0-9a-f]{64}$/.test(imageDigest);
 const dockerConfigValid=dockerAvailable && digestOK && userId>=1000 && userId<=65534 && groupId>=1000 && groupId<=65534 && cpuMilli>=100 && cpuMilli<=8000 && memoryMiB>=256 && memoryMiB<=16384 && workspaceMiB>=64 && workspaceMiB<=2048 && workspaceMiB<=memoryMiB && pids>=16 && pids<=512;
 const daytonaValid=daytonaConfigured && digestOK && credentials.some(c=>c.reference===daytonaCredential) && daytonaCPU>=1&&daytonaCPU<=2&&daytonaMemory>=1&&daytonaMemory<=4&&daytonaDisk>=3&&daytonaDisk<=10&&daytonaTTL>=15&&daytonaTTL<=120;
 const formValid=Boolean(name.trim()) && (selected==="docker"?dockerConfigValid:selected==="daytona"?daytonaValid:Boolean(root.trim()&&isolation));
 async function create(){
  if(!formValid||busy)return;
  setBusy(true);setError("");
  try{
   const sandbox=selected==="docker"?await client.createDockerSandbox({name:name.trim(),imageDigest,userId,groupId,cpuMilli,memoryMiB,workspaceMiB,pids}):selected==="daytona"?await client.createDaytonaSandbox({name:name.trim(),credentialRef:daytonaCredential,imageDigest,target:"us",cpu:daytonaCPU,memoryGiB:daytonaMemory,diskGiB:daytonaDisk,ttlMinutes:daytonaTTL}):await client.createLocalSandbox({name:name.trim(),root:root.trim(),isolation:isolation as "host"|"restricted_host"});
   void navigate(`/sandboxes/${sandbox.sandboxId}`);
  }catch(reason){setError(errorMessage(reason));setBusy(false);}
 }
 function numberField(label:string,value:number,onChange:(next:number)=>void,min:number,max:number){return <label className="block space-y-1.5 text-sm"><span>{label}</span><Input type="number" min={min} max={max} value={value} onChange={e=>onChange(Number(e.target.value))}/></label>;}
 return <>
  <PageHeader actions={<Button variant="outline" onClick={()=>{void navigate("/sandboxes");}}>{t("Back to sandboxes")}</Button>}/>
  {error?<ErrorNotice message={error}/>:null}
  <DataPanel title="Create sandbox" description="Choose a backend, configure isolation, then review the Sandbox authority before provisioning.">
   <div className="space-y-6 p-5">
    {step==="backend"?<div className="grid gap-3 sm:grid-cols-2">
     <button type="button" onClick={()=>{setSelected("local_workspace");setStep("configure");}} className="rounded-xl border border-primary/50 bg-primary/5 p-5 text-left transition hover:bg-primary/10 focus-visible:outline-2 focus-visible:outline-primary">
      <BoxIcon className="mb-3 size-6" aria-hidden="true"/><p className="font-medium">{t("Local workspace")}</p>
      <p className="mt-1 text-xs leading-5 text-muted-foreground">{t("Host and restricted-host execution. Not a container or VM.")}</p>
     </button>
     {dockerAvailable?<button type="button" onClick={()=>{setSelected("docker");setStep("configure");}} className="rounded-xl border border-primary/50 bg-primary/5 p-5 text-left transition hover:bg-primary/10 focus-visible:outline-2 focus-visible:outline-primary">
      <BoxIcon className="mb-3 size-6" aria-hidden="true"/><p className="font-medium">{t("Docker")}</p>
      <p className="mt-1 text-xs leading-5 text-muted-foreground">{t("Isolated, locally cached immutable image; no network, no host mounts.")}</p>
     </button>:<div aria-disabled="true" className="rounded-xl border border-border bg-muted/20 p-5 opacity-65">
      <BoxIcon className="mb-3 size-6" aria-hidden="true"/><p className="font-medium">{t("Docker")} · {t("Unavailable")}</p>
      <p className="mt-1 break-words text-xs text-muted-foreground">{dockerStatus||t("Configure MENDER_DOCKER_SOCKET on the Runtime host.")}</p>
     </div>}
     {daytonaConfigured?<button type="button" onClick={()=>{setSelected("daytona");setStep("configure");}} className="rounded-xl border border-primary/50 bg-primary/5 p-5 text-left transition hover:bg-primary/10 focus-visible:outline-2 focus-visible:outline-primary">
      <BoxIcon className="mb-3 size-6" aria-hidden="true"/><p className="font-medium">{t("Daytona")} · {t("Experimental")}</p>
      <p className="mt-1 text-xs leading-5 text-muted-foreground">{t("Remote Daytona Sandbox uses a Mender credential and incurs provider charges. Provider availability is verified on creation.")}</p>
     </button>:<div aria-disabled="true" className="rounded-xl border border-border bg-muted/20 p-5 opacity-65"><BoxIcon className="mb-3 size-6" aria-hidden="true"/><p className="font-medium">{t("Daytona")} · {t("Unavailable")}</p><p className="text-xs text-muted-foreground">{experimentalDaytonaStatus(daytonaStatus)}</p></div>}
     <div aria-disabled="true" className="rounded-xl border border-border bg-muted/20 p-5 opacity-65"><BoxIcon className="mb-3 size-6" aria-hidden="true"/><p className="font-medium">{t("E2B")} · {t("Planned")}</p><p className="mt-1 text-xs text-muted-foreground">{t("Backend not yet implemented")}</p></div>
    </div>:null}
    {step==="configure"?<div className="max-w-xl space-y-4">
     <label className="block space-y-1.5 text-sm"><span>{t("Sandbox name")}</span><Input aria-label={t("Sandbox name")} value={name} maxLength={160} onChange={e=>setName(e.target.value)}/></label>
     {selected==="local_workspace"?<>
      <label className="block space-y-1.5 text-sm"><span>{t("Absolute local directory")}</span><Input aria-label={t("Absolute local directory")} value={root} onChange={e=>setRoot(e.target.value)} placeholder={t("/Users/example/project")}/></label>
      <label className="block space-y-1.5 text-sm"><span>{t("Isolation")}</span><select aria-label={t("Isolation")} className="h-9 w-full rounded-lg border border-input bg-background px-3" value={isolation} onChange={e=>setIsolation(e.target.value as ""|"host"|"restricted_host")}>
       <option value="">{t("Choose isolation")}</option><option value="restricted_host" disabled={!restrictedAvailable}>{t("Restricted host")}{!restrictedAvailable?` · ${t("Unavailable")}`:""}</option><option value="host">{t("Host (not isolated)")}</option>
      </select></label>
      <p className="text-xs leading-5 text-muted-foreground">{t("Isolation choice is persisted per Sandbox; changing global Runtime settings will not weaken it.")}</p>
     </>:selected==="docker"?<>
      <label className="block space-y-1.5 text-sm"><span>{t("Immutable image digest")}</span><Input aria-label={t("Immutable image digest")} value={imageDigest} onChange={e=>setImageDigest(e.target.value.trim())} placeholder={t("docker.io/library/alpine@sha256:…")} autoComplete="off"/></label>
      <p className="text-xs leading-5 text-muted-foreground">{t("The exact image digest must already be cached locally. Mender never pulls images automatically.")}</p>
      <div className="grid gap-3 sm:grid-cols-2">
       {numberField(t("User ID (non-root)"),userId,setUserId,1000,65534)}
       {numberField(t("Group ID (non-root)"),groupId,setGroupId,1000,65534)}
       {numberField(t("CPU (millicores)"),cpuMilli,setCpuMilli,100,8000)}
       {numberField(t("Memory (MiB)"),memoryMiB,setMemoryMiB,256,16384)}
       {numberField(t("Workspace tmpfs (MiB)"),workspaceMiB,setWorkspaceMiB,64,2048)}
       {numberField(t("PID limit"),pids,setPids,16,512)}
      </div>
      <p className="text-xs leading-5 text-muted-foreground">{t("Docker execution uses read-only rootfs, non-root user, no network or host mounts. Workspace files are ephemeral across restarts.")}</p>
     </>:<>
      <label className="block space-y-1.5 text-sm"><span>{t("Mender Credential Reference")}</span>
       <select aria-label={t("Mender Credential Reference")} className="h-9 w-full rounded-lg border border-input bg-background px-3" value={daytonaCredential} onChange={e=>setDaytonaCredential(e.target.value)}><option value="">{t("Choose a credential")}</option>{credentials.map(c=><option key={c.reference} value={c.reference}>{c.displayName} · {c.reference}</option>)}</select>
      </label>
      {credentials.length===0?<p className="text-xs text-muted-foreground">{t("No credentials available. Add a Daytona API credential first.")} <Link to="/credentials" className="text-primary-ink underline-offset-4 hover:underline">{t("Credentials")}</Link></p>:null}
      <label className="block space-y-1.5 text-sm"><span>{t("Immutable image digest")}</span><Input aria-label={t("Immutable image digest")} value={imageDigest} onChange={e=>setImageDigest(e.target.value.trim())} placeholder={t("docker.io/library/python@sha256:…")} autoComplete="off"/></label>
      <div className="grid gap-3 sm:grid-cols-2">
       {numberField(t("Remote CPUs"),daytonaCPU,setDaytonaCPU,1,2)}
       {numberField(t("Remote memory (GiB)"),daytonaMemory,setDaytonaMemory,1,4)}
       {numberField(t("Remote disk (GiB)"),daytonaDisk,setDaytonaDisk,3,10)}
       {numberField(t("TTL (minutes)"),daytonaTTL,setDaytonaTTL,15,120)}
      </div>
      <p className="text-xs leading-5 text-muted-foreground">{t("Daytona network egress is blocked. Remote sandbox TTL limits wall-clock lifetime, NOT monetary charges. Creating a sandbox may incur provider fees.")}</p>
     </>}
     <div className="flex gap-2"><Button variant="outline" onClick={()=>setStep("backend")}>{t("Back")}</Button><Button disabled={!formValid} onClick={()=>setStep("review")}>{t("Review")}</Button></div>
    </div>:null}
    {step==="review"?<div className="max-w-xl space-y-4 text-sm">
     <p className="font-medium">{t("Review Sandbox authority")}</p>
     <Metadata name={t("Name")} value={name}/><Metadata name={t("Backend")} value={selected==="docker"?"Docker":selected==="daytona"?t("Daytona"):t("Local workspace")}/>
     {selected==="local_workspace"?<><Metadata name={t("Workspace")} value={root}/><Metadata name={t("Isolation")} value={isolationLabel(isolation)}/></>:selected==="docker"?<>
      <Metadata name={t("Immutable image digest")} value={imageDigest}/><Metadata name={t("Isolation")} value={t("Container")}/>
      <p className="rounded-lg border border-amber-500/40 p-3 text-xs">{t("Docker Workspace files are ephemeral; stopping and starting recreates the container and erases scratch files.")}</p>
     </>:null}
     {selected==="daytona"?<><Metadata name={t("Immutable image digest")} value={imageDigest}/><Metadata name={t("Mender Credential Reference")} value={daytonaCredential}/><Metadata name={t("TTL (minutes)")} value={String(daytonaTTL)}/><p role="alert" className="rounded-lg border border-amber-500/40 p-3 text-xs">{t("Creating a Daytona Sandbox can incur provider charges. TTL is not a USD spending cap. Maximum 2 active remote sandboxes per Principal.")}</p></>:null}
     {selected==="local_workspace"&&isolation==="host"?<div role="alert" className="rounded-lg border border-amber-500/40 bg-amber-500/5 p-3 text-sm">{t("Host mode is not isolated and uses the Runtime user's local file and process permissions.")}</div>:null}
     <div className="flex gap-2"><Button variant="outline" disabled={busy} onClick={()=>setStep("configure")}>{t("Back")}</Button><Button disabled={busy||!formValid} onClick={()=>void create()}>{busy?t("Creating…"):t("Create sandbox")}</Button></div>
    </div>:null}
   </div>
  </DataPanel>
 </>;
}

export function SandboxDetailPage({client}:{client:MenderClient}) {
  const {sandboxId}=useParams();
  const navigate=useNavigate();
  const [item,setItem]=useState<Sandbox|null>(null);
  const [error,setError]=useState("");
  const [loaded,setLoaded]=useState(false);
  const [busy,setBusy]=useState(false);
  useEffect(()=>{
    if (!sandboxId) return;
    let active=true;
    void client.getSandbox(sandboxId).then(value=>{if(active){setItem(value);setLoaded(true);}}).catch((reason:unknown)=>{if(active){setError(errorMessage(reason));setLoaded(true);}});
    return ()=>{active=false;};
  },[client,sandboxId]);
  async function changeLifecycle(action:"start"|"stop"|"destroy"){
    if(!item||busy)return;
    if(action==="destroy" && !window.confirm(t('Destroy Sandbox "{{name}}"? This only tombstones the Sandbox; host files are never deleted.',{name:item.name})))return;
    if(action==="start" && item.backend==="docker" && !window.confirm(t("Docker Sandbox restarts with an empty workspace. Continue?")))return;
    setBusy(true);setError("");
    try{
      const next=action==="start"?await client.startSandbox(item.sandboxId):action==="stop"?await client.stopSandbox(item.sandboxId):await client.destroySandbox(item.sandboxId);
      setItem(next);
    }catch(reason){setError(errorMessage(reason));}
    finally{setBusy(false);}
  }
  const manageable=Boolean(item?.backendAvailable && (item.backend==="local_workspace" || item.backend==="docker" || item.backend==="daytona"));
  const noActiveLeases=item?.activeLeases===0;
  const canStart=manageable && item?.state==="stopped";
  const canStop=manageable && item?.state==="ready";
  const canDestroy=manageable && (item?.state==="ready"||item?.state==="stopped");
  async function reconcile(){
    if(!item||busy||(item.backend!=="docker"&&item.backend!=="daytona"))return;
    setBusy(true);setError("");
    try{setItem(item.backend==="daytona"?await client.reconcileDaytonaSandbox(item.sandboxId):await client.reconcileDockerSandbox(item.sandboxId));}
    catch(reason){setError(errorMessage(reason));}
    finally{setBusy(false);}
  }
  return <>
    <PageHeader actions={<div className="flex flex-wrap justify-end gap-2">
      {canStart?<Button disabled={busy||!noActiveLeases} onClick={()=>void changeLifecycle("start")}>{t("Start sandbox")}</Button>:null}
      {canStop?<Button variant="outline" disabled={busy||!noActiveLeases} onClick={()=>void changeLifecycle("stop")}>{t("Stop sandbox")}</Button>:null}
      {canDestroy?<Button variant="destructive" disabled={busy||!noActiveLeases} onClick={()=>void changeLifecycle("destroy")}>{t("Destroy sandbox")}</Button>:null}
      {(item?.backend==="docker"||item?.backend==="daytona") && item.backendAvailable && item.state==="unknown"?<Button disabled={busy||!noActiveLeases} onClick={()=>void reconcile()}>{t(item.backend==="daytona"?"Reconcile Daytona sandbox":"Reconcile Docker sandbox")}</Button>:null}
      {item?<Button variant="outline" onClick={()=>{void navigate(`/sandboxes/${item.sandboxId}/activity`);}}>{t("Activity")}</Button>:null}
      <Button variant="outline" onClick={()=>{void navigate("/sandboxes");}}>{t("Back to sandboxes")}</Button>
    </div>}/>
    {error || !sandboxId ? <ErrorNotice message={error || t("The requested item was not found.")}/> : null}
    {sandboxId && !loaded ? <p className="text-sm text-muted-foreground">{t("Loading Sandboxes…")}</p> : item ? <div className="space-y-4">
      <DataPanel title="Sandbox overview" description={item.name}>
        <div className="grid gap-4 p-5 sm:grid-cols-2">
          <Metadata name={t("Sandbox ID")} value={item.sandboxId}/><Metadata name={t("Backend")} value={sandboxBackend(item)}/>
          <Metadata name={t("Isolation")} value={isolationLabel(item.isolation)}/><Metadata name={t("State")} value={sandboxStateLabel(item.state)}/>
          <Metadata name={t("Workspace")} value={item.localRoot||"—"}/><Metadata name={t("Owner type")} value={item.ownerKind}/>
          <Metadata name={t("Active execution leases")} value={String(item.activeLeases)}/>
          <Metadata name={t("Backend status")} value={item.backendAvailable?t("Available"):t("Unavailable")}/>
        </div>
        {item.activeLeases>0?<p role="status" className="border-t border-border px-5 py-3 text-xs text-amber-700 dark:text-amber-300">{t("Cannot change lifecycle while a Sandbox execution lease is active.")}</p>:null}
        {item.state==="destroyed"?<p role="status" className="border-t border-border px-5 py-3 text-xs text-muted-foreground">{t("Host files remain untouched after Sandbox destruction.")}</p>:null}
        {item.backend==="daytona"?<p role="status" className="border-t border-border px-5 py-3 text-xs text-muted-foreground">{t("Remote Sandbox incurs Daytona account charges. TTL is a wall-clock ceiling, not a billing cap.")}</p>:null}
        {item.backend==="docker"?<p role="status" className="border-t border-border px-5 py-3 text-xs text-muted-foreground">{t("Docker Workspace scratch is ephemeral. Stop/Start recreates the container; destruction never removes host files.")}</p>:null}
      </DataPanel>
      <DataPanel title="Security boundary" description="Isolation depends on the actual backend, not the Sandbox name.">
        <p className="p-5 text-sm leading-6 text-muted-foreground">{t(item.isolation === "host" ? "Host mode is not isolated and uses the Runtime user's local file and process permissions." : item.isolation === "restricted_host" ? "Restricted host confines local processes but is not a container or VM." : "Isolation guarantees depend on the selected backend and must be verified independently.")}</p>
      </DataPanel>
      <p className="text-xs text-muted-foreground">{t("State is last observed; unsupported lifecycle operations are not available.")}</p>
    </div> : null}
  </>;
}

function sandboxBackend(item:Sandbox){return item.backend==="local_workspace"?t("Local workspace"):item.backend;}
function isolationLabel(value:string){if(value==="host")return t("Host (not isolated)");if(value==="restricted_host")return t("Restricted host");return value;}
function Metadata({name,value}:{name:string;value:string}){return <div className="min-w-0"><div className="text-xs font-medium text-muted-foreground">{name}</div><div className="mt-1 break-all font-mono text-sm">{value}</div></div>;}

function sandboxStateLabel(state:Sandbox["state"]){
 switch(state){case "ready":return t("Ready");case "stopped":return t("Stopped");case "destroyed":return t("Destroyed");default:return state;}
}


// Backend status comes from the server. Translate only the exact reviewed
// host-owned states, never interpolate arbitrary remote error strings.
function experimentalDaytonaStatus(value: string): string {
 switch (value) {
 case "Experimental Daytona disabled; local operator must set MENDER_DAYTONA_EXPERIMENTAL=enabled before Runtime startup":
   return t("Experimental Daytona disabled; local operator must set MENDER_DAYTONA_EXPERIMENTAL=enabled before Runtime startup");
 case "Experimental Daytona enabled by operator; credential and live provider validation required; may incur charges":
   return t("Experimental Daytona enabled by operator; credential and live provider validation required; may incur charges");
 default: return value;
 }
}
