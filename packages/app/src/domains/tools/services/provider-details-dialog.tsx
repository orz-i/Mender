import type { Provider, ProviderPlugin } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import {
  Dialog,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@mender/ui/components/dialog";

import { formatDateTime, formatNumber, t } from "../../../i18n/i18n";
import {
  MCP_PLUGIN_ID,
  MODEL_TOOLS_PLUGIN_ID,
  OPENAPI_PLUGIN_ID,
  connectionSummary,
  formatHeaders,
  modelToolsEngineSummary,
  modelToolsModelSummary,
  openAPIFilterSummary,
  openAPISecuritySummary,
  openAPIServerSummary,
  openAPISourceSummary,
} from "./provider-form-model";
import { providerIsBuiltIn, providerPluginForProvider, providerPluginLabelForProvider } from "./provider-plugin-selection";

export function ProviderDetailsDialog({
  provider,
  plugins,
  onOpenChange,
}: {
  provider: Provider | null;
  plugins: ProviderPlugin[];
  onOpenChange: (open: boolean) => void;
}) {
  const selectedPlugin =
    provider === null ? undefined : providerPluginForProvider(plugins, provider);
  return (
    <Dialog open={provider !== null} onOpenChange={onOpenChange}>
      <DialogContent className="grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t("Provider details")}</DialogTitle>
          <DialogDescription>
            {t("Identity, connection configuration, and timestamps for this Provider.")}
          </DialogDescription>
        </DialogHeader>

        <DialogBody>
        {provider !== null ? (
          <div className="grid gap-3">
            <DetailRow label="Display name" value={provider.displayName} />
            <DetailRow label="Namespace" value={provider.namespace} mono />
            <DetailRow label="Provider ID" value={provider.providerId} mono />
            <DetailRow
              label="Provider plugin"
              value={providerPluginLabelForProvider(plugins, provider)}
            />
            {provider.pluginId ? (
              <DetailRow label="Plugin ID" value={provider.pluginId} mono />
            ) : null}
            {provider.installationId ? (
              <>
                <DetailRow
                  label="Installation"
                  value={provider.installationId}
                  mono
                />
                <DetailRow
                  label="Package version"
                  value={
                    selectedPlugin?.packageVersion ??
                    provider.pluginVersion ??
                    t("Unavailable")
                  }
                  mono
                />
                <DetailRow
                  label="Configuration"
                  value={JSON.stringify(provider.config ?? {}, null, 2)}
                  mono
                  preserve
                />
              </>
            ) : null}
            {providerIsBuiltIn(provider, MCP_PLUGIN_ID) ? (
              <DetailRow label="MCP transport" value={provider.transport} />
            ) : null}
            {providerIsBuiltIn(provider, MODEL_TOOLS_PLUGIN_ID) ? (
              <>
                <DetailRow
                  label="Engine"
                  value={modelToolsEngineSummary(provider)}
                />
                <DetailRow
                  label="Model"
                  value={modelToolsModelSummary(provider)}
                  mono
                />
                <DetailRow label="Capability" value={t("Web search")} />
              </>
            ) : null}
            {providerIsBuiltIn(provider, OPENAPI_PLUGIN_ID) ? (
              <>
                <DetailRow
                  label="Description source"
                  value={openAPISourceSummary(provider)}
                  mono
                />
                <DetailRow
                  label="Server override"
                  value={openAPIServerSummary(provider)}
                  mono
                />
                <DetailRow
                  label="Operation filters"
                  value={openAPIFilterSummary(provider)}
                  mono
                  preserve
                />
                <DetailRow
                  label="Security Schemes"
                  value={openAPISecuritySummary(provider)}
                  mono
                  preserve
                />
              </>
            ) : null}
            <DetailRow label="Synced Tools" value={formatNumber(provider.toolCount)} />
            <DetailRow
              label="Connection"
              value={connectionSummary(provider)}
              mono
            />
            {provider.transport === "http" &&
            provider.headers !== undefined &&
            Object.keys(provider.headers).length ? (
              <DetailRow
                label="Header templates"
                value={formatHeaders(provider.headers)}
                mono
                preserve
              />
            ) : null}
            {provider.transport === "stdio" && provider.args?.length ? (
              <DetailRow
                label="Arguments"
                value={provider.args.join("\n")}
                mono
                preserve
              />
            ) : null}
            {provider.transport === "stdio" && provider.directory ? (
              <DetailRow
                label="Working directory"
                value={provider.directory}
                mono
              />
            ) : null}
            <DetailRow
              label="Description"
              value={provider.description || "—"}
            />
            <DetailRow
              label="Created"
              value={formatTimestamp(provider.createdAt)}
            />
            <DetailRow
              label="Updated"
              value={formatTimestamp(provider.updatedAt)}
            />
          </div>
        ) : null}
        </DialogBody>

        <DialogFooter>
          <DialogClose render={<Button variant="outline" />}>{t("Close")}</DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function DetailRow({
  label,
  value,
  mono = false,
  preserve = false,
}: {
  label: string;
  value: string;
  mono?: boolean;
  preserve?: boolean;
}) {
  return (
    <div className="grid grid-cols-[130px_minmax(0,1fr)] gap-4 border-b border-border/60 pb-3 last:border-0 last:pb-0">
      <div className="text-sm text-muted-foreground">{t(label)}</div>
      <div
        className={[
          "min-w-0 break-words text-sm",
          mono ? "font-mono text-xs" : "",
          preserve ? "whitespace-pre-wrap" : "",
        ].join(" ")}
      >
        {value}
      </div>
    </div>
  );
}

function formatTimestamp(value: string) {
  return formatDateTime(value);
}
