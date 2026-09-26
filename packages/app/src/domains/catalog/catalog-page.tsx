import { useEffect, useState } from "react";

import type {
  InvocationResult,
  MenderClient,
  Provider,
  ToolDetail,
  ToolSummary,
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

export function CatalogPage({ client }: { client: MenderClient }) {
  const [query, setQuery] = useState("");
  const [providerId, setProviderId] = useState("");
  const [providers, setProviders] = useState<Provider[]>([]);
  const [results, setResults] = useState<ToolSummary[]>([]);
  const [searched, setSearched] = useState(false);
  const [loaded, setLoaded] = useState(false);

  const [selected, setSelected] = useState<ToolDetail | null>(null);
  const [argumentsText, setArgumentsText] = useState("{}");
  const [invocation, setInvocation] = useState<InvocationResult | null>(null);

  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [loadingToolId, setLoadingToolId] = useState("");

  useEffect(() => {
    let active = true;
    void Promise.all([
      client.listProviders(),
      client.browseTools({ limit: 100 }),
    ])
      .then(([providerResult, toolResult]) => {
        if (active) {
          setProviders(providerResult.providers);
          setResults(toolResult.tools);
          setSearched(false);
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

  async function browseAll() {
    setBusy("browse");
    setError("");
    try {
      const response = await client.browseTools({
        limit: 100,
        ...(providerId ? { providerId } : {}),
      });
      setResults(response.tools);
      setSearched(false);
      setSelected(null);
      setInvocation(null);
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function search() {
    if (!query.trim()) {
      return;
    }
    setBusy("search");
    setError("");
    try {
      const response = await client.searchTools({
        query,
        limit: 30,
        ...(providerId ? { providerId } : {}),
      });
      setResults(response.tools);
      setSearched(true);
      setSelected(null);
      setInvocation(null);
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function viewTool(tool: ToolSummary) {
    setLoadingToolId(tool.toolId);
    setError("");
    try {
      const detail = await client.getTool(tool.toolId);
      setSelected(detail);
      setArgumentsText("{}");
      setInvocation(null);
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setLoadingToolId("");
    }
  }

  async function callTool() {
    if (selected === null) {
      return;
    }

    let argumentsValue: unknown;
    try {
      argumentsValue = JSON.parse(argumentsText);
    } catch {
      setError("Arguments must be valid JSON.");
      return;
    }
    if (
      typeof argumentsValue !== "object" ||
      argumentsValue === null ||
      Array.isArray(argumentsValue)
    ) {
      setError("Arguments must be a JSON object.");
      return;
    }

    setBusy("invoke");
    setError("");
    try {
      const result = await client.callTool(
        selected.toolId,
        argumentsValue as Record<string, unknown>,
      );
      setInvocation(result);
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  function closeDetail() {
    setSelected(null);
    setArgumentsText("{}");
    setInvocation(null);
  }

  return (
    <>
      <PageHeader
        title="Catalog"
        description="Browse synchronized Tools or search compact metadata. Full schemas and invocation controls open only when you inspect a Tool."
      />
      {error ? <ErrorNotice message={error} /> : null}

      <Card>
        <CardContent className="pt-6">
          <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_180px_auto_auto] xl:grid-cols-[minmax(0,1fr)_220px_auto_auto]">
            <Input
              aria-label="Catalog search"
              placeholder="Search intent, e.g. create issue"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") {
                  void search();
                }
              }}
            />
            <select
              aria-label="Catalog provider filter"
              value={providerId}
              onChange={(event) => setProviderId(event.target.value)}
              className="h-8 rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40"
            >
              <option value="">All Providers</option>
              {providers.map((provider) => (
                <option key={provider.providerId} value={provider.providerId}>
                  {provider.displayName || provider.namespace}
                </option>
              ))}
            </select>
            <Button
              onClick={() => void search()}
              disabled={busy !== "" || !query.trim()}
            >
              {busy === "search" ? "Searching…" : "Search"}
            </Button>
            <Button
              variant="outline"
              onClick={() => void browseAll()}
              disabled={busy !== ""}
            >
              {busy === "browse" ? "Loading…" : "Browse"}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card className="mt-6">
        <CardHeader className="border-b">
          <CardTitle>Tools</CardTitle>
          <CardDescription>
            {!loaded
              ? "Loading synchronized Tools…"
              : searched
                ? `${results.length} compact match${results.length === 1 ? "" : "es"}`
                : `${results.length} synchronized Tool${results.length === 1 ? "" : "s"}`}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {results.length ? (
            <Table className="table-fixed">
              <colgroup>
                <col className="w-[28%]" />
                <col className="w-[24%]" />
                <col />
                <col className="hidden w-16 xl:table-column" />
                <col className="w-20" />
              </colgroup>
              <TableHeader>
                <TableRow>
                  <TableHead>Tool</TableHead>
                  <TableHead>Provider</TableHead>
                  <TableHead>Description</TableHead>
                  <TableHead className="hidden xl:table-cell">Rank</TableHead>
                  <TableHead className="w-20 text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {results.map((tool) => (
                  <TableRow key={tool.toolId}>
                    <TableCell className="min-w-0">
                      <div className="truncate font-medium">
                        {tool.title || tool.name}
                      </div>
                      <div className="truncate font-mono text-[11px] text-muted-foreground">
                        {tool.toolId}
                      </div>
                    </TableCell>
                    <TableCell className="min-w-0">
                      <div className="truncate text-sm">
                        {providerLabel(providers, tool.providerId)}
                      </div>
                      <div className="truncate font-mono text-[11px] text-muted-foreground">
                        {tool.providerId}
                      </div>
                    </TableCell>
                    <TableCell className="min-w-0">
                      <div className="truncate text-sm text-muted-foreground">
                        {tool.description || "No description"}
                      </div>
                    </TableCell>
                    <TableCell className="hidden xl:table-cell">
                      <Badge variant="outline">#{tool.rank}</Badge>
                    </TableCell>
                    <TableCell className="w-20 text-right">
                      <Button
                        variant="outline"
                        size="sm"
                        aria-label={`View ${tool.title || tool.name}`}
                        disabled={loadingToolId !== ""}
                        onClick={() => void viewTool(tool)}
                      >
                        {loadingToolId === tool.toolId ? "Loading…" : "View"}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <EmptyState
              title={
                searched ? "No matching tools" : "No synchronized tools"
              }
              description={
                searched
                  ? "Try a different search intent or Provider filter."
                  : "Sync a Provider to populate the Catalog. Schemas load only when you inspect a Tool."
              }
            />
          )}
        </CardContent>
      </Card>

      <Dialog
        open={selected !== null}
        onOpenChange={(open) => {
          if (!open) {
            closeDetail();
          }
        }}
      >
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>
              {selected ? selected.title || selected.name : "Tool details"}
            </DialogTitle>
            <DialogDescription>
              {selected?.toolId ?? "Canonical Tool detail"}
            </DialogDescription>
          </DialogHeader>

          {selected !== null ? (
            <div className="space-y-5">
              <div>
                <p className="text-sm leading-6 text-muted-foreground">
                  {selected.description || "No description"}
                </p>
                <div className="mt-3 flex flex-wrap gap-2">
                  <Badge variant="outline">{selected.name}</Badge>
                  {selected.tags.map((tag) => (
                    <Badge key={tag} variant="secondary">
                      {tag}
                    </Badge>
                  ))}
                </div>
              </div>

              <div className="grid gap-4 lg:grid-cols-2">
                <SchemaBlock title="Input schema" value={selected.inputSchema} />
                {selected.outputSchema !== undefined ? (
                  <SchemaBlock
                    title="Output schema"
                    value={selected.outputSchema}
                  />
                ) : (
                  <div>
                    <p className="mb-2 text-sm font-medium">Output schema</p>
                    <div className="rounded-lg border border-dashed border-border p-4 text-sm text-muted-foreground">
                      Not declared by this Tool.
                    </div>
                  </div>
                )}
              </div>

              <div className="rounded-lg border border-border p-4">
                <div className="mb-3">
                  <p className="font-medium">Invoke Tool</p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    Invocation still passes through Policy, Approval, Credential,
                    Executor, and Audit.
                  </p>
                </div>
                <Textarea
                  aria-label="Invocation arguments"
                  className="min-h-32 font-mono text-xs"
                  value={argumentsText}
                  onChange={(event) => setArgumentsText(event.target.value)}
                />
                <Button
                  className="mt-3"
                  onClick={() => void callTool()}
                  disabled={busy === "invoke"}
                >
                  {busy === "invoke" ? "Invoking…" : "Invoke through Mender"}
                </Button>
              </div>

              {invocation !== null ? (
                <SchemaBlock
                  title={`Result · ${invocation.kind}`}
                  value={invocation}
                />
              ) : null}
            </div>
          ) : null}

          <DialogFooter showCloseButton />
        </DialogContent>
      </Dialog>
    </>
  );
}

function SchemaBlock({ title, value }: { title: string; value: unknown }) {
  return (
    <div className="min-w-0">
      <p className="mb-2 text-sm font-medium">{title}</p>
      <pre className="max-h-72 overflow-auto rounded-lg border border-border bg-muted/60 p-3 text-xs leading-5">
        {JSON.stringify(value, null, 2)}
      </pre>
    </div>
  );
}

function providerLabel(providers: Provider[], providerId: string) {
  const provider = providers.find((item) => item.providerId === providerId);
  if (provider === undefined) {
    return "Unknown Provider";
  }
  return provider.displayName || provider.namespace;
}

function errorMessage(reason: unknown) {
  return reason instanceof Error ? reason.message : "Unexpected error";
}
