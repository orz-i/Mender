import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router";

import type { MenderClient, Sandbox } from "@mender/sdk";
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
import { StatusBadge } from "../../shared/status-badge";
import { EmptyPanelState, SearchField } from "../../shared/workbench";
import { isolationLabel, sandboxBackend, sandboxStateLabel } from "./sandbox-display";

export { CreateSandboxPage } from "./sandbox-create-page";
export { SandboxDetailPage } from "./sandbox-detail-page";

export function SandboxesPage({ client }: { client: MenderClient }) {
  const navigate = useNavigate();
  const [items, setItems] = useState<Sandbox[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [hasSnapshot, setHasSnapshot] = useState(false);
  const [reloadVersion, setReloadVersion] = useState(0);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const [backend, setBackend] = useState("all");
  const [category, setCategory] = useState<"all" | "ready" | "local" | "unavailable">("all");

  useEffect(() => {
    let active = true;
    void client.listSandboxes().then(({sandboxes}) => {
      if (!active) return;
      setItems(sandboxes);setHasSnapshot(true);setError("");setLoaded(true);
    }).catch((reason: unknown) => {
      if (active) {setError(errorMessage(reason));setLoaded(true);}
    });
    return () => {active=false;};
  }, [client, reloadVersion]);

  const filtered = useMemo(() => items.filter((item) =>
    (backend === "all" || item.backend === backend) &&
    (category === "all" || (category === "ready" && item.state === "ready") ||
      (category === "local" && item.backend === "local_workspace") ||
      (category === "unavailable" && item.state !== "ready")) &&
    `${item.name} ${item.sandboxId} ${item.localRoot}`.toLowerCase().includes(query.toLowerCase()),
  ),[items,backend,category,query]);
  const pagination = usePagination(filtered);

  if (error && !hasSnapshot) {
    return <div className="pt-4"><ErrorNotice message={error} onRetry={() => {setError("");setReloadVersion(version=>version+1);}} /></div>;
  }

  return <>
    <PageToolbar
      tabs={hasSnapshot ?
        <PageTabs label="Sandboxes">
          <PageTab label="Sandboxes" count={items.length} active={category === "all"} onClick={() => setCategory("all")} />
          <PageTab label="Ready sandboxes" count={items.filter((item) => item.state === "ready").length} active={category === "ready"} onClick={() => setCategory("ready")} />
          <PageTab label="Local workspaces" count={items.filter((item) => item.backend === "local_workspace").length} active={category === "local"} onClick={() => setCategory("local")} />
          <PageTab label="Unavailable sandboxes" count={items.filter((item) => item.state !== "ready").length} active={category === "unavailable"} onClick={() => setCategory("unavailable")} />
        </PageTabs> : <span className="text-xs text-muted-foreground">{t("Loading Sandboxes…")}</span>
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
            <option value="ssh_remote">{t("SSH Remote")}</option>
            <option value="e2b">{t("E2B")}</option>
            <option value="external">{t("External plugin")}</option>
          </select>
        </>
      }
      actions={<Button size="sm" onClick={() => void navigate("/sandboxes/new")}>{t("Create sandbox")}</Button>}
    />
    {error ? <ErrorNotice message={error} /> : null}
    <ResultPanel title="Sandbox inventory">
        {!loaded ? (
          <div className="p-8 text-sm text-muted-foreground">{t("Loading Sandboxes…")}</div>
        ) : filtered.length ? (
          <>
            <Table aria-label={t("Sandbox inventory")} className="max-md:block max-md:w-full">
              <TableHeader className="max-md:sr-only">
                <TableRow>
                  <TableHead>{t("Name")}</TableHead>
                  <TableHead>{t("Backend")}</TableHead>
                  <TableHead>{t("Isolation")}</TableHead>
                  <TableHead>{t("State")}</TableHead>
                  <TableHead>{t("Workspace")}</TableHead>
                  <TableHead className="text-right">{t("Actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody className="max-md:block">
                {pagination.items.map((item) => (
                  <TableRow
                    key={item.sandboxId}
                    className="md:[&>td]:py-3 max-md:grid max-md:grid-cols-[minmax(0,1fr)_auto] max-md:items-start max-md:gap-x-3 max-md:gap-y-1.5 max-md:px-4 max-md:py-3.5 max-md:border-b max-md:last:border-b-0"
                  >
                    <TableCell className="font-medium max-md:col-start-1 max-md:row-start-1 max-md:block max-md:min-w-0 max-md:p-0">
                      <span className="block truncate font-medium text-sm text-foreground" title={item.name}>
                        {item.name}
                      </span>
                    </TableCell>
                    <TableCell className="max-md:col-start-1 max-md:row-start-2 max-md:block max-md:min-w-0 max-md:p-0">
                      <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                        <span>{sandboxBackend(item)}</span>
                        <span className="text-muted-foreground/50 md:hidden">·</span>
                        <span className="md:hidden">{isolationLabel(item.isolation)}</span>
                      </div>
                    </TableCell>
                    <TableCell className="max-md:hidden">
                      {isolationLabel(item.isolation)}
                    </TableCell>
                    <TableCell className="max-md:col-start-2 max-md:row-start-1 max-md:block max-md:p-0 max-md:justify-self-end">
                      <StatusBadge status={item.state === "destroyed" || item.backendAvailable ? item.state : "unavailable"} className="max-md:px-2 max-md:py-0.5">
                        {item.state === "destroyed" || item.backendAvailable
                          ? sandboxStateLabel(item.state)
                          : t("Backend unavailable")}
                      </StatusBadge>
                    </TableCell>
                    <TableCell className="max-w-[240px] truncate font-mono text-xs text-muted-foreground max-md:col-start-1 max-md:row-start-3 max-md:block max-md:min-w-0 max-md:p-0">
                      <span className="block truncate font-mono text-xs text-muted-foreground" title={item.localRoot}>
                        {item.localRoot || "—"}
                      </span>
                    </TableCell>
                    <TableCell className="text-right max-md:col-start-2 max-md:row-start-2 max-md:row-span-2 max-md:block max-md:self-center max-md:p-0 max-md:justify-self-end">
                      <Button
                        size="sm"
                        variant="outline"
                        className="max-md:h-9 max-md:px-3 max-md:text-xs"
                        onClick={() => {
                          void navigate(`/sandboxes/${item.sandboxId}`);
                        }}
                      >
                        {t("View")}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </>
        ) : (
          <EmptyPanelState>
            <EmptyState
              title={items.length ? t("No matching sandboxes") : t("No sandboxes")}
              description={
                items.length
                  ? t("Try a different search or filter.")
                  : t("Create a local Sandbox before binding Workspace tools to it.")
              }
            />
          </EmptyPanelState>
        )}
    </ResultPanel>
    <TablePagination label="Sandboxes" pagination={pagination} />
  </>;
}
