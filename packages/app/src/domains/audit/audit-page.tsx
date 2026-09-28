import { useCallback, useEffect, useState } from "react";

import type { AuditRecord, MenderClient } from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@mender/ui/components/card";
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

import { EmptyState, ErrorNotice, PageHeader } from "../../shared/page";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";

import { t } from "../../i18n/i18n";

export function AuditPage({ client }: { client: MenderClient }) {
  const [records, setRecords] = useState<AuditRecord[]>([]);
  const [selected, setSelected] = useState<AuditRecord | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    setBusy(true);
    try {
      const result = await client.listAudit(100);
      setRecords(result.records);
      setError("");
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }, [client]);

  useEffect(() => {
    let active = true;
    void client
      .listAudit(100)
      .then((result) => {
        if (active) {
          setRecords(result.records);
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
        description="Data-minimized terminal invocation records with execution origin, scope, and canonical outcome classification. Arguments, full result payloads, credentials, progress payloads, and raw protocol traffic are not retained."
        actions={
          <Button variant="outline" onClick={() => void refresh()} disabled={busy}>
            {t("Refresh")}
          </Button>
        }
      />
      {error ? <ErrorNotice message={error} /> : null}

      <Card>
        <CardHeader>
          <CardTitle>{t("Invocation history")}</CardTitle>
        </CardHeader>
        <CardContent>
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
                      {new Date(record.finishedAt).toLocaleString()}
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
                    <TableCell className="max-w-60 truncate font-mono text-[11px]">
                      {record.toolId}
                    </TableCell>
                    <TableCell className="text-xs">
                      {record.policyKind || "—"}
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {record.errorClass || "—"}
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        variant="outline"
                        size="sm"
                        aria-label={"View audit " + record.invocationId}
                        onClick={() => setSelected(record)}
                      >
                        {t("View")}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
              </Table>
              <TablePagination
                label="Audit"
                pagination={auditPagination}
              />
            </>
          ) : (
            <EmptyState
              title="No audit records"
              description="Invocation outcomes will appear here after Tools are called."
            />
          )}
        </CardContent>
      </Card>

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
                  value={new Date(selected.startedAt).toLocaleString()}
                />
                <AuditField
                  label="Finished"
                  value={new Date(selected.finishedAt).toLocaleString()}
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

function errorMessage(reason: unknown) {
  return reason instanceof Error ? reason.message : t("Unexpected error");
}
