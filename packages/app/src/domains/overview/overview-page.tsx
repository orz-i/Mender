import { useEffect, useState } from "react";

import type {
  AuditRecord,
  MenderClient,
  Profile,
  Provider,
  SystemStatus,
} from "@mender/sdk";
import { Card, CardContent, CardHeader, CardTitle } from "@mender/ui/components/card";

import { ErrorNotice, PageHeader, StatusCard } from "../../shared/page";

type OverviewState = {
  status: SystemStatus;
  providers: Provider[];
  profiles: Profile[];
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
      client.listAudit(5),
    ])
      .then(([status, providers, profiles, audit]) => {
        if (!active) {
          return;
        }
        setData({
          status,
          providers: providers.providers,
          profiles: profiles.profiles,
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

  return (
    <>
      <PageHeader
        title="Overview"
        description="Local control-plane status and recent Mender activity."
      />
      {error ? <ErrorNotice message={error} /> : null}

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <StatusCard
          title="Runtime"
          value={data?.status.ready ? "Ready" : "Loading"}
          description="Management operations use the shared local Runtime."
          status={data?.status.ready ? "good" : "neutral"}
        />
        <StatusCard
          title="Providers"
          value={String(data?.providers.length ?? 0)}
          description="Registered tool sources."
        />
        <StatusCard
          title="Profiles"
          value={String(data?.profiles.length ?? 0)}
          description="MCP exposure boundaries."
        />
        <StatusCard
          title="Credentials"
          value={data?.status.credentials.resolver ?? "Loading"}
          description="Persistent credential storage is intentionally unavailable."
          status="warning"
        />
      </div>

      <Card className="mt-6">
        <CardHeader>
          <CardTitle>Recent invocations</CardTitle>
        </CardHeader>
        <CardContent>
          {data?.records.length ? (
            <div className="divide-y divide-border">
              {data.records.map((record) => (
                <div
                  key={record.invocationId}
                  className="grid grid-cols-[minmax(0,1fr)_120px_170px] gap-4 py-3 text-sm"
                >
                  <div className="min-w-0">
                    <p className="truncate font-mono text-xs">{record.toolId}</p>
                    <p className="mt-1 text-muted-foreground">{record.caller || "unknown caller"}</p>
                  </div>
                  <p>{record.status}</p>
                  <time className="text-right text-muted-foreground">
                    {new Date(record.finishedAt).toLocaleString()}
                  </time>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">
              No invocation records yet.
            </p>
          )}
        </CardContent>
      </Card>
    </>
  );
}

function errorMessage(reason: unknown) {
  return reason instanceof Error ? reason.message : "Unexpected error";
}
