import { useEffect, useState } from "react";
import { ActivityIcon, ShieldCheckIcon } from "lucide-react";
import { useNavigate, useParams } from "react-router";

import type { MenderClient, Sandbox } from "@mender/sdk";
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
import { ErrorNotice, PageHeader } from "../../shared/page";
import { DataPanel } from "../../shared/workbench";
import { isolationLabel, sandboxBackend, sandboxStateLabel } from "./sandbox-display";
import { SandboxMetadata } from "./sandbox-metadata";

type DetailTopic = "overview" | "security";
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
      (item.backend === "local_workspace" || item.backend === "docker" || item.backend === "daytona"),
  );
  const noActiveLeases = item?.activeLeases === 0;
  const canStart = manageable && item?.state === "stopped";
  const canStop = manageable && item?.state === "ready";
  const canDestroy = manageable && (item?.state === "ready" || item?.state === "stopped");
  const canReconcile = manageable && (item?.backend === "docker" || item?.backend === "daytona") &&
    item?.state === "unknown";
  const hasActions = canStart || canStop || canDestroy || canReconcile;

  async function changeLifecycle(action: LifecycleAction) {
    if (!item || busy || !noActiveLeases) return;
    if (action === "destroy" &&
      !window.confirm(t('Destroy Sandbox "{{name}}"? This only tombstones the Sandbox; host files are never deleted.', { name: item.name }))) {
      return;
    }
    if (action === "start" && item.backend === "docker" &&
      !window.confirm(t("Docker Sandbox restarts with an empty workspace. Continue?"))) {
      return;
    }

    setBusy(true);
    setError("");
    try {
      const next = action === "start"
        ? await client.startSandbox(item.sandboxId)
        : action === "stop"
          ? await client.stopSandbox(item.sandboxId)
          : await client.destroySandbox(item.sandboxId);
      setItem(next);
      if (action === "destroy") setManageOpen(false);
    } catch (reason) {
      setError(errorMessage(reason));
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
        : await client.reconcileDockerSandbox(item.sandboxId);
      setItem(next);
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <PageHeader
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
            <FocusSwitch
              label="Sandbox overview"
              value={topic}
              onChange={setTopic}
              items={[
                { value: "overview", label: "Sandbox overview" },
                { value: "security", label: "Security boundary" },
              ]}
            />
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
                  {item.backend === "daytona" ? (
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
        if (!busy) setManageOpen(open);
      }}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("Manage sandbox")}</DialogTitle>
            <DialogDescription>{item?.name ?? ""}</DialogDescription>
          </DialogHeader>
          {error ? <ErrorNotice message={error} /> : null}
          {item ? (
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
                  <Button disabled={busy || !noActiveLeases} onClick={() => void changeLifecycle("start")}>
                    {t("Start sandbox")}
                  </Button>
                ) : null}
                {canStop ? (
                  <Button disabled={busy || !noActiveLeases} onClick={() => void changeLifecycle("stop")}>
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
                  <Button variant="destructive" disabled={busy || !noActiveLeases} onClick={() => void changeLifecycle("destroy")}>
                    {t("Destroy sandbox")}
                  </Button>
                ) : null}
              </div>
            </div>
          ) : null}
          <DialogFooter>
            <DialogClose render={<Button variant="outline" disabled={busy} />}>{t("Close")}</DialogClose>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
