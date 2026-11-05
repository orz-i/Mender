import { useState } from "react";
import type { Credential, ManagementClient, Provider, ProviderPlugin } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogBody,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@mender/ui/components/dialog";

import { t } from "../../../i18n/i18n";
import { errorMessage } from "../../../shared/errors";
import {
  PluginConfigForm,
  genericConfigDefaults,
  genericConfigValidationError,
} from "../../../shared/plugin-configuration";
import { Field, ReadOnlyField } from "./provider-fields";
import { asRecord } from "./provider-form-model";
import { providerPluginSelectionKey } from "./provider-plugin-selection";

export function ProviderRebindDialog({
  client,
  provider,
  candidates,
  initialPlugin,
  credentials,
  busy,
  onBusyChange,
  onClose,
  onSave,
}: {
  client: ManagementClient;
  provider: Provider;
  candidates: ProviderPlugin[];
  initialPlugin: ProviderPlugin;
  credentials: Credential[];
  busy: string;
  onBusyChange: (providerId: string) => void;
  onClose: () => void;
  onSave: () => Promise<void>;
}) {
  const [plugin, setPlugin] = useState(initialPlugin);
  const [config, setConfig] = useState(() => {
    const current = asRecord(provider.config);
    return current === null
      ? genericConfigDefaults(initialPlugin.configSchema)
      : JSON.parse(JSON.stringify(current)) as Record<string, unknown>;
  });
  const [draftError, setDraftError] = useState<string | null>(null);
  const [submitError, setSubmitError] = useState("");
  const validationError = genericConfigValidationError(plugin.configSchema, config);
  const selectedInstallationId = plugin.installationId ?? "";

  async function submit() {
    if (plugin.installationId === undefined || draftError !== null ||
        validationError !== null || busy !== "") return;

    onBusyChange(provider.providerId);
    setSubmitError("");
    try {
      await client.rebindProvider(provider.providerId, {
        installationId: plugin.installationId,
        config: JSON.parse(JSON.stringify(config)) as Record<string, unknown>,
      });
      await onSave();
    } catch (reason) {
      setSubmitError(errorMessage(reason));
    } finally {
      onBusyChange("");
    }
  }

  return (
    <Dialog open onOpenChange={(open) => { if (!open && busy === "") onClose(); }}>
      <DialogContent className="grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t("Rebind Provider installation")}</DialogTitle>
          <DialogDescription className="sr-only">
            {t("Move this Provider to another exact installation of the same plugin. Provider identity and SourceKey-stable Tool identities remain unchanged.")}
          </DialogDescription>
          {submitError ? (
            <p role="alert" className="text-xs leading-5 text-destructive">{submitError}</p>
          ) : null}
        </DialogHeader>

        <DialogBody className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-2">
            <ReadOnlyField label="Provider" value={provider.namespace} />
            <ReadOnlyField
              label="Current installation"
              value={provider.installationId ?? t("Unavailable")}
            />
          </div>

          <Field label="Target installation">
            <select
              aria-label={t("Target Provider plugin installation")}
              value={selectedInstallationId}
              onChange={(event) => {
                const selected = candidates.find(
                  (candidate) =>
                    candidate.installationId === event.target.value,
                );
                if (selected !== undefined) {
                  setPlugin(selected);
                  setDraftError(null);
                  setSubmitError("");
                }
              }}
              className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus:ring-2 focus:ring-ring/40"
            >
              {candidates.map((candidate) => (
                <option
                  key={providerPluginSelectionKey(candidate)}
                  value={candidate.installationId}
                >
                  {candidate.packageVersion + " · " + candidate.installationId}
                </option>
              ))}
            </select>
          </Field>

          <div className="grid gap-3 sm:grid-cols-2">
            <ReadOnlyField label="Plugin" value={plugin.pluginId} />
            <ReadOnlyField
              label="Target package version"
              value={plugin.packageVersion}
            />
          </div>

          <div className="rounded-lg border border-border bg-muted/30 p-3">
            <div className="text-sm font-medium">{t("Target configuration")}</div>
            <p className="mt-1 text-xs leading-5 text-muted-foreground">
              {t("The current configuration is carried forward for review. Update it when the target package requires a different configuration contract.")}
            </p>
          </div>

          <PluginConfigForm
            rawObjectLabel={t("Provider configuration")}
            rawObjectTypeError={t("Provider configuration must be a JSON object.")}
            key={providerPluginSelectionKey(plugin)}
            schema={plugin.configSchema}
            uiSchema={plugin.uiSchema}
            value={config}
            credentials={credentials}
            onChange={(next) => { setConfig(next); setSubmitError(""); }}
            onDraftErrorChange={setDraftError}
          />
          {validationError ? (
            <p role="alert" className="text-xs text-destructive">{validationError}</p>
          ) : null}
        </DialogBody>

        <DialogFooter>
          <DialogClose render={<Button variant="outline" />}>
            {t("Cancel")}
          </DialogClose>
          <Button
            disabled={busy !== "" || validationError !== null || draftError !== null}
            onClick={() => void submit()}
          >
            {t(busy === provider.providerId ? "Rebinding…" : "Rebind installation")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
