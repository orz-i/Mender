import type { AuditRecord, ProviderActivationAuditRecord } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@mender/ui/components/dialog";

import { formatDateTime, t } from "../../i18n/i18n";
import { FocusSwitch } from "../../shared/focus-switch";
import { StatusBadge } from "../../shared/status-badge";

export type AuditSelection =
  | { kind: "invocations"; record: AuditRecord }
  | { kind: "activations"; record: ProviderActivationAuditRecord };

export type AuditDetailSection = "overview" | "governance" | "diagnostics";

export function AuditStatusBadge({ status }: { status: string }) {
  const normalized = status.toLowerCase();
  const label =
    normalized === "completed" ? t("Completed") :
    normalized === "failed" ? t("Failed") :
    normalized === "denied" ? t("Denied") : status;

  return <StatusBadge status={status}>{label}</StatusBadge>;
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
    <div className="min-w-0 rounded-xl border border-border/70 bg-background/65 px-4 py-3">
      <dt className="text-xs font-medium text-muted-foreground">{t(label)}</dt>
      <dd className={["mt-1 whitespace-pre-wrap break-words text-sm text-foreground", mono ? "break-all font-mono text-xs leading-5" : ""].join(" ")}>
        {value || "—"}
      </dd>
    </div>
  );
}

function principalLabel(kind: string, reference: string) {
  return reference ? `${kind}:${reference}` : kind || "—";
}

/** Only safe, bounded metadata returned by the Audit API is displayed. */
export function AuditDetailDialog({
  selection,
  section,
  onSectionChange,
  onClose,
}: {
  selection: AuditSelection | null;
  section: AuditDetailSection;
  onSectionChange: (value: AuditDetailSection) => void;
  onClose: () => void;
}) {
  const record = selection?.record;
  const failed = record !== undefined && record.status !== "completed";
  const isInvocation = selection?.kind === "invocations";

  return (
    <Dialog open={selection !== null} onOpenChange={(open) => { if (!open) onClose(); }}>
      <DialogContent className="max-h-[min(88vh,760px)] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {t(isInvocation ? "Invocation details" : "Provider activation details")}
          </DialogTitle>
          <DialogDescription>
            {t(isInvocation
              ? "Data-minimized terminal metadata and failure diagnostics."
              : "Data-minimized capability admission metadata recorded before Provider activation side effects.")}
          </DialogDescription>
        </DialogHeader>

        {selection !== null && record !== undefined ? (
          <div className="flex flex-col gap-4">
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl bg-background/70 px-4 py-3">
              <AuditStatusBadge status={record.status} />
              <time dateTime={record.finishedAt} className="text-xs tabular-nums text-muted-foreground">
                {formatDateTime(record.finishedAt)}
              </time>
            </div>

            <FocusSwitch
              label={isInvocation ? "Invocation details" : "Provider activation details"}
              value={section}
              onChange={onSectionChange}
              items={[
                { value: "overview", label: "Overview" },
                { value: "governance", label: "Governance" },
                ...(failed ? [{ value: "diagnostics" as const, label: "Diagnostics" }] : []),
              ]}
            />

            {section === "overview" ? (
              <dl className="grid min-w-0 gap-3 sm:grid-cols-2">
                <AuditField
                  label={isInvocation ? t("Invocation ID") : t("Activation ID")}
                  value={isInvocation ? selection.record.invocationId : selection.record.activationId}
                  mono
                />
                <AuditField
                  label={isInvocation ? "Tool ID" : "Provider ID"}
                  value={isInvocation ? selection.record.toolId : selection.record.providerId}
                  mono
                />
                {isInvocation && selection.record.providerId ? (
                  <AuditField label="Provider ID" value={selection.record.providerId} mono />
                ) : null}
                <AuditField label="Principal" value={principalLabel(record.principalKind, record.principalRef)} mono />
                <AuditField label="Origin" value={record.origin || "—"} />
                <AuditField label="Started" value={formatDateTime(record.startedAt)} />
                <AuditField label="Finished" value={formatDateTime(record.finishedAt)} />
                {isInvocation ? (
                  <>
                    <AuditField label="Outcome" value={selection.record.outcomeKind || "—"} />
                    <AuditField label="Result kind" value={selection.record.resultKind || "—"} />
                  </>
                ) : (
                  <AuditField label="Action" value={selection.record.action} />
                )}
              </dl>
            ) : section === "governance" ? (
              <div className="flex flex-col gap-3">
                <dl className="grid min-w-0 gap-3 sm:grid-cols-2">
                  <AuditField label="Policy" value={record.policyKind || "—"} />
                  <AuditField label="Approval" value={record.approval || "—"} />
                  <AuditField label="Scope" value={record.scopeKind || "—"} />
                  <AuditField label="Scope reference" value={record.scopeReference || "—"} mono />
                </dl>
                {record.policyReason ? (
                  <div className="rounded-xl border border-border/70 bg-background/65 p-4">
                    <p className="text-xs font-medium text-muted-foreground">{t("Policy reason")}</p>
                    <p className="mt-1 whitespace-pre-wrap break-words text-sm leading-6">{record.policyReason}</p>
                  </div>
                ) : null}
              </div>
            ) : failed ? (
              <div className="flex flex-col gap-4">
                <div className="rounded-2xl border border-destructive/15 bg-destructive/5 p-5">
                  <p className="text-sm font-semibold">{t("Error summary")}</p>
                  <p className="mt-2 whitespace-pre-wrap break-words text-sm leading-6 text-foreground">
                    {record.errorSummary ||
                      t(isInvocation
                        ? "No diagnostic summary was retained for this record. Older Audit records and failures without a safe upstream summary only contain the error class."
                        : "No diagnostic summary was retained for this record.")}
                  </p>
                </div>
                <dl className="grid min-w-0 gap-3 sm:grid-cols-2">
                  <AuditField label="Error class" value={record.errorClass || "—"} />
                  {isInvocation ? (
                    <>
                      <AuditField label="Failure source" value={selection.record.failureSource || "—"} />
                      <AuditField label="Failure code" value={selection.record.failureCode || "—"} mono />
                      <AuditField label="Rejection code" value={selection.record.rejectionCode || "—"} mono />
                    </>
                  ) : null}
                </dl>
              </div>
            ) : null}
          </div>
        ) : null}

        <DialogFooter>
          <DialogClose render={<Button variant="outline" />}>
            {t("Close")}
          </DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
