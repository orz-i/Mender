import { useState } from "react";
import type { Credential, ManagementClient, Provider, ProviderPlugin } from "@mender/sdk";

import { errorMessage } from "../../../shared/errors";
import { genericConfigValidationError } from "../../../shared/plugin-configuration";
import { ProviderConfigurationDialog } from "./provider-configuration-dialog";
import { formFromProvider, providerRenderer, updateRequestFromForm } from "./provider-form-model";
import { providerPluginForForm } from "./provider-plugin-selection";

export function ProviderEditDialog({
  client,
  provider,
  plugins,
  credentials,
  busy,
  onBusyChange,
  onClose,
  refresh,
}: {
  client: ManagementClient;
  provider: Provider;
  plugins: ProviderPlugin[];
  credentials: Credential[];
  busy: string;
  onBusyChange: (providerId: string) => void;
  onClose: () => void;
  refresh: () => Promise<unknown>;
}) {
  const [form, setForm] = useState(() => formFromProvider(provider));
  const [draftError, setDraftError] = useState<string | null>(null);
  const [submitError, setSubmitError] = useState("");

  async function save() {
    if (busy !== "") return;
    const schema = providerPluginForForm(plugins, form)?.configSchema;
    if (providerRenderer(form) === "generic" &&
        (draftError !== null || genericConfigValidationError(schema, form.genericConfig) !== null)) return;

    onBusyChange(provider.providerId);
    setSubmitError("");
    try {
      await client.updateProvider(provider.providerId, updateRequestFromForm(form));
      onClose();
      await refresh();
    } catch (reason) {
      const message = errorMessage(reason);
      await refresh();
      setSubmitError(message);
    } finally {
      onBusyChange("");
    }
  }

  return (
    <ProviderConfigurationDialog
      open
      mode="edit"
      form={form}
      plugins={plugins}
      credentials={credentials}
      busy={busy === provider.providerId}
      editError={submitError}
      draftError={draftError}
      onDraftErrorChange={setDraftError}
      onOpenChange={(open) => { if (!open && busy === "") onClose(); }}
      onChange={(next) => { setSubmitError(""); setForm(next); }}
      onSubmit={() => void save()}
    />
  );
}
