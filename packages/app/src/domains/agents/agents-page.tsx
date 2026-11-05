import { ChevronRightIcon } from "lucide-react";
import { Link } from "react-router";

import type { ManagementClient } from "@mender/sdk";

import { formatNumber, t, useLocale } from "../../i18n/i18n";
import { ErrorNotice } from "../../shared/page";
import { agentBackendLabel, agentBackendOptions, agentBackendSummary } from "./agent-backends";
import { useAgentPresetCatalog, projectAgentPreset, type AgentPresetEntry } from "./agent-preset-catalog";
import { AgentPresetIcon } from "./agent-preset-icon";

function presetStatus(entry: AgentPresetEntry) {
  switch (entry.state) {
    case "configured": return { text: t("Configured · CLI detected"), color: "bg-teal" };
    case "configured-unavailable": return { text: t("Configured · CLI unavailable"), color: "bg-sunlight" };
    case "ready": return { text: t("Ready to configure"), color: "bg-teal" };
    case "unavailable": return { text: t("Backend unavailable"), color: "bg-sunlight" };
    case "unknown": return { text: t("Status unverified"), color: "bg-muted-foreground/70" };
  }
}

function contextText(entry: AgentPresetEntry): string | null {
  if (entry.configuredCount === null) return null;
  if (entry.configuredCount > 0) {
    return t("{{count}} configured workspaces", { count: formatNumber(entry.configuredCount) });
  }
  return entry.state === "ready" ? t("No workspace configured yet") : null;
}

export function AgentsPage({ client }: { client: ManagementClient }) {
  useLocale();
  const { snapshot, refresh } = useAgentPresetCatalog(client);
  const entries = agentBackendOptions.map((backend) => projectAgentPreset(backend, snapshot));
  const errors = [snapshot.agentError, snapshot.catalogueError].filter((error): error is string => Boolean(error));

  return (
    <div className="min-w-0" data-slot="agent-presets">
      <p className="mb-5 text-[13px] leading-6 text-muted-foreground">
        {t("Choose a built-in Agent to configure its workspace, delegated tasks, and permissions.")}
      </p>
      {errors.length ? (
        <div className="mb-3">
          <ErrorNotice message={t("Some Agent information could not be verified.") + " " + errors.join(" · ")} onRetry={refresh} />
        </div>
      ) : null}
      <section aria-label={t("Available Agent backends")}>
        <div role="list" aria-label={t("Available Agent backends")}
          className="divide-y divide-border overflow-hidden rounded-2xl border border-border bg-card">
          {entries.map((entry) => {
            const status = presetStatus(entry);
            const backendName = agentBackendLabel(entry.backend);
            const countText = contextText(entry);
            return (
              <div key={entry.backend} role="listitem" className="min-w-0">
                <Link to={entry.destination} aria-label={t("Configure {{agent}}", { agent: backendName })}
                  className="group flex min-h-[86px] min-w-0 items-center gap-3 px-4 py-4 text-left transition-colors hover:bg-muted/35 focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-ring sm:gap-5 sm:px-6">
                  <span className="flex size-9 shrink-0 items-center justify-center">
                    <AgentPresetIcon backend={entry.backend} />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-[15px] font-semibold leading-5 tracking-[-0.015em]">{backendName}</span>
                    <span className="mt-1.5 flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-1 text-[12px] leading-[18px]">
                      <span className="inline-flex shrink-0 items-center gap-1.5 text-muted-foreground">
                        <span className={["size-[7px] rounded-full", status.color].join(" ")} aria-hidden="true" />
                        {status.text}
                      </span>
                      <span className="hidden text-muted-foreground/40 sm:inline" aria-hidden="true">·</span>
                      <span className="min-w-0 truncate text-muted-foreground" title={agentBackendSummary(entry.backend)}>
                        {agentBackendSummary(entry.backend)}
                      </span>
                    </span>
                  </span>
                  {countText ? (
                    <span className="hidden shrink-0 text-[12px] text-muted-foreground sm:inline" aria-label={countText}>
                      {countText}
                    </span>
                  ) : null}
                  <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground/60 transition-transform group-hover:translate-x-0.5 group-hover:text-foreground" aria-hidden="true" />
                </Link>
              </div>
            );
          })}
        </div>
        {!snapshot.loaded ? (
          <p role="status" className="mt-3 text-xs text-muted-foreground">{t("Checking Agent status…")}</p>
        ) : null}
      </section>
    </div>
  );
}
