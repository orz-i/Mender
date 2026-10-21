import { useEffect, useState } from "react";
import { ActivityIcon, BoxesIcon, ShieldCheckIcon, WorkflowIcon } from "lucide-react";

import type {
  MenderClient,
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
import { FocusSwitch } from "../../shared/focus-switch";
import { PageToolbar } from "../../shared/page-toolbar";
import { DataPanel, MetricStrip, MetricTile } from "../../shared/workbench";
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

export function RuntimePage({ client }: { client: MenderClient }) {
  const [status, setStatus] = useState<RuntimeGovernanceStatus | null>(null);
  const [budget, setBudget] = useState<RuntimeBudget | null>(null);
  const [budgetDraft, setBudgetDraft] = useState<BudgetDraft | null>(null);
  const [isolation, setIsolation] = useState<RuntimeIsolation | null>(null);
  const [error, setError] = useState("");
  const [reloadVersion, setReloadVersion] = useState(0);
  const [busy, setBusy] = useState("");
  const [editor, setEditor] = useState<"budget" | "isolation" | null>(null);
  const [focus, setFocus] = useState<"status" | "budget" | "isolation">("status");
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
      <PageToolbar
        tabs={
          <FocusSwitch
            label="Runtime governance"
            value={focus}
            onChange={setFocus}
            items={[
              { value: "status", label: "Runtime" },
              { value: "budget", label: "Runtime budget" },
              { value: "isolation", label: "Process isolation" },
            ]}
          />
        }
        actions={
          focus === "budget" ? (
            <Button size="sm" variant="outline" disabled={budget === null} onClick={() => { if (budget) { setBudgetDraft(budgetToDraft(budget)); setError(""); setEditor("budget"); } }}>{t("Edit")}</Button>
          ) : focus === "isolation" ? (
            <Button size="sm" variant="outline" disabled={isolation === null} onClick={() => { setError(""); setAcknowledgeExternalHost(false); setEditor("isolation"); }}>{t("Edit")}</Button>
          ) : null
        }
      />

      {focus === "status" ? <MetricStrip>
        <MetricTile icon={ActivityIcon} value={status?.usage.invocations ?? "—"} label="In-flight invocations" />
        <MetricTile icon={WorkflowIcon} value={status?.usage.activations ?? "—"} label="In-flight activations" />
        <MetricTile icon={BoxesIcon} value={status?.backend.name ?? "—"} label="Isolation backend" />
        <MetricTile
          icon={ShieldCheckIcon}
          value={restrictedAvailable ? t("Available") : t("Unavailable")}
          label="Restricted host"
        />
      </MetricStrip> : focus === "budget" ? (
        <DataPanel title="Runtime budget" description="Hard limits apply after governance admission. Profile constraints may only tighten them.">
          <div className="grid gap-x-6 gap-y-5 p-5 sm:grid-cols-2">
            {budgetFields.map((field) => <div key={field.key} className="min-w-0 border-b border-border/60 pb-3">
              <p className="truncate text-xs text-muted-foreground">{t(field.label)}</p>
              <p className="mt-1 text-xl font-semibold tabular-nums">{budget?.[field.key] ?? "—"}</p>
            </div>)}
          </div>
        </DataPanel>
      ) : (
        <DataPanel title="Process isolation" description="Isolation changes host filesystem authority; it does not change Policy decisions.">
          <div className="divide-y divide-border/70 px-5 pb-3">
            {(["workspaceShell", "externalProvider", "tunnelClient"] as const).map((key, i) => (
              <div key={key} className="flex items-center justify-between gap-4 py-4">
                <span className="text-sm">{[t("Workspace shell"), t("External Providers"), t("Tunnel client")][i]!}</span>
                <span className="rounded-full bg-muted px-3 py-1 text-xs font-medium">{t((isolation?.[key] ?? "host") === "host" ? "Host" : "Restricted host")}</span>
              </div>
            ))}
            {isolation?.externalProvider === "host" ? <p role="status" className="pt-4 text-xs leading-5 text-muted-foreground">{t("External native Providers use the Runtime user's full host process authority in Host mode. Install only trusted plugins; restricted_host limits filesystem access but not network egress.")}</p> : null}
            <p className="pt-4 text-xs leading-5 text-muted-foreground">{restrictedAvailable
              ? t("restricted_host provides host containment, not a VM or container. Network access remains available in this phase.")
              : t("restricted_host is unavailable on this Runtime. Host mode keeps the local process authority of the Runtime user.")}</p>
          </div>
        </DataPanel>
      )}
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
