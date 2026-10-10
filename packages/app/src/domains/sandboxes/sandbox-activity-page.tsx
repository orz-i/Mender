import { useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router";

import type { MenderClient, SandboxActivityEvent } from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@mender/ui/components/table";

import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { EmptyState, ErrorNotice } from "../../shared/page";
import { PageToolbar, ResultPanel } from "../../shared/page-toolbar";
import { usePagination } from "../../shared/pagination";
import { EmptyPanelState } from "../../shared/workbench";
import { TablePagination } from "../../shared/table-pagination";

type ActivityFilter = "all" | "admission" | "lifecycle";

/**
 * Read-only Sandbox governance history. The server intentionally withholds
 * principal references, approval reasoning, file arguments and remote secrets.
 */
export function SandboxActivityPage({ client }: { client: MenderClient }) {
  const { sandboxId } = useParams();
  const navigate = useNavigate();
  const [events, setEvents] = useState<SandboxActivityEvent[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [filter, setFilter] = useState<ActivityFilter>("all");

  useEffect(() => {
    if (!sandboxId) return;
    let active = true;
    void client.getSandboxActivity(sandboxId, 100).then((result) => {
      if (active) {
        setEvents(result.events);
        setLoaded(true);
        setError("");
      }
    }).catch((reason: unknown) => {
      if (active) {
        setError(errorMessage(reason));
        setLoaded(true);
      }
    });
    return () => { active = false; };
  }, [client, sandboxId]);

  const filtered = useMemo(() => events.filter((event) => filter === "all" || event.kind === filter), [events, filter]);
  const pagination = usePagination(filtered);

  return <>
    <PageToolbar
      filters={
        <label className="flex items-center gap-2 text-sm">
          <span className="text-muted-foreground">{t("Event type")}</span>
          <select aria-label={t("Event type")} value={filter} onChange={(event) => setFilter(event.target.value as ActivityFilter)} className="h-9 w-40 rounded-xl border border-input bg-card px-3 text-sm">
            <option value="all">{t("All events")}</option>
            <option value="admission">{t("Admissions")}</option>
            <option value="lifecycle">{t("Lifecycle")}</option>
          </select>
        </label>
      }
      actions={
        <Button size="sm" variant="outline" onClick={() => void navigate(sandboxId ? `/sandboxes/${sandboxId}` : "/sandboxes")}>{t("Back to sandbox")}</Button>
      }
    />
    {error || !sandboxId ? <ErrorNotice message={error || t("The requested item was not found.")} /> : null}
    <ResultPanel title="Sandbox activity">
      {!loaded && !error ? <div className="p-6 text-sm text-muted-foreground">{t("Loading activity…")}</div> : error || !sandboxId ? null : filtered.length === 0 ?
        <EmptyPanelState><EmptyState title={t("No activity yet")} description={t("Sandbox decisions and state transitions will appear here.")} /></EmptyPanelState> : <>
          <Table>
            <TableHeader><TableRow>
              <TableHead>{t("Time")}</TableHead>
              <TableHead>{t("Event type")}</TableHead>
              <TableHead>{t("Action")}</TableHead>
              <TableHead>{t("Outcome")}</TableHead>
              <TableHead>{t("Origin")}</TableHead>
              <TableHead>{t("Detail")}</TableHead>
            </TableRow></TableHeader>
            <TableBody>{pagination.items.map((event, index) => <TableRow key={`${event.occurredAt}-${event.action}-${index}`}>
              <TableCell className="whitespace-nowrap text-xs text-muted-foreground" title={event.occurredAt}>{new Date(event.occurredAt).toLocaleString()}</TableCell>
              <TableCell>{event.kind === "admission" ? t("Admissions") : t("Lifecycle")}</TableCell>
              <TableCell className="font-mono text-xs">{event.action}</TableCell>
              <TableCell><Badge variant="outline">{activityStatus(event.status)}</Badge></TableCell>
              <TableCell className="text-xs">{event.origin}</TableCell>
              <TableCell className="max-w-[220px] truncate text-xs text-muted-foreground" title={activityDetail(event)}>{activityDetail(event)}</TableCell>
            </TableRow>)}</TableBody>
          </Table>
          <div className="px-4 pb-3"><TablePagination label={t("Events")} pagination={pagination}/></div>
        </>}
    </ResultPanel>
  </>;
}

function activityStatus(status: SandboxActivityEvent["status"]): string {
  switch (status) {
    case "admitted": return t("Admitted");
    case "denied": return t("Denied");
    case "failed": return t("Failed");
    case "completed": return t("Completed");
  }
}

function activityDetail(event: SandboxActivityEvent): string {
  if (event.kind === "lifecycle") return `${event.previousState ?? "—"} → ${event.nextState ?? "—"} · ${t("Generation")} ${event.generation ?? "—"}`;
  return `${t("Principal type")}: ${event.principalKind}${event.policyDecision ? ` · ${event.policyDecision}` : ""}`;
}
