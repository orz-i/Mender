import { useCallback, useEffect, useState } from "react";

import type {
  Credential,
  MenderClient,
  Provider,
  ProviderPlugin,
  ProviderPluginInstallation,
  UpdateProviderRequest,
} from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@mender/ui/components/card";
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

import { EmptyState, ErrorNotice, PageHeader } from "../../shared/page";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import { GenericProviderConfigForm } from "./generic-provider-config-form";
import {
  genericConfigDefaults,
  genericConfigValidationError,
} from "./generic-provider-config-schema";

const MCP_PLUGIN_ID = "io.mender.provider.mcp";
const MODEL_TOOLS_PLUGIN_ID = "io.mender.provider.model-tools";
const OPENAPI_PLUGIN_ID = "io.mender.provider.openapi";

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
  return plugin !== undefined
    ? providerPluginChoiceLabel(plugin)
    : provider.pluginDisplayName ?? provider.pluginId ?? "Unconfigured";
}

function providerIsBuiltIn(provider: Provider, pluginId: string) {
  return provider.installationId === undefined && provider.pluginId === pluginId;
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
  installProviderPlugin,
}: {
  client: MenderClient;
  installProviderPlugin?: () => Promise<ProviderPluginInstallation | null>;
}) {
  const [providers, setProviders] = useState<Provider[]>([]);
  const [providerPlugins, setProviderPlugins] = useState<ProviderPlugin[]>([]);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [pluginManagerOpen, setPluginManagerOpen] = useState(false);

  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] =
    useState<ProviderFormState>(emptyProviderForm);

  const [viewProvider, setViewProvider] = useState<Provider | null>(null);
  const [editProvider, setEditProvider] = useState<Provider | null>(null);
  const [editForm, setEditForm] = useState<ProviderFormState | null>(null);
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
        `Delete provider "${provider.namespace}"? Profile references will block deletion.`,
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
        title="Providers"
        description="Manage Provider plugins and the tool sources they connect to Mender."
        actions={
          <div className="flex gap-2">
            <Button
              variant="outline"
              onClick={() => {
                setError("");
                setPluginManagerOpen(true);
              }}
            >
              Manage plugins
            </Button>
            <Button
              onClick={() => {
                setError("");
                setCreateOpen(true);
              }}
            >
              Add provider
            </Button>
          </div>
        }
      />

      {error ? <ErrorNotice message={error} /> : null}

      <Card>
        <CardHeader className="border-b">
          <CardTitle>Providers</CardTitle>
          <CardDescription>
            {providers.length} configured {providers.length === 1 ? "source" : "sources"}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {!loaded ? (
            <div className="py-10 text-center text-sm text-muted-foreground">
              Loading providers…
            </div>
          ) : providers.length ? (
            <>
              <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Provider</TableHead>
                  <TableHead>Plugin</TableHead>
                  <TableHead>Connection</TableHead>
                  <TableHead>Tools</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {providerPagination.items.map((provider) => (
                  <TableRow key={provider.providerId}>
                    <TableCell>
                      <div className="font-medium">{provider.displayName}</div>
                      <div className="font-mono text-xs text-muted-foreground">
                        {provider.namespace}
                      </div>
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
                        {provider.toolCount} synced
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          aria-label={`View ${provider.namespace}`}
                          onClick={() => setViewProvider(provider)}
                        >
                          View
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          aria-label={`Edit ${provider.namespace}`}
                          disabled={busy !== "" || !provider.pluginId}
                          onClick={() => beginEdit(provider)}
                        >
                          Edit
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          aria-label={`Authentication ${provider.namespace}`}
                          disabled={
                            busy !== "" ||
                            !providerSupportsFirstPartyAuthentication(provider)
                          }
                          onClick={() => openProviderAuthentication(provider)}
                        >
                          Authentication
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          aria-label={`Sync ${provider.namespace}`}
                          disabled={busy !== ""}
                          onClick={() => void syncProvider(provider)}
                        >
                          Sync
                        </Button>
                        <Button
                          variant="destructive"
                          size="sm"
                          aria-label={`Delete ${provider.namespace}`}
                          disabled={busy !== ""}
                          onClick={() => void deleteProvider(provider)}
                        >
                          Delete
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
              </Table>
              <TablePagination
                label="Providers"
                pagination={providerPagination}
              />
            </>
          ) : (
            <EmptyState
              title="No providers"
              description="Add a Provider plugin and configure its connection to populate the Catalog."
            />
          )}
        </CardContent>
      </Card>

      <ProviderPluginManagerDialog
        open={pluginManagerOpen}
        client={client}
        {...(installProviderPlugin === undefined
          ? {}
          : { installProviderPlugin })}
        onOpenChange={setPluginManagerOpen}
        onPluginsChanged={() => void refreshProviderPlugins()}
      />

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
    </>
  );
}

function ProviderPluginManagerDialog({
  open,
  client,
  installProviderPlugin,
  onOpenChange,
  onPluginsChanged,
}: {
  open: boolean;
  client: MenderClient;
  installProviderPlugin?: () => Promise<ProviderPluginInstallation | null>;
  onOpenChange: (open: boolean) => void;
  onPluginsChanged: () => void;
}) {
  const [installations, setInstallations] = useState<
    ProviderPluginInstallation[]
  >([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");

  const refreshInstallations = useCallback(async () => {
    try {
      const result = await client.listProviderPluginInstallations();
      setInstallations(result.installations);
      setError("");
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setLoaded(true);
    }
  }, [client]);

  useEffect(() => {
    if (!open) {
      return;
    }
    let active = true;
    void client
      .listProviderPluginInstallations()
      .then((result) => {
        if (active) {
          setInstallations(result.installations);
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
  }, [client, open]);

  async function installLocalPlugin() {
    if (installProviderPlugin === undefined) {
      return;
    }
    setBusy("install");
    setError("");
    try {
      const installed = await installProviderPlugin();
      if (installed === null) {
        return;
      }
      await refreshInstallations();
      onPluginsChanged();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function setInstallationEnabled(
    installation: ProviderPluginInstallation,
    enabled: boolean,
  ) {
    setBusy(installation.installationId);
    setError("");
    try {
      if (enabled) {
        await client.enableProviderPluginInstallation(
          installation.installationId,
        );
      } else {
        await client.disableProviderPluginInstallation(
          installation.installationId,
        );
      }
      await refreshInstallations();
      onPluginsChanged();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function removeInstallation(
    installation: ProviderPluginInstallation,
  ) {
    if (
      !window.confirm(
        "Remove installed plugin " +
          installation.pluginId +
          " " +
          installation.packageVersion +
          "? Configured Providers will block removal.",
      )
    ) {
      return;
    }
    setBusy(installation.installationId);
    setError("");
    try {
      await client.deleteProviderPluginInstallation(
        installation.installationId,
      );
      await refreshInstallations();
      onPluginsChanged();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Provider plugins</DialogTitle>
          <DialogDescription>
            Manage installed external Provider plugin packages. Providers remain
            pinned to exact installation IDs.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {error ? (
            <p className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">
              {error}
            </p>
          ) : null}

          <div className="flex items-center justify-between gap-3 rounded-lg border border-border p-3">
            <div>
              <div className="text-sm font-medium">Local package installation</div>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {installProviderPlugin === undefined
                  ? "Install new native plugins with the Mender CLI or Desktop app. Browser management never accepts a server filesystem path."
                  : "Choose a local plugin package directory. Mender copies it into its managed plugin root and verifies the executable before activation."}
              </p>
            </div>
            {installProviderPlugin !== undefined ? (
              <Button
                type="button"
                variant="outline"
                disabled={busy !== ""}
                onClick={() => void installLocalPlugin()}
              >
                {busy === "install" ? "Installing…" : "Install package"}
              </Button>
            ) : null}
          </div>

          {!loaded ? (
            <div className="py-8 text-center text-sm text-muted-foreground">
              Loading installed plugins…
            </div>
          ) : installations.length ? (
            <div className="space-y-3">
              {installations.map((installation) => {
                const itemBusy = busy === installation.installationId;
                return (
                  <div
                    key={installation.installationId}
                    className="rounded-lg border border-border p-4"
                  >
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div className="min-w-0">
                        <div className="font-medium">
                          {installation.pluginId}
                        </div>
                        <div className="mt-1 font-mono text-xs text-muted-foreground">
                          {installation.installationId}
                        </div>
                      </div>
                      <Badge
                        variant={
                          installation.status === "enabled"
                            ? "outline"
                            : "secondary"
                        }
                      >
                        {installation.status}
                      </Badge>
                    </div>

                    <div className="mt-3 grid gap-2 text-xs text-muted-foreground sm:grid-cols-2">
                      <div>
                        Version{" "}
                        <span className="font-mono text-foreground">
                          {installation.packageVersion}
                        </span>
                      </div>
                      <div>
                        Installed{" "}
                        <span className="text-foreground">
                          {formatTimestamp(installation.installedAt)}
                        </span>
                      </div>
                    </div>
                    <div className="mt-2 break-all font-mono text-[11px] text-muted-foreground">
                      {installation.digest}
                    </div>

                    <div className="mt-4 flex justify-end gap-2">
                      {installation.status === "enabled" ? (
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          aria-label={
                            "Disable plugin " + installation.installationId
                          }
                          disabled={busy !== ""}
                          onClick={() =>
                            void setInstallationEnabled(installation, false)
                          }
                        >
                          {itemBusy ? "Disabling…" : "Disable"}
                        </Button>
                      ) : (
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          aria-label={
                            "Enable plugin " + installation.installationId
                          }
                          disabled={busy !== ""}
                          onClick={() =>
                            void setInstallationEnabled(installation, true)
                          }
                        >
                          {itemBusy ? "Enabling…" : "Enable"}
                        </Button>
                      )}
                      <Button
                        type="button"
                        variant="destructive"
                        size="sm"
                        aria-label={
                          "Remove plugin " + installation.installationId
                        }
                        disabled={busy !== ""}
                        onClick={() => void removeInstallation(installation)}
                      >
                        Remove
                      </Button>
                    </div>
                  </div>
                );
              })}
            </div>
          ) : (
            <EmptyState
              title="No external plugins installed"
              description="Install a Provider plugin package to make a custom Provider type available."
            />
          )}
        </div>

        <DialogFooter showCloseButton />
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
            {creating ? "Add provider" : "Edit provider"}
          </DialogTitle>
          <DialogDescription>
            {creating
              ? "Choose a Provider plugin, then configure how that plugin connects to its source."
              : "Update mutable metadata and plugin connection settings. Provider identity and plugin remain stable."}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {creating ? (
            <div>
              <div className="mb-2 text-sm font-medium">Provider plugin</div>
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
              <div className="mb-2 text-sm font-medium">MCP connection</div>
              <div className="grid grid-cols-2 gap-2">
                <Button
                  type="button"
                  variant={form.transport === "http" ? "default" : "outline"}
                  aria-pressed={form.transport === "http"}
                  onClick={() => onChange({ ...form, transport: "http" })}
                >
                  Remote endpoint
                </Button>
                <Button
                  type="button"
                  variant={form.transport === "stdio" ? "default" : "outline"}
                  aria-pressed={form.transport === "stdio"}
                  onClick={() => onChange({ ...form, transport: "stdio" })}
                >
                  Local command
                </Button>
              </div>
            </div>
          ) : renderer === "model-tools" ? (
            <div>
              <div className="mb-2 text-sm font-medium">Model tool engine</div>
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
              <div className="mb-2 text-sm font-medium">OpenAPI description</div>
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
                  Remote URL
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
                  Local file
                </Button>
              </div>
            </div>
          ) : (
            <div className="rounded-lg border border-border bg-muted/30 p-3 text-sm text-muted-foreground">
              <div className="font-medium text-foreground">Schema-driven configuration</div>
              <div className="mt-1 text-xs leading-5">
                {selectedPlugin?.sourceKind === "external"
                  ? "This Provider is pinned to an installed external plugin package."
                  : "This plugin uses Mender's generic configuration renderer."}
              </div>
            </div>
          )}

          {creating ? (
            <Field label="Namespace">
              <Input
                aria-label="Namespace"
                placeholder="namespace"
                value={form.namespace}
                onChange={(event) =>
                  onChange({ ...form, namespace: event.target.value })
                }
              />
            </Field>
          ) : null}

          <Field label="Display name">
            <Input
              aria-label="Display name"
              value={form.displayName}
              onChange={(event) =>
                onChange({ ...form, displayName: event.target.value })
              }
            />
          </Field>

          <Field label="Description" optional>
            <Input
              aria-label="Description"
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
                    value={selectedPlugin?.packageVersion ?? "Unavailable"}
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
                  aria-label="Model Tools model"
                  placeholder={modelToolsModelPlaceholder(form.modelToolsEngine)}
                  value={form.modelToolsModel}
                  onChange={(event) =>
                    onChange({ ...form, modelToolsModel: event.target.value })
                  }
                />
              </Field>
              <ReadOnlyField label="Capability" value="Web search" />
              <p className="text-xs leading-5 text-muted-foreground">
                Mender exposes one stable web_search Tool. The selected model must execute the provider-hosted search tool; model-only answers are rejected.
              </p>
            </>
          ) : null}

          {renderer === "mcp" ? (
            form.transport === "http" ? (
            <>
              <Field label="Endpoint">
                <Input
                  aria-label="Endpoint"
                  placeholder="https://example.com/mcp"
                  value={form.endpoint}
                  onChange={(event) =>
                    onChange({ ...form, endpoint: event.target.value })
                  }
                />
                <p className="mt-1 text-xs text-muted-foreground">
                  Non-loopback endpoints require HTTPS.
                </p>
              </Field>

              <details className="rounded-lg border border-border">
                <summary className="cursor-pointer select-none px-4 py-3 text-sm font-medium">
                  Advanced settings
                </summary>
                <div className="space-y-4 border-t border-border px-4 py-4">
                  <ReadOnlyField
                    label="MCP type"
                    value="streamable-http"
                  />

                  <Field label="Legacy HTTP header templates" optional>
                    <Textarea
                      aria-label="HTTP header templates"
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
                      Compatibility path only. Sensitive headers must use environment
                      references such as <code>{"${ANYSEARCH_API_KEY}"}</code>.
                      Prefer the separate Authentication workflow for inventory-backed, Profile-scoped Credentials.
                    </p>
                  </Field>

                  {creating ? (
                    <div className="rounded-lg bg-muted/40 p-3">
                      <Field label="Import MCP JSON" optional>
                        <Textarea
                          aria-label="MCP JSON"
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
                        Import configuration
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
                  aria-label="Executable"
                  placeholder="/path/to/mcp-server"
                  value={form.executable}
                  onChange={(event) =>
                    onChange({ ...form, executable: event.target.value })
                  }
                />
              </Field>
              <Field label="Arguments" optional>
                <Textarea
                  aria-label="Arguments"
                  placeholder={"One argument per line\n--stdio"}
                  value={form.args}
                  onChange={(event) =>
                    onChange({ ...form, args: event.target.value })
                  }
                />
              </Field>
              <Field label="Working directory" optional>
                <Input
                  aria-label="Working directory"
                  value={form.directory}
                  onChange={(event) =>
                    onChange({ ...form, directory: event.target.value })
                  }
                />
              </Field>
              <p className="text-xs text-muted-foreground">
                Mender does not persist environment values or secrets for stdio
                Providers.
              </p>
            </>
            )
          ) : null}

          {renderer === "openapi" ? (
            <>
              {form.openapiSourceKind === "url" ? (
                <Field label="Description URL">
                  <Input
                    aria-label="OpenAPI description URL"
                    placeholder="https://example.com/openapi.yaml"
                    value={form.openapiSpecUrl}
                    onChange={(event) =>
                      onChange({ ...form, openapiSpecUrl: event.target.value })
                    }
                  />
                  <p className="mt-1 text-xs text-muted-foreground">
                    Non-loopback URLs require HTTPS. External $ref retrieval is disabled.
                  </p>
                </Field>
              ) : (
                <Field label="Description file">
                  <Input
                    aria-label="OpenAPI description file"
                    placeholder="/absolute/path/to/openapi.yaml"
                    value={form.openapiSpecFile}
                    onChange={(event) =>
                      onChange({ ...form, openapiSpecFile: event.target.value })
                    }
                  />
                  <p className="mt-1 text-xs text-muted-foreground">
                    Use an absolute local path. External file references are disabled.
                  </p>
                </Field>
              )}

              <Field label="API server override" optional>
                <Input
                  aria-label="OpenAPI server override"
                  placeholder="https://api.example.com"
                  value={form.openapiServerUrl}
                  onChange={(event) =>
                    onChange({ ...form, openapiServerUrl: event.target.value })
                  }
                />
              </Field>

              <details className="rounded-lg border border-border">
                <summary className="cursor-pointer select-none px-4 py-3 text-sm font-medium">
                  Operations
                </summary>
                <div className="space-y-4 border-t border-border px-4 py-4">
                  <p className="text-xs leading-5 text-muted-foreground">
                    Filters use stable OpenAPI operation SourceKeys. Use operationId when present; otherwise use values such as <code>get /users/{"{id}"}</code>.
                  </p>
                  <Field label="Include SourceKeys" optional>
                    <Textarea
                      aria-label="OpenAPI include SourceKeys"
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
                      aria-label="OpenAPI exclude SourceKeys"
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
            Cancel
          </DialogClose>
          {onAuthentication !== undefined && renderer !== "generic" ? (
            <Button
              type="button"
              variant="outline"
              disabled={busy}
              onClick={onAuthentication}
            >
              Configure authentication
            </Button>
          ) : null}
          <Button disabled={busy || !valid} onClick={onSubmit}>
            {busy
              ? creating
                ? "Adding…"
                : "Saving…"
              : creating
                ? "Add provider"
                : "Save changes"}
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
          <DialogTitle>Provider authentication</DialogTitle>
          <DialogDescription>
            Bind Provider authentication to inventory Credentials. Mender stores
            only Credential References and resolves secret values at connection
            and execution boundaries.
          </DialogDescription>
        </DialogHeader>

        {credentials.length ? (
          <div className="space-y-4">
            {form.pluginId === MODEL_TOOLS_PLUGIN_ID ? (
              <>
                <p className="text-xs leading-5 text-muted-foreground">
                  Select the inventory Credential used to authenticate the configured model API. The secret is resolved only during governed Tool execution and remains subject to Profile Credential scope.
                </p>
                <Field label="Model API Credential">
                  <select
                    aria-label="Model Tools API credential"
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
                      Choose a Credential
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
                  Bind OpenAPI Security Scheme names to inventory Credentials. The scheme name must match the description exactly; Mender keeps the Credential Reference in Provider config and resolves the secret only at execution.
                </p>
                <div className="space-y-3">
                  {form.openapiCredentials.map((binding, index) => (
                    <div
                      key={index}
                      className="grid gap-3 rounded-lg border border-border p-3 sm:grid-cols-[1fr_1.4fr_auto]"
                    >
                      <Field label="Security Scheme">
                        <Input
                          aria-label={
                            "OpenAPI security scheme " + String(index + 1)
                          }
                          placeholder="BearerAuth"
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
                          aria-label={
                            "OpenAPI security credential " + String(index + 1)
                          }
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
                          Remove
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
                  Add Security Scheme binding
                </Button>
              </>
            ) : form.transport === "http" ? (
              <>
                {form.headers.trim() ? (
                  <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-xs leading-5 text-muted-foreground">
                    Legacy HTTP header templates are still configured. They
                    remain compatible but are not governed by Profile Credential
                    scope. Prefer inventory-backed bindings below.
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
                          aria-label={"Credential header name " + String(index + 1)}
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
                          aria-label={
                            "Credential header scheme " + String(index + 1)
                          }
                          placeholder="Bearer"
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
                          aria-label={
                            "Credential header credential " + String(index + 1)
                          }
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
                          Remove
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
                  Add credential header
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
                          aria-label={
                            "Credential environment name " + String(index + 1)
                          }
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
                          aria-label={
                            "Credential environment credential " +
                            String(index + 1)
                          }
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
                          Remove
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
                  Add credential environment
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
            Cancel
          </Button>
          <Button disabled={busy} onClick={onSubmit}>
            {busy ? "Saving…" : submitLabel}
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
          <DialogTitle>Provider details</DialogTitle>
          <DialogDescription>
            Identity, connection configuration, and timestamps for this Provider.
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
                    "Unavailable"
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
                <DetailRow label="Capability" value="Web search" />
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
            <DetailRow label="Synced Tools" value={String(provider.toolCount)} />
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
        {label}
        {optional ? (
          <span className="ml-1 font-normal text-muted-foreground">
            (optional)
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
      <div className="mb-1.5 text-sm font-medium">{label}</div>
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
      <div className="text-sm text-muted-foreground">{label}</div>
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
      throw new Error("Choose a Model API Credential.");
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
        throw new Error("OpenAPI Security Scheme name is required.");
      }
      if (credentials[schemeName] !== undefined) {
        throw new Error(`OpenAPI Security Scheme "${schemeName}" is configured more than once.`);
      }
      if (!binding.credentialRef) {
        throw new Error(`Choose a Credential for OpenAPI Security Scheme "${schemeName}".`);
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
    throw new Error("Unsupported Provider renderer.");
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
        `Header line ${index + 1} must use "Name: value" format.`,
      );
    }
    const name = line.slice(0, separator).trim();
    const headerValue = line.slice(separator + 1).trim();
    if (!name) {
      throw new Error(`Header line ${index + 1} has an empty name.`);
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
    throw new Error("MCP JSON must be valid JSON.");
  }

  const root = asRecord(parsed);
  if (root === null) {
    throw new Error("MCP JSON must contain an object.");
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
    throw new Error('Expected an "mcpServers" or "mcp" object.');
  }

  if (entries.length !== 1) {
    throw new Error("Paste a configuration containing exactly one MCP server.");
  }

  const [namespace, rawConfig] = entries[0]!;
  const config = asRecord(rawConfig);
  if (config === null) {
    throw new Error("MCP server configuration must be an object.");
  }

  if (typeof config.type === "string") {
    const type = config.type.toLowerCase();
    if (!["remote", "streamable-http", "http"].includes(type)) {
      throw new Error(
        `Unsupported remote MCP type "${config.type}". Expected remote or streamable-http.`,
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
    throw new Error('Remote MCP configuration requires "url".');
  }

  const headers: Record<string, string> = {};
  if (config.headers !== undefined) {
    const rawHeaders = asRecord(config.headers);
    if (rawHeaders === null) {
      throw new Error('"headers" must be an object of string values.');
    }
    for (const [name, headerValue] of Object.entries(rawHeaders)) {
      if (typeof headerValue !== "string") {
        throw new Error(`Header "${name}" must have a string value.`);
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
    return `File · ${source.path}`;
  }
  if (typeof source?.url === "string") {
    return `URL · ${source.url}`;
  }
  return "—";
}

function openAPIServerSummary(provider: Provider) {
  const config = asRecord(provider.config);
  return typeof config?.serverUrl === "string" && config.serverUrl
    ? config.serverUrl
    : "Document server";
}

function openAPIFilterSummary(provider: Provider) {
  const config = asRecord(provider.config);
  const operations = asRecord(config?.operations);
  const include = stringListFromUnknown(operations?.include);
  const exclude = stringListFromUnknown(operations?.exclude);
  if (!include.length && !exclude.length) {
    return "All supported operations";
  }
  const lines = [
    ...(include.length ? [`Include: ${include.join(", ")}`] : []),
    ...(exclude.length ? [`Exclude: ${exclude.join(", ")}`] : []),
  ];
  return lines.join("\n");
}

function openAPISecuritySummary(provider: Provider) {
  const config = asRecord(provider.config);
  const credentials = asRecord(config?.credentials);
  if (credentials === null) {
    return "None configured";
  }
  const names = Object.keys(credentials).sort((left, right) =>
    left.localeCompare(right),
  );
  return names.length ? names.join("\n") : "None configured";
}

function connectionSummary(provider: Provider) {
  if (provider.installationId) {
    return "External plugin · " + provider.installationId;
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
    return "OpenAPI source not configured";
  }
  if (provider.transport === "http") {
    return provider.endpoint ?? "not configured";
  }
  if (provider.transport === "stdio") {
    return provider.executable ?? "not configured";
  }
  return "not configured";
}

function formatTimestamp(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}

function errorMessage(reason: unknown) {
  return reason instanceof Error ? reason.message : "Unexpected error";
}
