import { useCallback, useEffect, useState } from "react";

import type { MenderClient, Provider } from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@mender/ui/components/card";
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

export function ProvidersPage({ client }: { client: MenderClient }) {
  const [providers, setProviders] = useState<Provider[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [httpForm, setHTTPForm] = useState({
    namespace: "",
    displayName: "",
    endpoint: "",
    description: "",
  });
  const [stdioForm, setStdioForm] = useState({
    namespace: "",
    displayName: "",
    executable: "",
    args: "",
    directory: "",
    description: "",
  });

  const refresh = useCallback(async () => {
    try {
      const result = await client.listProviders();
      setProviders(result.providers);
      setError("");
    } catch (reason) {
      setError(errorMessage(reason));
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
        }
      })
      .catch((reason: unknown) => {
        if (active) {
          setError(errorMessage(reason));
        }
      });
    return () => {
      active = false;
    };
  }, [client]);

  async function addHTTP() {
    setBusy("add-http");
    try {
      await client.createHTTPProvider({
        namespace: httpForm.namespace,
        displayName: httpForm.displayName,
        endpoint: httpForm.endpoint,
        ...(httpForm.description ? { description: httpForm.description } : {}),
      });
      setHTTPForm({
        namespace: "",
        displayName: "",
        endpoint: "",
        description: "",
      });
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function addStdio() {
    setBusy("add-stdio");
    try {
      const args = stdioForm.args
        .split("\n")
        .map((value) => value.trim())
        .filter(Boolean);
      await client.createStdioProvider({
        namespace: stdioForm.namespace,
        displayName: stdioForm.displayName,
        executable: stdioForm.executable,
        ...(stdioForm.description ? { description: stdioForm.description } : {}),
        ...(stdioForm.directory ? { directory: stdioForm.directory } : {}),
        ...(args.length ? { args } : {}),
      });
      setStdioForm({
        namespace: "",
        displayName: "",
        executable: "",
        args: "",
        directory: "",
        description: "",
      });
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function syncProvider(provider: Provider) {
    setBusy(provider.providerId);
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
    if (!window.confirm(`Delete provider "${provider.namespace}"? Profile references will block deletion.`)) {
      return;
    }
    setBusy(provider.providerId);
    try {
      await client.deleteProvider(provider.providerId);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  return (
    <>
      <PageHeader
        title="Providers"
        description="Register and synchronize MCP tool sources. Persistent stdio configuration excludes environment values and secrets."
      />
      {error ? <ErrorNotice message={error} /> : null}

      <div className="grid gap-4 xl:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Remote MCP</CardTitle>
            <CardDescription>Streamable HTTP endpoint.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="grid grid-cols-2 gap-3">
              <Input
                placeholder="namespace"
                value={httpForm.namespace}
                onChange={(event) =>
                  setHTTPForm((current) => ({
                    ...current,
                    namespace: event.target.value,
                  }))
                }
              />
              <Input
                placeholder="Display name"
                value={httpForm.displayName}
                onChange={(event) =>
                  setHTTPForm((current) => ({
                    ...current,
                    displayName: event.target.value,
                  }))
                }
              />
            </div>
            <Input
              placeholder="https://example.com/mcp"
              value={httpForm.endpoint}
              onChange={(event) =>
                setHTTPForm((current) => ({
                  ...current,
                  endpoint: event.target.value,
                }))
              }
            />
            <Input
              placeholder="Description (optional)"
              value={httpForm.description}
              onChange={(event) =>
                setHTTPForm((current) => ({
                  ...current,
                  description: event.target.value,
                }))
              }
            />
            <Button
              onClick={() => void addHTTP()}
              disabled={
                busy !== "" ||
                !httpForm.namespace ||
                !httpForm.displayName ||
                !httpForm.endpoint
              }
            >
              Register HTTP provider
            </Button>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Local MCP</CardTitle>
            <CardDescription>
              Explicit executable and argv. No shell or persisted environment values.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="grid grid-cols-2 gap-3">
              <Input
                placeholder="namespace"
                value={stdioForm.namespace}
                onChange={(event) =>
                  setStdioForm((current) => ({
                    ...current,
                    namespace: event.target.value,
                  }))
                }
              />
              <Input
                placeholder="Display name"
                value={stdioForm.displayName}
                onChange={(event) =>
                  setStdioForm((current) => ({
                    ...current,
                    displayName: event.target.value,
                  }))
                }
              />
            </div>
            <Input
              placeholder="/path/to/mcp-server"
              value={stdioForm.executable}
              onChange={(event) =>
                setStdioForm((current) => ({
                  ...current,
                  executable: event.target.value,
                }))
              }
            />
            <Textarea
              placeholder={"Arguments, one per line\n--stdio"}
              value={stdioForm.args}
              onChange={(event) =>
                setStdioForm((current) => ({
                  ...current,
                  args: event.target.value,
                }))
              }
            />
            <div className="grid grid-cols-2 gap-3">
              <Input
                placeholder="Working directory (optional)"
                value={stdioForm.directory}
                onChange={(event) =>
                  setStdioForm((current) => ({
                    ...current,
                    directory: event.target.value,
                  }))
                }
              />
              <Input
                placeholder="Description (optional)"
                value={stdioForm.description}
                onChange={(event) =>
                  setStdioForm((current) => ({
                    ...current,
                    description: event.target.value,
                  }))
                }
              />
            </div>
            <Button
              onClick={() => void addStdio()}
              disabled={
                busy !== "" ||
                !stdioForm.namespace ||
                !stdioForm.displayName ||
                !stdioForm.executable
              }
            >
              Register stdio provider
            </Button>
          </CardContent>
        </Card>
      </div>

      <Card className="mt-6">
        <CardHeader>
          <CardTitle>Registered providers</CardTitle>
        </CardHeader>
        <CardContent>
          {providers.length ? (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Namespace</TableHead>
                  <TableHead>Transport</TableHead>
                  <TableHead>Connection</TableHead>
                  <TableHead>ID</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {providers.map((provider) => (
                  <TableRow key={provider.providerId}>
                    <TableCell>
                      <div className="font-medium">{provider.namespace}</div>
                      <div className="text-xs text-muted-foreground">
                        {provider.displayName}
                      </div>
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline">{provider.transport}</Badge>
                    </TableCell>
                    <TableCell className="max-w-[300px] truncate font-mono text-xs">
                      {provider.endpoint ??
                        provider.executable ??
                        "not configured"}
                    </TableCell>
                    <TableCell className="font-mono text-xs">
                      {provider.providerId}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-2">
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={busy !== ""}
                          onClick={() => void syncProvider(provider)}
                        >
                          Sync
                        </Button>
                        <Button
                          variant="destructive"
                          size="sm"
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
              description="Register an HTTP or stdio MCP Provider to populate the Catalog."
            />
          )}
        </CardContent>
      </Card>
    </>
  );
}

function errorMessage(reason: unknown) {
  return reason instanceof Error ? reason.message : "Unexpected error";
}
