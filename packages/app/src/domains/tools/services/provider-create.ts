import { useState } from "react";
import type { ManagementClient, ProviderPlugin } from "@mender/sdk";

import { canonicalizeOwnedIdentifier, isCanonicalOwnedSlug, ownedIdentifierCreateError } from "../../../shared/form-identifiers";
import { genericConfigValidationError } from "../../../shared/plugin-configuration";
import { type ProviderFormState, configFromForm, emptyProviderForm, providerRenderer } from "./provider-form-model";
import { providerFormSelectionKey, providerPluginForForm } from "./provider-plugin-selection";

export function useProviderCreation({
  client,
  plugins,
  busy,
  onBusyChange,
  onCreated,
  refresh,
}: {
  client: ManagementClient;
  plugins: ProviderPlugin[];
  busy: string;
  onBusyChange: (operation: string) => void;
  onCreated: () => void;
  refresh: () => Promise<unknown>;
}) {
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState(emptyProviderForm);
  const [draftError, setDraftError] = useState<string | null>(null);
  const [submitError, setSubmitError] = useState("");

  function change(next: ProviderFormState) {
    setSubmitError("");
    if (providerFormSelectionKey(next) !== providerFormSelectionKey(form)) setDraftError(null);
    setForm(next);
  }

  function reset() {
    setSubmitError("");
    setDraftError(null);
    setForm(emptyProviderForm);
  }

  async function submit() {
    const namespace = canonicalizeOwnedIdentifier(form.namespace);
    const schema = providerPluginForForm(plugins, form)?.configSchema;
    if (
      !isCanonicalOwnedSlug(namespace) || busy !== "" ||
      (providerRenderer(form) === "generic" &&
        (draftError !== null || genericConfigValidationError(schema, form.genericConfig) !== null))
    ) return;

    onBusyChange("create");
    setSubmitError("");
    try {
      await client.createProvider({
        namespace,
        displayName: form.displayName.trim(),
        pluginId: form.pluginId,
        ...(form.installationId ? { installationId: form.installationId } : {}),
        config: configFromForm(form),
        ...(form.description ? { description: form.description } : {}),
      });
      setOpen(false);
      reset();
      onCreated();
      await refresh();
    } catch (reason) {
      // Another window may have created the same name. An uncertain response
      // is never evidence of success.
      setSubmitError(ownedIdentifierCreateError(reason, "provider"));
      await refresh();
    } finally {
      onBusyChange("");
    }
  }

  // The page coordinates deep links and temporarily hands the form to the
  // authentication dialog. The create flow owns validation and submission.
  return { open, setOpen, form, setForm, draftError, setDraftError, submitError, change, reset, submit };
}
