import { useCallback, useEffect, useMemo, useState } from "react";
import { ChevronLeftIcon } from "lucide-react";
import { useNavigate, useParams } from "react-router";

import type { MenderClient, Profile, SkillDeployment, SkillInstallation } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import {
  Dialog, DialogBody, DialogContent, DialogDescription,
  DialogFooter, DialogHeader, DialogTitle,
} from "@mender/ui/components/dialog";

import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { EmptyState, ErrorNotice } from "../../shared/page";
import { DataPanel } from "../../shared/workbench";

type Target = SkillDeployment["target"];

const targets: { id: Target; label: string }[] = [
  { id: "agents-project", label: "Agents project" },
  { id: "github-project", label: "GitHub project" },
  { id: "claude-project", label: "Claude project" },
  { id: "agents-user", label: "Agents user" },
  { id: "copilot-user", label: "Copilot user" },
];

type DeployProfileSkills = (slug: string, target: Target) => Promise<SkillDeployment[] | null>;

export function ProfileSkillDeliveryPage({
  client, deployProfileSkills,
}: {
  client: MenderClient;
  deployProfileSkills?: DeployProfileSkills;
}) {
  const { slug } = useParams();
  return <ProfileSkillDeliveryContent key={slug} slug={slug} client={client}
    {...(deployProfileSkills === undefined ? {} : { deployProfileSkills })} />;
}

function ProfileSkillDeliveryContent({
  slug, client, deployProfileSkills,
}: {
  slug: string | undefined;
  client: MenderClient;
  deployProfileSkills?: DeployProfileSkills;
}) {
  const navigate = useNavigate();
  const [profile, setProfile] = useState<Profile | null>(null);
  const [library, setLibrary] = useState<SkillInstallation[]>([]);
  const [deployments, setDeployments] = useState<SkillDeployment[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [actionError, setActionError] = useState("");
  const [notice, setNotice] = useState("");
  const [target, setTarget] = useState<Target | "">("");
  const [busy, setBusy] = useState(false);
  const [removing, setRemoving] = useState<SkillDeployment | null>(null);
  const [version, setVersion] = useState(0);

  useEffect(() => {
    if (!slug) return;
    let active = true;
    void Promise.all([
      client.getProfile(slug),
      client.listSkills(),
      client.listProfileSkillDeployments(slug),
    ]).then(([item, skills, owned]) => {
      if (!active) return;
      setProfile(item);
      setLibrary(skills.installations);
      setDeployments(owned.deployments);
      setLoadError("");
    }).catch((reason: unknown) => {
      if (active) setLoadError(errorMessage(reason));
    }).finally(() => {
      if (active) setLoaded(true);
    });
    return () => { active = false; };
  }, [slug, client, version]);

  const byId = useMemo(() => new Map(library.map((item) => [item.installationId, item])), [library]);
  const selectedSkills = profile?.skills ?? [];
  const canDeploy = deployProfileSkills !== undefined && target !== "" && selectedSkills.length > 0;

  const refresh = useCallback(() => {
    setLoaded(false);
    setVersion((current) => current + 1);
  }, []);

  async function deploy() {
    if (!profile || !canDeploy || busy || !target || !deployProfileSkills) return;
    setActionError("");
    setNotice("");
    setBusy(true);
    try {
      const created = await deployProfileSkills(profile.slug, target);
      if (created === null) return; // Host directory dialog was cancelled.
      const owned = await client.listProfileSkillDeployments(profile.slug);
      setDeployments(owned.deployments);
      setNotice(t("Deployed {{count}} selected skills.", { count: created.length }));
    } catch (reason) {
      setActionError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  async function undeploy() {
    if (!removing || busy || !profile) return;
    setBusy(true);
    setActionError("");
    try {
      await client.undeployProfileSkill(profile.slug, removing.deploymentId);
      setDeployments((current) => current.filter((item) => item.deploymentId !== removing.deploymentId));
      setRemoving(null);
    } catch (reason) {
      setActionError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Button size="sm" variant="outline" disabled={busy}
          onClick={() => void navigate(slug ? "/profiles/" + encodeURIComponent(slug) : "/profiles",
            { state: { initialSection: "deliveries" } })}>
          <ChevronLeftIcon data-icon="inline-start" />
          {t("Back to profile")}
        </Button>
        <Button size="sm" variant="outline" disabled={busy} onClick={refresh}>{t("Refresh")}</Button>
      </div>
      {loadError || !slug ? <ErrorNotice message={loadError || t("The requested item was not found.")}
        onRetry={slug ? refresh : undefined} /> : null}
      {!loaded ? <p className="text-sm text-muted-foreground">{t("Loading skills…")}</p> : null}
      {profile ? <>
        <DataPanel title={t("Profile Skill delivery")} description={profile.displayName}>
          <div className="flex flex-col gap-4 p-5">
            <p className="text-sm text-muted-foreground">
              {t("Deliver the exact Skills selected for this Profile; this does not grant Tool permissions.")}
            </p>
            {selectedSkills.length ? <div className="divide-y divide-border rounded-lg border border-border">
              {selectedSkills.map((id) => {
                const skill = byId.get(id);
                return <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-3" key={id}>
                  <span className="min-w-0 text-sm font-medium">{skill?.name ?? t("Unavailable installation")}</span>
                  <span className="text-xs text-muted-foreground">{skill?.versionHint || t("Unversioned")}</span>
                </div>;
              })}
            </div> : <EmptyState title="No skills selected"
              description="Choose Skills for this Profile before delivering them to an Agent." />}

            {deployProfileSkills ? <>
              <div className="space-y-2">
                <label htmlFor="profile-skill-delivery-target" className="block text-sm font-medium">
                  {t("Deployment target")}
                </label>
                <select id="profile-skill-delivery-target" value={target} disabled={busy}
                  className="h-9 w-full rounded-lg border border-input bg-background px-3 text-sm"
                  onChange={(event) => {
                    setTarget(event.target.value as Target | "");
                    setActionError("");
                    setNotice("");
                  }}>
                  <option value="">{t("Choose deployment target")}</option>
                  {targets.map((item) => <option key={item.id} value={item.id}>{t(item.label)}</option>)}
                </select>
                {target ? <p className="text-xs text-muted-foreground" role="note">
                  {t(target.endsWith("-project")
                    ? "The native file chooser will ask for a project directory before writing any Skill files."
                    : "This writes selected Skills to the current operating-system user's Agent directory.")}
                </p> : null}
              </div>
              <Button disabled={busy || !canDeploy} onClick={() => void deploy()}>{t("Deploy selected skills")}</Button>
            </> : <p className="text-sm text-muted-foreground">
              {t("Local Skill deployment requires the Mender Desktop host. Remote management is read-only.")}
            </p>}
            {notice ? <p className="text-sm" role="status">{notice}</p> : null}
            {actionError ? <ErrorNotice message={actionError} /> : null}
          </div>
        </DataPanel>

        <DataPanel title={t("Profile Skill deployments")}>
          {deployments.length ? <div className="divide-y divide-border">
            {deployments.map((item) => {
              const stale = profile.revision !== undefined && item.profileRevision !== undefined &&
                item.profileRevision !== profile.revision;
              return <div key={item.deploymentId} className="flex flex-wrap items-center gap-3 px-5 py-4">
                <div className="min-w-0 flex-1 space-y-1">
                  <p className="text-sm font-medium">{byId.get(item.installationId)?.name ?? t("Unavailable installation")}</p>
                  <p className="break-all font-mono text-xs text-muted-foreground">{item.destination}</p>
                  <p className="text-xs text-muted-foreground">
                    {t(item.drift)}{stale ? " · " + t("Profile changed since delivery") : ""}
                  </p>
                </div>
                {deployProfileSkills ? <Button size="sm" variant="outline" disabled={busy} onClick={() => {
                  setRemoving(item);
                  setActionError("");
                }}>{t("Undeploy")}</Button> : null}
              </div>;
            })}
          </div> : <div className="p-5">
            <EmptyState title="No Profile Skill deployments"
              description="Deploy the selected Skills to an explicit Agent project or user directory." />
          </div>}
        </DataPanel>
        <p className="text-xs text-muted-foreground">
          {t("Legacy unassigned Skill deployments remain in the Skills library. They are not automatically claimed.")}
        </p>
      </> : null}
      <Dialog open={removing !== null} onOpenChange={(open) => { if (!open && !busy) setRemoving(null); }}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("Remove Skill deployment?")}</DialogTitle>
            <DialogDescription>{t("Only the selected recorded Skill directory will be removed. Other Profile deliveries are not affected.")}</DialogDescription>
          </DialogHeader>
          <DialogBody><p className="break-all font-mono text-xs">{removing?.destination}</p>
            {actionError ? <ErrorNotice message={actionError} /> : null}</DialogBody>
          <DialogFooter>
            <Button variant="outline" disabled={busy} onClick={() => setRemoving(null)}>{t("Cancel")}</Button>
            <Button variant="destructive" disabled={busy} onClick={() => void undeploy()}>{t("Undeploy")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
