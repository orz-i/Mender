import { useEffect, useState } from "react";
import { EyeIcon } from "lucide-react";

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
  Dialog,
  DialogContent,
  DialogBody,
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
import { EmptyState, ErrorNotice } from "../../shared/page";
import { PageToolbar, ResultPanel } from "../../shared/page-toolbar";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import {
  EmptyPanelState,
} from "../../shared/workbench";

import { formatNumber, t, tCount } from "../../i18n/i18n";

type ToolView = "overview" | "description" | "schemas" | "invoke";

export function CatalogPage({ client }: { client: MenderClient }) {
  const [query, setQuery] = useState("");
  const [providerId, setProviderId] = useState("");
  const [providers, setProviders] = useState<Provider[]>([]);
  const [results, setResults] = useState<ToolSummary[]>([]);
  const [searched, setSearched] = useState(false);
  const [loaded, setLoaded] = useState(false);

  const [selected, setSelected] = useState<ToolDetail | null>(null);
  const [toolView, setToolView] = useState<ToolView | null>(null);
  const [argumentsText, setArgumentsText] = useState("{}");
  const [invocation, setInvocation] = useState<InvocationResult | null>(null);

  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [loadingToolId, setLoadingToolId] = useState("");

  const catalogPagination = usePagination(results);

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
      catalogPagination.reset();
      setSearched(false);
      setSelected(null);
      setToolView(null);
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
      catalogPagination.reset();
      setSearched(true);
      setSelected(null);
      setToolView(null);
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
      setToolView("overview");
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
      setError(t("Arguments must be valid JSON."));
      return;
    }
    if (
      typeof argumentsValue !== "object" ||
      argumentsValue === null ||
      Array.isArray(argumentsValue)
    ) {
      setError(t("Arguments must be a JSON object."));
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
    setToolView(null);
    setArgumentsText("{}");
    setInvocation(null);
  }

  return (
    <>
      {error ? <ErrorNotice message={error} /> : null}

      <PageToolbar
        tabs={
          <span className="text-xs font-medium text-muted-foreground" aria-live="polite">
            {!loaded ? t("Loading synchronized Tools…") :
              searched ? tCount(results.length, "{{count}} compact match", "{{count}} compact matches") :
              tCount(results.length, "{{count}} synchronized Tool", "{{count}} synchronized Tools")}
          </span>
        }
        filters={
          <>
            <Input
              aria-label={t("Catalog search")}
              placeholder={t("Search intent, e.g. create issue")}
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              onKeyDown={(event) => { if (event.key === "Enter") void search(); }}
              className="h-9 w-full min-w-0 sm:w-60 sm:shrink-0"
            />
            <select
              aria-label={t("Catalog provider filter")}
              value={providerId}
              onChange={(event) => setProviderId(event.target.value)}
              className="h-9 w-40 min-w-0 rounded-xl border border-input bg-card px-3 text-sm text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <option value="">{t("All Providers")}</option>
              {providers.map((provider) => (
                <option key={provider.providerId} value={provider.providerId}>
                  {provider.displayName || provider.namespace}
                </option>
              ))}
            </select>
          </>
        }
        actions={
          <>
            <Button size="sm" onClick={() => void search()} disabled={busy !== "" || !query.trim()}>
              {t(busy === "search" ? "Searching…" : "Search")}
            </Button>
            <Button size="sm" variant="outline" onClick={() => void browseAll()} disabled={busy !== ""}>
              {t(busy === "browse" ? "Loading…" : "Browse")}
            </Button>
          </>
        }
      />
      <ResultPanel title="Tools">
          {results.length ? (
            <>
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
                  <TableHead>{t("Tool")}</TableHead>
                  <TableHead>{t("Provider")}</TableHead>
                  <TableHead>{t("Description")}</TableHead>
                  <TableHead className="hidden xl:table-cell">{t("Rank")}</TableHead>
                  <TableHead className="w-20 text-right">{t("Actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {catalogPagination.items.map((tool) => (
                  <TableRow key={tool.toolId}>
                    <TableCell className="min-w-0">
                      <div className="truncate font-medium">
                        {tool.title || tool.name}
                      </div>
                    </TableCell>
                    <TableCell className="min-w-0">
                      <div className="truncate text-sm">
                        {providerLabel(providers, tool.providerId)}
                      </div>
                    </TableCell>
                    <TableCell className="min-w-0">
                      <div className="truncate text-sm text-muted-foreground">
                        {tool.description || t("No description")}
                      </div>
                    </TableCell>
                    <TableCell className="hidden xl:table-cell">
                      <Badge variant="outline">#{formatNumber(tool.rank)}</Badge>
                    </TableCell>
                    <TableCell className="w-20 text-right">
                      <IconButton
                        variant="outline"
                        label={t(
                          loadingToolId === tool.toolId
                            ? "Loading…"
                            : "View {{tool}}",
                          { tool: tool.title || tool.name },
                        )}
                        disabled={loadingToolId !== ""}
                        onClick={() => void viewTool(tool)}
                      >
                        <EyeIcon />
                      </IconButton>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
              </Table>
            </>
          ) : (
            <EmptyPanelState>
              <EmptyState
                title={
                  searched ? t("No matching tools") : t("No synchronized tools")
                }
                description={
                  searched
                    ? t("Try a different search intent or Provider filter.")
                    : t("Sync a Provider to populate the Catalog. Schemas load only when you inspect a Tool.")
                }
              />
            </EmptyPanelState>
          )}
      </ResultPanel>

      <TablePagination
        label="Catalog tools"
        pagination={catalogPagination}
      />

      <Dialog
        open={selected !== null}
        onOpenChange={(open) => {
          if (!open) {
            closeDetail();
          }
        }}
      >
        <DialogContent className="grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>
              {toolView === "description"
                ? t("Tool description")
                : toolView === "schemas"
                  ? t("Tool schemas")
                  : toolView === "invoke"
                    ? t("Invoke tool")
                    : selected?.title || selected?.name || t("Tool details")}
            </DialogTitle>
            <DialogDescription>
              {selected === null
                ? t("Canonical Tool detail")
                : toolView === "overview" || toolView === null
                  ? providerLabel(providers, selected.providerId)
                  : selected.title || selected.name}
            </DialogDescription>
          </DialogHeader>

          <DialogBody>
          {selected !== null && (toolView === "overview" || toolView === null) ? (
            <div className="space-y-5">
              <div className="grid gap-3 sm:grid-cols-2">
                <ToolMeta label="Provider" value={providerLabel(providers, selected.providerId)} />
                <ToolMeta label="Name" value={selected.name} />
                <ToolMeta label="Tool ID" value={selected.toolId} mono />
                <ToolMeta label="Source key" value={selected.sourceKey} mono />
              </div>

              <div className="rounded-lg border border-border p-4">
                <p className="text-sm font-medium">{t("Description")}</p>
                <p className="mt-2 line-clamp-3 text-sm leading-6 text-muted-foreground">
                  {selected.description || t("No description")}
                </p>
                {selected.description ? (
                  <Button
                    className="mt-2 px-0"
                    variant="ghost"
                    size="sm"
                    onClick={() => setToolView("description")}
                  >
                    {t("Read full description")}
                  </Button>
                ) : null}
              </div>

              {selected.tags.length ? (
                <div className="flex flex-wrap gap-2">
                  {selected.tags.map((tag) => (
                    <Badge key={tag} variant="secondary">
                      {tag}
                    </Badge>
                  ))}
                </div>
              ) : null}

              <div className="grid gap-3 sm:grid-cols-2">
                <Button
                  variant="outline"
                  onClick={() => setToolView("schemas")}
                >
                  {t("View schemas")}
                </Button>
                <Button onClick={() => setToolView("invoke")}>{t("Invoke tool")}</Button>
              </div>
            </div>
          ) : null}

          {selected !== null && toolView === "description" ? (
            <div className="rounded-lg border border-border p-4">
              <p className="whitespace-pre-wrap text-sm leading-6 text-muted-foreground">
                {selected.description || t("No description")}
              </p>
            </div>
          ) : null}

          {selected !== null && toolView === "schemas" ? (
            <div className="grid gap-4 lg:grid-cols-2">
              <JsonBlock title="Input schema" value={selected.inputSchema} />
              {selected.outputSchema !== undefined ? (
                <JsonBlock title="Output schema" value={selected.outputSchema} />
              ) : (
                <div>
                  <p className="mb-2 text-sm font-medium">{t("Output schema")}</p>
                  <div className="rounded-lg border border-dashed border-border p-4 text-sm text-muted-foreground">
                    {t("Not declared by this Tool.")}
                  </div>
                </div>
              )}
            </div>
          ) : null}

          {selected !== null && toolView === "invoke" ? (
            <div className="space-y-4">
              <div className="rounded-lg border border-border p-4">
                <p className="font-medium">{t("Arguments")}</p>
                <p className="mt-1 text-xs text-muted-foreground">
                  {t("Invocation passes through Policy, Approval, Credential, Executor, and Audit.")}
                </p>
                <Textarea
                  aria-label={t("Invocation arguments")}
                  className="mt-3 min-h-40 font-mono text-xs"
                  value={argumentsText}
                  onChange={(event) => setArgumentsText(event.target.value)}
                />
                <Button
                  className="mt-3"
                  onClick={() => void callTool()}
                  disabled={busy === "invoke"}
                >
                  {t(busy === "invoke" ? "Invoking…" : "Invoke through Mender")}
                </Button>
              </div>

              {invocation !== null ? (
                <JsonBlock
                  title={t("Result · {{kind}}", { kind: invocation.kind })}
                  value={invocation}
                />
              ) : null}
            </div>
          ) : null}

          </DialogBody>
          <DialogFooter showCloseButton>
            {toolView !== "overview" && toolView !== null ? (
              <Button variant="outline" onClick={() => setToolView("overview")}>
                {t("Back")}
              </Button>
            ) : null}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function ToolMeta({
  label,
  value,
  mono = false,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div className="min-w-0 rounded-lg border border-border bg-muted/40 p-3">
      <p className="text-xs text-muted-foreground">{t(label)}</p>
      <p
        className={[
          "mt-1 truncate text-sm",
          mono ? "font-mono text-xs" : "",
        ].join(" ")}
      >
        {value}
      </p>
    </div>
  );
}

function JsonBlock({ title, value }: { title: string; value: unknown }) {
  return (
    <div className="min-w-0">
      <p className="mb-2 text-sm font-medium">{t(title)}</p>
      <pre className="max-h-72 overflow-auto rounded-lg border border-border bg-muted/60 p-3 text-xs leading-5">
        {JSON.stringify(value, null, 2)}
      </pre>
    </div>
  );
}

function providerLabel(providers: Provider[], providerId: string) {
  const provider = providers.find((item) => item.providerId === providerId);
  if (provider === undefined) {
    return t("Unknown Provider");
  }
  return provider.displayName || provider.namespace;
}
