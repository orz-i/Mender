import { useEffect, useState } from "react";
import {
  ActivityIcon,
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

import { formatDateTime, t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { ErrorNotice, PageHeader } from "../../shared/page";
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

function toolLabel(
  toolsById: Record<string, ToolSummary>,
  toolId: string,
) {
  const tool = toolsById[toolId];
  return tool?.title || tool?.name || t("Unknown Tool");
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
        if (!active) {
          return;
        }
        setData({
          status,
          providers: providers.providers,
          profiles: profiles.profiles,
          skills: skills.installations,
          records: audit.records,
          toolsById:
            toolResult === null ? {} : indexTools(toolResult.tools),
        });
        setError("");
      })
      .catch((reason: unknown) => {
        if (active) {
          setError(errorMessage(reason));
        }
      });
    return () => {
      active = false;
    };
  }, [client]);

  const invocationPagination = usePagination(data?.records ?? [], {
    initialPageSize: 5,
    pageSizeOptions: [5, 10, 25],
  });

  return (
    <>
      <PageHeader
        title="Overview"
        description="Local control-plane status and recent Mender activity."
      />
      {error ? <ErrorNotice message={error} /> : null}

      <MetricStrip>
        <MetricTile
          icon={ActivityIcon}
          label="Runtime"
          value={t(data?.status.ready ? "Ready" : "Loading")}
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

      <DataPanel
        className="mt-5"
        title="Recent invocations"
        description="Latest tool execution activity across the local control plane."
      >
        {data?.records.length ? (
          <>
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
                    <TableCell className="font-medium">
                      {toolLabel(data.toolsById, record.toolId)}
                    </TableCell>
                    <TableCell>{record.caller || t("unknown caller")}</TableCell>
                    <TableCell>
                      <Badge variant="outline">{record.status}</Badge>
                    </TableCell>
                    <TableCell className="text-right text-xs text-muted-foreground">
                      {formatDateTime(record.finishedAt)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            <div className="px-4 pb-3">
              <TablePagination
                label="Recent invocations"
                pagination={invocationPagination}
              />
            </div>
          </>
        ) : (
          <EmptyPanelState>
            <p className="text-sm text-muted-foreground">
              {t("No invocation records yet.")}
            </p>
          </EmptyPanelState>
        )}
      </DataPanel>
    </>
  );
}
