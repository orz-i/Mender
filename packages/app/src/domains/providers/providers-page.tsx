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

  const refresh = useCallback(async () => {
    try {
      const result = await client.listProviders();
      setProviders(result.providers);
      setError("");
    } catch (reason) {
      setError(errorMessage(reason));
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
    setBusy("create");
    setError("");
    try {
      if (createForm.transport === "http") {
        await client.createHTTPProvider({
          namespace: createForm.namespace,
          displayName: createForm.displayName,
          endpoint: createForm.endpoint,
          ...(createForm.description
            ? { description: createForm.description }
            : {}),
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
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  function beginEdit(provider: Provider) {
    if (provider.transport === "unconfigured") {
      return;
    }
    setEditProvider(provider);
    setEditForm(formFromProvider(provider));
  }

  async function saveEdit() {
    if (editProvider === null || editForm === null) {
      return;
    }

    const request: UpdateProviderRequest =
      editForm.transport === "http"
        ? {
            transport: "http",
            displayName: editForm.displayName,
            endpoint: editForm.endpoint,
            ...(editForm.description
              ? { description: editForm.description }
              : {}),
          }
        : {
            transport: "stdio",
            displayName: editForm.displayName,
            executable: editForm.executable,
            ...(editForm.description
              ? { description: editForm.description }
              : {}),
            ...(editForm.directory ? { directory: editForm.directory } : {}),
            ...(parseArgs(editForm.args).length
              ? { args: parseArgs(editForm.args) }
              : {}),
          };

    setBusy(editProvider.providerId);
    setError("");
    try {
      await client.updateProvider(editProvider.providerId, request);
      setEditProvider(null);
      setEditForm(null);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
      await refresh();
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
          <Button onClick={() => setCreateOpen(true)}>Add provider</Button>
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
  const creating = mode === "create";
  const valid =
    form.displayName.trim() !== "" &&
    (!creating || form.namespace.trim() !== "") &&
    (form.transport === "http"
      ? form.endpoint.trim() !== ""
      : form.executable.trim() !== "");

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
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
            <DetailRow
              label="Connection"
              value={connectionSummary(provider)}
              mono
            />
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
    executable: provider.executable ?? "",
    args: provider.args?.join("\n") ?? "",
    directory: provider.directory ?? "",
  };
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
