import { useEffect, useState } from "react";
import { ActivityIcon, BoxesIcon, ShieldCheckIcon, WorkflowIcon } from "lucide-react";

import type {
  ManagementClient,
  RuntimeBudget,
  RuntimeGovernanceStatus,
  RuntimeIsolation,
} from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import { Input } from "@mender/ui/components/input";
import { Dialog, DialogBody, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@mender/ui/components/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@mender/ui/components/select";

import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { ErrorNotice } from "../../shared/page";
import { DataPanel } from "../../shared/workbench";
import { FormInfo } from "../../shared/form-info";

const budgetFields: Array<{
  key: keyof RuntimeBudget;
  label: string;
  min: number;
  max: number;
}> = [
  { key: "maxInvocations", label: "Global invocations", min: 1, max: 4096 },
  { key: "maxPrincipalInvocations", label: "Per Principal", min: 1, max: 4096 },
  { key: "maxProviderInvocations", label: "Per Provider", min: 1, max: 4096 },
  { key: "maxToolInvocations", label: "Per Tool", min: 1, max: 4096 },
  { key: "maxProviderActivations", label: "Provider activations", min: 1, max: 4096 },
  { key: "maxExecutionTimeoutMs", label: "Max timeout (ms)", min: 1, max: 86_400_000 },
  { key: "maxArgumentBytes", label: "Max argument bytes", min: 1, max: 67_108_864 },
  { key: "maxResultBytes", label: "Max result bytes", min: 1, max: 67_108_864 },
];

function budgetDescription(key: keyof RuntimeBudget): string {
  switch (key) {
    case "maxInvocations":
      return t("Maximum in-flight Tool invocations.");
    case "maxPrincipalInvocations":
      return t("Maximum in-flight invocations for one Principal.");
    case "maxProviderInvocations":
      return t("Maximum in-flight invocations for one Provider.");
    case "maxToolInvocations":
      return t("Maximum in-flight invocations for one Tool.");
    case "maxProviderActivations":
      return t("Maximum concurrent Provider activation operations.");
    case "maxExecutionTimeoutMs":
      return t("Hard upper bound for Tool execution time.");
    case "maxArgumentBytes":
      return t("Maximum serialized Tool argument size.");
    case "maxResultBytes":
      return t("Maximum canonical Tool result size.");
  }
}

type BudgetDraft = Record<keyof RuntimeBudget, string>;

function budgetToDraft(budget: RuntimeBudget): BudgetDraft {
  const draft = {} as BudgetDraft;
  for (const field of budgetFields) draft[field.key] = String(budget[field.key]);
  return draft;
}

function validBudgetEntry(raw: string | undefined, field: (typeof budgetFields)[number]): boolean {
  if (raw === undefined || !/^\d+$/.test(raw.trim())) return false;
  const value = Number(raw);
  return Number.isSafeInteger(value) && value >= field.min && value <= field.max;
}

function budgetFromDraft(draft: BudgetDraft | null): RuntimeBudget | null {
  if (!draft) return null;
  const next = {} as RuntimeBudget;
  for (const field of budgetFields) {
    if (!validBudgetEntry(draft[field.key], field)) return null;
    next[field.key] = Number(draft[field.key]);
  }
  return next;
}

export function RuntimePage({ client }: { client: ManagementClient }) {
  const [status, setStatus] = useState<RuntimeGovernanceStatus | null>(null);
  const [budget, setBudget] = useState<RuntimeBudget | null>(null);
  const [budgetDraft, setBudgetDraft] = useState<BudgetDraft | null>(null);
  const [isolation, setIsolation] = useState<RuntimeIsolation | null>(null);
  const [error, setError] = useState("");
  const [reloadVersion, setReloadVersion] = useState(0);
  const [busy, setBusy] = useState("");
  const [editor, setEditor] = useState<"budget" | "isolation" | null>(null);
  const [acknowledgeExternalHost, setAcknowledgeExternalHost] = useState(false);

  useEffect(() => {
    let active = true;
    void Promise.resolve()
      .then(() => client.getRuntimeGovernance())
      .then((next) => {
        if (!active) return;
        setStatus(next);
        setBudget(next.budget);
        setIsolation(next.isolation);
        setError("");
      })
      .catch((reason: unknown) => {
        if (active) setError(errorMessage(reason));
      });
    return () => {
      active = false;
    };
  }, [client, reloadVersion]);

  async function saveBudget() {
    const candidate = budgetFromDraft(budgetDraft);
    if (!candidate || busy) return;
    setBusy("budget");
    setError("");
    try {
      const next = await client.updateRuntimeBudget(candidate);
      setStatus(next);
      setBudget(next.budget);
      setIsolation(next.isolation);
      setBudgetDraft(null);
      setEditor(null);
    } catch (reason) {
      // Leave the draft and the form open so users can retry without re-entering limits.
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function saveIsolation() {
    if (isolation === null || busy || (externalHostEscalation && !acknowledgeExternalHost)) return;
    setBusy("isolation");
    setError("");
    try {
      const next = await client.updateRuntimeIsolation(isolation);
      setStatus(next);
      setBudget(next.budget);
      setIsolation(next.isolation);
      setError("");
      setEditor(null);
      setAcknowledgeExternalHost(false);
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  const externalHostEscalation = status?.isolation.externalProvider === "restricted_host" && isolation?.externalProvider === "host";
  const restrictedAvailable = status?.backend.restrictedHostAvailable ?? false;
  const parsedBudget = budgetFromDraft(budgetDraft);
  function closeEditor() {
    if (busy) return;
    setEditor(null);
    setBudgetDraft(null);
    setIsolation(status?.isolation ?? null);
    setAcknowledgeExternalHost(false);
    setError("");
  }

  if (status === null) {
    return <div className="pt-4">
      {error
        ? <ErrorNotice message={error} onRetry={() => {setError("");setReloadVersion(version=>version+1);}} />
        : <p role="status" className="py-10 text-center text-sm text-muted-foreground">{t("Loading…")}</p>}
    </div>;
  }

  return (
    <div className="space-y-4">
      {error && !editor ? <ErrorNotice message={error} /> : null}
      <div className="flex flex-wrap items-center justify-between gap-3 pb-1">
        <div>
          <p className="flex items-center gap-2 text-[12px] font-semibold tracking-[0.045em] text-muted-foreground uppercase">
            {t("Runtime")}
            <span className="size-1.5 rounded-full bg-teal" aria-hidden="true" />
            <span className="font-normal normal-case tracking-normal">{t("Local control plane")}</span>
          </p>
          <p className="mt-1.5 text-[13px] text-muted-foreground">{t("Current execution capacity, limits and process authority.")}</p>
        </div>
        <Button variant="outline" size="sm" onClick={() => setReloadVersion((n) => n + 1)}>
          {t("Refresh")}
        </Button>
      </div>

      <section aria-label={t("Runtime summary")} className="grid overflow-hidden rounded-xl border border-border bg-card sm:grid-cols-2 xl:grid-cols-4">
        {([
          { icon: ActivityIcon, label: "In-flight invocations", value: String(status.usage.invocations ?? 0) },
          { icon: WorkflowIcon, label: "In-flight activations", value: String(status.usage.activations ?? 0) },
          { icon: BoxesIcon, label: "Isolation backend", value: status.backend.name ?? "—" },
          { icon: ShieldCheckIcon, label: "Restricted host", value: t(restrictedAvailable ? "Available" : "Unavailable") },
        ] as const).map(({ icon: Icon, label, value }, i) => (
          <div key={label} className={[
            "min-w-0 px-5 py-4",
            i > 0 ? "border-t border-border sm:border-t-0" : "",
            i % 2 === 1 ? "sm:border-l" : "",
            i >= 2 ? "sm:border-t xl:border-t-0" : "",
            i > 0 ? "xl:border-l" : "",
          ].join(" ")}>
            <p className="flex items-center gap-2 text-[12px] text-muted-foreground">
              <Icon className="size-4 shrink-0" aria-hidden="true" />
              {t(label)}
            </p>
            <p className="mt-2 truncate text-[22px] leading-7 font-semibold tracking-[-0.02em]" title={value}>{value}</p>
          </div>
        ))}
      </section>

      <div className="grid min-w-0 items-start gap-4 xl:grid-cols-2">
        <DataPanel title="Runtime budget"
          description="Hard limits apply after governance admission. Profile constraints may only tighten them."
          actions={<Button size="sm" variant="outline" disabled={!budget} onClick={() => {
            if (budget) { setBudgetDraft(budgetToDraft(budget)); setError(""); setEditor("budget"); }
          }}>{t("Edit")}</Button>}>
          <dl className="grid grid-cols-2 gap-x-5 gap-y-0 p-4 sm:grid-cols-4 xl:grid-cols-2 2xl:grid-cols-4">
            {budgetFields.map((field) => (
              <div key={field.key} className="min-w-0 border-b border-border/75 px-1 py-3">
                <dt className="truncate text-[11px] text-muted-foreground" title={t(field.label)}>{t(field.label)}</dt>
                <dd className="mt-1.5 truncate text-[17px] font-semibold tabular-nums" title={String(budget?.[field.key] ?? "—")}>
                  {budget?.[field.key] ?? "—"}
                </dd>
              </div>
            ))}
          </dl>
        </DataPanel>

        <DataPanel title="Process isolation"
          description="Isolation changes host filesystem authority; it does not change Policy decisions."
          actions={<Button size="sm" variant="outline" disabled={!isolation} onClick={() => {
            setError(""); setAcknowledgeExternalHost(false); setEditor("isolation");
          }}>{t("Edit")}</Button>}>
          <dl className="divide-y divide-border/75 px-5">
            {(["workspaceShell", "externalProvider", "tunnelClient"] as const).map((key, i) => (
              <div key={key} className="flex min-w-0 items-center justify-between gap-3 py-4">
                <dt className="text-[13px]">{t(["Workspace shell", "External Providers", "Tunnel client"][i]!)}</dt>
                <dd className="shrink-0 text-[12px] font-medium text-foreground">
                  {t(isolation?.[key] === "host" ? "Host" : "Restricted host")}
                </dd>
              </div>
            ))}
          </dl>
          <div className="border-t border-border px-5 py-3 text-[12px] leading-5 text-muted-foreground">
            {isolation?.externalProvider === "host"
              ? <p role="status" className="mb-1">{t("External native Providers use the Runtime user's full host process authority in Host mode. Install only trusted plugins; restricted_host limits filesystem access but not network egress.")}</p>
              : null}
            {t(restrictedAvailable
              ? "restricted_host provides host containment, not a VM or container. Network access remains available in this phase."
              : "restricted_host is unavailable on this Runtime. Host mode keeps the local process authority of the Runtime user.")}
          </div>
        </DataPanel>
      </div>
      <Dialog open={editor === "budget"} onOpenChange={(open) => { if (!open) closeEditor(); }}>
        <DialogContent className="grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t("Runtime budget")}</DialogTitle>
            <DialogDescription>{t("Hard limits apply after governance admission. Profile constraints may only tighten them.")}</DialogDescription>
            {error ? <p role="alert" className="text-xs leading-5 text-destructive">{error}</p> : null}
          </DialogHeader>
          <DialogBody className="grid gap-4 sm:grid-cols-2">
            {budgetFields.map((field) => {
              const id = `runtime-budget-${field.key}`;
              const invalid = budgetDraft !== null && !validBudgetEntry(budgetDraft[field.key], field);
              return <div key={field.key} className="grid min-w-0 content-start gap-1.5 text-sm">
                <div className="flex items-center justify-between gap-2">
                  <div className="flex min-w-0 items-center gap-1">
                    <label htmlFor={id} className="font-medium">{t(field.label)}</label>
                    <FormInfo label={t(field.label)}>{budgetDescription(field.key)}</FormInfo>
                  </div>
                  <span id={`${id}-range`} className="shrink-0 text-xs tabular-nums text-muted-foreground">
                    <span className="sr-only">{t("Allowed range: {{min}} to {{max}}", { min: field.min, max: field.max })}</span>
                    <span aria-hidden="true">{field.min}–{field.max}</span>
                  </span>
                </div>
                <Input id={id} type="number" inputMode="numeric" step="1" min={field.min} max={field.max}
                  aria-invalid={invalid}
                  aria-describedby={`${id}-range${invalid ? ` ${id}-error` : ""}`}
                  value={budgetDraft?.[field.key] ?? ""}
                  onChange={(event) => {
                    setBudgetDraft((current) => current ? { ...current, [field.key]: event.target.value } : null);
                    setError("");
                  }} />
                {invalid ? <span id={`${id}-error`} role="alert" className="text-xs text-destructive">
                  {t("Enter a whole number from {{min}} to {{max}}.", { min: field.min, max: field.max })}
                </span> : null}
              </div>;
            })}
          </DialogBody>
          <DialogFooter>
            <Button variant="outline" disabled={busy !== ""} onClick={closeEditor}>{t("Cancel")}</Button>
            <Button disabled={busy !== "" || !parsedBudget} onClick={() => void saveBudget()}>{busy === "budget" ? t("Saving…") : t("Save budget")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={editor === "isolation"} onOpenChange={(open) => { if (!open) closeEditor(); }}>
        <DialogContent className="grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{t("Process isolation")}</DialogTitle>
            <DialogDescription>{t("Isolation changes host filesystem authority; it does not change Policy decisions.")}</DialogDescription>
            {error ? <p role="alert" className="text-xs leading-5 text-destructive">{error}</p> : null}
          </DialogHeader>
          <DialogBody className="grid gap-5">
            {(["workspaceShell", "externalProvider", "tunnelClient"] as const).map((key, i) => <IsolationField key={key}
              label={[t("Workspace shell"), t("External Providers"), t("Tunnel client")][i]!}
              value={isolation?.[key] ?? "host"} restrictedAvailable={restrictedAvailable}
              onChange={(value) => { setError(""); setAcknowledgeExternalHost(false); setIsolation((current) => current === null ? null : { ...current, [key]: value }); }} />)}
            {externalHostEscalation ? <label className="flex items-start gap-2 rounded-lg border border-border p-3 text-xs leading-5">
              <input type="checkbox" className="mt-1 size-4 accent-foreground" checked={acknowledgeExternalHost} onChange={(event) => setAcknowledgeExternalHost(event.target.checked)} />
              <span>{t("I trust the installed external native plugins and accept unrestricted Host process access.")}</span>
            </label> : null}
          </DialogBody>
          <DialogFooter>
            <Button variant="outline" disabled={busy !== ""} onClick={closeEditor}>{t("Cancel")}</Button>
            <Button disabled={busy !== "" || (externalHostEscalation && !acknowledgeExternalHost)} onClick={() => void saveIsolation()}>{busy === "isolation" ? t("Saving…") : t("Save isolation")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

    </div>
  );
}

function IsolationField({
  label,
  value,
  restrictedAvailable,
  onChange,
}: {
  label: string;
  value: RuntimeIsolation["workspaceShell"];
  restrictedAvailable: boolean;
  onChange: (value: RuntimeIsolation["workspaceShell"]) => void;
}) {
  return (
    <div className="space-y-2">
      <p className="text-xs font-medium">{t(label)}</p>
      <Select value={value} onValueChange={(next) => onChange(next as RuntimeIsolation["workspaceShell"])}>
        <SelectTrigger aria-label={t(label)}>
          <SelectValue>{t(value === "host" ? "Host" : "Restricted host")}</SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="host">{t("Host")}</SelectItem>
          <SelectItem value="restricted_host" disabled={!restrictedAvailable}>{t("Restricted host")}</SelectItem>
        </SelectContent>
      </Select>
      <p className="text-xs leading-5 text-muted-foreground">
        {value === "host"
          ? t("Host mode uses the unrestricted local process authority of the Runtime user.")
          : t("Restricted host confines host filesystem access but is not VM/container isolation.")}
      </p>
    </div>
  );
}
