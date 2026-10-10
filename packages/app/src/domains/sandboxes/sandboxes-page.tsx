import { useEffect, useMemo, useState } from "react";
import { BoxIcon, HardDriveIcon, ShieldAlertIcon, SquareTerminalIcon } from "lucide-react";
import { useNavigate } from "react-router";

import type { MenderClient, Sandbox } from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@mender/ui/components/table";

import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { EmptyState, ErrorNotice, PageHeader } from "../../shared/page";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import { DataPanel, EmptyPanelState, MetricStrip, MetricTile, SearchField } from "../../shared/workbench";
import { isolationLabel, sandboxBackend } from "./sandbox-display";

export { CreateSandboxPage } from "./sandbox-create-page";
export { SandboxDetailPage } from "./sandbox-detail-page";

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
