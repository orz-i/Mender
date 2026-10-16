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
          <Table aria-label={t("Sandbox activity")} className="max-md:block max-md:w-full">
            <TableHeader className="max-md:sr-only">
              <TableRow>
                <TableHead>{t("Time")}</TableHead>
                <TableHead>{t("Event type")}</TableHead>
                <TableHead>{t("Action")}</TableHead>
                <TableHead>{t("Outcome")}</TableHead>
                <TableHead>{t("Origin")}</TableHead>
                <TableHead>{t("Detail")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody className="max-md:block">
              {pagination.items.map((event, index) => (
                <TableRow
                  key={`${event.occurredAt}-${event.action}-${index}`}
                  className="md:[&>td]:py-3 max-md:grid max-md:grid-cols-[minmax(0,1fr)_auto] max-md:items-start max-md:gap-x-3 max-md:gap-y-1.5 max-md:px-4 max-md:py-3.5 max-md:border-b max-md:last:border-b-0"
                >
                  <TableCell className="font-mono text-xs max-md:col-start-1 max-md:row-start-1 max-md:block max-md:min-w-0 max-md:p-0">
                    <span className="font-semibold text-foreground">{event.action}</span>
                  </TableCell>
                  <TableCell className="max-md:col-start-2 max-md:row-start-1 max-md:block max-md:p-0 max-md:justify-self-end">
                    <Badge variant="outline" className="max-md:text-[11px] max-md:px-2 max-md:py-0.5">
                      {activityStatus(event.status)}
                    </Badge>
                  </TableCell>
                  <TableCell className="max-md:col-start-1 max-md:row-start-2 max-md:block max-md:min-w-0 max-md:p-0">
                    <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                      <span>{event.kind === "admission" ? t("Admissions") : t("Lifecycle")}</span>
                      <span className="text-muted-foreground/50">·</span>
                      <span>{event.origin}</span>
                    </div>
                  </TableCell>
                  <TableCell className="whitespace-nowrap text-xs text-muted-foreground max-md:col-start-2 max-md:row-start-2 max-md:block max-md:p-0 max-md:justify-self-end max-md:text-[11px]" title={event.occurredAt}>
                    {new Date(event.occurredAt).toLocaleString()}
                  </TableCell>
                  <TableCell className="max-md:hidden">
                    {event.origin}
                  </TableCell>
                  <TableCell className="max-w-[220px] truncate text-xs text-muted-foreground max-md:col-span-2 max-md:row-start-3 max-md:block max-md:min-w-0 max-md:p-0 max-md:pt-1 text-muted-foreground/80 max-md:text-[11px]" title={activityDetail(event)}>
                    {activityDetail(event)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </>}
    </ResultPanel>
    <TablePagination label={t("Events")} pagination={pagination} />
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
