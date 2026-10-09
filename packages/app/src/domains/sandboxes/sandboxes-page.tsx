import { useEffect, useMemo, useState } from "react";
import { BoxIcon, HardDriveIcon, ShieldAlertIcon, SquareTerminalIcon } from "lucide-react";
import { useNavigate, useParams } from "react-router";

import type { MenderClient, Sandbox } from "@mender/sdk";
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
              <TableCell><Badge variant="outline">{item.state === "ready" ? t("Ready") : item.state}</Badge></TableCell>
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

export function CreateSandboxPage({ client }: { client: MenderClient }) {
  const navigate = useNavigate();
  const [step, setStep] = useState<"backend"|"configure"|"review">("backend");
  const [name, setName] = useState("");
  const [root, setRoot] = useState("");
  const [isolation, setIsolation] = useState<""|"host"|"restricted_host">("");
  const [restrictedAvailable, setRestrictedAvailable] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let active = true;
    void client.getRuntimeGovernance().then((info)=>{
      if (active) setRestrictedAvailable(info.backend.restrictedHostAvailable);
    }).catch((reason: unknown)=>{if(active)setError(errorMessage(reason));});
    return ()=>{active=false;};
  },[client]);

  async function create(){
    if (!name.trim() || !root.trim() || isolation === "") return;
    setBusy(true);setError("");
    try {
      const sandbox = await client.createLocalSandbox({name:name.trim(), root:root.trim(), isolation});
      void navigate(`/sandboxes/${sandbox.sandboxId}`);
    } catch(reason) {setError(errorMessage(reason));setBusy(false);}
  }

  return <>
    <PageHeader actions={<Button variant="outline" onClick={()=>{void navigate("/sandboxes");}}>{t("Back to sandboxes")}</Button>}/>
    {error ? <ErrorNotice message={error}/> : null}
    <DataPanel title="Create sandbox" description="Choose a backend, configure isolation, then review the Sandbox authority before provisioning.">
      <div className="space-y-6 p-5">
        {step === "backend" ? <div className="grid gap-3 sm:grid-cols-2">
          <button type="button" onClick={()=>setStep("configure")} className="rounded-xl border border-primary/50 bg-primary/5 p-5 text-left transition hover:bg-primary/10 focus-visible:outline-2 focus-visible:outline-primary">
            <BoxIcon className="mb-3 size-6" aria-hidden="true"/><p className="font-medium">{t("Local workspace")}</p>
            <p className="mt-1 text-xs leading-5 text-muted-foreground">{t("Host and restricted-host execution. Not a container or VM.")}</p>
          </button>
          {(["Docker","Daytona","E2B"] as const).map((item)=><div key={item} aria-disabled="true" className="rounded-xl border border-border bg-muted/20 p-5 opacity-65">
            <BoxIcon className="mb-3 size-6" aria-hidden="true"/><p className="font-medium">{item} · {t("Planned")}</p>
            <p className="mt-1 text-xs text-muted-foreground">{t("Backend not yet implemented")}</p>
          </div>)}
        </div> : null}
        {step === "configure" ? <div className="max-w-xl space-y-4">
          <label className="block space-y-1.5 text-sm"><span>{t("Sandbox name")}</span><Input aria-label={t("Sandbox name")} value={name} onChange={e=>setName(e.target.value)} maxLength={160}/></label>
          <label className="block space-y-1.5 text-sm"><span>{t("Absolute local directory")}</span><Input aria-label={t("Absolute local directory")} value={root} onChange={e=>setRoot(e.target.value)} placeholder={t("/Users/example/project")}/></label>
          <label className="block space-y-1.5 text-sm"><span>{t("Isolation")}</span>
            <select aria-label={t("Isolation")} value={isolation} onChange={e=>setIsolation(e.target.value as ""|"host"|"restricted_host")} className="h-9 w-full rounded-lg border border-input bg-background px-3">
              <option value="">{t("Choose isolation")}</option>
              <option value="restricted_host" disabled={!restrictedAvailable}>{t("Restricted host")}{!restrictedAvailable ? ` · ${t("Unavailable")}` : ""}</option>
              <option value="host">{t("Host (not isolated)")}</option>
            </select>
          </label>
          <p className="text-xs leading-5 text-muted-foreground">{t("Isolation choice is persisted per Sandbox; changing global Runtime settings will not weaken it.")}</p>
          <div className="flex gap-2"><Button variant="outline" onClick={()=>setStep("backend")}>{t("Back")}</Button><Button disabled={!name.trim()||!root.trim()||!isolation} onClick={()=>setStep("review")}>{t("Review")}</Button></div>
        </div> : null}
        {step === "review" ? <div className="max-w-xl space-y-4 text-sm">
          <p className="font-medium">{t("Review Sandbox authority")}</p>
          <Metadata name={t("Name")} value={name}/><Metadata name={t("Backend")} value={t("Local workspace")}/>
          <Metadata name={t("Workspace")} value={root}/><Metadata name={t("Isolation")} value={isolationLabel(isolation)}/>
          {isolation === "host" ? <div role="alert" className="rounded-lg border border-amber-500/40 bg-amber-500/5 p-3 text-sm">{t("Host mode is not isolated and uses the Runtime user's local file and process permissions.")}</div> : null}
          <div className="flex gap-2"><Button variant="outline" disabled={busy} onClick={()=>setStep("configure")}>{t("Back")}</Button><Button disabled={busy} onClick={()=>void create()}>{busy?t("Creating…"):t("Create sandbox")}</Button></div>
        </div> : null}
      </div>
    </DataPanel>
  </>;
}

export function SandboxDetailPage({client}:{client:MenderClient}) {
  const {sandboxId}=useParams();const navigate=useNavigate();
  const [item,setItem]=useState<Sandbox|null>(null);const [error,setError]=useState("");const [loaded,setLoaded]=useState(false);
  useEffect(()=>{
    if (!sandboxId) return;
    let active=true;
    void client.getSandbox(sandboxId).then(value=>{if(active){setItem(value);setLoaded(true);}}).catch((reason:unknown)=>{if(active){setError(errorMessage(reason));setLoaded(true);}});
    return ()=>{active=false;};
  },[client,sandboxId]);
  return <>
    <PageHeader actions={<Button variant="outline" onClick={()=>{void navigate("/sandboxes");}}>{t("Back to sandboxes")}</Button>}/>
    {error || !sandboxId ? <ErrorNotice message={error || t("The requested item was not found.")}/> : null}
    {sandboxId && !loaded ? <p className="text-sm text-muted-foreground">{t("Loading Sandboxes…")}</p> : item ? <div className="space-y-4">
      <DataPanel title="Sandbox overview" description={item.name}>
        <div className="grid gap-4 p-5 sm:grid-cols-2">
          <Metadata name={t("Sandbox ID")} value={item.sandboxId}/><Metadata name={t("Backend")} value={sandboxBackend(item)}/>
          <Metadata name={t("Isolation")} value={isolationLabel(item.isolation)}/><Metadata name={t("State")} value={item.state}/>
          <Metadata name={t("Workspace")} value={item.localRoot||"—"}/><Metadata name={t("Owner type")} value={item.ownerKind}/>
        </div>
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
