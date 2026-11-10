import { useEffect, useMemo, useState } from "react";
import { ArrowRightIcon, ChevronLeftIcon, ChevronRightIcon } from "lucide-react";
import { useLocation, useNavigate, useParams } from "react-router";

import type { MCPServerInstance, ManagementClient, Profile, Provider, Agent, SkillInstallation, ToolSummary } from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";

import { t, tCount } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { EmptyState, ErrorNotice } from "../../shared/page";
import { PageToolbar } from "../../shared/page-toolbar";
import { usePageHeading } from "../../shell/page-chrome";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import { DataPanel } from "../../shared/workbench";
import { DetailWorkspace } from "../../shared/detail-workspace";
import { profileToolSources } from "./profile-source-label";

type Section = "overview" | "tools" | "skills" | "deliveries";

export function ProfileDetailPage({ client }: { client: ManagementClient }) {
  const { slug } = useParams();
  return <ProfileDetailContent key={slug} client={client} slug={slug} />;
}

function ProfileDetailContent({ client, slug }: { client: ManagementClient; slug: string | undefined }) {
  const navigate = useNavigate();
  const location = useLocation();
  const [profile, setProfile] = useState<Profile | null>(null);
  usePageHeading(profile?.displayName);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [agents,setAgents]=useState<Agent[]>([]);
  const [tools, setTools] = useState<ToolSummary[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [version, setVersion] = useState(0);
  const [section, setSection] = useState<Section>(
    (location.state as { initialSection?: string } | null)?.initialSection === "skills" ? "skills" :
      (location.state as { initialSection?: string } | null)?.initialSection === "deliveries" ? "deliveries" : "overview",
  );
  const [skillLibrary, setSkillLibrary] = useState<SkillInstallation[]>([]);
  const [skillError, setSkillError] = useState("");
  const [skillsLoaded, setSkillsLoaded] = useState(false);
  const [mcpInstances, setMcpInstances] = useState<MCPServerInstance[]>([]);
  const [deliveriesLoaded, setDeliveriesLoaded] = useState(false);
  const [deliveryError, setDeliveryError] = useState("");
  const [sourceId, setSourceId] = useState("");

  useEffect(() => {
    if (!slug) return;
    let active = true;
    void Promise.all([
      client.getProfile(slug),
      client.listProviders(),
      client.listAgents(),
      client.browseTools({ limit: 100 }),
    ]).then(async ([item, sources, agentResult, catalog]) => {
      if (!active) return;
      setProfile(item);
      setProviders(sources.providers);
      setAgents(agentResult.agents);
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
          recovered.push({ rank: 0, toolId: tool.toolId, sourceId: tool.sourceId,
            sourceKind: tool.sourceKind, ...(tool.providerId ? {providerId: tool.providerId} : {}),
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

  useEffect(() => {
    if (section !== "skills") return;
    let active = true;
    void client.listSkills().then((library) => {
      if (!active) return;
      setSkillLibrary(library.installations);
      setSkillError("");
    }).catch((reason: unknown) => {
      if (active) setSkillError(errorMessage(reason));
    }).finally(() => { if (active) setSkillsLoaded(true); });
    return () => { active = false; };
  }, [client, section, version]);

  useEffect(() => {
    if (section !== "deliveries") return;
    let active = true;
    void client.listMCPServers().then(({ servers }) => {
      if (!active) return;
      setMcpInstances(servers.filter((item) => item.profile === slug));
      setDeliveryError("");
    }).catch((reason: unknown) => {
      if (active) setDeliveryError(errorMessage(reason));
    }).finally(() => { if (active) setDeliveriesLoaded(true); });
    return () => { active = false; };
  }, [client, section, slug, version]);

  const skillById = useMemo(() =>
    new Map(skillLibrary.map((item) => [item.installationId, item])), [skillLibrary]);
  const toolById = useMemo(() => new Map(tools.map((tool) => [tool.toolId, tool])), [tools]);
  const sourceLabels=useMemo(()=>new Map(profileToolSources(providers,agents,tools).map(source=>[source.sourceId,source.label])),[providers,agents,tools]);
  const sourceGroups = useMemo(() => {
    const counts = new Map<string, number>();
    for (const binding of profile?.tools ?? []) {
      const id = toolById.get(binding.toolId)?.sourceId ?? "__unknown__";
      counts.set(id, (counts.get(id) ?? 0) + 1);
    }
    return [...counts].map(([id, count]) => ({
      id, count, label: sourceLabels.get(id) ?? t("Unknown source"),
    })).sort((a, b) => a.label.localeCompare(b.label) || a.id.localeCompare(b.id));
  }, [profile, toolById, sourceLabels]);

  const sourceTools = useMemo(() =>
    (profile?.tools ?? []).filter((binding) =>
      (toolById.get(binding.toolId)?.sourceId ?? "__unknown__") === sourceId),
  [profile, toolById, sourceId]);
  const sourcePagination = usePagination(sourceGroups);
  const toolPagination = usePagination(sourceTools);

  return (
    <>
      <PageToolbar actions={profile ? <Button onClick={() => void navigate("/profiles/" +
        encodeURIComponent(profile.slug) + "/edit", { state: { initialSection: section } })}>
        {t("Edit profile")}
      </Button> : undefined} />
      <DetailWorkspace<Section> label="Profile sections" sections={profile ? [
        { value: "overview", label: "Overview" },
        { value: "tools", label: "Tools", count: profile.tools.length },
        { value: "skills", label: "Skills", count: profile.skills?.length ?? 0 },
        { value: "deliveries", label: "Deliveries" },
      ] : []} active={section}
        onChange={(next) => {
          setSection(next);
          setSourceId("");
          if (next === "deliveries") setDeliveriesLoaded(false);
          sourcePagination.reset();
          toolPagination.reset();
        }}>
      {error || !slug ? <ErrorNotice message={error || t("The requested item was not found.")}
        onRetry={slug ? () => setVersion((v) => v + 1) : undefined} /> : null}
      {!loaded ? <p className="text-sm text-muted-foreground">{t("Loading profiles…")}</p> : null}
      {profile ? (
        <DataPanel title={section === "overview" ? profile.displayName : section === "tools" ? "Tools" : section === "skills" ? "Skills" : "Deliveries"} description={section === "overview" ? profile.slug : undefined}>
          {section === "overview" ? (
            <>
            <dl className="grid gap-x-8 gap-y-6 p-5 sm:grid-cols-2">
              <Metadata name={t("Slug")} value={profile.slug} />
              <Metadata name={t("Display name")} value={profile.displayName} />
              <Metadata name={t("Tools")} value={tCount(profile.tools.length, "{{count}} selected Tool", "{{count}} selected Tools")} />
              <Metadata name={t("Skills")} value={String(profile.skills?.length ?? 0)} />
            </dl>
            <ProfileSupplyPath profile={profile} sources={sourceGroups}
              onManageDelivery={() => setSection("deliveries")} />
            </>
          ) : section === "skills" ? (
            <div className="flex flex-col gap-4 p-5">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <p className="text-sm text-muted-foreground">{t("Exact Skill versions selected for this Profile.")}</p>
                <Button size="sm" variant="outline" onClick={() =>
                  void navigate("/profiles/" + encodeURIComponent(profile.slug) + "/skills")}>
                  {t("Manage skills")}
                </Button>
              </div>
              {skillError ? <ErrorNotice message={skillError} onRetry={() => { setSkillsLoaded(false); setVersion((v) => v + 1); }} /> : null}
              {!skillsLoaded ? <p className="text-sm text-muted-foreground">{t("Loading skills…")}</p> : null}
              {(profile.skills?.length ?? 0) > 0 ? (
                <div className="divide-y divide-border rounded-lg border border-border">
                  {(profile.skills ?? []).map((id) => {
                    const skill = skillById.get(id);
                    return <div className="px-4 py-3" key={id}>
                      <p className="text-sm font-medium">{skill?.name ?? t("Unavailable installation")}</p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {skill ? (skill.versionHint || t("Unversioned")) : id}
                        {skill && (!skill.enabled || !skill.trusted) ? " · " + t("Unavailable for new selection") : ""}
                      </p>
                    </div>;
                  })}
                </div>
              ) : <EmptyState title="No skills selected" description="Choose Skills for this Profile before delivering them to an Agent." />}
            </div>
          ) : section === "deliveries" ? (
            <div className="flex flex-col gap-4 p-5">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="min-w-0">
                  <p className="text-sm font-medium">{t("MCP")}</p>
                  <p className="text-xs text-muted-foreground">
                    {t("Profile-scoped local HTTP servers and optional secure Tunnel.")}
                  </p>
                </div>
                <Button size="sm" variant="outline" onClick={() =>
                  void navigate("/profiles/" + encodeURIComponent(profile.slug) + "/deliveries/mcp")}>
                  {t("Manage MCP delivery")}
                </Button>
              </div>
              <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border pt-4">
                <div className="min-w-0">
                  <p className="text-sm font-medium">{t("Agent Skills")}</p>
                  <p className="text-xs text-muted-foreground">
                    {t("Deliver the {{count}} selected Skills to an Agent project or user directory.", {
                      count: profile.skills?.length ?? 0,
                    })}
                  </p>
                </div>
                <Button size="sm" variant="outline" onClick={() =>
                  void navigate("/profiles/" + encodeURIComponent(profile.slug) + "/deliveries/skills")}>
                  {t("Manage Skill delivery")}
                </Button>
              </div>
              <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border pt-4">
                <div className="min-w-0">
                  <p className="text-sm font-medium">{t("Local CLI")}</p>
                  <p className="text-xs text-muted-foreground">
                    {t("Use this Profile's Tools directly from a local command line without starting HTTP.")}
                  </p>
                </div>
                <Button size="sm" variant="outline" onClick={() =>
                  void navigate("/profiles/" + encodeURIComponent(profile.slug) + "/deliveries/cli")}>
                  {t("View CLI commands")}
                </Button>
              </div>
              <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border pt-4">
                <div className="min-w-0">
                  <p className="text-sm font-medium">{t("Agent Plugin")}</p>
                  <p className="text-xs text-muted-foreground">
                    {t("Export the selected Skills and an optional Profile-scoped HTTPS MCP connection.")}
                  </p>
                </div>
                <Button size="sm" variant="outline" onClick={() =>
                  void navigate("/profiles/" + encodeURIComponent(profile.slug) + "/deliveries/plugin")}>
                  {t("Export Agent Plugin")}
                </Button>
              </div>
              {deliveryError ? <ErrorNotice message={deliveryError}
                onRetry={() => { setDeliveriesLoaded(false); setVersion((v) => v + 1); }} /> : null}
              {!deliveriesLoaded ? <p className="text-sm text-muted-foreground">{t("Loading servers…")}</p> : null}
              {deliveriesLoaded && !deliveryError && (mcpInstances.length ? (
                <div className="divide-y divide-border rounded-lg border border-border">
                  {mcpInstances.map((server) => <div key={server.serverId} className="px-4 py-3">
                    <p className="break-all font-mono text-xs">{server.endpoint}</p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {t(server.status)}{server.tunnelStatus ? " · " + t(server.tunnelStatus) : ""}
                    </p>
                  </div>)}
                </div>
              ) : <p className="text-sm text-muted-foreground">{t("No running MCP servers for this Profile.")}</p>)}
            </div>
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
      </DetailWorkspace>
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


/**
 * The relation is factual: selected source bindings -> this Profile -> ways
 * to configure delivery. Delivery entries intentionally make no claim about
 * running endpoints or existing Agent grants.
 */
function ProfileSupplyPath({
  profile,
  sources,
  onManageDelivery,
}: {
  profile: Profile;
  sources: ReadonlyArray<{ id: string; count: number; label: string }>;
  onManageDelivery: () => void;
}) {
  return (
    <section aria-label={t("Supply path")} className="border-t border-border px-4 py-4 sm:px-5">
      <div className="mb-3 flex items-center justify-between gap-2">
        <h3 className="text-[12px] font-semibold tracking-[0.04em] text-muted-foreground uppercase">{t("Supply path")}</h3>
        <span className="text-[11px] text-muted-foreground">{t("Selected resources, not active connections.")}</span>
      </div>
      <div className="grid min-w-0 items-stretch gap-2 lg:grid-cols-[minmax(0,1fr)_20px_minmax(0,1fr)_20px_minmax(0,1fr)] lg:gap-3">
        <div className="min-w-0 rounded-xl border border-border bg-background/50 px-3.5 py-3">
          <p className="mb-2 text-[11px] font-medium text-muted-foreground">{t("Tool sources")}</p>
          {sources.length ? sources.slice(0, 4).map((source) => (
            <div key={source.id} className="flex items-center justify-between gap-2 border-t border-border/70 py-2 first:border-t-0">
              <p className="min-w-0 truncate text-[12px] font-medium" title={source.label}>{source.label}</p>
              <span className="shrink-0 font-mono text-[11px] text-muted-foreground">{source.count}</span>
            </div>
          )) : <p className="py-2 text-[12px] text-muted-foreground">{t("No tools selected")}</p>}
          {sources.length > 4 ? <p className="text-[11px] text-muted-foreground">{t("More sources selected")}: +{sources.length - 4}</p> : null}
        </div>
        <ArrowRightIcon aria-hidden="true" className="hidden size-4 self-center justify-self-center text-muted-foreground lg:block" />
        <div className="flex min-w-0 flex-col items-center justify-center rounded-xl border border-primary/25 bg-accent/45 px-3.5 py-5 text-center">
          <p className="text-[10px] font-medium tracking-wider text-primary-ink uppercase">{t("Profile")}</p>
          <p className="mt-2 max-w-full truncate text-[15px] font-semibold" title={profile.displayName}>{profile.displayName}</p>
          <p className="mt-1 text-[11px] text-muted-foreground">
            {t("{{count}} tools in this Profile", { count: profile.tools.length })}
            {" · "}
            {t("{{count}} Skills", { count: profile.skills?.length ?? 0 })}
          </p>
        </div>
        <ArrowRightIcon aria-hidden="true" className="hidden size-4 self-center justify-self-center text-muted-foreground lg:block" />
        <div className="min-w-0 rounded-xl border border-border bg-background/50 px-3.5 py-3">
          <p className="mb-2 text-[11px] font-medium text-muted-foreground">{t("Delivery methods")}</p>
          {(["MCP", "Agent Skills", "Local CLI"] as const).map((method) => (
            <button key={method} type="button" onClick={onManageDelivery}
              className="flex w-full items-center justify-between gap-2 border-t border-border/70 py-2 text-left text-[12px] font-medium hover:text-primary-ink focus-visible:outline-2 focus-visible:outline-ring first:border-t-0">
              {t(method)}
              <ChevronRightIcon className="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
            </button>
          ))}
        </div>
      </div>
    </section>
  );
}
