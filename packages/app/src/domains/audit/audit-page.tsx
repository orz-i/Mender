import { useCallback, useEffect, useState } from "react";
import { EyeIcon } from "lucide-react";

import type { AuditRecord, MenderClient, ToolSummary } from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@mender/ui/components/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@mender/ui/components/table";

import { errorMessage } from "../../shared/errors";
import { IconButton } from "../../shared/icon-button";
import { EmptyState, ErrorNotice, PageHeader } from "../../shared/page";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import { DataPanel, EmptyPanelState } from "../../shared/workbench";

import { formatDateTime, t } from "../../i18n/i18n";

export function AuditPage({ client }: { client: MenderClient }) {
  const [records, setRecords] = useState<AuditRecord[]>([]);
  const [toolsById, setToolsById] = useState<Record<string, ToolSummary>>({});
  const [selected, setSelected] = useState<AuditRecord | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    setBusy(true);
    try {
      const [result, toolResult] = await Promise.all([
        client.listAudit(100),
        browseAuditTools(client),
      ]);
      setRecords(result.records);
      if (toolResult !== null) {
        setToolsById(indexTools(toolResult.tools));
      }
      setError("");
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }, [client]);

  useEffect(() => {
    let active = true;
    void Promise.all([
      client.listAudit(100),
      browseAuditTools(client),
    ])
      .then(([result, toolResult]) => {
        if (active) {
          setRecords(result.records);
          if (toolResult !== null) {
            setToolsById(indexTools(toolResult.tools));
          }
          setError("");
        }
      })
      .catch((reason: unknown) => {
        if (active) {
          setError(errorMessage(reason));
        }
      });
    return () => {
      active = false;
    };
  }, [client]);

  const auditPagination = usePagination(records);

  return (
    <>
      <PageHeader
        title="Audit"
        actions={
          <Button variant="outline" onClick={() => void refresh()} disabled={busy}>
            {t("Refresh")}
          </Button>
        }
      />
      {error ? <ErrorNotice message={error} /> : null}

      <DataPanel
        title="Invocation history"
        description="Recent terminal invocation outcomes and governance decisions."
      >
          {records.length ? (
            <>
              <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("Finished")}</TableHead>
                  <TableHead>{t("Status")}</TableHead>
                  <TableHead>{t("Caller")}</TableHead>
                  <TableHead>{t("Tool")}</TableHead>
                  <TableHead>{t("Policy")}</TableHead>
                  <TableHead>{t("Error")}</TableHead>
                  <TableHead className="text-right">{t("Actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {auditPagination.items.map((record) => (
                  <TableRow key={record.invocationId}>
                    <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                      {formatDateTime(record.finishedAt)}
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={
                          record.status === "completed" ? "outline" : "secondary"
                        }
                      >
                        {record.status}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-xs">{record.caller || "—"}</TableCell>
                    <TableCell className="max-w-60 truncate">
                      {toolLabel(toolsById, record.toolId)}
                    </TableCell>
                    <TableCell className="text-xs">
                      {record.policyKind || "—"}
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {record.errorClass || "—"}
                    </TableCell>
                    <TableCell className="text-right">
                      <IconButton
                        variant="outline"
                        label={t("View audit record")}
                        onClick={() => setSelected(record)}
                      >
                        <EyeIcon />
                      </IconButton>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
              </Table>
              <div className="px-4 pb-3">
                <TablePagination
                  label="Audit"
                  pagination={auditPagination}
                />
              </div>
            </>
          ) : (
            <EmptyPanelState>
              <EmptyState
                title="No audit records"
                description="Invocation outcomes will appear here after Tools are called."
              />
            </EmptyPanelState>
          )}
      </DataPanel>

      <Dialog
        open={selected !== null}
        onOpenChange={(open) => {
          if (!open) {
            setSelected(null);
          }
        }}
      >
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t("Invocation details")}</DialogTitle>
            <DialogDescription>
              {t("Data-minimized terminal metadata and failure diagnostics.")}
            </DialogDescription>
          </DialogHeader>

          {selected !== null ? (
            <div className="space-y-5">
              <div className="grid gap-3 sm:grid-cols-2">
                <AuditField label="Status" value={selected.status} />
                <AuditField label="Outcome" value={selected.outcomeKind || "—"} />
                <AuditField label="Origin" value={selected.origin || "—"} />
                <AuditField label="Caller" value={selected.caller || "—"} />
                <AuditField label="Scope" value={selected.scopeKind || "—"} />
                <AuditField
                  label="Scope reference"
                  value={selected.scopeReference || "—"}
                  mono
                />
                <AuditField
                  label="Invocation ID"
                  value={selected.invocationId}
                  mono
                />
                <AuditField label="Tool ID" value={selected.toolId} mono />
                <AuditField
                  label="Provider ID"
                  value={selected.providerId || "—"}
                  mono
                />
                <AuditField
                  label="Result kind"
                  value={selected.resultKind || "—"}
                />
                <AuditField
                  label="Started"
                  value={formatDateTime(selected.startedAt)}
                />
                <AuditField
                  label="Finished"
                  value={formatDateTime(selected.finishedAt)}
                />
              </div>

              <div className="grid gap-3 sm:grid-cols-3">
                <AuditField
                  label="Policy"
                  value={selected.policyKind || "—"}
                />
                <AuditField
                  label="Approval"
                  value={selected.approval || "—"}
                />
                <AuditField
                  label="Error class"
                  value={selected.errorClass || "—"}
                />
                <AuditField
                  label="Rejection code"
                  value={selected.rejectionCode || "—"}
                />
                <AuditField
                  label="Failure source"
                  value={selected.failureSource || "—"}
                />
                <AuditField
                  label="Failure code"
                  value={selected.failureCode || "—"}
                />
              </div>

              {selected.policyReason ? (
                <div>
                  <p className="text-sm font-medium">{t("Policy reason")}</p>
                  <p className="mt-1 text-sm text-muted-foreground">
                    {selected.policyReason}
                  </p>
                </div>
              ) : null}

              {selected.status !== "completed" ? (
                <div className="rounded-lg border border-border p-4">
                  <p className="text-sm font-medium">{t("Error summary")}</p>
                  {selected.errorSummary ? (
                    <p className="mt-2 whitespace-pre-wrap break-words text-sm leading-6 text-muted-foreground">
                      {selected.errorSummary}
                    </p>
                  ) : (
                    <p className="mt-2 text-sm leading-6 text-muted-foreground">
                      {t("No diagnostic summary was retained for this record. Older Audit records and failures without a safe upstream summary only contain the error class.")}
                    </p>
                  )}
                </div>
              ) : null}
            </div>
          ) : null}

          <DialogFooter showCloseButton />
        </DialogContent>
      </Dialog>
    </>
  );
}

function browseAuditTools(client: MenderClient) {
  const browseTools = (client as Partial<MenderClient>).browseTools;
  if (typeof browseTools !== "function") {
    return Promise.resolve(null);
  }
  return browseTools.call(client, { limit: 100 }).catch(() => null);
}

function indexTools(tools: ToolSummary[]) {
  return Object.fromEntries(tools.map((tool) => [tool.toolId, tool]));
}

function toolLabel(
  toolsById: Record<string, ToolSummary>,
  toolId: string,
) {
  const tool = toolsById[toolId];
  return tool?.title || tool?.name || t("Unknown Tool");
}

function AuditField({
  label,
  value,
  mono = false,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div className="min-w-0 rounded-lg border border-border bg-muted/40 p-3">
      <p className="text-xs text-muted-foreground">{t(label)}</p>
      <p
        className={[
          "mt-1 truncate text-sm",
          mono ? "font-mono text-xs" : "",
        ].join(" ")}
      >
        {value}
      </p>
    </div>
  );
}
