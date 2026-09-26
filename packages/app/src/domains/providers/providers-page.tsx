import { useCallback, useEffect, useState } from "react";

import type {
  MenderClient,
  Provider,
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

type ProviderTransport = "http" | "stdio";

interface ProviderFormState {
  transport: ProviderTransport;
  namespace: string;
  displayName: string;
  description: string;
  endpoint: string;
  headers: string;
  importJson: string;
  executable: string;
  args: string;
  directory: string;
}

const emptyProviderForm: ProviderFormState = {
  transport: "http",
  namespace: "",
  displayName: "",
  description: "",
  endpoint: "",
  headers: "",
  importJson: "",
  executable: "",
  args: "",
  directory: "",
};

export function ProvidersPage({ client }: { client: MenderClient }) {
  const [providers, setProviders] = useState<Provider[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");

  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] =
    useState<ProviderFormState>(emptyProviderForm);

  const [viewProvider, setViewProvider] = useState<Provider | null>(null);
  const [editProvider, setEditProvider] = useState<Provider | null>(null);
  const [editForm, setEditForm] = useState<ProviderFormState | null>(null);

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

    void client
      .listProviders()
      .then((result) => {
        if (active) {
          setProviders(result.providers);
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
      if (createForm.transport === "http") {
        const headers = parseHeaders(createForm.headers);
        await client.createHTTPProvider({
          namespace: createForm.namespace,
          displayName: createForm.displayName,
          endpoint: createForm.endpoint,
          ...(createForm.description
            ? { description: createForm.description }
            : {}),
          ...(Object.keys(headers).length ? { headers } : {}),
        });
      } else {
        const args = parseArgs(createForm.args);
        await client.createStdioProvider({
          namespace: createForm.namespace,
          displayName: createForm.displayName,
          executable: createForm.executable,
          ...(createForm.description
            ? { description: createForm.description }
            : {}),
          ...(createForm.directory ? { directory: createForm.directory } : {}),
          ...(args.length ? { args } : {}),
        });
      }

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
      let request: UpdateProviderRequest;
      if (editForm.transport === "http") {
        const headers = parseHeaders(editForm.headers);
        request = {
          transport: "http",
          displayName: editForm.displayName,
          endpoint: editForm.endpoint,
          ...(Object.keys(headers).length ? { headers } : {}),
          ...(editForm.description
            ? { description: editForm.description }
            : {}),
        };
      } else {
        const args = parseArgs(editForm.args);
        request = {
          transport: "stdio",
          displayName: editForm.displayName,
          executable: editForm.executable,
          ...(editForm.description
            ? { description: editForm.description }
            : {}),
          ...(editForm.directory ? { directory: editForm.directory } : {}),
          ...(args.length ? { args } : {}),
        };
      }

      await client.updateProvider(editProvider.providerId, request);
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
        description="Manage the MCP tool sources connected to Mender."
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
                  <TableHead>Transport</TableHead>
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
                      <Badge variant="outline">{provider.transport}</Badge>
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
                          disabled={
                            busy !== "" || provider.transport === "unconfigured"
                          }
                          onClick={() => beginEdit(provider)}
                        >
                          Edit
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
              description="Add an HTTP or stdio MCP Provider to populate the Catalog."
            />
          )}
        </CardContent>
      </Card>

      <ProviderFormDialog
        open={createOpen}
        mode="create"
        form={createForm}
        busy={busy === "create"}
        onOpenChange={setCreateDialogOpen}
        onChange={setCreateForm}
        onSubmit={() => void addProvider()}
      />

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
  busy,
  onOpenChange,
  onChange,
  onSubmit,
}: {
  open: boolean;
  mode: "create" | "edit";
  form: ProviderFormState;
  busy: boolean;
  onOpenChange: (open: boolean) => void;
  onChange: (value: ProviderFormState) => void;
  onSubmit: () => void;
}) {
  const [importError, setImportError] = useState("");
  const creating = mode === "create";
  const valid =
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
              ? "Register one MCP source. Choose the transport, then provide only the connection fields it needs."
              : "Update mutable metadata and connection settings. Namespace and transport remain stable."}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {creating ? (
            <div>
              <div className="mb-2 text-sm font-medium">Transport</div>
              <div className="grid grid-cols-2 gap-2">
                <Button
                  type="button"
                  variant={form.transport === "http" ? "default" : "outline"}
                  aria-pressed={form.transport === "http"}
                  onClick={() =>
                    onChange({ ...form, transport: "http" })
                  }
                >
                  Remote MCP
                </Button>
                <Button
                  type="button"
                  variant={form.transport === "stdio" ? "default" : "outline"}
                  aria-pressed={form.transport === "stdio"}
                  onClick={() =>
                    onChange({ ...form, transport: "stdio" })
                  }
                >
                  Local MCP
                </Button>
              </div>
            </div>
          ) : (
            <div className="grid grid-cols-2 gap-3">
              <ReadOnlyField label="Namespace" value={form.namespace} />
              <ReadOnlyField
                label="Transport"
                value={form.transport === "http" ? "HTTP" : "stdio"}
              />
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

          {form.transport === "http" ? (
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

                  <Field label="HTTP header templates" optional>
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
                      One header per line. Sensitive headers must use environment
                      references such as <code>{"${ANYSEARCH_API_KEY}"}</code>.
                      The resolved secret is never stored in Mender.
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
          )}
        </div>

        <DialogFooter>
          <DialogClose render={<Button variant="outline" />}>
            Cancel
          </DialogClose>
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
            <DetailRow label="Transport" value={provider.transport} />
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
    transport: provider.transport === "stdio" ? "stdio" : "http",
    namespace: provider.namespace,
    displayName: provider.displayName,
    description: provider.description,
    endpoint: provider.endpoint ?? "",
    headers: formatHeaders(provider.headers ?? {}),
    importJson: "",
    executable: provider.executable ?? "",
    args: provider.args?.join("\n") ?? "",
    directory: provider.directory ?? "",
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
