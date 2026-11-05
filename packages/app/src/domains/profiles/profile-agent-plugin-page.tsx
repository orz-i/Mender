import { useEffect, useState } from "react";
import { ChevronLeftIcon } from "lucide-react";
import { useNavigate, useParams } from "react-router";

import type {
  ExportProfilePluginInput, ManagementClient, Profile, ProfileAgentPluginExport,
} from "@mender/sdk";
import { Button } from "@mender/ui/components/button";

import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { EmptyState, ErrorNotice } from "../../shared/page";
import { DataPanel } from "../../shared/workbench";

export type ExportProfilePlugin = (
  slug: string,
  input: ExportProfilePluginInput,
) => Promise<ProfileAgentPluginExport | null>;

export function ProfileAgentPluginPage({
  client, exportProfilePlugin,
}: {
  client: ManagementClient;
  exportProfilePlugin?: ExportProfilePlugin;
}) {
  const { slug } = useParams();
  return <ProfileAgentPluginContent key={slug} slug={slug} client={client}
    {...(exportProfilePlugin === undefined ? {} : { exportProfilePlugin })} />;
}

function ProfileAgentPluginContent({
  client, slug, exportProfilePlugin,
}: {
  client: ManagementClient;
  slug: string | undefined;
  exportProfilePlugin?: ExportProfilePlugin;
}) {
  const navigate = useNavigate();
  const [profile, setProfile] = useState<Profile | null>(null);
  const [loaded, setLoaded] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [actionError, setActionError] = useState("");
  const [name, setName] = useState(slug ?? "");
  const [version, setVersion] = useState("1.0.0");
  const [description, setDescription] = useState("");
  const [includeMCP, setIncludeMCP] = useState(false);
  const [remoteURL, setRemoteURL] = useState("");
  const [busy, setBusy] = useState(false);
  const [exported, setExported] = useState<ProfileAgentPluginExport | null>(null);
  const [retry, setRetry] = useState(0);

  useEffect(() => {
    if (!slug) return;
    let active = true;
    void client.getProfile(slug).then((item) => {
      if (!active) return;
      setProfile(item);
      setLoadError("");
    }).catch((reason: unknown) => {
      if (active) setLoadError(errorMessage(reason));
    }).finally(() => {
      if (active) setLoaded(true);
    });
    return () => { active = false; };
  }, [client, slug, retry]);

  const selectedSkills = profile?.skills?.length ?? 0;
  const selectedTools = profile?.tools.length ?? 0;
  const validName = /^[a-z0-9](?:[a-z0-9.-]{0,62}[a-z0-9])?$/.test(name) &&
    !name.includes("--") && !name.includes("..");
  const hasItems = selectedSkills > 0 || (includeMCP && selectedTools > 0);
  const validRemote = !includeMCP || (selectedTools > 0 &&
    remoteURL.startsWith("https://") && remoteURL.length > 8 &&
    !/[?#@\s]/.test(remoteURL));
  const canExport = !!profile && exportProfilePlugin !== undefined && validName &&
    hasItems && validRemote && !busy;

  async function exportPackage() {
    if (!canExport || !profile || !exportProfilePlugin) return;
    setActionError("");
    setExported(null);
    setBusy(true);
    try {
      const output = await exportProfilePlugin(profile.slug, {
        name, version, description,
        remoteMCPURL: includeMCP ? remoteURL.trim() : "",
      });
      // Native folder chooser can be cancelled: do not claim delivery.
      if (output !== null) setExported(output);
    } catch (reason) {
      setActionError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Button variant="outline" size="sm" disabled={busy} onClick={() => void navigate(
          slug ? "/profiles/" + encodeURIComponent(slug) : "/profiles",
          { state: { initialSection: "deliveries" } },
        )}>
          <ChevronLeftIcon data-icon="inline-start" />
          {t("Back to profile")}
        </Button>
        <Button variant="outline" size="sm" disabled={busy} onClick={() => {
          setLoaded(false);
          setExported(null);
          setRetry((value) => value + 1);
        }}>{t("Refresh")}</Button>
      </div>
      {loadError || !slug ? <ErrorNotice
        message={loadError || t("The requested item was not found.")}
        onRetry={slug ? () => { setLoaded(false); setRetry((value) => value + 1); } : undefined} /> : null}
      {!loaded ? <p className="text-sm text-muted-foreground">{t("Loading profiles…")}</p> : null}
      {profile ? <DataPanel title={t("Agent Plugin delivery")} description={profile.displayName}>
        <div className="flex flex-col gap-5 p-5">
          <p className="text-sm text-muted-foreground">
            {t("Export a portable Agent Plugin from selected Skills and an optional HTTPS MCP endpoint you control.")}
          </p>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="rounded-lg border border-border p-3">
              <p className="text-xs text-muted-foreground">{t("Selected Skills")}</p>
              <p className="mt-1 text-lg font-semibold">{selectedSkills}</p>
            </div>
            <div className="rounded-lg border border-border p-3">
              <p className="text-xs text-muted-foreground">{t("Selected Tools")}</p>
              <p className="mt-1 text-lg font-semibold">{selectedTools}</p>
            </div>
          </div>
          {exportProfilePlugin ? (
            <>
              <div className="grid gap-4 sm:grid-cols-2">
                <label className="grid gap-2 text-sm font-medium">
                  {t("Plugin package name")}
                  <input aria-label={t("Plugin package name")} className="h-9 w-full rounded-lg border border-input bg-background px-3 font-mono text-sm"
                    value={name} disabled={busy} onChange={(event) => {
                      setName(event.target.value);
                      setActionError("");
                    }} />
                </label>
                <label className="grid gap-2 text-sm font-medium">
                  {t("Plugin version")}
                  <input aria-label={t("Plugin version")} className="h-9 w-full rounded-lg border border-input bg-background px-3 text-sm"
                    value={version} disabled={busy} onChange={(event) => setVersion(event.target.value)} />
                </label>
              </div>
              <label className="grid gap-2 text-sm font-medium">
                {t("Plugin description")}
                <input aria-label={t("Plugin description")} className="h-9 w-full rounded-lg border border-input bg-background px-3 text-sm"
                  value={description} disabled={busy} onChange={(event) => setDescription(event.target.value)} />
              </label>
              <label className="flex items-start gap-3 text-sm">
                <input type="checkbox" className="mt-1 size-4 accent-primary"
                  aria-label={t("Include remote MCP connection")}
                  checked={includeMCP} disabled={busy || selectedTools === 0}
                  onChange={(event) => {
                    setIncludeMCP(event.target.checked);
                    setActionError("");
                  }} />
                <span>
                  <span className="block font-medium">{t("Include remote MCP connection")}</span>
                  <span className="mt-1 block text-xs text-muted-foreground">
                    {t("Only use a stable HTTPS endpoint that serves this exact Profile. Mender cannot verify an external URL's Tool scope.")}
                  </span>
                </span>
              </label>
              {includeMCP ? <label className="grid gap-2 text-sm font-medium">
                {t("Remote MCP URL")}
                <input aria-label={t("Remote MCP URL")} type="url" className="h-9 w-full rounded-lg border border-input bg-background px-3 font-mono text-xs"
                  value={remoteURL} placeholder={"https://mcp.example.com/mcp"} disabled={busy}
                  onChange={(event) => {
                    setRemoteURL(event.target.value);
                    setActionError("");
                  }} />
                <span className="text-xs font-normal text-muted-foreground">
                  {t("Do not include credentials, query tokens, a localhost address, or a private Tunnel endpoint.")}
                </span>
              </label> : null}
              {selectedSkills === 0 && !includeMCP ? (
                <EmptyState title="No exportable components"
                  description="Select one or more Skills or explicitly configure a Profile-scoped remote MCP connection." />
              ) : null}
              {actionError ? <ErrorNotice message={actionError} /> : null}
              <Button disabled={!canExport} onClick={() => void exportPackage()}>
                {busy ? t("Exporting…") : t("Choose folder and export")}
              </Button>
              <p className="text-xs text-muted-foreground">
                {t("A new package directory is created under the folder you choose. No existing files will be overwritten.")}
              </p>
            </>
          ) : <p role="note" className="text-sm text-muted-foreground">
            {t("Portable package export requires Mender Desktop and is unavailable from remote management.")}
          </p>}
          {exported ? <div className="rounded-lg border border-border p-4" role="status">
            <p className="text-sm font-medium">{t("Plugin package exported")}</p>
            <p className="mt-1 break-all font-mono text-xs">{exported.destination}</p>
            <p className="mt-2 text-xs text-muted-foreground">
              {t("Exported Profile revision {{revision}}. Republish after changing its selected resources.", {
                revision: exported.profileRevision,
              })}
            </p>
          </div> : null}
        </div>
      </DataPanel> : null}
    </div>
  );
}
