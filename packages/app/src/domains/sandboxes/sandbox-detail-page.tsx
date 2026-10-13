import { useEffect, useState } from "react";
import { ActivityIcon, ShieldCheckIcon } from "lucide-react";
import { useNavigate, useParams } from "react-router";

import { MenderApiError, type MenderClient, type Sandbox } from "@mender/sdk";
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

import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { FocusSwitch } from "../../shared/focus-switch";
import { ErrorNotice } from "../../shared/page";
import { PageToolbar } from "../../shared/page-toolbar";
import { DataPanel } from "../../shared/workbench";
import { isolationLabel, sandboxBackend, sandboxStateLabel } from "./sandbox-display";
import { SandboxMetadata } from "./sandbox-metadata";
import { SandboxAccessPanel } from "./sandbox-access-panel";
import { SandboxToolsPanel } from "./sandbox-tools-panel";

type DetailTopic = "overview" | "security" | "access" | "tools";
type LifecycleAction = "start" | "stop" | "destroy";

export function SandboxDetailPage({ client }: { client: MenderClient }) {
  const { sandboxId } = useParams();
  const navigate = useNavigate();
  const [item, setItem] = useState<Sandbox | null>(null);
  const [error, setError] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [topic, setTopic] = useState<DetailTopic>("overview");
  const [manageOpen, setManageOpen] = useState(false);
  const [pendingAction, setPendingAction] = useState<"destroy" | "docker-restart" | null>(null);

  useEffect(() => {
    if (!sandboxId) return;
    let active = true;
    void client.getSandbox(sandboxId)
      .then((sandbox) => {
        if (!active) return;
        setItem(sandbox);
        setLoaded(true);
        setError("");
      })
      .catch((reason: unknown) => {
        if (!active) return;
        setError(errorMessage(reason));
        setLoaded(true);
      });
    return () => { active = false; };
  }, [client, sandboxId]);

  const manageable = Boolean(
    item?.backendAvailable &&
      (item.backend === "local_workspace" || item.backend === "docker" || item.backend === "daytona" || item.backend === "ssh_remote" || item.backend === "external"),
  );
  const noActiveLeases = item?.activeLeases === 0;
  const canStart = manageable && item?.state === "stopped";
  const canStop = manageable && item?.state === "ready";
  const canDestroy = manageable && (item?.state === "ready" || item?.state === "stopped");
  const canReconcile = manageable && (item?.backend === "docker" || item?.backend === "daytona" || item?.backend === "external") &&
    (item?.state === "unknown" || item?.state === "orphaned");
  const hasActions = canStart || canStop || canDestroy || canReconcile;

  function requestLifecycle(action: LifecycleAction) {
    if (!item || busy || !noActiveLeases) return;
    if (action === "destroy" && canDestroy) {
      setError("");
      setPendingAction("destroy");
      return;
    }
    if (action === "start" && canStart && item.backend === "docker") {
      setError("");
      setPendingAction("docker-restart");
      return;
    }
    if ((action === "start" && canStart) || (action === "stop" && canStop)) {
      void changeLifecycle(action);
    }
  }

  async function changeLifecycle(action: LifecycleAction) {
    if (!item || busy || !noActiveLeases) return;
    // Confirmation lives in the existing management modal. No native JS
    // confirm() dependency (WKWebView does not implement its delegate).
    if (action === "destroy" && (!canDestroy || pendingAction !== "destroy")) return;
    if (action === "start" && item.backend === "docker" &&
      (!canStart || pendingAction !== "docker-restart")) return;

    setBusy(true);
    setError("");
    try {
      const next = action === "start"
        ? await client.startSandbox(item.sandboxId)
        : action === "stop"
          ? await client.stopSandbox(item.sandboxId)
          : await client.destroySandbox(item.sandboxId);
      setItem(next);
      setPendingAction(null);
      if (action === "destroy") setManageOpen(false);
    } catch (reason) {
      setError(sandboxLifecycleError(reason));
    } finally {
      setBusy(false);
    }
  }

  async function reconcile() {
    if (!item || busy || !noActiveLeases || !canReconcile) return;
    setBusy(true);
    setError("");
    try {
      const next = item.backend === "daytona"
        ? await client.reconcileDaytonaSandbox(item.sandboxId)
        : await client.reconcileSandbox(item.sandboxId);
      setItem(next);
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <PageToolbar
        tabs={item ? (
          <FocusSwitch
            label="Sandbox overview"
            value={topic}
            onChange={setTopic}
            items={[
              { value: "overview", label: "Sandbox overview" },
              { value: "security", label: "Security boundary" },
              { value: "access", label: "Profile access" },
              { value: "tools", label: "Tools" },
            ]}
          />
        ) : undefined}
        actions={
          <div className="flex flex-wrap justify-end gap-2">
            <Button variant="outline" onClick={() => void navigate("/sandboxes")}>
              {t("Back to sandboxes")}
            </Button>
            {item ? (
              <Button variant="outline" onClick={() => void navigate(`/sandboxes/${item.sandboxId}/activity`)}>
                <ActivityIcon data-icon="inline-start" />
                {t("Activity")}
              </Button>
            ) : null}
            {hasActions ? (
              <Button onClick={() => setManageOpen(true)}>{t("Manage sandbox")}</Button>
            ) : null}
          </div>
        }
      />

      {(error && !manageOpen) || !sandboxId
        ? <ErrorNotice message={error || t("The requested item was not found.")} />
        : null}

      {sandboxId && !loaded
        ? <p className="text-sm text-muted-foreground">{t("Loading Sandboxes…")}</p>
        : item ? (
          <div className="flex flex-col gap-4">
            {topic === "overview" ? (
              <DataPanel title="Sandbox overview" description={item.name}>
                <dl className="grid gap-x-8 gap-y-4 p-5 sm:grid-cols-2">
                  <SandboxMetadata name={t("Sandbox ID")} value={item.sandboxId} />
                  <SandboxMetadata name={t("Backend")} value={sandboxBackend(item)} />
                  <SandboxMetadata name={t("Isolation")} value={isolationLabel(item.isolation)} />
                  <SandboxMetadata name={t("State")} value={sandboxStateLabel(item.state)} />
                  <SandboxMetadata name={t("Workspace")} value={item.localRoot || "—"} />
                  <SandboxMetadata name={t("Owner type")} value={item.ownerKind} />
                  <SandboxMetadata name={t("Active execution leases")} value={String(item.activeLeases)} />
                  <SandboxMetadata name={t("Backend status")} value={t(item.backendAvailable ? "Available" : "Unavailable")} />
                </dl>
                {item.activeLeases > 0 ? (
                  <p role="status" className="border-t border-border px-5 py-3 text-xs text-muted-foreground">
                    {t("Cannot change lifecycle while a Sandbox execution lease is active.")}
                  </p>
                ) : null}
                {item.state === "destroyed" ? (
                  <p role="status" className="border-t border-border px-5 py-3 text-xs text-muted-foreground">
                    {t("Host files remain untouched after Sandbox destruction.")}
                  </p>
                ) : null}
                <p className="px-5 pb-4 text-xs text-muted-foreground">
                  {t("State is last observed; unsupported lifecycle operations are not available.")}
                </p>
              </DataPanel>
            ) : topic === "access" ? (
              <SandboxAccessPanel key={item.sandboxId} client={client} sandbox={item} />
            ) : topic === "tools" ? (
              <SandboxToolsPanel key={item.sandboxId} client={client} sandbox={item}/>
            ) : (
              <DataPanel title="Security boundary" description="Isolation depends on the actual backend, not the Sandbox name.">
                <div className="flex flex-col gap-4 p-5">
                  <span className="flex size-11 items-center justify-center rounded-xl bg-foreground text-background dark:bg-muted dark:text-foreground">
                    <ShieldCheckIcon className="size-5" aria-hidden="true" />
                  </span>
                  <p className="max-w-2xl text-sm leading-6 text-muted-foreground">
                    {t(item.isolation === "host"
                      ? "Host mode is not isolated and uses the Runtime user's local file and process permissions."
                      : item.isolation === "restricted_host"
                        ? "Restricted host confines local processes but is not a container or VM."
                        : "Isolation guarantees depend on the selected backend and must be verified independently.")}
                  </p>
                  {item.backend === "ssh_remote" ? (
                    <p className="text-xs leading-5 text-muted-foreground">
                      {t("SSH Remote is not container or VM isolation. The remote shell can access everything permitted to its login account. Connect only to a trusted host; prefer a non-root account.")}
                    </p>
                  ) : item.backend === "daytona" ? (
                    <p className="text-xs leading-5 text-muted-foreground">
                      {t("Remote Sandbox incurs Daytona account charges. TTL is a wall-clock ceiling, not a billing cap.")}
                    </p>
                  ) : item.backend === "docker" ? (
                    <p className="text-xs leading-5 text-muted-foreground">
                      {t("Docker Workspace scratch is ephemeral. Stop/Start recreates the container; destruction never removes host files.")}
                    </p>
                  ) : null}
                </div>
              </DataPanel>
            )}
          </div>
        ) : null}

      <Dialog open={manageOpen && hasActions} onOpenChange={(open) => {
        if (!busy) {
          setManageOpen(open);
          if (!open) setPendingAction(null);
        }
      }}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t(pendingAction === "destroy"
              ? "Confirm sandbox destruction"
              : pendingAction === "docker-restart"
                ? "Confirm Docker sandbox restart"
                : "Manage sandbox")}</DialogTitle>
            <DialogDescription>{item?.name ?? ""}</DialogDescription>
          </DialogHeader>
          {error ? <ErrorNotice message={error} /> : null}
          {item && pendingAction ? (
            <div className="space-y-3 text-sm">
              <p>{pendingAction === "destroy"
                ? t('Destroy Sandbox "{{name}}"? This only tombstones the Sandbox; host files are never deleted.', { name: item.name })
                : t("Docker Sandbox restarts with an empty workspace. Continue?")}</p>
              <p className="text-muted-foreground">
                {pendingAction === "destroy"
                  ? t("The Sandbox remains in inventory as a destroyed record. Host files are not deleted.")
                  : t("The Docker Sandbox workspace will be empty after restart.")}
              </p>
            </div>
          ) : item ? (
            <div className="flex flex-col gap-4">
              <div className="grid grid-cols-2 gap-3 rounded-2xl border border-border/70 bg-background/60 p-4">
                <SandboxMetadata name={t("State")} value={sandboxStateLabel(item.state)} />
                <SandboxMetadata name={t("Active execution leases")} value={String(item.activeLeases)} />
              </div>
              {!noActiveLeases ? (
                <p role="status" className="text-xs leading-5 text-muted-foreground">
                  {t("Cannot change lifecycle while a Sandbox execution lease is active.")}
                </p>
              ) : null}
              <div className="flex flex-wrap items-center gap-2">
                {canStart ? (
                  <Button disabled={busy || !noActiveLeases} onClick={() => requestLifecycle("start")}>
                    {t("Start sandbox")}
                  </Button>
                ) : null}
                {canStop ? (
                  <Button disabled={busy || !noActiveLeases} onClick={() => requestLifecycle("stop")}>
                    {t("Stop sandbox")}
                  </Button>
                ) : null}
                {canReconcile && item.backend === "docker" ? (
                  <Button disabled={busy || !noActiveLeases} onClick={() => void reconcile()}>
                    {t("Reconcile Docker sandbox")}
                  </Button>
                ) : null}
                {canReconcile && item.backend === "daytona" ? (
                  <Button disabled={busy || !noActiveLeases} onClick={() => void reconcile()}>
                    {t("Reconcile Daytona sandbox")}
                  </Button>
                ) : null}
                {canDestroy ? (
                  <Button variant="destructive" disabled={busy || !noActiveLeases} onClick={() => requestLifecycle("destroy")}>
                    {t("Destroy sandbox")}
                  </Button>
                ) : null}
              </div>
            </div>
          ) : null}
          <DialogFooter>
            {pendingAction ? (
              <>
                <Button variant="outline" disabled={busy} onClick={() => {
                  setPendingAction(null);
                  setError("");
                }}>{t("Back")}</Button>
                <Button
                  variant={pendingAction === "destroy" ? "destructive" : "default"}
                  disabled={busy || !noActiveLeases}
                  onClick={() => void changeLifecycle(pendingAction === "destroy" ? "destroy" : "start")}
                >
                  {t(pendingAction === "destroy" ? "Confirm destruction" : "Confirm restart")}
                </Button>
              </>
            ) : (
              <DialogClose render={<Button variant="outline" disabled={busy} />}>{t("Close")}</DialogClose>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function sandboxLifecycleError(reason: unknown): string {
  const detail = typeof reason === "string"
    ? reason
    : reason instanceof Error ? reason.message : "";
  if (detail.includes("sandbox state conflict") ||
    (reason instanceof MenderApiError && reason.code === "conflict")) {
    return t("Sandbox lifecycle conflicts with an active lease, Workspace Provider binding, or a changed state. Review dependencies and refresh.");
  }
  return errorMessage(reason);
}
