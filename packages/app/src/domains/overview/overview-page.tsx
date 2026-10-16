import { useEffect, useState } from "react";
import { Link } from "react-router";
import {
  ActivityIcon,
  ArrowUpRightIcon,
  BookOpenIcon,
  Layers3Icon,
  PlugIcon,
  CheckCircle2Icon,
  Clock3Icon,
} from "lucide-react";

import type {
  AuditRecord,
  MenderClient,
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

import { formatDateTime, formatNumber, t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { ErrorNotice, InsightBanner } from "../../shared/page";
import { FocusSwitch } from "../../shared/focus-switch";
import { PageToolbar } from "../../shared/page-toolbar";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import {
  DataPanel,
  EmptyPanelState,
  MetricStrip,
  MetricTile,
} from "../../shared/workbench";

type OverviewState = {
  status: SystemStatus;
  providers: Provider[];
  profiles: Profile[];
  skills: SkillInstallation[];
  records: AuditRecord[];
  toolsById: Record<string, ToolSummary>;
};

function browseOverviewTools(client: MenderClient) {
  const browseTools = (client as Partial<MenderClient>).browseTools;
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

function PanelLink({ to, label, destination = label }: { to: string; label: string; destination?: string }) {
  return (
    <Link
      to={to}
      aria-label={t("View {{plugin}}", { plugin: t(destination) })}
      className="inline-flex h-8 items-center gap-1.5 rounded-xl border border-border bg-card px-3 text-xs font-medium text-foreground transition-colors hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring"
    >
      {t(label)}
      <ArrowUpRightIcon className="size-3.5" aria-hidden="true" />
    </Link>
  );
}

export function OverviewPage({ client }: { client: MenderClient }) {
  const [data, setData] = useState<OverviewState | null>(null);
  const [error, setError] = useState("");
  const [view, setView] = useState<"activity" | "resources">("activity");
  const [dismissedInsight, setDismissedInsight] = useState(false);

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
  }, [client]);

  const invocationPagination = usePagination(data?.records ?? [], {
    initialPageSize: 5,
    pageSizeOptions: [5, 10, 25],
  });

  return (
    <>
      {error ? <ErrorNotice message={error} /> : null}

      <PageToolbar
        tabs={
          <FocusSwitch
            label="Overview"
            value={view}
            onChange={setView}
            items={[
              { value: "activity", label: "Recent invocations" },
              { value: "resources", label: `${t("Providers")} / ${t("Profiles")}` },
            ]}
          />
        }
      />

      {!dismissedInsight && data !== null && data.providers.length === 0 ? (
        <InsightBanner
          title="Get started with tool providers"
          description="Register external MCP servers, OpenAPI services, or local workspaces to equip your profiles with governed tools."
          onDismiss={() => setDismissedInsight(true)}
          actions={
            <Link
              to="/plugins"
              className="inline-flex h-8 items-center justify-center rounded-xl bg-primary px-3 text-xs font-semibold text-primary-foreground transition-all hover:bg-primary/90"
            >
              {t("Explore Marketplace")}
            </Link>
          }
        />
      ) : !dismissedInsight && data !== null && data.providers.length > 0 && data.profiles.length === 0 ? (
        <InsightBanner
          title="Configure an exposure boundary"
          description="Tool providers are registered. Create a Profile to define which tools and sandboxes are exposed to agent clients."
          onDismiss={() => setDismissedInsight(true)}
          actions={
            <Link
              to="/profiles"
              className="inline-flex h-8 items-center justify-center rounded-xl bg-primary px-3 text-xs font-semibold text-primary-foreground transition-all hover:bg-primary/90"
            >
              {t("Create profile")}
            </Link>
          }
        />
      ) : null}

      <MetricStrip>
        <MetricTile
          icon={ActivityIcon}
          label="Runtime"
          value={data === null ? t("Loading") : t(data.status.ready ? "Ready" : "Unavailable")}
          description="Shared local control plane"
        />
        <MetricTile
          icon={PlugIcon}
          label="Providers"
          value={data?.providers.length ?? 0}
          description="Registered tool sources"
        />
        <MetricTile
          icon={BookOpenIcon}
          label="Skills"
          value={data?.skills.length ?? 0}
          description="Portable agent workflows"
        />
        <MetricTile
          icon={Layers3Icon}
          label="Profiles"
          value={data?.profiles.length ?? 0}
          description="MCP exposure boundaries"
        />
      </MetricStrip>

      {view === "activity" ? (
        <div className="min-w-0">
        <DataPanel
          title="Recent invocations"
          description="Latest tool execution activity across the local control plane."
          actions={<PanelLink to="/audit" label="Audit" />}
        >
          {data?.records.length ? (
            <>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("Tool")}</TableHead>
                    <TableHead>{t("Principal")}</TableHead>
                    <TableHead>{t("Status")}</TableHead>
                    <TableHead className="text-right">{t("Finished")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {invocationPagination.items.map((record) => (
                    <TableRow key={record.invocationId}>
                      <TableCell className="max-w-[230px] truncate font-medium" title={toolLabel(data.toolsById, record.toolId)}>
                        {toolLabel(data.toolsById, record.toolId)}
                      </TableCell>
                      <TableCell className="max-w-[220px] truncate" title={record.principalKind + ":" + record.principalRef}>
                        {record.principalKind + ":" + record.principalRef}
                      </TableCell>
                      <TableCell><Badge variant="outline">{record.status}</Badge></TableCell>
                      <TableCell className="text-right text-xs text-muted-foreground">
                        {formatDateTime(record.finishedAt)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </>
          ) : (
            <div className="px-5 pb-5">
              <div className="grid grid-cols-3 divide-x divide-border rounded-2xl border border-border/65 bg-background/65 py-4">
                <div className="px-4"><CheckCircle2Icon className="mb-2 size-4 text-primary-ink" /><p className="text-2xl font-semibold tabular-nums">0</p><p className="text-xs text-muted-foreground">{t("Completed")}</p></div>
                <div className="px-4"><ActivityIcon className="mb-2 size-4 text-muted-foreground" /><p className="text-2xl font-semibold tabular-nums">0</p><p className="text-xs text-muted-foreground">{t("Invocations")}</p></div>
                <div className="px-4"><Clock3Icon className="mb-2 size-4 text-muted-foreground" /><p className="text-2xl font-semibold tabular-nums">0</p><p className="text-xs text-muted-foreground">{t("Failed")}</p></div>
              </div>
              <div className="flex min-h-36 flex-col items-center justify-center gap-2 text-center">
                <ActivityIcon className="size-6 text-muted-foreground/60" aria-hidden="true" />
                <p className="text-sm font-medium">{t("No invocation records yet.")}</p>
                <p className="text-xs text-muted-foreground">{t("Latest tool execution activity across the local control plane.")}</p>
              </div>
            </div>
          )}
        </DataPanel>
        {data?.records.length ? (
          <TablePagination label="Recent invocations" pagination={invocationPagination} />
        ) : null}

        </div>
      ) : (
        <div className="grid min-w-0 items-start gap-5 lg:grid-cols-2">

          <DataPanel
            title="Providers"
            description="Registered tool sources"
            actions={<PanelLink to="/providers" label="View" destination="Providers" />}
          >
            {data?.providers.length ? (
              <div className="divide-y divide-border/70 px-5 pb-2">
                {data.providers.slice(0, 4).map((provider) => (
                  <div key={provider.providerId} className="flex min-w-0 items-center justify-between gap-3 py-3">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium">{provider.displayName || provider.namespace}</p>
                      <p className="mt-0.5 truncate text-xs text-muted-foreground">{provider.pluginDisplayName || provider.namespace}</p>
                    </div>
                    <Badge variant="secondary">{formatNumber(provider.toolCount)} {t("Tools")}</Badge>
                  </div>
                ))}
              </div>
            ) : (
              <EmptyPanelState>
                <p className="text-sm text-muted-foreground">{t(data === null ? "Loading" : "No providers")}</p>
              </EmptyPanelState>
            )}
          </DataPanel>

          <DataPanel
            title="Profiles"
            description="MCP exposure boundaries"
            actions={<PanelLink to="/profiles" label="View" destination="Profiles" />}
          >
            {data?.profiles.length ? (
              <div className="divide-y divide-border/70 px-5 pb-2">
                {data.profiles.slice(0, 4).map((profile) => (
                  <div key={profile.profileId} className="flex min-w-0 items-center justify-between gap-3 py-3">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium">{profile.displayName || profile.slug}</p>
                      <p className="mt-0.5 truncate text-xs text-muted-foreground">{profile.slug}</p>
                    </div>
                    <Badge variant="outline">{profile.mode}</Badge>
                  </div>
                ))}
              </div>
            ) : (
              <EmptyPanelState>
                <p className="text-sm text-muted-foreground">{t(data === null ? "Loading" : "No profiles")}</p>
              </EmptyPanelState>
            )}
          </DataPanel>
        </div>
      )}
    </>
  );
}
