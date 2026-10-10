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
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@mender/ui/components/dialog";
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
import { DataPanel, MetricStrip, MetricTile } from "../../shared/workbench";

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

export function RuntimePage({ client }: { client: MenderClient }) {
  const [status, setStatus] = useState<RuntimeGovernanceStatus | null>(null);
  const [budget, setBudget] = useState<RuntimeBudget | null>(null);
  const [isolation, setIsolation] = useState<RuntimeIsolation | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [editor, setEditor] = useState<"budget" | "isolation" | null>(null);
  const [focus, setFocus] = useState<"status" | "budget" | "isolation">("status");

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
  }, [client]);

  async function saveBudget() {
    if (budget === null) return;
    setBusy("budget");
    try {
      const next = await client.updateRuntimeBudget(budget);
      setStatus(next);
      setBudget(next.budget);
      setIsolation(next.isolation);
      setError("");
      setEditor(null);
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function saveIsolation() {
    if (isolation === null) return;
    setBusy("isolation");
    try {
      const next = await client.updateRuntimeIsolation(isolation);
      setStatus(next);
      setBudget(next.budget);
      setIsolation(next.isolation);
      setError("");
      setEditor(null);
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  const restrictedAvailable = status?.backend.restrictedHostAvailable ?? false;

  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">{t("Runtime governance")}</h1>
        <p className="text-sm text-muted-foreground">
          {t("Bound Runtime capacity and local process authority independently.")}
        </p>
      </div>
      {error ? <ErrorNotice message={error} /> : null}
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
        <DataPanel title="Runtime budget" description="Hard limits apply after governance admission. Profile constraints may only tighten them."
          actions={<Button variant="outline" disabled={budget === null} onClick={() => setEditor("budget")}>{t("Edit")}</Button>}>
          <div className="grid gap-x-6 gap-y-5 p-5 sm:grid-cols-2">
            {budgetFields.map((field) => <div key={field.key} className="min-w-0 border-b border-border/60 pb-3">
              <p className="truncate text-xs text-muted-foreground">{t(field.label)}</p>
              <p className="mt-1 text-xl font-semibold tabular-nums">{budget?.[field.key] ?? "—"}</p>
            </div>)}
          </div>
        </DataPanel>
      ) : (
        <DataPanel title="Process isolation" description="Isolation changes host filesystem authority; it does not change Policy decisions."
          actions={<Button variant="outline" disabled={isolation === null} onClick={() => setEditor("isolation")}>{t("Edit")}</Button>}>
          <div className="divide-y divide-border/70 px-5 pb-3">
            {(["workspaceShell", "externalProvider", "tunnelClient"] as const).map((key, i) => (
              <div key={key} className="flex items-center justify-between gap-4 py-4">
                <span className="text-sm">{[t("Workspace shell"), t("External Providers"), t("Tunnel client")][i]!}</span>
                <span className="rounded-full bg-muted px-3 py-1 text-xs font-medium">{t((isolation?.[key] ?? "host") === "host" ? "Host" : "Restricted host")}</span>
              </div>
            ))}
            <p className="pt-4 text-xs leading-5 text-muted-foreground">{restrictedAvailable
              ? t("restricted_host provides host containment, not a VM or container. Network access remains available in this phase.")
              : t("restricted_host is unavailable on this Runtime. Host mode keeps the local process authority of the Runtime user.")}</p>
          </div>
        </DataPanel>
      )}
      <Dialog open={editor === "budget"} onOpenChange={(open) => { if (!open && !busy) { setEditor(null); setBudget(status?.budget ?? null); } }}>
        <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader><DialogTitle>{t("Runtime budget")}</DialogTitle><DialogDescription>{t("Hard limits apply after governance admission. Profile constraints may only tighten them.")}</DialogDescription></DialogHeader>
          <div className="grid gap-4 sm:grid-cols-2">
            {budgetFields.map((field) => <label key={field.key} className="grid gap-1.5 text-sm">
              <span className="font-medium">{t(field.label)}</span>
              <Input type="number" aria-label={t(field.label)} min={field.min} max={field.max} value={budget?.[field.key] ?? ""}
                onChange={(event) => setBudget((current) => current === null ? null : { ...current, [field.key]: Number(event.target.value) })} />
              <span className="text-xs text-muted-foreground">{budgetDescription(field.key)}</span>
            </label>)}
          </div>
          <DialogFooter><Button variant="outline" disabled={busy !== ""} onClick={() => { setBudget(status?.budget ?? null); setEditor(null); }}>{t("Cancel")}</Button><Button disabled={busy !== ""} onClick={() => void saveBudget()}>{busy === "budget" ? t("Saving…") : t("Save budget")}</Button></DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={editor === "isolation"} onOpenChange={(open) => { if (!open && !busy) { setEditor(null); setIsolation(status?.isolation ?? null); } }}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader><DialogTitle>{t("Process isolation")}</DialogTitle><DialogDescription>{t("Isolation changes host filesystem authority; it does not change Policy decisions.")}</DialogDescription></DialogHeader>
          <div className="grid gap-5">
            {(["workspaceShell", "externalProvider", "tunnelClient"] as const).map((key, i) => <IsolationField key={key}
              label={[t("Workspace shell"), t("External Providers"), t("Tunnel client")][i]!}
              value={isolation?.[key] ?? "host"} restrictedAvailable={restrictedAvailable}
              onChange={(value) => setIsolation((current) => current === null ? null : { ...current, [key]: value })} />)}
          </div>
          <DialogFooter><Button variant="outline" disabled={busy !== ""} onClick={() => { setIsolation(status?.isolation ?? null); setEditor(null); }}>{t("Cancel")}</Button><Button disabled={busy !== ""} onClick={() => void saveIsolation()}>{busy === "isolation" ? t("Saving…") : t("Save isolation")}</Button></DialogFooter>
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
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="host">{t("Host")}</SelectItem>
          <SelectItem value="restricted_host" disabled={!restrictedAvailable}>{t("Restricted host")}</SelectItem>
        </SelectContent>
      </Select>
      <p className="text-[11px] text-muted-foreground">
        {value === "host"
          ? t("Host mode uses the unrestricted local process authority of the Runtime user.")
          : t("Restricted host confines host filesystem access but is not VM/container isolation.")}
      </p>
    </div>
  );
}
