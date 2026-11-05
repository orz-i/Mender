import { useState } from "react";

import type { Credential, ProviderPlugin } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@mender/ui/components/dialog";
import { Input } from "@mender/ui/components/input";
import { Textarea } from "@mender/ui/components/textarea";

import { t } from "../../../i18n/i18n";
import { errorMessage } from "../../../shared/errors";
import { canonicalizeOwnedIdentifier, isCanonicalOwnedSlug } from "../../../shared/form-identifiers";
import { FormHeading } from "../../../shared/form-info";
import { PluginConfigForm, genericConfigValidationError } from "../../../shared/plugin-configuration";
import { Field, ReadOnlyField } from "./provider-fields";
import {
  type ProviderFormState,
  formatHeaders,
  modelToolsEngineLabel,
  modelToolsModelPlaceholder,
  parseMCPJSON,
  providerRenderer,
} from "./provider-form-model";
import {
  providerFormSelectionKey,
  providerPluginChoiceLabel,
  providerPluginForForm,
} from "./provider-plugin-selection";

export function ProviderConfigurationDialog({
  open,
  mode,
  form,
  plugins,
  credentials,
  busy,
  createError,
  editError,
  draftError,
  onDraftErrorChange,
  onOpenChange,
  onChange,
  onAuthentication,
  onSubmit,
}: {
  open: boolean;
  mode: "create" | "edit";
  form: ProviderFormState;
  plugins: ProviderPlugin[];
  credentials: Credential[];
  busy: boolean;
  createError?: string;
  editError?: string;
  draftError: string | null;
  onDraftErrorChange: (error: string | null) => void;
  onOpenChange: (open: boolean) => void;
  onChange: (value: ProviderFormState) => void;
  onAuthentication?: () => void;
  onSubmit: () => void;
}) {
  const [importError, setImportError] = useState("");
  const creating = mode === "create";
  const selectedPlugin = providerPluginForForm(plugins, form);
  const renderer = providerRenderer(form);
  const canonicalNamespace = canonicalizeOwnedIdentifier(form.namespace);
  const namespaceValid = isCanonicalOwnedSlug(canonicalNamespace);
  const genericError =
    renderer === "generic"
      ? genericConfigValidationError(
          selectedPlugin?.configSchema,
          form.genericConfig,
        )
      : null;
  const valid =
    form.displayName.trim() !== "" &&
    new TextEncoder().encode(form.displayName.trim()).length <= 160 &&
    new TextEncoder().encode(form.description.trim()).length <= 4096 &&
    (!creating || namespaceValid) &&
    (renderer === "mcp"
      ? form.transport === "http"
        ? form.endpoint.trim() !== ""
        : form.executable.trim() !== ""
      : renderer === "openapi"
        ? form.openapiSourceKind === "url"
          ? form.openapiSpecUrl.trim() !== ""
          : form.openapiSpecFile.trim() !== ""
        : renderer === "model-tools"
          ? form.modelToolsModel.trim() !== "" &&
            form.modelToolsCredentialRef.trim() !== ""
          : selectedPlugin !== undefined && genericError === null && draftError === null);

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) {
          setImportError("");
        }
        onOpenChange(nextOpen);
      }}
    >
      <DialogContent className="max-h-[calc(100dvh-2rem)] grid-rows-[auto_minmax(0,1fr)_auto] sm:max-w-xl">
        <div className="grid gap-3">
          <DialogHeader>
          <DialogTitle>
            {creating
              ? selectedPlugin !== undefined
                ? t("Add {{plugin}} provider", {
                    plugin: providerPluginChoiceLabel(selectedPlugin),
                  })
                : t("Add provider")
              : t("Edit provider")}
          </DialogTitle>
          <DialogDescription className="sr-only">
            {creating
              ? selectedPlugin !== undefined
                ? t(
                    "Configure {{plugin}}. This form is scoped to the selected Provider plugin.",
                    {
                      plugin: providerPluginChoiceLabel(selectedPlugin),
                    },
                  )
                : t("The requested item was not found.")
              : t(
                  "Update mutable metadata and plugin connection settings. Provider identity and plugin remain stable.",
                )}
          </DialogDescription>
          </DialogHeader>
          {(creating ? createError : editError) ? (
            <div role="alert" className="rounded-xl border border-destructive/20 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {creating ? createError : editError}
            </div>
          ) : null}
        </div>

        <div className="min-h-0 space-y-4 overflow-y-auto pr-1">
          {!creating ? (
            <div className="grid gap-3 sm:grid-cols-2">
              <ReadOnlyField label="Namespace" value={form.namespace} />
              <ReadOnlyField
                label="Provider plugin"
                value={
                  selectedPlugin !== undefined
                    ? providerPluginChoiceLabel(selectedPlugin)
                    : form.pluginId
                }
              />
            </div>
          ) : null}

          {renderer === "mcp" ? (
            <div>
              <div className="mb-2 text-sm font-medium">{t("MCP connection")}</div>
              <div className="grid grid-cols-2 gap-2">
                <Button
                  type="button"
                  variant={form.transport === "http" ? "default" : "outline"}
                  aria-pressed={form.transport === "http"}
                  onClick={() => onChange({ ...form, transport: "http" })}
                >
                  {t("Remote endpoint")}
                </Button>
                <Button
                  type="button"
                  variant={form.transport === "stdio" ? "default" : "outline"}
                  aria-pressed={form.transport === "stdio"}
                  onClick={() => onChange({ ...form, transport: "stdio" })}
                >
                  {t("Local command")}
                </Button>
              </div>
            </div>
          ) : renderer === "model-tools" ? (
            <div>
              <div className="mb-2 text-sm font-medium">{t("Model tool engine")}</div>
              <div className="grid grid-cols-3 gap-2">
                {(["openai", "anthropic", "gemini"] as const).map(
                  (engine) => (
                    <Button
                      key={engine}
                      type="button"
                      variant={
                        form.modelToolsEngine === engine ? "default" : "outline"
                      }
                      aria-pressed={form.modelToolsEngine === engine}
                      onClick={() =>
                        onChange({ ...form, modelToolsEngine: engine })
                      }
                    >
                      {modelToolsEngineLabel(engine)}
                    </Button>
                  ),
                )}
              </div>
            </div>
          ) : renderer === "openapi" ? (
            <div>
              <div className="mb-2 text-sm font-medium">{t("OpenAPI description")}</div>
              <div className="grid grid-cols-2 gap-2">
                <Button
                  type="button"
                  variant={
                    form.openapiSourceKind === "url" ? "default" : "outline"
                  }
                  aria-pressed={form.openapiSourceKind === "url"}
                  onClick={() =>
                    onChange({ ...form, openapiSourceKind: "url" })
                  }
                >
                  {t("Remote URL")}
                </Button>
                <Button
                  type="button"
                  variant={
                    form.openapiSourceKind === "file" ? "default" : "outline"
                  }
                  aria-pressed={form.openapiSourceKind === "file"}
                  onClick={() =>
                    onChange({ ...form, openapiSourceKind: "file" })
                  }
                >
                  {t("Local file")}
                </Button>
              </div>
            </div>
          ) : (
            <FormHeading label={t("Schema-driven configuration")} help={t(
              selectedPlugin?.sourceKind === "external"
                ? "This Provider is pinned to an installed external plugin package."
                : "This plugin uses Mender's generic configuration renderer.",
            )} />
          )}

          {creating ? (
            <Field label="Namespace" help={t("Uppercase letters and spaces become a lowercase hyphenated identifier.")}>
              <Input
                aria-label={t("Namespace")}
                placeholder={t("namespace")}
                value={form.namespace}
                aria-invalid={form.namespace.trim() !== "" && !namespaceValid}
                onChange={(event) =>
                  onChange({ ...form, namespace: event.target.value })
                }
              />
              {form.namespace.trim() !== "" ? (
                <p className={namespaceValid ? "mt-1 break-all font-mono text-xs" : "mt-1 text-xs text-destructive"}>
                  {namespaceValid
                    ? t("Result: {{value}}", { value: canonicalNamespace })
                    : t("Use 1–63 characters, starting with a letter and ending with a letter or digit; only lowercase letters, digits and hyphens are allowed.")}
                </p>
              ) : null}
            </Field>
          ) : null}

          <Field label="Display name">
            <Input
              aria-label={t("Display name")}
              value={form.displayName}
              aria-invalid={form.displayName.trim() !== "" && new TextEncoder().encode(form.displayName.trim()).length > 160}
              onChange={(event) =>
                onChange({ ...form, displayName: event.target.value })
              }
            />
            {form.displayName.trim() !== "" && new TextEncoder().encode(form.displayName.trim()).length > 160 ? (
              <p className="mt-1 text-xs text-destructive">{t("Display name is too long (maximum 160 bytes).")}</p>
            ) : null}
          </Field>

          <Field label="Description" optional>
            <Input
              aria-label={t("Description")}
              value={form.description}
              aria-invalid={new TextEncoder().encode(form.description.trim()).length > 4096}
              onChange={(event) =>
                onChange({ ...form, description: event.target.value })
              }
            />
            {new TextEncoder().encode(form.description.trim()).length > 4096 ? (
              <p className="mt-1 text-xs text-destructive">{t("Description is too long (maximum 4096 bytes).")}</p>
            ) : null}
          </Field>

          {renderer === "generic" ? (
            <div className="space-y-3">
              {form.installationId ? (
                <details className="rounded-xl border border-border/70">
                  <summary className="cursor-pointer px-3 py-2 text-sm font-medium">
                    {t("Installation details")}
                  </summary>
                  <div className="grid gap-3 border-t border-border/70 p-3 sm:grid-cols-2">
                    <ReadOnlyField label="Installation" value={form.installationId} />
                    <ReadOnlyField
                      label="Package version"
                      value={selectedPlugin?.packageVersion ?? t("Unavailable")}
                    />
                  </div>
                </details>
              ) : null}
              <PluginConfigForm
                rawObjectLabel={t("Provider configuration")}
                rawObjectTypeError={t("Provider configuration must be a JSON object.")}
                key={providerFormSelectionKey(form)}
                schema={selectedPlugin?.configSchema}
                uiSchema={selectedPlugin?.uiSchema}
                value={form.genericConfig}
                credentials={credentials}
                onDraftErrorChange={onDraftErrorChange}
                onChange={(genericConfig) =>
                  onChange({ ...form, genericConfig })
                }
              />
              {genericError ? (
                <p role="alert" className="text-xs text-destructive">{genericError}</p>
              ) : null}
            </div>
          ) : null}

          {renderer === "model-tools" ? (
            <>
              <Field label="Model" help={t("Mender exposes one stable web_search Tool. The selected model must execute the provider-hosted search tool; model-only answers are rejected.")}>
                <Input
                  aria-label={t("Model Tools model")}
                  placeholder={modelToolsModelPlaceholder(form.modelToolsEngine)}
                  value={form.modelToolsModel}
                  onChange={(event) =>
                    onChange({ ...form, modelToolsModel: event.target.value })
                  }
                />
              </Field>
              <ReadOnlyField label="Capability" value={t("Web search")} />
            </>
          ) : null}

          {renderer === "mcp" ? (
            form.transport === "http" ? (
            <>
              <Field label="Endpoint" help={t("Non-loopback endpoints require HTTPS.")}>
                <Input
                  aria-label={t("Endpoint")}
                  placeholder={t("https://example.com/mcp")}
                  value={form.endpoint}
                  onChange={(event) =>
                    onChange({ ...form, endpoint: event.target.value })
                  }
                />
              </Field>

              <details className="rounded-lg border border-border">
                <summary className="cursor-pointer select-none px-4 py-3 text-sm font-medium">
                  {t("Advanced settings")}
                </summary>
                <div className="space-y-4 border-t border-border px-4 py-4">
                  <ReadOnlyField
                    label="MCP type"
                    value="streamable-http"
                  />

                  <Field label="Legacy HTTP header templates" optional>
                    <Textarea
                      aria-label={t("HTTP header templates")}
                      className="min-h-24 font-mono text-xs"
                      placeholder={
                        "Authorization: Bearer ${ANYSEARCH_API_KEY}\nX-Anysearch-Client: mcp/1.0.0"
                      }
                      value={form.headers}
                      onChange={(event) =>
                        onChange({ ...form, headers: event.target.value })
                      }
                    />
                    <p className="mt-1 text-xs leading-5 text-muted-foreground">
                      {t("Compatibility path only. Sensitive headers must use environment references such as")} <code>{"${ANYSEARCH_API_KEY}"}</code>{t(". Prefer the separate Authentication workflow for inventory-backed, Profile-scoped Credentials.")}
                    </p>
                  </Field>

                  {creating ? (
                    <div className="rounded-lg bg-muted/40 p-3">
                      <Field label="Import MCP JSON" optional>
                        <Textarea
                          aria-label={t("MCP JSON")}
                          className="min-h-28 font-mono text-xs"
                          placeholder={
                            '{"mcpServers":{"anysearch":{"type":"streamable-http","url":"https://api.anysearch.com/mcp","headers":{"Authorization":"Bearer ${ANYSEARCH_API_KEY}"}}}}'
                          }
                          value={form.importJson}
                          onChange={(event) =>
                            onChange({ ...form, importJson: event.target.value })
                          }
                        />
                      </Field>
                      {importError ? (
                        <p className="mt-2 text-xs text-destructive">
                          {importError}
                        </p>
                      ) : null}
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        className="mt-3"
                        disabled={!form.importJson.trim()}
                        onClick={() => {
                          try {
                            const imported = parseMCPJSON(form.importJson);
                            onChange({
                              ...form,
                              transport: "http",
                              namespace: imported.namespace,
                              displayName:
                                form.displayName || imported.namespace,
                              endpoint: imported.endpoint,
                              headers: formatHeaders(imported.headers),
                            });
                            setImportError("");
                          } catch (reason) {
                            setImportError(errorMessage(reason));
                          }
                        }}
                      >
                        {t("Import configuration")}
                      </Button>
                    </div>
                  ) : null}
                </div>
              </details>
            </>
          ) : (
            <>
              <Field label="Executable">
                <Input
                  aria-label={t("Executable")}
                  placeholder={t("/path/to/mcp-server")}
                  value={form.executable}
                  onChange={(event) =>
                    onChange({ ...form, executable: event.target.value })
                  }
                />
              </Field>
              <Field label="Arguments" optional>
                <Textarea
                  aria-label={t("Arguments")}
                  placeholder={"One argument per line\n--stdio"}
                  value={form.args}
                  onChange={(event) =>
                    onChange({ ...form, args: event.target.value })
                  }
                />
              </Field>
              <Field label="Working directory" optional>
                <Input
                  aria-label={t("Working directory")}
                  value={form.directory}
                  onChange={(event) =>
                    onChange({ ...form, directory: event.target.value })
                  }
                />
              </Field>
              <p className="text-xs text-muted-foreground">
                {t("Mender does not persist environment values or secrets for stdio Providers.")}
              </p>
            </>
            )
          ) : null}

          {renderer === "openapi" ? (
            <>
              {form.openapiSourceKind === "url" ? (
                <Field label="Description URL" help={t("Non-loopback URLs require HTTPS. External $ref retrieval is disabled.")}>
                  <Input
                    aria-label={t("OpenAPI description URL")}
                    placeholder={t("https://example.com/openapi.yaml")}
                    value={form.openapiSpecUrl}
                    onChange={(event) =>
                      onChange({ ...form, openapiSpecUrl: event.target.value })
                    }
                  />
                </Field>
              ) : (
                <Field label="Description file" help={t("Use an absolute local path. External file references are disabled.")}>
                  <Input
                    aria-label={t("OpenAPI description file")}
                    placeholder={t("/absolute/path/to/openapi.yaml")}
                    value={form.openapiSpecFile}
                    onChange={(event) =>
                      onChange({ ...form, openapiSpecFile: event.target.value })
                    }
                  />
                </Field>
              )}

              <Field label="API server override" optional>
                <Input
                  aria-label={t("OpenAPI server override")}
                  placeholder={t("https://api.example.com")}
                  value={form.openapiServerUrl}
                  onChange={(event) =>
                    onChange({ ...form, openapiServerUrl: event.target.value })
                  }
                />
              </Field>

              <details className="rounded-lg border border-border">
                <summary className="cursor-pointer select-none px-4 py-3 text-sm font-medium">
                  {t("Operations")}
                </summary>
                <div className="space-y-4 border-t border-border px-4 py-4">
                  <p className="text-xs leading-5 text-muted-foreground">
                    {t("Filters use stable OpenAPI operation SourceKeys. Use operationId when present; otherwise use values such as")} <code>get /users/{"{id}"}</code>.
                  </p>
                  <Field label="Include SourceKeys" optional>
                    <Textarea
                      aria-label={t("OpenAPI include SourceKeys")}
                      className="min-h-24 font-mono text-xs"
                      placeholder={"One SourceKey per line\ngetUser"}
                      value={form.openapiInclude}
                      onChange={(event) =>
                        onChange({ ...form, openapiInclude: event.target.value })
                      }
                    />
                  </Field>
                  <Field label="Exclude SourceKeys" optional>
                    <Textarea
                      aria-label={t("OpenAPI exclude SourceKeys")}
                      className="min-h-24 font-mono text-xs"
                      placeholder={"One SourceKey per line\ndeleteUser"}
                      value={form.openapiExclude}
                      onChange={(event) =>
                        onChange({ ...form, openapiExclude: event.target.value })
                      }
                    />
                  </Field>
                </div>
              </details>
            </>
          ) : null}
        </div>

        <DialogFooter>
          <DialogClose render={<Button variant="outline" />}>
            {t("Cancel")}
          </DialogClose>
          {onAuthentication !== undefined && renderer !== "generic" ? (
            <Button
              type="button"
              variant="outline"
              disabled={busy}
              onClick={onAuthentication}
            >
              {t("Configure authentication")}
            </Button>
          ) : null}
          <Button disabled={busy || !valid} onClick={() => {
            if (!busy && valid) onSubmit();
          }}>
            {t(
              busy
                ? creating
                  ? "Adding…"
                  : "Saving…"
                : creating
                  ? "Add provider"
                  : "Save changes",
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
