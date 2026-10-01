import { useCallback, useEffect, useState } from "react";
import {
  EyeIcon,
  KeyRoundIcon,
  Link2Icon,
  PencilIcon,
  RefreshCwIcon,
  Trash2Icon,
} from "lucide-react";

import type {
  Credential,
  MenderClient,
  Provider,
  ProviderPlugin,
  ProviderPluginIssue,
  UpdateProviderRequest,
} from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@mender/ui/components/table";
import { Textarea } from "@mender/ui/components/textarea";

import { errorMessage } from "../../shared/errors";
import { IconButton } from "../../shared/icon-button";
import { EmptyState, ErrorNotice, PageHeader } from "../../shared/page";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import {
  DataPanel,
  EmptyPanelState,
} from "../../shared/workbench";
import { GenericProviderConfigForm } from "./generic-provider-config-form";
import {
  genericConfigDefaults,
  genericConfigValidationError,
} from "./generic-provider-config-schema";

import { formatDateTime, formatNumber, t, tCount } from "../../i18n/i18n";

const MCP_PLUGIN_ID = "io.mender.provider.mcp";
const MODEL_TOOLS_PLUGIN_ID = "io.mender.provider.model-tools";
const OPENAPI_PLUGIN_ID = "io.mender.provider.openapi";
const WORKSPACE_PLUGIN_ID = "io.mender.provider.workspace";

type ProviderTransport = "http" | "stdio";
type ModelToolsEngine = "openai" | "anthropic" | "gemini";
type OpenAPISourceKind = "url" | "file";
type CredentialHeaderBinding = NonNullable<Provider["credentialHeaders"]>[number];
type CredentialEnvironmentBinding = NonNullable<
  Provider["credentialEnvironment"]
>[number];

interface OpenAPICredentialBinding {
  schemeName: string;
  credentialRef: string;
}

interface ProviderFormState {
  pluginId: string;
  installationId: string;
  genericConfig: Record<string, unknown>;
  transport: ProviderTransport;
  namespace: string;
  displayName: string;
  description: string;
  endpoint: string;
  headers: string;
  credentialHeaders: CredentialHeaderBinding[];
  importJson: string;
  executable: string;
  args: string;
  directory: string;
  credentialEnvironment: CredentialEnvironmentBinding[];
  openapiSourceKind: OpenAPISourceKind;
  openapiSpecUrl: string;
  openapiSpecFile: string;
  openapiServerUrl: string;
  openapiInclude: string;
  openapiExclude: string;
  openapiCredentials: OpenAPICredentialBinding[];
  modelToolsEngine: ModelToolsEngine;
  modelToolsModel: string;
  modelToolsCredentialRef: string;
}

type ProviderRenderer = "mcp" | "model-tools" | "openapi" | "generic";
type FirstPartyProviderRenderer = Exclude<ProviderRenderer, "generic">;

const FIRST_PARTY_PROVIDER_RENDERERS: Partial<
  Record<string, FirstPartyProviderRenderer>
> = {
  [MCP_PLUGIN_ID]: "mcp",
  [MODEL_TOOLS_PLUGIN_ID]: "model-tools",
  [OPENAPI_PLUGIN_ID]: "openapi",
};

function providerRenderer(form: ProviderFormState): ProviderRenderer {
  if (form.installationId) {
    return "generic";
  }
  return FIRST_PARTY_PROVIDER_RENDERERS[form.pluginId] ?? "generic";
}

function providerPluginSelectionKey(plugin: ProviderPlugin) {
  return plugin.installationId
    ? plugin.pluginId + "@" + plugin.installationId
    : plugin.pluginId + "@builtin";
}

function providerFormSelectionKey(form: ProviderFormState) {
  return form.installationId
    ? form.pluginId + "@" + form.installationId
    : form.pluginId + "@builtin";
}

function providerPluginMatchesForm(
  plugin: ProviderPlugin,
  form: ProviderFormState,
) {
  return (
    plugin.pluginId === form.pluginId &&
    (plugin.installationId ?? "") === form.installationId
  );
}

function providerPluginForForm(
  plugins: ProviderPlugin[],
  form: ProviderFormState,
) {
  return plugins.find((plugin) => providerPluginMatchesForm(plugin, form));
}

function providerPluginChoiceLabel(plugin: ProviderPlugin) {
  return plugin.installationId
    ? plugin.displayName + " · " + plugin.packageVersion
    : plugin.displayName;
}

function providerPluginForProvider(
  plugins: ProviderPlugin[],
  provider: Provider,
) {
  return plugins.find(
    (plugin) =>
      plugin.pluginId === provider.pluginId &&
      (plugin.installationId ?? "") === (provider.installationId ?? ""),
  );
}

function providerPluginLabelForProvider(
  plugins: ProviderPlugin[],
  provider: Provider,
) {
  const plugin = providerPluginForProvider(plugins, provider);
  if (plugin !== undefined) {
    return providerPluginChoiceLabel(plugin);
  }
  if (provider.pluginDisplayName) {
    return provider.pluginDisplayName;
  }
  switch (provider.pluginId) {
    case MCP_PLUGIN_ID:
      return t("MCP");
    case MODEL_TOOLS_PLUGIN_ID:
      return "Model Tools";
    case OPENAPI_PLUGIN_ID:
      return "OpenAPI";
    case WORKSPACE_PLUGIN_ID:
      return t("Workspace");
    default:
      return t("Unconfigured");
  }
}

function providerIsBuiltIn(provider: Provider, pluginId: string) {
  return provider.installationId === undefined && provider.pluginId === pluginId;
}

function providerRebindCandidates(
  plugins: ProviderPlugin[],
  provider: Provider,
) {
  if (!provider.installationId || !provider.pluginId) {
    return [];
  }
  return plugins.filter(
    (plugin) =>
      plugin.sourceKind === "external" &&
      plugin.pluginId === provider.pluginId &&
      plugin.installationId !== undefined &&
      plugin.installationId !== provider.installationId,
  );
}

function providerSupportsFirstPartyAuthentication(provider: Provider) {
  return (
    provider.installationId === undefined &&
    (provider.pluginId === MCP_PLUGIN_ID ||
      provider.pluginId === OPENAPI_PLUGIN_ID ||
      provider.pluginId === MODEL_TOOLS_PLUGIN_ID)
  );
}

function formForProviderPlugin(
  form: ProviderFormState,
  plugin: ProviderPlugin,
): ProviderFormState {
  const installationId = plugin.installationId ?? "";
  const next = {
    ...form,
    pluginId: plugin.pluginId,
    installationId,
  };
  return {
    ...next,
    genericConfig:
      providerRenderer(next) === "generic"
        ? genericConfigDefaults(plugin.configSchema)
        : {},
  };
}

const emptyProviderForm: ProviderFormState = {
  pluginId: MCP_PLUGIN_ID,
  installationId: "",
  genericConfig: {},
  transport: "http",
  namespace: "",
  displayName: "",
  description: "",
  endpoint: "",
  headers: "",
  credentialHeaders: [],
  importJson: "",
  executable: "",
  args: "",
  directory: "",
  credentialEnvironment: [],
  openapiSourceKind: "url",
  openapiSpecUrl: "",
  openapiSpecFile: "",
  openapiServerUrl: "",
  openapiInclude: "",
  openapiExclude: "",
  openapiCredentials: [],
  modelToolsEngine: "openai",
  modelToolsModel: "",
  modelToolsCredentialRef: "",
};

export function ProvidersPage({
  client,
}: {
  client: MenderClient;
}) {
  const [providers, setProviders] = useState<Provider[]>([]);
  const [providerPlugins, setProviderPlugins] = useState<ProviderPlugin[]>([]);
  const [providerPluginIssues, setProviderPluginIssues] = useState<
    ProviderPluginIssue[]
  >([]);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");

  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] =
    useState<ProviderFormState>(emptyProviderForm);

  const [viewProvider, setViewProvider] = useState<Provider | null>(null);
  const [editProvider, setEditProvider] = useState<Provider | null>(null);
  const [editForm, setEditForm] = useState<ProviderFormState | null>(null);
  const [rebindProvider, setRebindProvider] = useState<Provider | null>(null);
  const [rebindPlugin, setRebindPlugin] = useState<ProviderPlugin | null>(null);
  const [rebindConfig, setRebindConfig] = useState<Record<string, unknown>>({});
  const [authenticationProvider, setAuthenticationProvider] =
    useState<Provider | null>(null);
  const [authenticationForm, setAuthenticationForm] =
    useState<ProviderFormState | null>(null);
  const [authenticationReturnsToCreate, setAuthenticationReturnsToCreate] =
    useState(false);
  const [authenticationOriginForm, setAuthenticationOriginForm] =
    useState<ProviderFormState | null>(null);

  const refreshProviderPlugins = useCallback(async (): Promise<void> => {
    try {
      const result = await client.listProviderPlugins();
      setProviderPlugins(result.plugins);
      setProviderPluginIssues(result.issues);
      setError("");
    } catch (reason) {
      setError(errorMessage(reason));
    }
  }, [client]);

  const refresh = useCallback(async (): Promise<Provider[] | null> => {
    try {
      const result = await client.listProviders();
      setProviders(result.providers);
      setError("");
      return result.providers;
    } catch (reason) {
      setError(errorMessage(reason));
      return null;
    } finally {
      setLoaded(true);
    }
  }, [client]);

  useEffect(() => {
    let active = true;

    void Promise.all([
      client.listProviderPlugins(),
      client.listProviders(),
      client.listCredentials(),
    ])
      .then(([pluginResult, providerResult, credentialResult]) => {
        if (active) {
          setProviderPlugins(pluginResult.plugins);
          setProviderPluginIssues(pluginResult.issues);
          setProviders(providerResult.providers);
          setCredentials(credentialResult.credentials);
          setError("");
          setLoaded(true);
        }
      })
      .catch((reason: unknown) => {
        if (active) {
          setError(errorMessage(reason));
          setLoaded(true);
        }
      });

    return () => {
      active = false;
    };
  }, [client]);

  const providerPagination = usePagination(providers);

  async function addProvider() {
    const namespaceExisted = providers.some(
      (provider) => provider.namespace === createForm.namespace,
    );

    setBusy("create");
    setError("");
    try {
      await client.createProvider({
        namespace: createForm.namespace,
        displayName: createForm.displayName,
        pluginId: createForm.pluginId,
        ...(createForm.installationId
          ? { installationId: createForm.installationId }
          : {}),
        config: configFromForm(createForm),
        ...(createForm.description
          ? { description: createForm.description }
          : {}),
      });

      setCreateOpen(false);
      setCreateForm(emptyProviderForm);
      await refresh();
    } catch (reason) {
      const message = errorMessage(reason);
      const current = await refresh();
      setError(message);
      if (
        !namespaceExisted &&
        current?.some((provider) => provider.namespace === createForm.namespace)
      ) {
        setCreateOpen(false);
        setCreateForm(emptyProviderForm);
      }
    } finally {
      setBusy("");
    }
  }

  function beginEdit(provider: Provider) {
    if (!provider.pluginId) {
      return;
    }
    setError("");
    setEditProvider(provider);
    setEditForm(formFromProvider(provider));
  }

  async function saveEdit() {
    if (editProvider === null || editForm === null) {
      return;
    }

    setBusy(editProvider.providerId);
    setError("");
    try {
      await client.updateProvider(
        editProvider.providerId,
        updateRequestFromForm(editForm),
      );
      setEditProvider(null);
      setEditForm(null);
      await refresh();
    } catch (reason) {
      const message = errorMessage(reason);
      await refresh();
      setError(message);
    } finally {
      setBusy("");
    }
  }

  function beginRebind(provider: Provider) {
    const target = providerRebindCandidates(providerPlugins, provider)[0];
    if (target === undefined) {
      return;
    }
    const currentConfig = asRecord(provider.config);
    setError("");
    setRebindProvider(provider);
    setRebindPlugin(target);
    setRebindConfig(
      currentConfig === null
        ? genericConfigDefaults(target.configSchema)
        : JSON.parse(JSON.stringify(currentConfig)) as Record<string, unknown>,
    );
  }

  function closeRebind() {
    setRebindProvider(null);
    setRebindPlugin(null);
    setRebindConfig({});
  }

  async function saveRebind() {
    if (
      rebindProvider === null ||
      rebindPlugin === null ||
      rebindPlugin.installationId === undefined
    ) {
      return;
    }

    setBusy(rebindProvider.providerId);
    setError("");
    try {
      await client.rebindProvider(rebindProvider.providerId, {
        installationId: rebindPlugin.installationId,
        config: JSON.parse(JSON.stringify(rebindConfig)) as Record<string, unknown>,
      });
      closeRebind();
      await refresh();
      await refreshProviderPlugins();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function syncProvider(provider: Provider) {
    setBusy(provider.providerId);
    setError("");
    try {
      await client.syncProvider(provider.providerId);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function deleteProvider(provider: Provider) {
    if (
      !window.confirm(
        t('Delete provider "{{name}}"? Profile references will block deletion.', {
          name: provider.namespace,
        }),
      )
    ) {
      return;
    }
    setBusy(provider.providerId);
    setError("");
    try {
      await client.deleteProvider(provider.providerId);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  function openCreateAuthentication() {
    setAuthenticationOriginForm(createForm);
    setCreateOpen(false);
    setAuthenticationReturnsToCreate(true);
    window.setTimeout(() => {
      setAuthenticationProvider(null);
      setAuthenticationForm({ ...createForm });
    }, 0);
  }

  function openProviderAuthentication(provider: Provider) {
    if (!providerSupportsFirstPartyAuthentication(provider)) {
      return;
    }
    setError("");
    setAuthenticationReturnsToCreate(false);
    setAuthenticationProvider(provider);
    setAuthenticationForm(formFromProvider(provider));
  }

  function closeAuthentication() {
    const returnToCreate = authenticationReturnsToCreate;
    const origin = authenticationOriginForm;
    setAuthenticationProvider(null);
    setAuthenticationForm(null);
    setAuthenticationReturnsToCreate(false);
    setAuthenticationOriginForm(null);
    if (returnToCreate) {
      if (origin !== null) {
        setCreateForm(origin);
      }
      window.setTimeout(() => setCreateOpen(true), 0);
    }
  }

  async function saveAuthentication() {
    if (authenticationForm === null) {
      return;
    }
    if (authenticationProvider === null) {
      setCreateForm(authenticationForm);
      setAuthenticationForm(null);
      setAuthenticationReturnsToCreate(false);
      setAuthenticationOriginForm(null);
      window.setTimeout(() => setCreateOpen(true), 0);
      return;
    }

    setBusy(authenticationProvider.providerId);
    setError("");
    try {
      await client.updateProvider(
        authenticationProvider.providerId,
        updateRequestFromForm(authenticationForm),
      );
      setAuthenticationProvider(null);
      setAuthenticationForm(null);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  function setCreateDialogOpen(open: boolean) {
    setCreateOpen(open);
    if (!open) {
      setCreateForm(emptyProviderForm);
    }
  }

  return (
    <>
      <PageHeader
        actions={
          <Button
            onClick={() => {
              setError("");
              setCreateOpen(true);
            }}
          >
            {t("Add provider")}
          </Button>
        }
      />

      {error ? <ErrorNotice message={error} /> : null}
      {providerPluginIssues.length ? (
        <ErrorNotice
          message={t(
            "Some Provider plugins need attention. Review Plugin Marketplace for installation details.",
          )}
        />
      ) : null}

      <DataPanel
        title="Provider inventory"
        description={tCount(
          providers.length,
          "{{count}} configured source",
          "{{count}} configured sources",
        )}
      >
        {!loaded ? (
          <div className="px-4 py-12 text-center text-sm text-muted-foreground">
            {t("Loading providers…")}
          </div>
        ) : providers.length ? (
          <>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("Provider")}</TableHead>
                  <TableHead>{t("Plugin")}</TableHead>
                  <TableHead>{t("Connection")}</TableHead>
                  <TableHead>{t("Tools")}</TableHead>
                  <TableHead className="text-right">{t("Actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {providerPagination.items.map((provider) => (
                  <TableRow key={provider.providerId}>
                    <TableCell className="font-medium">
                      {provider.displayName || provider.namespace}
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline">
                        {providerPluginLabelForProvider(
                          providerPlugins,
                          provider,
                        )}
                      </Badge>
                    </TableCell>
                    <TableCell className="max-w-[360px] truncate font-mono text-xs">
                      {connectionSummary(provider)}
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={provider.toolCount ? "outline" : "secondary"}
                      >
                        {tCount(
                          provider.toolCount,
                          "{{count}} synced Tool",
                          "{{count}} synced Tools",
                        )}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
                        <IconButton
                          variant="ghost"
                          label={t("View {{provider}}", {
                            provider: provider.displayName || provider.namespace,
                          })}
                          onClick={() => setViewProvider(provider)}
                        >
                          <EyeIcon />
                        </IconButton>
                        <IconButton
                          variant="ghost"
                          label={t("Edit {{provider}}", {
                            provider: provider.displayName || provider.namespace,
                          })}
                          disabled={busy !== "" || !provider.pluginId}
                          onClick={() => beginEdit(provider)}
                        >
                          <PencilIcon />
                        </IconButton>
                        <IconButton
                          variant="outline"
                          label={t("Authentication {{provider}}", {
                            provider: provider.displayName || provider.namespace,
                          })}
                          disabled={
                            busy !== "" ||
                            !providerSupportsFirstPartyAuthentication(provider)
                          }
                          onClick={() => openProviderAuthentication(provider)}
                        >
                          <KeyRoundIcon />
                        </IconButton>
                        {providerRebindCandidates(
                          providerPlugins,
                          provider,
                        ).length ? (
                          <IconButton
                            variant="outline"
                            label={t("Rebind {{provider}}", {
                              provider: provider.displayName || provider.namespace,
                            })}
                            disabled={busy !== ""}
                            onClick={() => beginRebind(provider)}
                          >
                            <Link2Icon />
                          </IconButton>
                        ) : null}
                        <IconButton
                          variant="outline"
                          label={t("Sync {{provider}}", {
                            provider: provider.displayName || provider.namespace,
                          })}
                          disabled={busy !== ""}
                          onClick={() => void syncProvider(provider)}
                        >
                          <RefreshCwIcon />
                        </IconButton>
                        <IconButton
                          variant="destructive"
                          label={t("Delete {{provider}}", {
                            provider: provider.displayName || provider.namespace,
                          })}
                          disabled={busy !== ""}
                          onClick={() => void deleteProvider(provider)}
                        >
                          <Trash2Icon />
                        </IconButton>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            <div className="px-4 pb-3">
              <TablePagination
                label="Providers"
                pagination={providerPagination}
              />
            </div>
          </>
        ) : (
          <EmptyPanelState>
            <EmptyState
              title="No providers"
              description="Add a Provider plugin and configure its connection to populate the Catalog."
            />
          </EmptyPanelState>
        )}
      </DataPanel>

      <ProviderFormDialog
        open={createOpen}
        mode="create"
        form={createForm}
        plugins={providerPlugins}
        credentials={credentials}
        busy={busy === "create"}
        onOpenChange={setCreateDialogOpen}
        onChange={setCreateForm}
        onAuthentication={openCreateAuthentication}
        onSubmit={() => void addProvider()}
      />

      {authenticationForm !== null ? (
        <ProviderAuthenticationDialog
          open
          form={authenticationForm}
          credentials={credentials}
          busy={busy !== ""}
          onOpenChange={(open) => {
            if (!open) {
              closeAuthentication();
            }
          }}
          onChange={setAuthenticationForm}
          onSubmit={() => void saveAuthentication()}
          submitLabel={
            authenticationProvider === null
              ? "Use authentication"
              : "Save authentication"
          }
        />
      ) : null}

      <ProviderDetailsDialog
        provider={viewProvider}
        plugins={providerPlugins}
        onOpenChange={(open) => {
          if (!open) {
            setViewProvider(null);
          }
        }}
      />

      {editForm !== null ? (
        <ProviderFormDialog
          open={editProvider !== null}
          mode="edit"
          form={editForm}
          plugins={providerPlugins}
          credentials={credentials}
          busy={
            editProvider !== null && busy === editProvider.providerId
          }
          onOpenChange={(open) => {
            if (!open) {
              setEditProvider(null);
              setEditForm(null);
            }
          }}
          onChange={setEditForm}
          onSubmit={() => void saveEdit()}
        />
      ) : null}

      {rebindProvider !== null && rebindPlugin !== null ? (
        <ProviderRebindDialog
          open
          provider={rebindProvider}
          candidates={providerRebindCandidates(
            providerPlugins,
            rebindProvider,
          )}
          plugin={rebindPlugin}
          config={rebindConfig}
          credentials={credentials}
          busy={busy === rebindProvider.providerId}
          onOpenChange={(open) => {
            if (!open) {
              closeRebind();
            }
          }}
          onPluginChange={(plugin) => {
            setRebindPlugin(plugin);
          }}
          onConfigChange={setRebindConfig}
          onSubmit={() => void saveRebind()}
        />
      ) : null}
    </>
  );
}

function ProviderRebindDialog({
  open,
  provider,
  candidates,
  plugin,
  config,
  credentials,
  busy,
  onOpenChange,
  onPluginChange,
  onConfigChange,
  onSubmit,
}: {
  open: boolean;
  provider: Provider;
  candidates: ProviderPlugin[];
  plugin: ProviderPlugin;
  config: Record<string, unknown>;
  credentials: Credential[];
  busy: boolean;
  onOpenChange: (open: boolean) => void;
  onPluginChange: (plugin: ProviderPlugin) => void;
  onConfigChange: (value: Record<string, unknown>) => void;
  onSubmit: () => void;
}) {
  const validationError = genericConfigValidationError(
    plugin.configSchema,
    config,
  );
  const selectedInstallationId = plugin.installationId ?? "";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t("Rebind Provider installation")}</DialogTitle>
          <DialogDescription>
            {t("Move this Provider to another exact installation of the same plugin. Provider identity and SourceKey-stable Tool identities remain unchanged.")}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="grid grid-cols-2 gap-3">
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
                  onPluginChange(selected);
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

          <div className="grid grid-cols-2 gap-3">
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

          <GenericProviderConfigForm
            key={providerPluginSelectionKey(plugin)}
            schema={plugin.configSchema}
            uiSchema={plugin.uiSchema}
            value={config}
            credentials={credentials}
            onChange={onConfigChange}
          />
          {validationError ? (
            <p className="text-xs text-destructive">{validationError}</p>
          ) : null}
        </div>

        <DialogFooter>
          <DialogClose render={<Button variant="outline" />}>
            {t("Cancel")}
          </DialogClose>
          <Button
            disabled={busy || validationError !== null}
            onClick={onSubmit}
          >
            {t(busy ? "Rebinding…" : "Rebind installation")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}


function ProviderFormDialog({
  open,
  mode,
  form,
  plugins,
  credentials,
  busy,
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
  onOpenChange: (open: boolean) => void;
  onChange: (value: ProviderFormState) => void;
  onAuthentication?: () => void;
  onSubmit: () => void;
}) {
  const [importError, setImportError] = useState("");
  const creating = mode === "create";
  const selectedPlugin = providerPluginForForm(plugins, form);
  const renderer = providerRenderer(form);
  const genericError =
    renderer === "generic"
      ? genericConfigValidationError(
          selectedPlugin?.configSchema,
          form.genericConfig,
        )
      : null;
  const valid =
    form.displayName.trim() !== "" &&
    (!creating || form.namespace.trim() !== "") &&
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
          : selectedPlugin !== undefined && genericError === null);

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
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>
            {t(creating ? "Add provider" : "Edit provider")}
          </DialogTitle>
          <DialogDescription>
            {t(
              creating
                ? "Choose a Provider plugin, then configure how that plugin connects to its source."
                : "Update mutable metadata and plugin connection settings. Provider identity and plugin remain stable.",
            )}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {creating ? (
            <div>
              <div className="mb-2 text-sm font-medium">{t("Provider plugin")}</div>
              <div className="grid gap-2 sm:grid-cols-2">
                {plugins.map((plugin) => {
                  const selected = providerPluginMatchesForm(plugin, form);
                  return (
                    <Button
                      key={providerPluginSelectionKey(plugin)}
                      type="button"
                      variant={selected ? "default" : "outline"}
                      aria-pressed={selected}
                      onClick={() => onChange(formForProviderPlugin(form, plugin))}
                    >
                      {providerPluginChoiceLabel(plugin)}
                    </Button>
                  );
                })}
              </div>
            </div>
          ) : (
            <div className="grid grid-cols-2 gap-3">
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
          )}

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
            <div className="rounded-lg border border-border bg-muted/30 p-3 text-sm text-muted-foreground">
              <div className="font-medium text-foreground">{t("Schema-driven configuration")}</div>
              <div className="mt-1 text-xs leading-5">
                {t(
                  selectedPlugin?.sourceKind === "external"
                    ? "This Provider is pinned to an installed external plugin package."
                    : "This plugin uses Mender's generic configuration renderer.",
                )}
              </div>
            </div>
          )}

          {creating ? (
            <Field label="Namespace">
              <Input
                aria-label={t("Namespace")}
                placeholder={t("namespace")}
                value={form.namespace}
                onChange={(event) =>
                  onChange({ ...form, namespace: event.target.value })
                }
              />
            </Field>
          ) : null}

          <Field label="Display name">
            <Input
              aria-label={t("Display name")}
              value={form.displayName}
              onChange={(event) =>
                onChange({ ...form, displayName: event.target.value })
              }
            />
          </Field>

          <Field label="Description" optional>
            <Input
              aria-label={t("Description")}
              value={form.description}
              onChange={(event) =>
                onChange({ ...form, description: event.target.value })
              }
            />
          </Field>

          {renderer === "generic" ? (
            <div className="space-y-3">
              {form.installationId ? (
                <div className="grid grid-cols-2 gap-3">
                  <ReadOnlyField
                    label="Installation"
                    value={form.installationId}
                  />
                  <ReadOnlyField
                    label="Package version"
                    value={selectedPlugin?.packageVersion ?? t("Unavailable")}
                  />
                </div>
              ) : null}
              <GenericProviderConfigForm
                key={providerFormSelectionKey(form)}
                schema={selectedPlugin?.configSchema}
                uiSchema={selectedPlugin?.uiSchema}
                value={form.genericConfig}
                credentials={credentials}
                onChange={(genericConfig) =>
                  onChange({ ...form, genericConfig })
                }
              />
              {genericError ? (
                <p className="text-xs text-destructive">{genericError}</p>
              ) : null}
            </div>
          ) : null}

          {renderer === "model-tools" ? (
            <>
              <Field label="Model">
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
              <p className="text-xs leading-5 text-muted-foreground">
                {t("Mender exposes one stable web_search Tool. The selected model must execute the provider-hosted search tool; model-only answers are rejected.")}
              </p>
            </>
          ) : null}

          {renderer === "mcp" ? (
            form.transport === "http" ? (
            <>
              <Field label="Endpoint">
                <Input
                  aria-label={t("Endpoint")}
                  placeholder={t("https://example.com/mcp")}
                  value={form.endpoint}
                  onChange={(event) =>
                    onChange({ ...form, endpoint: event.target.value })
                  }
                />
                <p className="mt-1 text-xs text-muted-foreground">
                  {t("Non-loopback endpoints require HTTPS.")}
                </p>
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
                <Field label="Description URL">
                  <Input
                    aria-label={t("OpenAPI description URL")}
                    placeholder={t("https://example.com/openapi.yaml")}
                    value={form.openapiSpecUrl}
                    onChange={(event) =>
                      onChange({ ...form, openapiSpecUrl: event.target.value })
                    }
                  />
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t("Non-loopback URLs require HTTPS. External $ref retrieval is disabled.")}
                  </p>
                </Field>
              ) : (
                <Field label="Description file">
                  <Input
                    aria-label={t("OpenAPI description file")}
                    placeholder={t("/absolute/path/to/openapi.yaml")}
                    value={form.openapiSpecFile}
                    onChange={(event) =>
                      onChange({ ...form, openapiSpecFile: event.target.value })
                    }
                  />
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t("Use an absolute local path. External file references are disabled.")}
                  </p>
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
          <Button disabled={busy || !valid} onClick={onSubmit}>
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

function ProviderAuthenticationDialog({
  open,
  form,
  credentials,
  busy,
  onOpenChange,
  onChange,
  onSubmit,
  submitLabel,
}: {
  open: boolean;
  form: ProviderFormState;
  credentials: Credential[];
  busy: boolean;
  onOpenChange: (open: boolean) => void;
  onChange: (value: ProviderFormState) => void;
  onSubmit: () => void;
  submitLabel: string;
}) {
  const firstCredential = credentials[0]?.reference ?? "";

  function addHTTPBinding() {
    if (!firstCredential) {
      return;
    }
    onChange({
      ...form,
      credentialHeaders: [
        ...form.credentialHeaders,
        {
          name: "Authorization",
          scheme: "Bearer",
          credentialRef: firstCredential,
        },
      ],
    });
  }

  function addStdioBinding() {
    if (!firstCredential) {
      return;
    }
    onChange({
      ...form,
      credentialEnvironment: [
        ...form.credentialEnvironment,
        {
          name: "TOKEN",
          credentialRef: firstCredential,
        },
      ],
    });
  }

  function addOpenAPIBinding() {
    if (!firstCredential) {
      return;
    }
    onChange({
      ...form,
      openapiCredentials: [
        ...form.openapiCredentials,
        {
          schemeName: "",
          credentialRef: firstCredential,
        },
      ],
    });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("Provider authentication")}</DialogTitle>
          <DialogDescription>
            {t("Bind Provider authentication to inventory Credentials. Mender stores only Credential References and resolves secret values at connection and execution boundaries.")}
          </DialogDescription>
        </DialogHeader>

        {credentials.length ? (
          <div className="space-y-4">
            {form.pluginId === MODEL_TOOLS_PLUGIN_ID ? (
              <>
                <p className="text-xs leading-5 text-muted-foreground">
                  {t("Select the inventory Credential used to authenticate the configured model API. The secret is resolved only during governed Tool execution and remains subject to Profile Credential scope.")}
                </p>
                <Field label="Model API Credential">
                  <select
                    aria-label={t("Model Tools API credential")}
                    value={form.modelToolsCredentialRef}
                    onChange={(event) =>
                      onChange({
                        ...form,
                        modelToolsCredentialRef: event.target.value,
                      })
                    }
                    className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus:ring-2 focus:ring-ring/40"
                  >
                    <option value="" disabled>
                      {t("Choose a Credential")}
                    </option>
                    {credentials.map((item) => (
                      <option key={item.reference} value={item.reference}>
                        {item.displayName} · {item.reference}
                      </option>
                    ))}
                  </select>
                </Field>
              </>
            ) : form.pluginId === OPENAPI_PLUGIN_ID ? (
              <>
                <p className="text-xs leading-5 text-muted-foreground">
                  {t("Bind OpenAPI Security Scheme names to inventory Credentials. The scheme name must match the description exactly; Mender keeps the Credential Reference in Provider config and resolves the secret only at execution.")}
                </p>
                <div className="space-y-3">
                  {form.openapiCredentials.map((binding, index) => (
                    <div
                      key={index}
                      className="grid gap-3 rounded-lg border border-border p-3 sm:grid-cols-[1fr_1.4fr_auto]"
                    >
                      <Field label="Security Scheme">
                        <Input
                          aria-label={t("OpenAPI security scheme {{index}}", {
                            index: index + 1,
                          })}
                          placeholder={t("BearerAuth")}
                          value={binding.schemeName}
                          onChange={(event) =>
                            onChange({
                              ...form,
                              openapiCredentials: form.openapiCredentials.map(
                                (item, itemIndex) =>
                                  itemIndex === index
                                    ? {
                                        ...item,
                                        schemeName: event.target.value,
                                      }
                                    : item,
                              ),
                            })
                          }
                        />
                      </Field>
                      <Field label="Credential">
                        <select
                          aria-label={t("OpenAPI security credential {{index}}", {
                            index: index + 1,
                          })}
                          value={binding.credentialRef}
                          onChange={(event) =>
                            onChange({
                              ...form,
                              openapiCredentials: form.openapiCredentials.map(
                                (item, itemIndex) =>
                                  itemIndex === index
                                    ? {
                                        ...item,
                                        credentialRef: event.target.value,
                                      }
                                    : item,
                              ),
                            })
                          }
                          className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus:ring-2 focus:ring-ring/40"
                        >
                          {credentials.map((item) => (
                            <option key={item.reference} value={item.reference}>
                              {item.displayName} · {item.reference}
                            </option>
                          ))}
                        </select>
                      </Field>
                      <div className="flex items-end">
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={() =>
                            onChange({
                              ...form,
                              openapiCredentials: form.openapiCredentials.filter(
                                (_, itemIndex) => itemIndex !== index,
                              ),
                            })
                          }
                        >
                          {t("Remove")}
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={addOpenAPIBinding}
                >
                  {t("Add Security Scheme binding")}
                </Button>
              </>
            ) : form.transport === "http" ? (
              <>
                {form.headers.trim() ? (
                  <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-xs leading-5 text-muted-foreground">
                    {t("Legacy HTTP header templates are still configured. They remain compatible but are not governed by Profile Credential scope. Prefer inventory-backed bindings below.")}
                  </div>
                ) : null}

                <div className="space-y-3">
                  {form.credentialHeaders.map((binding, index) => (
                    <div
                      key={index}
                      className="grid gap-3 rounded-lg border border-border p-3 sm:grid-cols-[1fr_120px_1.4fr_auto]"
                    >
                      <Field label="Header">
                        <Input
                          aria-label={t("Credential header name {{index}}", {
                            index: index + 1,
                          })}
                          value={binding.name}
                          onChange={(event) =>
                            onChange({
                              ...form,
                              credentialHeaders: form.credentialHeaders.map(
                                (item, itemIndex) =>
                                  itemIndex === index
                                    ? { ...item, name: event.target.value }
                                    : item,
                              ),
                            })
                          }
                        />
                      </Field>
                      <Field label="Scheme">
                        <Input
                          aria-label={t("Credential header scheme {{index}}", {
                            index: index + 1,
                          })}
                          placeholder={t("Bearer")}
                          value={binding.scheme}
                          onChange={(event) =>
                            onChange({
                              ...form,
                              credentialHeaders: form.credentialHeaders.map(
                                (item, itemIndex) =>
                                  itemIndex === index
                                    ? { ...item, scheme: event.target.value }
                                    : item,
                              ),
                            })
                          }
                        />
                      </Field>
                      <Field label="Credential">
                        <select
                          aria-label={t("Credential header credential {{index}}", {
                            index: index + 1,
                          })}
                          value={binding.credentialRef}
                          onChange={(event) =>
                            onChange({
                              ...form,
                              credentialHeaders: form.credentialHeaders.map(
                                (item, itemIndex) =>
                                  itemIndex === index
                                    ? {
                                        ...item,
                                        credentialRef: event.target.value,
                                      }
                                    : item,
                              ),
                            })
                          }
                          className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus:ring-2 focus:ring-ring/40"
                        >
                          {credentials.map((item) => (
                            <option key={item.reference} value={item.reference}>
                              {item.displayName} · {item.reference}
                            </option>
                          ))}
                        </select>
                      </Field>
                      <div className="flex items-end">
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={() =>
                            onChange({
                              ...form,
                              credentialHeaders: form.credentialHeaders.filter(
                                (_, itemIndex) => itemIndex !== index,
                              ),
                            })
                          }
                        >
                          {t("Remove")}
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={addHTTPBinding}
                >
                  {t("Add credential header")}
                </Button>
              </>
            ) : (
              <>
                <div className="space-y-3">
                  {form.credentialEnvironment.map((binding, index) => (
                    <div
                      key={index}
                      className="grid gap-3 rounded-lg border border-border p-3 sm:grid-cols-[1fr_1.4fr_auto]"
                    >
                      <Field label="Environment name">
                        <Input
                          aria-label={t("Credential environment name {{index}}", {
                            index: index + 1,
                          })}
                          value={binding.name}
                          onChange={(event) =>
                            onChange({
                              ...form,
                              credentialEnvironment:
                                form.credentialEnvironment.map(
                                  (item, itemIndex) =>
                                    itemIndex === index
                                      ? { ...item, name: event.target.value }
                                      : item,
                                ),
                            })
                          }
                        />
                      </Field>
                      <Field label="Credential">
                        <select
                          aria-label={t("Credential environment credential {{index}}", {
                            index: index + 1,
                          })}
                          value={binding.credentialRef}
                          onChange={(event) =>
                            onChange({
                              ...form,
                              credentialEnvironment:
                                form.credentialEnvironment.map(
                                  (item, itemIndex) =>
                                    itemIndex === index
                                      ? {
                                          ...item,
                                          credentialRef: event.target.value,
                                        }
                                      : item,
                                ),
                            })
                          }
                          className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus:ring-2 focus:ring-ring/40"
                        >
                          {credentials.map((item) => (
                            <option key={item.reference} value={item.reference}>
                              {item.displayName} · {item.reference}
                            </option>
                          ))}
                        </select>
                      </Field>
                      <div className="flex items-end">
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={() =>
                            onChange({
                              ...form,
                              credentialEnvironment:
                                form.credentialEnvironment.filter(
                                  (_, itemIndex) => itemIndex !== index,
                                ),
                            })
                          }
                        >
                          {t("Remove")}
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={addStdioBinding}
                >
                  {t("Add credential environment")}
                </Button>
              </>
            )}
          </div>
        ) : (
          <EmptyState
            title="No Credentials in inventory"
            description="Create a Credential first, then return here to configure Provider authentication."
          />
        )}

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("Cancel")}
          </Button>
          <Button disabled={busy} onClick={onSubmit}>
            {busy ? t("Saving…") : t(submitLabel)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ProviderDetailsDialog({
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
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t("Provider details")}</DialogTitle>
          <DialogDescription>
            {t("Identity, connection configuration, and timestamps for this Provider.")}
          </DialogDescription>
        </DialogHeader>

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

        <DialogFooter showCloseButton />
      </DialogContent>
    </Dialog>
  );
}

function Field({
  label,
  optional = false,
  children,
}: {
  label: string;
  optional?: boolean;
  children: React.ReactNode;
}) {
  return (
    <label className="block space-y-1.5">
      <span className="text-sm font-medium">
        {t(label)}
        {optional ? (
          <span className="ml-1 font-normal text-muted-foreground">
            {t("(optional)")}
          </span>
        ) : null}
      </span>
      {children}
    </label>
  );
}

function ReadOnlyField({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="mb-1.5 text-sm font-medium">{t(label)}</div>
      <div className="rounded-lg border border-border bg-muted/40 px-3 py-2 font-mono text-sm text-muted-foreground">
        {value}
      </div>
    </div>
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

function formFromProvider(provider: Provider): ProviderFormState {
  const config = asRecord(provider.config);
  const source = asRecord(config?.source);
  const operations = asRecord(config?.operations);
  const credentialConfig = asRecord(config?.credentials);
  const sourceKind: OpenAPISourceKind =
    source?.kind === "file" ? "file" : "url";

  const openapiCredentials: OpenAPICredentialBinding[] = credentialConfig
    ? Object.entries(credentialConfig)
        .filter((entry): entry is [string, string] => typeof entry[1] === "string")
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([schemeName, credentialRef]) => ({ schemeName, credentialRef }))
    : [];

  return {
    pluginId: provider.pluginId ?? MCP_PLUGIN_ID,
    installationId: provider.installationId ?? "",
    genericConfig: config ?? {},
    transport: provider.transport === "stdio" ? "stdio" : "http",
    namespace: provider.namespace,
    displayName: provider.displayName,
    description: provider.description,
    endpoint: provider.endpoint ?? "",
    headers: formatHeaders(provider.headers ?? {}),
    credentialHeaders: [...(provider.credentialHeaders ?? [])],
    importJson: "",
    executable: provider.executable ?? "",
    args: provider.args?.join("\n") ?? "",
    directory: provider.directory ?? "",
    credentialEnvironment: [...(provider.credentialEnvironment ?? [])],
    openapiSourceKind: sourceKind,
    openapiSpecUrl: typeof source?.url === "string" ? source.url : "",
    openapiSpecFile: typeof source?.path === "string" ? source.path : "",
    openapiServerUrl:
      typeof config?.serverUrl === "string" ? config.serverUrl : "",
    openapiInclude: stringListFromUnknown(operations?.include).join("\n"),
    openapiExclude: stringListFromUnknown(operations?.exclude).join("\n"),
    openapiCredentials,
    modelToolsEngine: modelToolsEngineFromUnknown(config?.engine),
    modelToolsModel: typeof config?.model === "string" ? config.model : "",
    modelToolsCredentialRef:
      typeof config?.credentialRef === "string" ? config.credentialRef : "",
  };
}

function updateRequestFromForm(form: ProviderFormState): UpdateProviderRequest {
  return {
    displayName: form.displayName,
    ...(form.description ? { description: form.description } : {}),
    config: configFromForm(form),
  };
}

function configFromForm(form: ProviderFormState): Record<string, unknown> {
  const renderer = providerRenderer(form);
  if (renderer === "generic") {
    return JSON.parse(JSON.stringify(form.genericConfig)) as Record<string, unknown>;
  }

  if (renderer === "model-tools") {
    if (!form.modelToolsCredentialRef) {
      throw new Error(t("Choose a Model API Credential."));
    }
    return {
      engine: form.modelToolsEngine,
      model: form.modelToolsModel.trim(),
      credentialRef: form.modelToolsCredentialRef,
      capabilities: ["web_search"],
    };
  }

  if (renderer === "openapi") {
    const include = parseLines(form.openapiInclude);
    const exclude = parseLines(form.openapiExclude);
    const credentials: Record<string, string> = {};
    for (const binding of form.openapiCredentials) {
      const schemeName = binding.schemeName.trim();
      if (!schemeName) {
        throw new Error(t("OpenAPI Security Scheme name is required."));
      }
      if (credentials[schemeName] !== undefined) {
        throw new Error(t('OpenAPI Security Scheme "{{name}}" is configured more than once.', { name: schemeName }));
      }
      if (!binding.credentialRef) {
        throw new Error(t('Choose a Credential for OpenAPI Security Scheme "{{name}}".', { name: schemeName }));
      }
      credentials[schemeName] = binding.credentialRef;
    }

    return {
      source:
        form.openapiSourceKind === "url"
          ? { kind: "url", url: form.openapiSpecUrl.trim() }
          : { kind: "file", path: form.openapiSpecFile.trim() },
      ...(form.openapiServerUrl.trim()
        ? { serverUrl: form.openapiServerUrl.trim() }
        : {}),
      ...(include.length || exclude.length
        ? {
            operations: {
              ...(include.length ? { include } : {}),
              ...(exclude.length ? { exclude } : {}),
            },
          }
        : {}),
      ...(Object.keys(credentials).length ? { credentials } : {}),
    };
  }

  if (renderer !== "mcp") {
    throw new Error(t("Unsupported Provider renderer."));
  }
  if (form.transport === "http") {
    const headers = parseHeaders(form.headers);
    return {
      transport: {
        kind: "streamable-http",
        endpoint: form.endpoint,
        ...(Object.keys(headers).length ? { headers } : {}),
        ...(form.credentialHeaders.length
          ? { credentialHeaders: form.credentialHeaders }
          : {}),
      },
    };
  }

  const args = parseArgs(form.args);
  return {
    transport: {
      kind: "stdio",
      executable: form.executable,
      ...(form.directory ? { directory: form.directory } : {}),
      ...(args.length ? { args } : {}),
      ...(form.credentialEnvironment.length
        ? { credentialEnvironment: form.credentialEnvironment }
        : {}),
    },
  };
}

function parseHeaders(value: string): Record<string, string> {
  const result: Record<string, string> = {};
  const lines = value.split("\n");

  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index]?.trim() ?? "";
    if (!line) {
      continue;
    }
    const separator = line.indexOf(":");
    if (separator <= 0) {
      throw new Error(
        t('Header line {{line}} must use "Name: value" format.', { line: index + 1 }),
      );
    }
    const name = line.slice(0, separator).trim();
    const headerValue = line.slice(separator + 1).trim();
    if (!name) {
      throw new Error(t("Header line {{line}} has an empty name.", { line: index + 1 }));
    }
    result[name] = headerValue;
  }

  return result;
}

function formatHeaders(headers: Record<string, string>): string {
  return Object.entries(headers)
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([name, value]) => `${name}: ${value}`)
    .join("\n");
}

function parseMCPJSON(value: string): {
  namespace: string;
  endpoint: string;
  headers: Record<string, string>;
} {
  let parsed: unknown;
  try {
    parsed = JSON.parse(value);
  } catch {
    throw new Error(t("MCP JSON must be valid JSON."));
  }

  const root = asRecord(parsed);
  if (root === null) {
    throw new Error(t("MCP JSON must contain an object."));
  }

  let entries: [string, unknown][];
  const mcpServers = asRecord(root.mcpServers);
  const mcp = asRecord(root.mcp);
  if (mcpServers !== null) {
    entries = Object.entries(mcpServers);
  } else if (mcp !== null) {
    entries = Object.entries(mcp);
  } else if (
    typeof root.url === "string" ||
    typeof root.endpoint === "string"
  ) {
    entries = [["provider", root]];
  } else {
    throw new Error(t('Expected an "mcpServers" or "mcp" object.'));
  }

  if (entries.length !== 1) {
    throw new Error(t("Paste a configuration containing exactly one MCP server."));
  }

  const [namespace, rawConfig] = entries[0]!;
  const config = asRecord(rawConfig);
  if (config === null) {
    throw new Error(t("MCP server configuration must be an object."));
  }

  if (typeof config.type === "string") {
    const type = config.type.toLowerCase();
    if (!["remote", "streamable-http", "http"].includes(type)) {
      throw new Error(
        t('Unsupported remote MCP type "{{type}}". Expected remote or streamable-http.', { type: config.type }),
      );
    }
  }

  const endpoint =
    typeof config.url === "string"
      ? config.url
      : typeof config.endpoint === "string"
        ? config.endpoint
        : "";
  if (!endpoint) {
    throw new Error(t('Remote MCP configuration requires "url".'));
  }

  const headers: Record<string, string> = {};
  if (config.headers !== undefined) {
    const rawHeaders = asRecord(config.headers);
    if (rawHeaders === null) {
      throw new Error(t('"headers" must be an object of string values.'));
    }
    for (const [name, headerValue] of Object.entries(rawHeaders)) {
      if (typeof headerValue !== "string") {
        throw new Error(t('Header "{{name}}" must have a string value.', { name }));
      }
      headers[name] = headerValue;
    }
  }

  return { namespace, endpoint, headers };
}

function asRecord(value: unknown): Record<string, unknown> | null {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return null;
  }
  return value as Record<string, unknown>;
}

function parseArgs(value: string) {
  return parseLines(value);
}

function parseLines(value: string) {
  return value
    .split("\n")
    .map((item) => item.trim())
    .filter(Boolean);
}

function stringListFromUnknown(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.filter((item): item is string => typeof item === "string");
}

function modelToolsEngineFromUnknown(value: unknown): ModelToolsEngine {
  return value === "anthropic" || value === "gemini" ? value : "openai";
}

function modelToolsEngineLabel(engine: ModelToolsEngine) {
  switch (engine) {
    case "openai":
      return "OpenAI";
    case "anthropic":
      return "Anthropic";
    case "gemini":
      return "Gemini";
  }
}

function modelToolsModelPlaceholder(engine: ModelToolsEngine) {
  switch (engine) {
    case "openai":
      return "gpt-5.5";
    case "anthropic":
      return "claude-opus-5-5";
    case "gemini":
      return "gemini-3.8-flash";
  }
}

function modelToolsEngineSummary(provider: Provider) {
  const config = asRecord(provider.config);
  return modelToolsEngineLabel(modelToolsEngineFromUnknown(config?.engine));
}

function modelToolsModelSummary(provider: Provider) {
  const config = asRecord(provider.config);
  return typeof config?.model === "string" && config.model
    ? config.model
    : "—";
}

function openAPISourceSummary(provider: Provider) {
  const config = asRecord(provider.config);
  const source = asRecord(config?.source);
  if (source?.kind === "file" && typeof source.path === "string") {
    return t("File · {{path}}", { path: source.path });
  }
  if (typeof source?.url === "string") {
    return t("URL · {{url}}", { url: source.url });
  }
  return "—";
}

function openAPIServerSummary(provider: Provider) {
  const config = asRecord(provider.config);
  return typeof config?.serverUrl === "string" && config.serverUrl
    ? config.serverUrl
    : t("Document server");
}

function openAPIFilterSummary(provider: Provider) {
  const config = asRecord(provider.config);
  const operations = asRecord(config?.operations);
  const include = stringListFromUnknown(operations?.include);
  const exclude = stringListFromUnknown(operations?.exclude);
  if (!include.length && !exclude.length) {
    return t("All supported operations");
  }
  const lines = [
    ...(include.length ? [t("Include: {{value}}", { value: include.join(", ") })] : []),
    ...(exclude.length ? [t("Exclude: {{value}}", { value: exclude.join(", ") })] : []),
  ];
  return lines.join("\n");
}

function openAPISecuritySummary(provider: Provider) {
  const config = asRecord(provider.config);
  const credentials = asRecord(config?.credentials);
  if (credentials === null) {
    return t("None configured");
  }
  const names = Object.keys(credentials).sort((left, right) =>
    left.localeCompare(right),
  );
  return names.length ? names.join("\n") : t("None configured");
}

function connectionSummary(provider: Provider) {
  if (provider.connectionSummary?.trim()) {
    return provider.connectionSummary;
  }
  if (provider.installationId) {
    return t("External plugin · {{installation}}", { installation: provider.installationId });
  }
  if (provider.pluginId === MODEL_TOOLS_PLUGIN_ID) {
    return (
      modelToolsEngineSummary(provider) +
      " · " +
      modelToolsModelSummary(provider)
    );
  }
  if (provider.pluginId === OPENAPI_PLUGIN_ID) {
    const config = asRecord(provider.config);
    const source = asRecord(config?.source);
    if (source?.kind === "file" && typeof source.path === "string") {
      return source.path;
    }
    if (typeof source?.url === "string") {
      return source.url;
    }
    return t("OpenAPI source not configured");
  }
  if (provider.transport === "http") {
    return provider.endpoint ?? t("not configured");
  }
  if (provider.transport === "stdio") {
    return provider.executable ?? t("not configured");
  }
  return t("not configured");
}

function formatTimestamp(value: string) {
  return formatDateTime(value);
}
