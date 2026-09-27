import { useCallback, useEffect, useState } from "react";

import type {
  Credential,
  MenderClient,
  Provider,
  ProviderPlugin,
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

const MCP_PLUGIN_ID = "io.mender.provider.mcp";

type ProviderTransport = "http" | "stdio";
type CredentialHeaderBinding = NonNullable<Provider["credentialHeaders"]>[number];
type CredentialEnvironmentBinding = NonNullable<
  Provider["credentialEnvironment"]
>[number];

interface ProviderFormState {
  pluginId: string;
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
}

const emptyProviderForm: ProviderFormState = {
  pluginId: MCP_PLUGIN_ID,
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
};

export function ProvidersPage({ client }: { client: MenderClient }) {
  const [providers, setProviders] = useState<Provider[]>([]);
  const [providerPlugins, setProviderPlugins] = useState<ProviderPlugin[]>([]);
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
  const [authenticationProvider, setAuthenticationProvider] =
    useState<Provider | null>(null);
  const [authenticationForm, setAuthenticationForm] =
    useState<ProviderFormState | null>(null);
  const [authenticationReturnsToCreate, setAuthenticationReturnsToCreate] =
    useState(false);
  const [authenticationOriginForm, setAuthenticationOriginForm] =
    useState<ProviderFormState | null>(null);

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
    if (provider.transport === "unconfigured") {
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
    if (provider.pluginId !== MCP_PLUGIN_ID) {
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
          <Button
            onClick={() => {
              setError("");
              setCreateOpen(true);
            }}
          >
            Add provider
          </Button>
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
                {providers.map((provider) => (
                  <TableRow key={provider.providerId}>
                    <TableCell>
                      <div className="font-medium">{provider.displayName}</div>
                      <div className="font-mono text-xs text-muted-foreground">
                        {provider.namespace}
                      </div>
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline">
                        {provider.pluginDisplayName ??
                          provider.pluginId ??
                          "Unconfigured"}
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
                            busy !== "" || provider.pluginId !== MCP_PLUGIN_ID
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
          ) : (
            <EmptyState
              title="No providers"
              description="Add a Provider plugin and configure its connection to populate the Catalog."
            />
          )}
        </CardContent>
      </Card>

      <ProviderFormDialog
        open={createOpen}
        mode="create"
        form={createForm}
        plugins={providerPlugins}
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

function ProviderFormDialog({
  open,
  mode,
  form,
  plugins,
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
  busy: boolean;
  onOpenChange: (open: boolean) => void;
  onChange: (value: ProviderFormState) => void;
  onAuthentication?: () => void;
  onSubmit: () => void;
}) {
  const [importError, setImportError] = useState("");
  const creating = mode === "create";
  const valid =
    form.pluginId === MCP_PLUGIN_ID &&
    form.displayName.trim() !== "" &&
    (!creating || form.namespace.trim() !== "") &&
    (form.transport === "http"
      ? form.endpoint.trim() !== ""
      : form.executable.trim() !== "");

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
                {plugins.map((plugin) => (
                  <Button
                    key={plugin.pluginId}
                    type="button"
                    variant={
                      form.pluginId === plugin.pluginId ? "default" : "outline"
                    }
                    aria-pressed={form.pluginId === plugin.pluginId}
                    onClick={() =>
                      onChange({ ...form, pluginId: plugin.pluginId })
                    }
                  >
                    {plugin.displayName}
                  </Button>
                ))}
              </div>
            </div>
          ) : (
            <div className="grid grid-cols-2 gap-3">
              <ReadOnlyField label="Namespace" value={form.namespace} />
              <ReadOnlyField
                label="Provider plugin"
                value={
                  plugins.find((plugin) => plugin.pluginId === form.pluginId)
                    ?.displayName ?? form.pluginId
                }
              />
            </div>
          )}

          {form.pluginId === MCP_PLUGIN_ID ? (
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
          ) : (
            <p className="rounded-lg border border-border bg-muted/30 p-3 text-sm text-muted-foreground">
              This plugin does not yet provide a first-party configuration form.
            </p>
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

          {form.pluginId === MCP_PLUGIN_ID ? (
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
        </div>

        <DialogFooter>
          <DialogClose render={<Button variant="outline" />}>
            Cancel
          </DialogClose>
          {onAuthentication !== undefined && form.pluginId === MCP_PLUGIN_ID ? (
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
            {form.transport === "http" ? (
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
  onOpenChange,
}: {
  provider: Provider | null;
  onOpenChange: (open: boolean) => void;
}) {
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
              value={provider.pluginDisplayName ?? provider.pluginId ?? "Unconfigured"}
            />
            {provider.pluginId ? (
              <DetailRow label="Plugin ID" value={provider.pluginId} mono />
            ) : null}
            <DetailRow label="MCP transport" value={provider.transport} />
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
  return {
    pluginId: provider.pluginId ?? MCP_PLUGIN_ID,
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
  if (form.pluginId !== MCP_PLUGIN_ID) {
    throw new Error(`Unsupported Provider plugin: ${form.pluginId}`);
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
  return value
    .split("\n")
    .map((item) => item.trim())
    .filter(Boolean);
}

function connectionSummary(provider: Provider) {
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
