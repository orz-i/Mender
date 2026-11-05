import { ChevronRightIcon } from "lucide-react";
import { Link, useParams } from "react-router";

import type { ManagementClient } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";

import { t, useLocale } from "../../i18n/i18n";
import { ErrorNotice } from "../../shared/page";
import { usePageHeading } from "../../shell/page-chrome";
import { agentBackendDescription, agentBackendDetails, agentBackendLabel, agentBackendOptions, type AgentBackend } from "./agent-backends";
import { AgentPresetIcon } from "./agent-preset-icon";
import { projectAgentPreset, useAgentPresetCatalog } from "./agent-preset-catalog";

export function AgentBackendPage({ client }: { client: ManagementClient }) {
  useLocale();
  const { backend: value } = useParams();
  const backend: AgentBackend | null = agentBackendOptions.find((item) => item === value) ?? null;
  usePageHeading(backend === null ? undefined : agentBackendLabel(backend));
  const { snapshot, refresh } = useAgentPresetCatalog(client);

  if (backend === null) {
    return <ErrorNotice message={t("Unknown Agent backend plugin.")} />;
  }
  const preset = projectAgentPreset(backend, snapshot);
  const name = agentBackendLabel(backend);
  const configureTo = "/agents/new?pluginId=" +
    encodeURIComponent(agentBackendDetails[backend].pluginId) + "&from=agents";
  const errors = [snapshot.agentError, snapshot.catalogueError].filter((error): error is string => Boolean(error));

  return (
    <div className="min-w-0" data-slot="agent-backend-configuration">
      <div className="mb-5 flex min-w-0 items-center gap-4">
        <AgentPresetIcon backend={backend} className="size-9" />
        <div className="min-w-0 flex-1">
          <h2 className="text-lg font-semibold">{name}</h2>
          <p className="mt-1 text-[13px] leading-5 text-muted-foreground">{agentBackendDescription(backend)}</p>
        </div>
      </div>
      {errors.length ? <div className="mb-4">
        <ErrorNotice message={t("Some Agent information could not be verified.") + " " + errors.join(" · ")} onRetry={refresh} />
      </div> : null}
      {!snapshot.loaded ? <p role="status" className="py-8 text-sm text-muted-foreground">{t("Checking Agent status…")}</p> :
        snapshot.agents === null ? (
          <p className="rounded-xl border border-border bg-card p-5 text-sm text-muted-foreground">
            {t("Configured Agent workspaces could not be loaded. Retry to continue.")}
          </p>
        ) : preset.instances.length ? (
          <section aria-label={t("Configured Agent workspaces")} className="overflow-hidden rounded-xl border border-border bg-card">
            <div className="flex items-center justify-between gap-4 border-b border-border px-5 py-3.5">
              <h3 className="text-sm font-semibold">{t("Configured Agent workspaces")}</h3>
              {preset.canConfigure && snapshot.agents !== null ? <Button size="sm" variant="outline"
                render={<Link to={configureTo} />}>{t("Configure another workspace")}</Button> : null}
            </div>
            <div role="list" className="divide-y divide-border">
              {preset.instances.map((agent) => (
                <div key={agent.agentId} role="listitem">
                  <Link to={"/agents/" + encodeURIComponent(agent.agentId) + "?view=configuration"}
                    className="group flex min-h-[72px] min-w-0 items-center gap-3 px-5 py-3 transition-colors hover:bg-muted/35 focus-visible:outline-2 focus-visible:outline-ring">
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-semibold">{agent.name}</p>
                      <p className="mt-1 truncate text-xs text-muted-foreground" title={agent.workspaceRoot}>{agent.workspaceRoot}</p>
                    </div>
                    <span className="flex shrink-0 items-center gap-1.5 text-xs text-muted-foreground">
                      <span aria-hidden="true" className={["size-2 rounded-full", agent.backendAvailable ? "bg-teal" : "bg-sunlight"].join(" ")} />
                      {t(agent.backendAvailable ? "CLI detected" : "CLI missing")}
                    </span>
                    <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground/50 group-hover:text-foreground" aria-hidden="true" />
                  </Link>
                </div>
              ))}
            </div>
          </section>
        ) : (
          <section className="rounded-xl border border-border bg-card px-5 py-6">
            <h3 className="text-[15px] font-semibold">{preset.state === "unknown" ? t("Status unverified") : preset.canConfigure ? t("Ready to configure") : t("Backend unavailable")}</h3>
            <p className="mt-2 max-w-[680px] text-[13px] leading-6 text-muted-foreground">
              {preset.issue ?? (preset.state === "unknown" ?
                t("Agent backend status could not be verified. Retry before configuring.") : preset.canConfigure ?
                t("Choose a workspace to set up governed delegation for this preset.") :
                t("This Agent backend is not available on the current Runtime. Review its plugin and CLI availability before configuring."))}
            </p>
            {preset.canConfigure ? (
              <Button size="sm" className="mt-4" render={<Link to={configureTo} />}>{t("Configure workspace")}</Button>
            ) : null}
          </section>
        )}
    </div>
  );
}
