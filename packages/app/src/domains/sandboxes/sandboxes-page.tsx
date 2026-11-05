import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router";
import { BoxIcon, ChevronRightIcon } from "lucide-react";

import type { ManagementClient, Sandbox } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";

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

export function SandboxesPage({ client }: { client: ManagementClient }) {
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
            className="h-9 w-36 min-w-0 rounded-lg border border-input bg-card px-3 text-sm text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring"
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
          <div role="list" aria-label={t("Sandbox inventory")} data-slot="sandbox-collection"
            className="divide-y divide-border/80">
            {pagination.items.map((item) => (
              <div role="listitem" key={item.sandboxId}>
                <Link to={"/sandboxes/" + item.sandboxId} aria-label={item.name}
                  className="group flex min-h-[78px] min-w-0 items-center gap-3 px-4 py-3 transition-colors hover:bg-muted/40 focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-ring sm:gap-4 sm:px-5">
                  <span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-secondary text-muted-foreground">
                    <BoxIcon className="size-5" aria-hidden="true" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-[14px] font-semibold text-foreground group-hover:text-primary-ink">{item.name}</span>
                    <span className="mt-1 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-[12px] text-muted-foreground">
                      <span>{sandboxBackend(item)}</span>
                      <span className="text-border" aria-hidden="true">·</span>
                      <span>{isolationLabel(item.isolation)}</span>
                    </span>
                  </span>
                  <span className="hidden min-w-0 max-w-[28%] flex-col items-end gap-1 md:flex">
                    <span className="text-[11px] text-muted-foreground">{t("Workspace")}</span>
                    <span className="max-w-full truncate font-mono text-[11px] text-foreground" title={item.localRoot}>
                      {item.localRoot || "—"}
                    </span>
                  </span>
                  <StatusBadge status={item.state === "destroyed" || item.backendAvailable ? item.state : "unavailable"}
                    className="shrink-0">
                    {item.state === "destroyed" || item.backendAvailable ? sandboxStateLabel(item.state) : t("Backend unavailable")}
                  </StatusBadge>
                  <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground/60" aria-hidden="true" />
                </Link>
              </div>
            ))}
          </div>
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
