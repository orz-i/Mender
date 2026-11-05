import { useEffect, useState } from "react";
import { Link } from "react-router";
import { ArrowUpRightIcon } from "lucide-react";

import type {
  AuditRecord,
  ManagementClient,
  Profile,
  Provider,
  SkillInstallation,
  SystemStatus,
  ToolSummary,
} from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@mender/ui/components/table";

import { formatDateTime, formatNumber, t, tCount } from "../../i18n/i18n";
import { StatusBadge } from "../../shared/status-badge";
import { auditPrincipalLabel } from "../../shared/audit-principal";
import { errorMessage } from "../../shared/errors";
import { ErrorNotice, InsightBanner } from "../../shared/page";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import {
  DataPanel,
  EmptyPanelState,
} from "../../shared/workbench";

type OverviewState = {
  status: SystemStatus;
  providers: Provider[];
  profiles: Profile[];
  skills: SkillInstallation[];
  records: AuditRecord[];
  toolsById: Record<string, ToolSummary>;
};

function browseOverviewTools(client: ManagementClient) {
  const browseTools = (client as Partial<ManagementClient>).browseTools;
  if (typeof browseTools !== "function") {
    return Promise.resolve(null);
  }
  return browseTools.call(client, { limit: 100 }).catch(() => null);
}

function indexTools(tools: ToolSummary[]) {
  return Object.fromEntries(tools.map((tool) => [tool.toolId, tool]));
}

function toolLabel(toolsById: Record<string, ToolSummary>, toolId: string) {
  const tool = toolsById[toolId];
  return tool?.title || tool?.name || t("Unknown Tool");
}

function invocationStatusLabel(status: string) {
  switch (status.toLowerCase()) {
    case "completed": return t("Completed");
    case "failed": return t("Failed");
    case "denied": return t("Denied");
    default: return status;
  }
}

function PanelLink({ to, label, destination = label }: { to: string; label: string; destination?: string }) {
  return (
    <Link
      to={to}
      aria-label={t("View {{plugin}}", { plugin: t(destination) })}
      className="inline-flex h-8 items-center gap-1.5 rounded-lg border border-border bg-card px-3 text-xs font-medium text-foreground transition-colors hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring"
    >
      {t(label)}
      <ArrowUpRightIcon className="size-3.5" aria-hidden="true" />
    </Link>
  );
}

export function OverviewPage({ client }: { client: ManagementClient }) {
  const [data, setData] = useState<OverviewState | null>(null);
  const [error, setError] = useState("");
  const [dismissedInsight, setDismissedInsight] = useState(false);
  const [reloadVersion, setReloadVersion] = useState(0);

  useEffect(() => {
    let active = true;
    void Promise.all([
      client.getSystemStatus(),
      client.listProviders(),
      client.listProfiles(),
      client.listSkills(),
      client.listAudit(50),
      browseOverviewTools(client),
    ])
      .then(([status, providers, profiles, skills, audit, toolResult]) => {
        if (!active) return;
        setData({
          status,
          providers: providers.providers,
          profiles: profiles.profiles,
          skills: skills.installations,
          records: audit.records,
          toolsById: toolResult === null ? {} : indexTools(toolResult.tools),
        });
        setError("");
      })
      .catch((reason: unknown) => {
        if (active) setError(errorMessage(reason));
      });
    return () => { active = false; };
  }, [client, reloadVersion]);

  const invocationPagination = usePagination(data?.records ?? [], {
    initialPageSize: 5,
    pageSizeOptions: [5, 10, 25],
  });

  if (error) {
    return <div className="pt-4"><ErrorNotice message={error} onRetry={() => { setData(null); setError(""); setReloadVersion((version) => version + 1); }} /></div>;
  }

  if (data === null) {
    return <div role="status" className="py-12 text-center text-sm text-muted-foreground">{t("Loading")}</div>;
  }

  const recentFailures = data.records.filter((record) => record.status === "failed" || record.status === "denied").length;

  return (
    <>
      {!dismissedInsight && data.providers.length === 0 ? (
        <InsightBanner
          title="Get started with tool providers"
          description="Register external MCP servers, OpenAPI services, or local workspaces to equip your profiles with governed tools."
          onDismiss={() => setDismissedInsight(true)}
          actions={<Link to="/plugins" className="inline-flex h-8 items-center justify-center rounded-lg bg-primary px-3 text-xs font-medium text-primary-foreground transition-colors hover:bg-primary/90">
            {t("Explore Marketplace")}
          </Link>}
        />
      ) : !dismissedInsight && data.profiles.length === 0 ? (
        <InsightBanner
          title="Configure an exposure boundary"
          description="Tool providers are registered. Create a Profile to define which tools and sandboxes are exposed to agent clients."
          onDismiss={() => setDismissedInsight(true)}
          actions={<Link to="/profiles/new" className="inline-flex h-8 items-center justify-center rounded-lg bg-primary px-3 text-xs font-medium text-primary-foreground transition-colors hover:bg-primary/90">
            {t("Create profile")}
          </Link>}
        />
      ) : null}

      <section aria-label={t("Workspace summary")} data-slot="workspace-summary"
        className="mb-4 flex min-w-0 flex-wrap items-center justify-between gap-x-6 gap-y-2 rounded-xl border border-border bg-card px-4 py-3 text-[13px]">
        <div className="flex min-w-0 items-center gap-2">
          <span className={["size-2 shrink-0 rounded-full", data.status.ready ? "bg-teal" : "bg-destructive"].join(" ")} aria-hidden="true" />
          <span className="text-muted-foreground">{t("Runtime")}</span>
          <span className="font-semibold">{t(data.status.ready ? "Ready" : "Unavailable")}</span>
        </div>
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-muted-foreground">
          <span>{t("Providers")} <strong className="font-semibold text-foreground tabular-nums">{formatNumber(data.providers.length)}</strong></span>
          <span>{t("Profiles")} <strong className="font-semibold text-foreground tabular-nums">{formatNumber(data.profiles.length)}</strong></span>
          <span>{t("Skills")} <strong className="font-semibold text-foreground tabular-nums">{formatNumber(data.skills.length)}</strong></span>
          {recentFailures > 0 ? (
            <Link to="/audit?status=attention" className="rounded-md font-medium text-primary-ink hover:underline focus-visible:outline-2 focus-visible:outline-ring">
              {t("{{count}} failures in recent calls", { count: formatNumber(recentFailures) })}
            </Link>
          ) : null}
        </div>
      </section>

      <div className={data.records.length
        ? "grid min-w-0 items-start gap-4 xl:grid-cols-[minmax(0,1.5fr)_minmax(290px,1fr)]"
        : "min-w-0"}>
        {data.records.length > 0 ? <div className="min-w-0">
          <DataPanel title="Recent invocations"
            description="Latest tool execution activity across the local control plane."
            actions={<PanelLink to="/audit" label="Audit" />}>
            {data.records.length ? (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("Tool")}</TableHead>
                    <TableHead>{t("Caller")}</TableHead>
                    <TableHead>{t("Status")}</TableHead>
                    <TableHead className="text-right">{t("Finished")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {invocationPagination.items.map((record) => (
                    <TableRow key={record.invocationId}>
                      <TableCell className="max-w-[230px] truncate font-medium" title={toolLabel(data.toolsById, record.toolId)}>
                        <Link to={"/audit?q=" + encodeURIComponent(record.invocationId)}
                          aria-label={t("View invocation") + ": " + toolLabel(data.toolsById, record.toolId) + " · " + formatDateTime(record.finishedAt)}
                          className="block max-w-full truncate rounded-sm hover:text-primary-ink hover:underline focus-visible:outline-2 focus-visible:outline-ring">
                          {toolLabel(data.toolsById, record.toolId)}
                        </Link>
                      </TableCell>
                      <TableCell className="max-w-[220px] truncate" title={record.principalKind + ":" + record.principalRef}>
                        {auditPrincipalLabel(record, data.profiles)}
                      </TableCell>
                      <TableCell>
                        <StatusBadge status={record.status}>{invocationStatusLabel(record.status)}</StatusBadge>
                      </TableCell>
                      <TableCell className="text-right text-xs text-muted-foreground">
                        {formatDateTime(record.finishedAt)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            ) : (
              <div className="flex min-h-[124px] items-center justify-center px-4 py-6 text-center">
                <p className="text-[13px] text-muted-foreground">{t("No invocation records yet.")}</p>
              </div>
            )}
          </DataPanel>
          {invocationPagination.total > 0 ? (
            <TablePagination label="Recent invocations" pagination={invocationPagination} />
          ) : null}
        </div> : null}

        <div className={data.records.length ? "grid min-w-0 gap-4" : "grid min-w-0 gap-4 md:grid-cols-2"}>
          <DataPanel
            title="Providers"
            description="Registered tool sources"
            actions={<PanelLink to="/tools" label="View" destination="Tool services" />}
          >
            {data.providers.length ? (
              <div className="divide-y divide-border/70 px-4 pb-1">
                {data.providers.slice(0, 4).map((provider) => (
                  <div key={provider.providerId} className="flex min-w-0 items-center justify-between gap-3 py-2.5">
                    <div className="min-w-0">
                      <p className="truncate text-[13px] font-medium">{provider.displayName || provider.namespace}</p>
                      <p className="mt-0.5 truncate text-xs text-muted-foreground">{provider.pluginDisplayName || provider.namespace}</p>
                    </div>
                    <Badge variant="secondary">{formatNumber(provider.toolCount)} {t("Tools")}</Badge>
                  </div>
                ))}
              </div>
            ) : (
              <EmptyPanelState className="min-h-[100px]!">
                <p className="text-sm text-muted-foreground">{t("No providers")}</p>
              </EmptyPanelState>
            )}
          </DataPanel>
          <DataPanel
            title="Profiles"
            description="Selected Tools"
            actions={<PanelLink to="/profiles" label="View" destination="Profiles" />}
          >
            {data.profiles.length ? (
              <div className="divide-y divide-border/70 px-4 pb-1">
                {data.profiles.slice(0, 4).map((profile) => (
                  <div key={profile.profileId} className="flex min-w-0 items-center justify-between gap-3 py-2.5">
                    <div className="min-w-0">
                      <p className="truncate text-[13px] font-medium">{profile.displayName || profile.slug}</p>
                      <p className="mt-0.5 truncate text-xs text-muted-foreground">{profile.slug}</p>
                    </div>
                    <Badge variant="outline">{tCount(profile.tools.length, "{{count}} selected Tool", "{{count}} selected Tools")}</Badge>
                  </div>
                ))}
              </div>
            ) : (
              <EmptyPanelState className="min-h-[100px]!">
                <p className="text-sm text-muted-foreground">{t("No profiles")}</p>
              </EmptyPanelState>
            )}
          </DataPanel>
        </div>
      </div>
      {data.records.length === 0 ? (
        <div className="mt-3 flex flex-wrap items-center justify-between gap-2 rounded-lg border border-border/70 px-4 py-2.5 text-[13px] text-muted-foreground">
          <span>{t("No invocation records yet.")}</span>
          <PanelLink to="/audit" label="Audit" />
        </div>
      ) : null}
    </>
  );
}
