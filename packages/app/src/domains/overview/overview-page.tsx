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
} from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";

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
};

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
    ])
      .then(([status, providers, profiles, skills, audit]) => {
        if (!active) {
          return;
        }
        setData({
          status,
          providers: providers.providers,
          profiles: profiles.profiles,
          skills: skills.installations,
          records: audit.records,
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
            <div className="divide-y divide-border/60">
              {invocationPagination.items.map((record) => (
                <div
                  key={record.invocationId}
                  className="grid min-h-16 grid-cols-[minmax(0,1fr)_120px_170px] items-center gap-4 px-4 py-3 text-sm hover:bg-muted/20"
                >
                  <div className="min-w-0">
                    <p className="truncate font-mono text-xs">{record.toolId}</p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {record.caller || t("unknown caller")}
                    </p>
                  </div>
                  <div>
                    <Badge variant="outline">{record.status}</Badge>
                  </div>
                  <time className="text-right text-xs text-muted-foreground">
                    {formatDateTime(record.finishedAt)}
                  </time>
                </div>
              ))}
            </div>
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
