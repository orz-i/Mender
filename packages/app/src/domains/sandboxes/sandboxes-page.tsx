import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router";
import { BoxIcon } from "lucide-react";

import type { ManagementClient, Sandbox } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";

import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { EmptyState, ErrorNotice } from "../../shared/page";
import { PageTab, PageTabs, PageToolbar, ResultPanel } from "../../shared/page-toolbar";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import { StatusInline } from "../../shared/status-badge";
import { ResourceList, ResourceRow } from "../../shared/resource-list";
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
  const [category, setCategory] = useState<"active" | "ready" | "not_ready" | "archived">("active");

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
    ((category === "active" && item.state !== "destroyed") ||
      (category === "ready" && item.state === "ready") ||
      (category === "not_ready" && item.state !== "ready" && item.state !== "destroyed") ||
      (category === "archived" && item.state === "destroyed")) &&
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
          <PageTab label="Active sandboxes" count={items.filter((item) => item.state !== "destroyed").length} active={category === "active"} onClick={() => setCategory("active")} />
          <PageTab label="Ready sandboxes" count={items.filter((item) => item.state === "ready").length} active={category === "ready"} onClick={() => setCategory("ready")} />
          <PageTab label="Not ready" count={items.filter((item) => item.state !== "ready" && item.state !== "destroyed").length} active={category === "not_ready"} onClick={() => setCategory("not_ready")} />
          <PageTab label="Archived sandboxes" count={items.filter((item) => item.state === "destroyed").length} active={category === "archived"} onClick={() => setCategory("archived")} />
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
          <ResourceList label="Sandbox inventory" slot="sandbox-collection">
            {pagination.items.map((item) => {
              const status = item.state === "destroyed" || item.backendAvailable
                ? item.state : "unavailable";
              return (
                <ResourceRow key={item.sandboxId}
                  to={"/sandboxes/" + item.sandboxId}
                  label={item.name}
                  icon={BoxIcon}
                  title={item.name}
                  description={<>{sandboxBackend(item)} <span aria-hidden="true">·</span> {isolationLabel(item.isolation)}</>}
                  detail={item.localRoot ? (
                    <span className="block max-w-64 truncate font-mono text-[11px]" title={item.localRoot}>
                      {item.localRoot}
                    </span>
                  ) : undefined}
                  status={<StatusInline status={status}>
                    {status === "unavailable" ? t("Backend unavailable") : sandboxStateLabel(item.state)}
                  </StatusInline>}
                />
              );
            })}
          </ResourceList>
        ) : (
          <EmptyPanelState>
            <EmptyState
              title={category === "active" && items.length > 0 && items.every((item) => item.state === "destroyed")
                ? t("No active sandboxes")
                : items.length ? t("No matching sandboxes") : t("No sandboxes")}
              description={category === "active" && items.length > 0 && items.every((item) => item.state === "destroyed")
                ? t("Previously destroyed sandboxes remain in the Archived sandboxes view.")
                : items.length ? t("Try a different search or filter.")
                  : t("Create a local Sandbox before binding Workspace tools to it.")}
            />
          </EmptyPanelState>
        )}
    </ResultPanel>
    <TablePagination label="Sandboxes" pagination={pagination} />
  </>;
}
