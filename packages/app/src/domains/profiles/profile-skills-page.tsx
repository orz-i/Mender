import { useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router";

import type { ManagementClient, Profile, SkillInstallation } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";

import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { EmptyState, ErrorNotice } from "../../shared/page";
import { PageToolbar } from "../../shared/page-toolbar";
import { DataPanel } from "../../shared/workbench";

function equalSelections(a: readonly string[], b: ReadonlySet<string>): boolean {
  return a.length === b.size && a.every((id) => b.has(id));
}

/** Explicit, Profile-scoped resource selection; never materializes Skill files. */
export function ProfileSkillsPage({ client }: { client: ManagementClient }) {
  const { slug } = useParams();
  return <ProfileSkillsEditor key={slug} client={client} slug={slug} />;
}

function ProfileSkillsEditor({ client, slug }: { client: ManagementClient; slug: string | undefined }) {
  const navigate = useNavigate();
  const [profile, setProfile] = useState<Profile | null>(null);
  const [installations, setInstallations] = useState<SkillInstallation[]>([]);
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [loaded, setLoaded] = useState(false);
  const [loadingError, setLoadingError] = useState("");
  const [saveError, setSaveError] = useState("");
  const [busy, setBusy] = useState(false);
  const [version, setVersion] = useState(0);

  useEffect(() => {
    if (!slug) return;
    let active = true;
    void Promise.all([client.getProfile(slug), client.listSkills()])
      .then(([item, library]) => {
        if (!active) return;
        setProfile(item);
        setInstallations(library.installations);
        setSelected(new Set(item.skills ?? []));
        setLoadingError("");
        setSaveError("");
      }).catch((reason: unknown) => {
        if (active) setLoadingError(errorMessage(reason));
      }).finally(() => {
        if (active) setLoaded(true);
      });
    return () => { active = false; };
  }, [client, slug, version]);

  const selectedIds = profile?.skills ?? [];
  const visibleSkills = useMemo(() => installations.filter(
    (item) => (item.trusted && item.enabled) || selected.has(item.installationId),
  ), [installations, selected]);
  const unavailable = useMemo(
    () => [...selected].filter((id) => !installations.some((item) => item.installationId === id)),
    [installations, selected],
  );
  const changed = !equalSelections(selectedIds, selected);

  function toggle(id: string, checked: boolean) {
    setSelected((current) => {
      const next = new Set(current);
      if (checked) next.add(id);
      else next.delete(id);
      return next;
    });
    setSaveError("");
  }

  async function save() {
    if (!profile || !changed || busy) return;
    setBusy(true);
    setSaveError("");
    try {
      await client.replaceProfileSkills(profile.slug, { installationIds: [...selected].sort() });
      void navigate("/profiles/" + encodeURIComponent(profile.slug), {
        state: { initialSection: "skills" },
      });
    } catch (reason) {
      setSaveError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <PageToolbar actions={<>
        <Button variant="outline" disabled={busy}
          onClick={() => void navigate(slug ? "/profiles/" + encodeURIComponent(slug) : "/profiles")}>
          {t("Cancel")}
        </Button>
        <Button disabled={!profile || !changed || busy} onClick={() => void save()}>
          {busy ? t("Saving…") : t("Save skills")}
        </Button>
      </>} />
      {!loaded ? <p className="px-1 py-4 text-sm text-muted-foreground">{t("Loading skills…")}</p> : null}
      {loadingError || !slug ? <ErrorNotice message={loadingError || t("The requested item was not found.")}
        onRetry={slug ? () => { setLoaded(false); setVersion((current) => current + 1); } : undefined} /> : null}
      {loaded && !loadingError && !profile ? <ErrorNotice message={t("The requested item was not found.")} /> : null}
      {profile ? <DataPanel title={t("Choose Profile skills")} description={profile.displayName}>
        <div className="space-y-4 p-5">
          <p className="text-sm text-muted-foreground">
            {t("Choose exact enabled Skill installations. This does not deploy files or grant Tool permissions.")}
          </p>
          {saveError ? <ErrorNotice message={saveError} /> : null}
          {visibleSkills.length === 0 && unavailable.length === 0
            ? <EmptyState title="No enabled skills" description="Install, trust and enable a Skill in the Skills library first." />
            : <div className="divide-y divide-border rounded-lg border border-border">
              {visibleSkills.map((item) => {
                const eligible = item.trusted && item.enabled;
                const checked = selected.has(item.installationId);
                return <label key={item.installationId}
                  className="flex cursor-pointer items-start gap-3 px-4 py-3 hover:bg-muted/30">
                  <input type="checkbox" className="mt-1 size-4 shrink-0 accent-primary"
                    checked={checked} disabled={busy || (!eligible && !checked)}
                    onChange={(event) => toggle(item.installationId, event.target.checked)}
                    aria-label={t("Select skill {{name}}", { name: item.name })} />
                  <span className="min-w-0 flex-1">
                    <span className="block break-words text-sm font-medium">{item.name}</span>
                    <span className="mt-1 block text-xs text-muted-foreground">
                      {item.versionHint || t("Unversioned")} · {item.builtIn ? t("Built-in") : t("Installed")}
                      {!eligible ? " · " + t("Unavailable for new selection") : ""}
                    </span>
                    <span className="mt-1 block break-words text-xs text-muted-foreground">{item.description}</span>
                  </span>
                </label>;
              })}
              {unavailable.map((id) => <label key={id}
                className="flex cursor-pointer items-center gap-3 px-4 py-3">
                <input type="checkbox" className="size-4 shrink-0 accent-primary" checked
                  disabled={busy} onChange={() => toggle(id, false)}
                  aria-label={t("Remove unavailable skill {{id}}", { id })} />
                <span className="min-w-0 break-all text-sm">
                  {t("Unavailable installation")} · {id}
                </span>
              </label>)}
            </div>}
          <p className="text-xs text-muted-foreground">
            {t("{{count}} skills selected", { count: selected.size })}
          </p>
        </div>
      </DataPanel> : null}
    </>
  );
}
