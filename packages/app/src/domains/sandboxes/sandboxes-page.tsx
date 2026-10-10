import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router";

import type { MenderClient, Sandbox } from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@mender/ui/components/table";

import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { EmptyState, ErrorNotice } from "../../shared/page";
import { PageTab, PageTabs, PageToolbar, ResultPanel } from "../../shared/page-toolbar";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import { EmptyPanelState, SearchField } from "../../shared/workbench";
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
  const [category, setCategory] = useState<"all" | "ready" | "local" | "unavailable">("all");

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
    (category === "all" || (category === "ready" && item.state === "ready") ||
      (category === "local" && item.backend === "local_workspace") ||
      (category === "unavailable" && item.state !== "ready")) &&
    `${item.name} ${item.sandboxId} ${item.localRoot}`.toLowerCase().includes(query.toLowerCase()),
  ),[items,backend,category,query]);
  const pagination = usePagination(filtered);

  return <>
    <PageToolbar
      tabs={
        <PageTabs label="Sandboxes">
          <PageTab label="Sandboxes" count={items.length} active={category === "all"} onClick={() => setCategory("all")} />
          <PageTab label="Ready sandboxes" count={items.filter((item) => item.state === "ready").length} active={category === "ready"} onClick={() => setCategory("ready")} />
          <PageTab label="Local workspaces" count={items.filter((item) => item.backend === "local_workspace").length} active={category === "local"} onClick={() => setCategory("local")} />
          <PageTab label="Unavailable sandboxes" count={items.filter((item) => item.state !== "ready").length} active={category === "unavailable"} onClick={() => setCategory("unavailable")} />
        </PageTabs>
      }
      filters={
        <>
          <div className="w-full min-w-0 sm:w-44 sm:shrink-0">
            <SearchField label="Search sandboxes" placeholder="Search sandboxes" value={query} onChange={setQuery} />
          </div>
          <select
            aria-label={t("Backend filter")}
            value={backend}
            onChange={(event) => setBackend(event.target.value)}
            className="h-9 w-36 min-w-0 rounded-xl border border-input bg-card px-3 text-sm text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <option value="all">{t("All backends")}</option>
            <option value="local_workspace">{t("Local workspace")}</option>
            <option value="docker">{t("Docker")}</option>
            <option value="daytona">{t("Daytona")}</option>
            <option value="e2b">{t("E2B")}</option>
          </select>
        </>
      }
      actions={<Button size="sm" onClick={() => void navigate("/sandboxes/new")}>{t("Create sandbox")}</Button>}
    />
    {error ? <ErrorNotice message={error} /> : null}
    <ResultPanel title="Sandbox inventory">
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
        </> : <EmptyPanelState><EmptyState title={items.length ? t("No matching sandboxes") : t("No sandboxes")} description={items.length ? t("Try a different search or filter.") : t("Create a local Sandbox before binding Workspace tools to it.")} /></EmptyPanelState>}
    </ResultPanel>
  </>;
}
