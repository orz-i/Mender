import { useCallback, useEffect, useMemo, useState } from "react";
import { EyeIcon, RefreshCwIcon } from "lucide-react";

import type {
  AuditRecord,
  MenderClient,
  ProviderActivationAuditRecord,
  ToolSummary,
} from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@mender/ui/components/table";

import { formatDateTime, t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { FocusSwitch } from "../../shared/focus-switch";
import { PageToolbar, ResultPanel } from "../../shared/page-toolbar";
import { IconButton } from "../../shared/icon-button";
import { EmptyState, ErrorNotice } from "../../shared/page";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import {
  FilterSelect,
  SearchField,
} from "../../shared/workbench";
import {
  AuditDetailDialog,
  AuditStatusBadge,
  type AuditDetailSection,
  type AuditSelection,
} from "./audit-detail-dialog";

type AuditTopic = "invocations" | "activations";
type StatusFilter = "all" | "completed" | "failed" | "denied";

type AuditPreview = {
  key: string;
  finishedAt: string;
  target: string;
  targetId: string;
  principal: string;
  status: string;
  policy: string;
  errorClass: string;
  selection: AuditSelection;
};

function browseAuditTools(client: MenderClient) {
  const browseTools = (client as Partial<MenderClient>).browseTools;
  if (typeof browseTools !== "function") return Promise.resolve(null);
  return browseTools.call(client, { limit: 100 }).catch(() => null);
}

function indexTools(tools: ToolSummary[]) {
  return Object.fromEntries(tools.map((tool) => [tool.toolId, tool]));
}

function toolLabel(toolsById: Record<string, ToolSummary>, toolId: string) {
  const tool = toolsById[toolId];
  return tool?.title || tool?.name || t("Unknown Tool");
}

function principalLabel(kind: string, reference: string) {
  return reference ? kind + ":" + reference : kind || "—";
}

function policyLabel(value: string) {
  switch (value.toLowerCase()) {
    case "allow": return t("Allow");
    case "deny": return t("Deny");
    default: return value || "—";
  }
}

function matchesPreview(row: AuditPreview, search: string, status: StatusFilter) {
  if (status !== "all" && row.status.toLowerCase() !== status) return false;
  if (!search) return true;
  const localizedStatus = row.status === "completed"
    ? t("Completed")
    : row.status === "failed"
      ? t("Failed")
      : row.status === "denied" ? t("Denied") : row.status;
  return [
    row.target,
    row.targetId,
    row.principal,
    row.status,
    localizedStatus,
    row.policy,
    policyLabel(row.policy),
    row.errorClass,
    formatDateTime(row.finishedAt),
  ].some((value) => value.toLocaleLowerCase().includes(search));
}

export function AuditPage({ client }: { client: MenderClient }) {
  const [records, setRecords] = useState<AuditRecord[]>([]);
  const [activationRecords, setActivationRecords] = useState<ProviderActivationAuditRecord[]>([]);
  const [toolsById, setToolsById] = useState<Record<string, ToolSummary>>({});
  const [topic, setTopic] = useState<AuditTopic>("invocations");
  const [query, setQuery] = useState("");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [selected, setSelected] = useState<AuditSelection | null>(null);
  const [detailSection, setDetailSection] = useState<AuditDetailSection>("overview");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [loaded, setLoaded] = useState(false);

  const refresh = useCallback(async () => {
    setBusy(true);
    try {
      const [invocations, activations, tools] = await Promise.all([
        client.listAudit(100),
        client.listProviderActivationAudit(100),
        browseAuditTools(client),
      ]);
      setRecords(invocations.records);
      setActivationRecords(activations.records);
      if (tools !== null) setToolsById(indexTools(tools.tools));
      setError("");
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
      setLoaded(true);
    }
  }, [client]);

  useEffect(() => {
    let active = true;
    void Promise.all([
      client.listAudit(100),
      client.listProviderActivationAudit(100),
      browseAuditTools(client),
    ])
      .then(([invocations, activations, tools]) => {
        if (!active) return;
        setRecords(invocations.records);
        setActivationRecords(activations.records);
        if (tools !== null) setToolsById(indexTools(tools.tools));
        setError("");
      })
      .catch((reason: unknown) => {
        if (active) setError(errorMessage(reason));
      })
      .finally(() => {
        if (active) setLoaded(true);
      });
    return () => { active = false; };
  }, [client]);

  const invocationRows = useMemo<AuditPreview[]>(
    () => records.map((record) => ({
      key: record.invocationId,
      finishedAt: record.finishedAt,
      target: toolLabel(toolsById, record.toolId),
      targetId: record.toolId,
      principal: principalLabel(record.principalKind, record.principalRef),
      status: record.status,
      policy: record.policyKind,
      errorClass: record.errorClass,
      selection: { kind: "invocations", record },
    })),
    [records, toolsById],
  );
  const activationRows = useMemo<AuditPreview[]>(
    () => activationRecords.map((record) => ({
      key: record.activationId,
      finishedAt: record.finishedAt,
      target: record.providerId,
      targetId: record.providerId,
      principal: principalLabel(record.principalKind, record.principalRef),
      status: record.status,
      policy: record.policyKind,
      errorClass: record.errorClass,
      selection: { kind: "activations", record },
    })),
    [activationRecords],
  );

  const search = query.trim().toLocaleLowerCase();
  const filteredInvocations = useMemo(
    () => invocationRows.filter((row) => matchesPreview(row, search, statusFilter)),
    [invocationRows, search, statusFilter],
  );
  const filteredActivations = useMemo(
    () => activationRows.filter((row) => matchesPreview(row, search, statusFilter)),
    [activationRows, search, statusFilter],
  );
  const invocationPagination = usePagination(filteredInvocations, {
    initialPageSize: 10, pageSizeOptions: [10, 25, 50],
  });
  const activationPagination = usePagination(filteredActivations, {
    initialPageSize: 10, pageSizeOptions: [10, 25, 50],
  });

  const activePagination = topic === "invocations" ? invocationPagination : activationPagination;
  const activeRecords = topic === "invocations" ? records : activationRecords;
  const panelTitle = topic === "invocations" ? "Invocation history" : "Provider activation history";

  function resetFilters() {
    setQuery("");
    setStatusFilter("all");
    invocationPagination.reset();
    activationPagination.reset();
  }

  function changeTopic(next: AuditTopic) {
    setTopic(next);
    resetFilters();
  }

  function openDetails(selection: AuditSelection) {
    setSelected(selection);
    setDetailSection(selection.record.status === "completed" ? "overview" : "diagnostics");
  }

  return (
    <>
      <PageToolbar
        tabs={
          <FocusSwitch
            label="Audit"
            value={topic}
            onChange={changeTopic}
            items={[
              { value: "invocations", label: "Invocation history" },
              { value: "activations", label: "Provider activation history" },
            ]}
          />
        }
        filters={
          <>
            <div className="w-full min-w-0 sm:w-48 sm:shrink-0">
              <SearchField
                label="Search audit records"
                placeholder="Search audit records…"
                value={query}
                onChange={(value) => {
                  setQuery(value);
                  invocationPagination.reset();
                  activationPagination.reset();
                }}
              />
            </div>
            <FilterSelect
              label="Audit status"
              value={statusFilter}
              onValueChange={(value) => {
                setStatusFilter(value as StatusFilter);
                invocationPagination.reset();
                activationPagination.reset();
              }}
              options={[
                { value: "all", label: "All statuses" },
                { value: "completed", label: "Completed" },
                { value: "failed", label: "Failed" },
                { value: "denied", label: "Denied" },
              ]}
              className="w-32 min-w-0"
            />
          </>
        }
        actions={
          <Button
            variant="outline"
            size="sm"
            aria-label={t("Refresh")}
            disabled={busy || !loaded}
            onClick={() => void refresh()}
          >
            <RefreshCwIcon data-icon="inline-start" className={busy ? "animate-spin" : undefined} />
            {t("Refresh")}
          </Button>
        }
      />
      {error ? <ErrorNotice message={error} /> : null}
      <ResultPanel title={panelTitle}>
        {!loaded && !error ? (
          <p className="px-5 py-12 text-sm text-muted-foreground">{t("Loading audit history…")}</p>
        ) : activePagination.total ? (
          <>
            <Table aria-label={t(panelTitle)} className="max-md:block max-md:w-full">
              <TableHeader className="max-md:sr-only">
                <TableRow>
                  <TableHead>{t("Finished")}</TableHead>
                  <TableHead>{t(topic === "invocations" ? "Tool" : "Provider")}</TableHead>
                  <TableHead>{t("Principal")}</TableHead>
                  <TableHead>{t("Status")}</TableHead>
                  <TableHead>{t("Policy")}</TableHead>
                  <TableHead className="text-right">{t("Actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody className="max-md:block">
                {activePagination.items.map((row) => (
                  <TableRow key={row.key} className="md:[&>td]:py-3 max-md:grid max-md:grid-cols-[minmax(0,1fr)_auto] max-md:items-start max-md:gap-x-3 max-md:gap-y-1 max-md:px-4 max-md:py-3">
                    <TableCell className="max-md:col-start-1 max-md:row-start-3 max-md:block max-md:p-0">
                      <time dateTime={row.finishedAt} className="whitespace-nowrap text-xs tabular-nums text-muted-foreground max-md:text-[11px]">
                        {formatDateTime(row.finishedAt)}
                      </time>
                    </TableCell>
                    <TableCell className="font-medium max-md:col-start-1 max-md:row-start-1 max-md:block max-md:min-w-0 max-md:p-0">
                      <span title={row.target} className="block max-w-48 truncate">{row.target}</span>
                    </TableCell>
                    <TableCell className="text-xs max-md:col-start-1 max-md:row-start-2 max-md:block max-md:min-w-0 max-md:p-0">
                      <span title={row.principal} className="block max-w-48 truncate text-muted-foreground">{row.principal}</span>
                    </TableCell>
                    <TableCell className="max-md:col-start-2 max-md:row-start-1 max-md:block max-md:p-0">
                      <div className="flex flex-col items-start gap-1 max-md:items-end">
                        <AuditStatusBadge status={row.status} />
                        {row.status !== "completed" && row.errorClass ? (
                          <span className="max-w-32 truncate text-[11px] text-muted-foreground" title={row.errorClass}>
                            {row.errorClass}
                          </span>
                        ) : null}
                      </div>
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground max-md:hidden">
                      {policyLabel(row.policy)}
                    </TableCell>
                    <TableCell className="text-right max-md:col-start-2 max-md:row-start-2 max-md:row-span-2 max-md:block max-md:self-center max-md:p-0">
                      <IconButton
                        variant="outline"
                        size="icon-sm"
                        label={t(topic === "invocations" ? "View audit record" : "View Provider activation record")}
                        onClick={() => openDetails(row.selection)}
                      >
                        <EyeIcon />
                      </IconButton>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            <div className="px-5 pb-4">
              <TablePagination
                label={topic === "invocations" ? "Audit" : "Provider activations"}
                pagination={activePagination}
              />
            </div>
          </>
        ) : (
          <div className="px-5 py-5">
            <EmptyState
              title={activeRecords.length
                ? t("No matching audit records")
                : topic === "invocations" ? t("No audit records") : t("No Provider activation records")}
              description={activeRecords.length
                ? t("Try another search or status filter.")
                : topic === "invocations"
                  ? t("Invocation outcomes will appear here after Tools are called.")
                  : t("Provider activation admission outcomes will appear here when Providers are created, synchronized, rebound, or activated for Tool execution.")}
            />
            {activeRecords.length ? (
              <div className="mt-3 flex justify-center">
                <Button size="sm" variant="outline" onClick={resetFilters}>{t("Clear filters")}</Button>
              </div>
            ) : null}
          </div>
        )}
      </ResultPanel>

      <AuditDetailDialog
        selection={selected}
        section={detailSection}
        onSectionChange={setDetailSection}
        onClose={() => setSelected(null)}
      />
    </>
  );
}
