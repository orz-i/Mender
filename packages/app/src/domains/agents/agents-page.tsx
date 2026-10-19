import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router";

import type { Agent, MenderClient } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@mender/ui/components/table";

import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { EmptyState, ErrorNotice } from "../../shared/page";
import { PageToolbar, PageTab, PageTabs, ResultPanel } from "../../shared/page-toolbar";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import { StatusBadge } from "../../shared/status-badge";
import { agentBackendLabel } from "./agent-backends";
import { EmptyPanelState, SearchField } from "../../shared/workbench";

type Filter = "all" | "available" | "unavailable";

export function AgentsPage({ client }: { client: MenderClient }) {
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
    }).catch((reason:unknown) => {
      if (active) {setError(errorMessage(reason));setLoaded(true);}
    });
    return () => {active=false;};
  }, [client,reload]);

  const filtered = useMemo(() => agents.filter((agent) => {
    const matches = filter === "all" ||
      (filter === "available" && agent.backendAvailable) ||
      (filter === "unavailable" && !agent.backendAvailable);
    return matches && `${agent.name} ${agent.backend} ${agent.workspaceRoot}`
      .toLocaleLowerCase().includes(query.toLocaleLowerCase());
  }),[agents,filter,query]);
  const pagination = usePagination(filtered);

  return <>
    {hasSnapshot ? <PageToolbar
      tabs={<PageTabs label="Agents">
        <PageTab label="All agents" active={filter==="all"} count={agents.length} onClick={()=>setFilter("all")}/>
        <PageTab label="Available agents" active={filter==="available"} count={agents.filter(a=>a.backendAvailable).length} onClick={()=>setFilter("available")}/>
        <PageTab label="Unavailable agents" active={filter==="unavailable"} count={agents.filter(a=>!a.backendAvailable).length} onClick={()=>setFilter("unavailable")}/>
      </PageTabs>}
      filters={
        <div className="w-full min-w-0 sm:w-56 sm:shrink-0">
          <SearchField value={query} onChange={setQuery} placeholder="Search agents…" label="Search agents" />
        </div>
      }
      actions={<>
        <Button size="sm" variant="outline" onClick={()=>{setLoaded(false);setError("");setReload(n=>n+1);}}>{t("Refresh")}</Button>
        <Button size="sm" onClick={()=>void navigate("/agents/new")}>{t("Add agent")}</Button>
      </>}
    /> : null}
    {error ? <ErrorNotice message={error} onRetry={!hasSnapshot ? () => {setLoaded(false);setError("");setReload(n => n + 1);} : undefined} /> : null}
    {error && !hasSnapshot ? null : <ResultPanel title="Agent inventory">
      {!loaded?<div className="p-8 text-sm text-muted-foreground">{t("Loading agents…")}</div>:
      filtered.length?<>
        <Table>
          <TableHeader><TableRow>
            <TableHead>{t("Name")}</TableHead>
            <TableHead>{t("Backend")}</TableHead>
            <TableHead>{t("Isolation")}</TableHead>
            <TableHead>{t("Status")}</TableHead>
            <TableHead>{t("Workspace")}</TableHead>
            <TableHead className="text-right">{t("Actions")}</TableHead>
          </TableRow></TableHeader>
          <TableBody>{pagination.items.map(a=><TableRow key={a.agentId}>
            <TableCell className="max-w-[200px] truncate font-medium">{a.name}</TableCell>
            <TableCell>{agentBackendLabel(a.backend)}</TableCell>
            <TableCell>{a.isolation==="host"?t("Host (not isolated)"):t("Restricted host")}</TableCell>
            <TableCell><StatusBadge status={a.backendAvailable ? "ready" : "unavailable"}>{a.backendAvailable ? t("CLI detected") : t("CLI missing")}</StatusBadge></TableCell>
            <TableCell className="max-w-[280px] truncate font-mono text-xs text-muted-foreground" title={a.workspaceRoot}>{a.workspaceRoot}</TableCell>
            <TableCell className="text-right"><Button size="sm" variant="outline"
              onClick={()=>void navigate("/agents/"+a.agentId)}>{t("View")}</Button></TableCell>
          </TableRow>)}</TableBody>
        </Table>
      </>:<EmptyPanelState><EmptyState
        title={agents.length?t("No matching agents"):t("No agents")}
        description={agents.length?t("Try a different search or filter."):t("Add an Agent to delegate tasks through a Profile.")}/>
      </EmptyPanelState>}
    </ResultPanel>}
    {hasSnapshot && pagination.total > 0 ? <TablePagination label="Agents" pagination={pagination} /> : null}
  </>;
}
