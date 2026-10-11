import { useEffect, useState } from "react";
import { KeyRoundIcon } from "lucide-react";

import type {
  MenderClient,
  Profile,
  Sandbox,
  SandboxProfileCapability,
  SandboxProfileGrant,
  SandboxProfileGrantEvent,
} from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@mender/ui/components/dialog";

import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { ErrorNotice } from "../../shared/page";
import { FocusSwitch } from "../../shared/focus-switch";
import { DataPanel } from "../../shared/workbench";

const CAPS = [
  { value: "file.read", label: "Read files" },
  { value: "file.search", label: "Search files" },
  { value: "file.edit", label: "Edit files" },
  { value: "process.shell", label: "Run shell" },
] as const satisfies ReadonlyArray<{ value: SandboxProfileCapability; label: string }>;

type AccessView = "current" | "history";
type DialogMode = "grant" | "revoke";
type Pending = {
  mode: DialogMode;
  profileId: string;
  capabilities: SandboxProfileCapability[];
  reviewing: boolean;
};

export function SandboxAccessPanel({ client, sandbox }: { client: MenderClient; sandbox: Sandbox }) {
  const [grants, setGrants] = useState<SandboxProfileGrant[]>([]);
  const [events, setEvents] = useState<SandboxProfileGrantEvent[]>([]);
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [view, setView] = useState<AccessView>("current");
  const [loaded, setLoaded] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [dialogError, setDialogError] = useState("");
  const [busy, setBusy] = useState(false);
  const [pending, setPending] = useState<Pending | null>(null);

  useEffect(() => {
    let active = true;
    void Promise.all([
      client.getSandboxProfileGrants(sandbox.sandboxId),
      client.listProfiles(),
      client.getSandboxProfileGrantEvents(sandbox.sandboxId),
    ]).then(([current, available, history]) => {
      if (!active) return;
      setGrants(current.grants);
      setProfiles(available.profiles);
      setEvents(history.events);
      setLoadError("");
      setLoaded(true);
    }).catch((reason: unknown) => {
      if (!active) return;
      setLoadError(errorMessage(reason));
      setLoaded(true);
    });
    return () => { active = false; };
  }, [client, sandbox.sandboxId]);

  const editable = sandbox.ownerKind === "local_user" &&
    sandbox.state !== "destroyed" && sandbox.state !== "destroying";
  const availableProfiles = profiles.filter((profile) =>
    !grants.some((grant) => grant.profileId === profile.profileId));
  const selectedProfile = profiles.find((profile) => profile.profileId === pending?.profileId);
  const profileName = (profileId: string) => {
    const profile = profiles.find((candidate) => candidate.profileId === profileId);
    return profile?.displayName || profile?.slug || t("Unavailable profile");
  };
  const capName = (capability: SandboxProfileCapability) =>
    t(CAPS.find((choice) => choice.value === capability)?.label ?? capability);

  function openAdd() {
    if (!editable || availableProfiles.length === 0) return;
    setDialogError("");
    setPending({
      mode: "grant", profileId: availableProfiles[0]!.profileId,
      capabilities: ["file.read", "file.search"], reviewing: false,
    });
  }

  function openModify(grant: SandboxProfileGrant) {
    if (!editable) return;
    setDialogError("");
    setPending({
      mode: "grant", profileId: grant.profileId,
      capabilities: [...grant.capabilities], reviewing: false,
    });
  }

  function openRevoke(grant: SandboxProfileGrant) {
    if (!editable) return;
    setDialogError("");
    setPending({
      mode: "revoke", profileId: grant.profileId,
      capabilities: [], reviewing: true,
    });
  }

  function toggleCapability(capability: SandboxProfileCapability, checked: boolean) {
    setDialogError("");
    setPending((current) => current ? {
      ...current,
      capabilities: checked
        ? [...current.capabilities, capability]
        : current.capabilities.filter((value) => value !== capability),
      reviewing: false,
    } : null);
  }

  async function submit() {
    if (!pending || busy) return;
    if (pending.mode === "grant" && pending.capabilities.length === 0) return;
    setBusy(true);
    setDialogError("");
    try {
      const result = pending.mode === "revoke"
        ? await client.revokeSandboxProfileGrants(sandbox.sandboxId, pending.profileId)
        : await client.putSandboxProfileGrants(sandbox.sandboxId, pending.profileId, {
          capabilities: pending.capabilities,
        });
      setGrants(result.grants);
      setPending(null);
      void client.getSandboxProfileGrantEvents(sandbox.sandboxId)
        .then((history) => setEvents(history.events))
        .catch(() => { /* Access mutation has already succeeded; history can be reloaded. */ });
    } catch (reason) {
      setDialogError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <DataPanel
        title="Profile access"
        icon={KeyRoundIcon}
        description={sandbox.name}
        toolbar={
          <FocusSwitch label="Profile access view" value={view} onChange={setView} items={[
            { value: "current", label: "Current access" },
            { value: "history", label: "Access history" },
          ]} />
        }
        actions={view === "current" && editable ? (
          <Button size="sm" onClick={openAdd} disabled={!loaded || !!loadError || !availableProfiles.length}>
            {t("Grant Profile access")}
          </Button>
        ) : undefined}
      >
        {loadError ? <div className="p-5"><ErrorNotice message={loadError} /></div> : null}
        {!loaded ? (
          <p className="p-5 text-sm text-muted-foreground">{t("Loading access…")}</p>
        ) : loadError ? null : view === "current" ? (
          grants.length === 0 ? (
            <div className="px-5 py-12 text-center text-sm text-muted-foreground">
              {t("No Profile access granted. Profiles cannot execute Sandbox tools until the owner authorizes them.")}
            </div>
          ) : (
            <div className="divide-y divide-border/70">
              {grants.map((grant) => (
                <div key={grant.profileId} className="flex flex-wrap items-center justify-between gap-3 px-5 py-4">
                  <div className="min-w-0">
                    <p className="font-medium">{profileName(grant.profileId)}</p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {grant.capabilities.map(capName).join(" · ")}
                    </p>
                  </div>
                  {editable ? (
                    <div className="flex items-center gap-2">
                      <Button size="sm" variant="outline" onClick={() => openModify(grant)}>
                        {t("Change access")}
                      </Button>
                      <Button size="sm" variant="destructive" onClick={() => openRevoke(grant)}>
                        {t("Revoke access")}
                      </Button>
                    </div>
                  ) : null}
                </div>
              ))}
            </div>
          )
        ) : (
          events.length === 0 ? (
            <p className="p-5 text-sm text-muted-foreground">{t("No access changes recorded.")}</p>
          ) : (
            <div className="divide-y divide-border/70">
              {events.map((event, index) => (
                <div key={index} className="flex flex-wrap items-center justify-between gap-3 px-5 py-3 text-sm">
                  <span>{profileName(event.profileId)} · {capName(event.capability)}</span>
                  <span className="text-xs text-muted-foreground">
                    {t(event.action === "grant" ? "Granted" : "Revoked")} · {new Date(event.occurredAt).toLocaleString()}
                  </span>
                </div>
              ))}
            </div>
          )
        )}
      </DataPanel>

      <Dialog open={pending !== null} onOpenChange={(open) => {
        if (!open && !busy) { setPending(null); setDialogError(""); }
      }}>
        <DialogContent className="max-h-[calc(100dvh-2rem)] grid-rows-[auto_minmax(0,1fr)_auto] sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t(pending?.mode === "revoke" ? "Revoke Profile access" :
              pending?.reviewing ? "Review Profile access" : "Configure Profile access")}</DialogTitle>
            <DialogDescription>{selectedProfile?.displayName || selectedProfile?.slug || ""}</DialogDescription>
          </DialogHeader>
          <div className="min-h-0 space-y-4 overflow-y-auto pr-1">
            {dialogError ? <div role="alert"><ErrorNotice message={dialogError} /></div> : null}
            {pending?.mode === "grant" && !pending.reviewing ? (
              <>
                {availableProfiles.some((profile) => profile.profileId === pending.profileId) ? (
                  <label className="grid gap-2 text-sm">
                    <span className="font-medium">{t("Profile")}</span>
                    <select aria-label={t("Choose Profile for access")}
                      className="h-9 w-full rounded-xl border border-input bg-background px-3"
                      value={pending.profileId}
                      onChange={(event) => setPending({
                        ...pending, profileId: event.target.value, reviewing: false,
                      })}>
                      {availableProfiles.map((profile) => (
                        <option key={profile.profileId} value={profile.profileId}>
                          {profile.displayName || profile.slug}
                        </option>
                      ))}
                    </select>
                  </label>
                ) : null}
                <div className="grid gap-3">
                  <p className="text-sm font-medium">{t("Allowed Sandbox capabilities")}</p>
                  {CAPS.map((choice) => (
                    <label key={choice.value} className="flex items-center gap-3 text-sm">
                      <input type="checkbox" className="size-4 accent-foreground"
                        checked={pending.capabilities.includes(choice.value)}
                        onChange={(event) => toggleCapability(choice.value, event.target.checked)} />
                      {t(choice.label)}
                    </label>
                  ))}
                </div>
              </>
            ) : (
              <div className="space-y-2 text-sm">
                <p className="font-medium">{selectedProfile?.displayName || selectedProfile?.slug || ""}</p>
                <p>{pending?.mode === "revoke"
                  ? t("Revoke all new Sandbox execution access for this Profile?")
                  : t("This Profile will be allowed these Sandbox capabilities:")}</p>
                {pending?.mode === "grant" ? (
                  <p className="text-muted-foreground">{pending.capabilities.map(capName).join(" · ")}</p>
                ) : null}
              </div>
            )}
            <p className="text-xs leading-5 text-muted-foreground">
              {t("Every remote caller of this Profile may use its granted capabilities, subject to Tool bindings and Policy.")}
            </p>
            {pending?.capabilities.includes("process.shell") && sandbox.isolation === "host" ? (
              <p className="text-xs font-medium text-destructive">
                {t("Host shell is not isolated and may execute with your local user permissions.")}
              </p>
            ) : null}
          </div>
          <DialogFooter>
            <Button variant="outline" disabled={busy} onClick={() => {
              if (pending?.mode === "grant" && pending.reviewing) {
                setPending({ ...pending, reviewing: false });
              } else {
                setPending(null);
              }
              setDialogError("");
            }}>{t(pending?.reviewing && pending.mode === "grant" ? "Back" : "Cancel")}</Button>
            {pending?.mode === "grant" && !pending.reviewing ? (
              <Button disabled={!pending.capabilities.length || busy} onClick={() =>
                setPending({ ...pending, reviewing: true })}>
                {t("Review access")}
              </Button>
            ) : (
              <Button disabled={busy} variant={pending?.mode === "revoke" ? "destructive" : "default"}
                onClick={() => void submit()}>
                {t(pending?.mode === "revoke" ? "Confirm revoke" : "Confirm access")}
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
