import { useEffect, useMemo, useState } from "react";
import { BotIcon, ChevronRightIcon } from "lucide-react";
import { Link, useNavigate } from "react-router";

import type { Agent, ManagementClient } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";

import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { EmptyState, ErrorNotice } from "../../shared/page";
import { PageToolbar, PageTab, PageTabs, ResultPanel } from "../../shared/page-toolbar";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import { agentBackendLabel } from "./agent-backends";
import { EmptyPanelState, SearchField } from "../../shared/workbench";

type Filter = "all" | "available" | "unavailable";

// Show just enough context in the collection; detail retains the full location.
function workspacePreview(path: string): string {
  const parts = path.split(/[/\\]+/).filter(Boolean);
  if (parts.length <= 2) return path || "—";
  return "…/" + parts.slice(-2).join("/");
}

export function AgentsPage({ client }: { client: ManagementClient }) {
  const navigate = useNavigate();
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [hasSnapshot, setHasSnapshot] = useState(false);
  const [error, setError] = useState("");
  const [reload, setReload] = useState(0);
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<Filter>("all");

  useEffect(() => {
    let active = true;
    void client.listAgents().then((result) => {
      if (!active) return;
      setAgents(result.agents);
      setHasSnapshot(true);
      setError("");
      setLoaded(true);
    }).catch((reason: unknown) => {
      if (active) { setError(errorMessage(reason)); setLoaded(true); }
    });
    return () => { active = false; };
  }, [client, reload]);

  const filtered = useMemo(() => agents.filter((agent) => {
    const matches = filter === "all" ||
      (filter === "available" && agent.backendAvailable) ||
      (filter === "unavailable" && !agent.backendAvailable);
    return matches && `${agent.name} ${agent.backend} ${agent.workspaceRoot}`
      .toLocaleLowerCase().includes(query.toLocaleLowerCase());
  }), [agents, filter, query]);
  const pagination = usePagination(filtered);

  return (
    <>
      <p className="mb-5 max-w-[950px] text-[13px] leading-6 text-muted-foreground">
        {t("Select an Agent to review its workspace, delegated tasks and permissions.")}
      </p>
      {hasSnapshot ? <PageToolbar
        tabs={<PageTabs label="Agents">
          <PageTab label="All agents" active={filter === "all"} count={agents.length} onClick={() => setFilter("all")} />
          <PageTab label="Available agents" active={filter === "available"}
            count={agents.filter((a) => a.backendAvailable).length} onClick={() => setFilter("available")} />
          <PageTab label="Unavailable agents" active={filter === "unavailable"}
            count={agents.filter((a) => !a.backendAvailable).length} onClick={() => setFilter("unavailable")} />
        </PageTabs>}
        filters={<div className="w-full min-w-0 sm:w-52 sm:shrink-0">
          <SearchField value={query} onChange={setQuery} placeholder="Search agents…" label="Search agents" />
        </div>}
        actions={<Button size="sm" onClick={() => void navigate("/agents/new")}>{t("Add agent")}</Button>}
      /> : null}
      {error ? <ErrorNotice message={error} onRetry={!hasSnapshot ? () => {
        setLoaded(false); setError(""); setReload((n) => n + 1);
      } : undefined} /> : null}
      {error && !hasSnapshot ? null : (
        <ResultPanel title="Agent inventory">
          {!loaded ? (
            <p role="status" className="p-8 text-[13px] text-muted-foreground">{t("Loading agents…")}</p>
          ) : filtered.length ? (
            <div role="list" aria-label={t("Agent inventory")} data-slot="agent-collection" className="divide-y divide-border/80">
              {pagination.items.map((agent) => (
                <div key={agent.agentId} role="listitem">
                  <Link to={"/agents/" + agent.agentId} aria-label={agent.name}
                    className="group flex min-h-[78px] min-w-0 items-center gap-3 px-4 py-3 transition-colors hover:bg-muted/45 focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-ring sm:gap-4 sm:px-5">
                    <span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-secondary text-muted-foreground">
                      <BotIcon className="size-[21px]" aria-hidden="true" />
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-[14px] font-semibold text-foreground group-hover:text-primary-ink">{agent.name}</span>
                      <span className="mt-1 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-[12px] text-muted-foreground">
                        <span className={["size-1.5 shrink-0 rounded-full", agent.backendAvailable ? "bg-teal" : "bg-sunlight"].join(" ")} aria-hidden="true" />
                        <span>{t(agent.backendAvailable ? "CLI detected" : "CLI missing")}</span>
                        <span className="text-border" aria-hidden="true">·</span>
                        <span>{agentBackendLabel(agent.backend)}</span>
                      </span>
                    </span>
                    <span className="hidden min-w-0 max-w-[34%] flex-col items-end gap-1 text-right md:flex">
                      <span className="truncate text-[11px] text-muted-foreground">{t("Workspace")}</span>
                      <span className="max-w-full truncate font-mono text-[12px] text-foreground" title={agent.workspaceRoot}
                        aria-label={t("Workspace") + ": " + agent.workspaceRoot}>
                        {workspacePreview(agent.workspaceRoot)}
                      </span>
                    </span>
                    <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground/55 transition-transform group-hover:translate-x-0.5 group-hover:text-foreground" aria-hidden="true" />
                  </Link>
                </div>
              ))}
            </div>
          ) : (
            <EmptyPanelState><EmptyState
              title={agents.length ? t("No matching agents") : t("No agents")}
              description={agents.length ? t("Try a different search or filter.") : t("Add an Agent to delegate tasks through a Profile.")}
            /></EmptyPanelState>
          )}
        </ResultPanel>
      )}
      {hasSnapshot && pagination.total > 0 ? <TablePagination label="Agents" pagination={pagination} /> : null}
    </>
  );
}
