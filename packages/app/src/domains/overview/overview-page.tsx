import { useEffect, useState } from "react";
import { Link } from "react-router";
import {
  ActivityIcon,
  ArrowUpRightIcon,
  BookOpenIcon,
  Layers3Icon,
  PlugIcon,
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
import { ErrorNotice } from "../../shared/page";
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

      <div className="mt-5 grid min-w-0 gap-5 xl:grid-cols-[minmax(0,1.65fr)_minmax(310px,1fr)]">
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
              <div className="px-4 pb-3">
                <TablePagination label="Recent invocations" pagination={invocationPagination} />
              </div>
            </>
          ) : (
            <EmptyPanelState>
              <p className="text-sm text-muted-foreground">{t("No invocation records yet.")}</p>
            </EmptyPanelState>
          )}
        </DataPanel>

        <div className="grid min-w-0 content-start gap-5 sm:grid-cols-2 xl:grid-cols-1">
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
      </div>
    </>
  );
}
