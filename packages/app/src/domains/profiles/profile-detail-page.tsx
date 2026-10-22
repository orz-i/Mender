import { useEffect, useMemo, useState } from "react";
import { ChevronLeftIcon, ChevronRightIcon } from "lucide-react";
import { useNavigate, useParams } from "react-router";

import type { MenderClient, Profile, Provider, ToolSummary, Tunnel } from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";

import { t, tCount } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { FocusSwitch } from "../../shared/focus-switch";
import { EmptyState, ErrorNotice } from "../../shared/page";
import { PageToolbar } from "../../shared/page-toolbar";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import { DataPanel } from "../../shared/workbench";
import { profileSourceLabels } from "./profile-source-label";

type Section = "overview" | "tools";

export function ProfileDetailPage({ client }: { client: MenderClient }) {
  const { slug } = useParams();
  return <ProfileDetailContent key={slug} client={client} slug={slug} />;
}

function ProfileDetailContent({ client, slug }: { client: MenderClient; slug: string | undefined }) {
  const navigate = useNavigate();
  const [profile, setProfile] = useState<Profile | null>(null);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [tunnels, setTunnels] = useState<Tunnel[]>([]);
  const [tools, setTools] = useState<ToolSummary[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [version, setVersion] = useState(0);
  const [section, setSection] = useState<Section>("overview");
  const [sourceId, setSourceId] = useState("");

  useEffect(() => {
    if (!slug) return;
    let active = true;
    void Promise.all([
      client.getProfile(slug),
      client.listProviders(),
      client.listTunnels(),
      client.browseTools({ limit: 100 }),
    ]).then(async ([item, sources, tunnelResult, catalog]) => {
      if (!active) return;
      setProfile(item);
      setProviders(sources.providers);
      setTunnels(tunnelResult.tunnels);
      setTools(catalog.tools);
      setError("");
      setLoaded(true);
      const seen = new Set(catalog.tools.map((tool) => tool.toolId));
      const missing = item.tools.filter((binding) => !seen.has(binding.toolId));
      const recovered: ToolSummary[] = [];
      for (let index = 0; index < missing.length && active; index += 8) {
        const results = await Promise.allSettled(missing.slice(index, index + 8)
          .map((binding) => client.getTool(binding.toolId)));
        for (const result of results) {
          if (result.status !== "fulfilled") continue;
          const tool = result.value;
          recovered.push({ rank: 0, toolId: tool.toolId, providerId: tool.providerId,
            name: tool.name, title: tool.title, description: tool.description });
        }
      }
      if (active && recovered.length) setTools((current) => [...current, ...recovered]);
    }).catch((reason: unknown) => {
      if (!active) return;
      setError(errorMessage(reason));
      setLoaded(true);
    });
    return () => { active = false; };
  }, [client, slug, version]);

  const toolById = useMemo(() => new Map(tools.map((tool) => [tool.toolId, tool])), [tools]);
  const sourceLabels = useMemo(() => profileSourceLabels(providers), [providers]);
  const sourceGroups = useMemo(() => {
    const counts = new Map<string, number>();
    for (const binding of profile?.tools ?? []) {
      const id = toolById.get(binding.toolId)?.providerId ?? "__unknown__";
      counts.set(id, (counts.get(id) ?? 0) + 1);
    }
    return [...counts].map(([id, count]) => ({
      id, count, label: sourceLabels.get(id) ?? t("Unknown source"),
    })).sort((a, b) => a.label.localeCompare(b.label) || a.id.localeCompare(b.id));
  }, [profile, toolById, sourceLabels]);

  const sourceTools = useMemo(() =>
    (profile?.tools ?? []).filter((binding) =>
      (toolById.get(binding.toolId)?.providerId ?? "__unknown__") === sourceId),
  [profile, toolById, sourceId]);
  const sourcePagination = usePagination(sourceGroups);
  const toolPagination = usePagination(sourceTools);

  return (
    <>
      <PageToolbar
        tabs={profile ? <FocusSwitch label="Profile sections" value={section}
          onChange={(next) => { setSection(next); setSourceId(""); sourcePagination.reset(); toolPagination.reset(); }}
          items={[{ value: "overview", label: "Overview" }, { value: "tools", label: "Tools" }]} /> : undefined}
        actions={<>
          <Button variant="outline" onClick={() => void navigate("/profiles")}>{t("Back to profiles")}</Button>
          {profile ? <Button onClick={() => void navigate("/profiles/" + encodeURIComponent(profile.slug) + "/edit", {
            state: { initialSection: section },
          })}>
            {t("Edit profile")}
          </Button> : null}
        </>}
      />
      {error || !slug ? <ErrorNotice message={error || t("The requested item was not found.")}
        onRetry={slug ? () => setVersion((v) => v + 1) : undefined} /> : null}
      {!loaded ? <p className="text-sm text-muted-foreground">{t("Loading profiles…")}</p> : null}
      {profile ? (
        <DataPanel title={profile.displayName} description={profile.slug}>
          {section === "overview" ? (
            <dl className="grid gap-x-8 gap-y-6 p-5 sm:grid-cols-2">
              <Metadata name={t("Slug")} value={profile.slug} />
              <Metadata name={t("Display name")} value={profile.displayName} />
              <Metadata name={t("Exposure mode")} value={t(profile.mode)} />
              <Metadata name={t("Tools")} value={tCount(profile.tools.length, "{{count}} selected Tool", "{{count}} selected Tools")} />
              <Metadata name={t("Tunnel")} value={profile.tunnelId
                ? (tunnels.find((item) => item.tunnelId === profile.tunnelId)?.displayName ?? t("Configured"))
                : t("Local only")} />
            </dl>
          ) : (
            <div className="flex flex-col gap-4 p-5">
              {sourceId ? (
                <div className="flex items-center gap-3">
                  <Button size="sm" variant="ghost" onClick={() => { setSourceId(""); sourcePagination.reset(); toolPagination.reset(); }}>
                    <ChevronLeftIcon data-icon="inline-start" />
                    {t("Back to sources")}
                  </Button>
                  <span className="min-w-0 truncate text-sm font-medium">
                    {sourceGroups.find((group) => group.id === sourceId)?.label ?? t("Unknown source")}
                  </span>
                </div>
              ) : <p className="text-sm text-muted-foreground">
                {t("Choose a source to inspect its selected Tools.")}
              </p>}
              {sourceId ? (
                sourceTools.length ? (
                  <div className="divide-y divide-border rounded-lg border border-border">
                    {toolPagination.items.map((item) => {
                      const tool = toolById.get(item.toolId);
                      const toolTitle = item.titleOverride || tool?.title || tool?.name || t("Unavailable Tool");
                      const description = item.descriptionOverride || tool?.description;
                      return <div key={item.toolId} className="flex flex-col gap-2 px-4 py-4">
                        <p className="text-sm font-medium">{toolTitle}</p>
                        {description ? <ProfileToolDescription description={description} toolTitle={toolTitle} /> : null}
                        <div className="flex flex-wrap gap-2">
                          <Badge variant="outline">{t(item.direct ? "Direct" : "Dynamic")}</Badge>
                          {item.alias ? <Badge variant="secondary">{t("Alias:")} {item.alias}</Badge> : null}
                          {item.policyConstraint !== "inherit" ? (
                            <Badge variant={item.policyConstraint === "deny" ? "destructive" : "secondary"}>
                              {t(item.policyConstraint === "deny" ? "Denied" : "Approval required")}
                            </Badge>
                          ) : null}
                          {item.concurrencyConstraint === "serial" ? <Badge variant="secondary">{t("Serial")}</Badge> : null}
                          {item.credentialRefs.length ? <Badge variant="secondary">
                            {tCount(item.credentialRefs.length, "{{count}} credential", "{{count}} credentials")}
                          </Badge> : null}
                        </div>
                      </div>;
                    })}
                  </div>
                ) : <EmptyState title="No tools selected" description="Choose Tools from the Catalog to define this Profile's exposure boundary." />
              ) : sourceGroups.length ? (
                <div className="divide-y divide-border rounded-lg border border-border">
                  {sourcePagination.items.map((group) => {
                    return <button key={group.id} type="button"
                      className="flex w-full items-center gap-3 px-4 py-4 text-left hover:bg-muted/40 focus-visible:outline-2 focus-visible:outline-ring"
                      onClick={() => { setSourceId(group.id); toolPagination.reset(); }}>
                      <span className="min-w-0 flex-1">
                        <span className="block truncate text-sm font-medium">{group.label}</span>
                        <span className="mt-1 block text-xs text-muted-foreground">
                          {tCount(group.count, "{{count}} selected Tool", "{{count}} selected Tools")}
                        </span>
                      </span>
                      <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground" />
                    </button>;
                  })}
                </div>
              ) : <EmptyState title="No tools selected" description="Choose Tools from the Catalog to define this Profile's exposure boundary." />}
              {sourceGroups.length ? (sourceId
                ? <TablePagination label="Selected Tools" pagination={toolPagination} />
                : <TablePagination label="Selected sources" pagination={sourcePagination} />) : null}
            </div>
          )}
        </DataPanel>
      ) : null}
    </>
  );
}

/** Long Catalog descriptions should not bury the other ToolBindings on a read-only detail page. */
function ProfileToolDescription({ description, toolTitle }: { description: string; toolTitle: string }) {
  if (description.length <= 160) {
    return <p className="min-w-0 break-words text-xs leading-5 text-muted-foreground [overflow-wrap:anywhere]">{description}</p>;
  }
  return (
    <details className="group min-w-0 text-xs leading-5 text-muted-foreground">
      <summary aria-label={t("Toggle full description for {{tool}}", { tool: toolTitle })}
        className="cursor-pointer list-none rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring/50 [&::-webkit-details-marker]:hidden">
        <span className="line-clamp-2 break-words group-open:hidden [overflow-wrap:anywhere]">{description}</span>
        <span className="mt-1 inline-block font-medium text-foreground group-open:hidden">{t("Show full description")}</span>
        <span className="hidden font-medium text-foreground group-open:inline">{t("Hide full description")}</span>
      </summary>
      <p className="mt-2 whitespace-pre-wrap break-words [overflow-wrap:anywhere]">{description}</p>
    </details>
  );
}

function Metadata({ name, value }: { name: string; value: string }) {
  return <div className="min-w-0">
    <dt className="mb-1 text-xs text-muted-foreground">{name}</dt>
    <dd className="break-words text-sm font-medium">{value}</dd>
  </div>;
}
