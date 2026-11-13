import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router";
import {
  CheckIcon,
  CopyIcon,
  FileCode2Icon,
  InfoIcon,
  PlayIcon,
  RefreshCwIcon,
  SearchIcon,
  TerminalIcon,
  WrenchIcon,
} from "lucide-react";

import type {
  InvocationResult,
  ManagementClient,
  Provider,
  ToolDetail,
  ToolSummary,
} from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import { Card } from "@mender/ui/components/card";
import { Input } from "@mender/ui/components/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@mender/ui/components/select";
import { Textarea } from "@mender/ui/components/textarea";

import { t } from "../../../i18n/i18n";
import { errorMessage } from "../../../shared/errors";
import { toolSourceLabel } from "../tool-source-label";

type PlaygroundTab = "arguments" | "schema" | "description";
type ArgumentEditorMode = "form" | "json";

interface SchemaPropertyDef {
  type?: string | string[];
  title?: string;
  description?: string;
  default?: unknown;
  enum?: unknown[];
}

function getSchemaProperties(
  schema: unknown,
): { properties: Array<[string, SchemaPropertyDef]>; required: Set<string> } {
  if (!schema || typeof schema !== "object") {
    return { properties: [], required: new Set() };
  }
  const record = schema as Record<string, unknown>;
  const rawProps = record.properties;
  if (!rawProps || typeof rawProps !== "object" || Array.isArray(rawProps)) {
    return { properties: [], required: new Set() };
  }
  const properties = Object.entries(rawProps as Record<string, unknown>).filter(
    (entry): entry is [string, SchemaPropertyDef] =>
      entry[1] !== null && typeof entry[1] === "object" && !Array.isArray(entry[1]),
  );
  const required = new Set(
    Array.isArray(record.required)
      ? record.required.filter((item): item is string => typeof item === "string")
      : [],
  );
  return { properties, required };
}

function resolveSchemaType(type: string | string[] | undefined): string {
  if (Array.isArray(type)) {
    return type.find((item) => item !== "null") ?? "string";
  }
  return type ?? "string";
}

interface RunHistoryItem {
  id: string;
  toolId: string;
  toolName: string;
  timestamp: string;
  durationMs: number;
  args: Record<string, unknown>;
  result: InvocationResult;
}

export function ToolPlaygroundPage({ client }: { client: ManagementClient }) {
  const [searchParams, setSearchParams] = useSearchParams();
  const initialToolId = searchParams.get("toolId") ?? "";

  const [providers, setProviders] = useState<Provider[]>([]);
  const [tools, setTools] = useState<ToolSummary[]>([]);
  const [selectedToolId, setSelectedToolId] = useState<string>(initialToolId);
  const [activeTool, setActiveTool] = useState<ToolDetail | null>(null);

  const [searchQuery, setSearchQuery] = useState("");
  const [selectedProviderId, setSelectedProviderId] = useState<string>("all");
  const [activeTab, setActiveTab] = useState<PlaygroundTab>("arguments");
  const [argMode, setArgMode] = useState<ArgumentEditorMode>("json");

  const [argumentsText, setArgumentsText] = useState("{}");
  const [jsonError, setJsonError] = useState<string | null>(null);

  const [busy, setBusy] = useState<"load" | "invoke" | "">("load");
  const [error, setError] = useState("");
  const [invocation, setInvocation] = useState<InvocationResult | null>(null);
  const [executionMs, setExecutionMs] = useState<number | null>(null);
  const [copied, setCopied] = useState(false);
  const [history, setHistory] = useState<RunHistoryItem[]>([]);
  const [inventoryRevision, setInventoryRevision] = useState(0);

  // Load initial providers and tools
  useEffect(() => {
    let active = true;
    void Promise.all([
      client.listProviders(),
      client.browseTools({ limit: 100 }),
    ])
      .then(([provRes, toolRes]) => {
        if (!active) return;
        setProviders(provRes.providers);
        setTools(toolRes.tools);
        setBusy("");
        const candidateId = initialToolId || toolRes.tools[0]?.toolId;
        if (candidateId) {
          setSelectedToolId(candidateId);
        }
      })
      .catch((err: unknown) => {
        if (!active) return;
        setError(errorMessage(err));
        setBusy("");
      });
    return () => {
      active = false;
    };
  }, [client, initialToolId, inventoryRevision]);

  // Load active tool details
  useEffect(() => {
    if (!selectedToolId) return;
    let active = true;
    void client
      .getTool(selectedToolId)
      .then((detail) => {
        if (!active) return;
        setActiveTool(detail);
        setInvocation(null);
        setExecutionMs(null);

        // Derive template from inputSchema
        const template: Record<string, unknown> = {};
        const { properties } = getSchemaProperties(detail.inputSchema);
        for (const [key, prop] of properties) {
          const propType = resolveSchemaType(prop.type);
          template[key] =
            prop.default !== undefined
              ? prop.default
              : propType === "number" || propType === "integer"
                ? 0
                : propType === "boolean"
                  ? false
                  : Array.isArray(prop.enum) && prop.enum.length > 0
                    ? prop.enum[0]
                    : "";
        }
        const formatted = Object.keys(template).length > 0 ? JSON.stringify(template, null, 2) : "{}";
        setArgumentsText(formatted);
        setJsonError(null);
      })
      .catch((err: unknown) => {
        if (active) setError(errorMessage(err));
      });
    return () => {
      active = false;
    };
  }, [client, selectedToolId]);

  // Validate JSON on change
  const handleArgumentsChange = (text: string) => {
    setArgumentsText(text);
    try {
      const parsed: unknown = JSON.parse(text);
      if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
        setJsonError(t("Arguments must be a JSON object."));
      } else {
        setJsonError(null);
      }
    } catch {
      setJsonError(t("Arguments must be valid JSON."));
    }
  };

  const handleFormatJson = () => {
    try {
      const parsed: unknown = JSON.parse(argumentsText);
      setArgumentsText(JSON.stringify(parsed, null, 2));
      setJsonError(null);
    } catch {
      // Keep invalid text as is
    }
  };

  const handleRunTool = async () => {
    if (!activeTool || busy === "invoke") return;
    let parsedArgs: Record<string, unknown>;
    try {
      const parsed: unknown = JSON.parse(argumentsText);
      if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
        setError(t("Arguments must be a JSON object."));
        return;
      }
      parsedArgs = parsed as Record<string, unknown>;
    } catch {
      setError(t("Arguments must be valid JSON."));
      return;
    }

    setBusy("invoke");
    setError("");
    const startTime = performance.now();
    try {
      const result = await client.callTool(activeTool.toolId, parsedArgs);
      const elapsed = Math.round(performance.now() - startTime);
      setInvocation(result);
      setExecutionMs(elapsed);

      setHistory((prev) => [
        {
          id: result.invocationId || String(Date.now()),
          toolId: activeTool.toolId,
          toolName: activeTool.title || activeTool.name,
          timestamp: new Date().toLocaleTimeString(),
          durationMs: elapsed,
          args: parsedArgs,
          result,
        },
        ...prev.slice(0, 19),
      ]);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy("");
    }
  };

  const handleCopyResult = () => {
    if (!invocation) return;
    const content = JSON.stringify(invocation.content ?? invocation, null, 2);
    void navigator.clipboard?.writeText(content);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  // Keyboard shortcut Ctrl/Cmd + Enter to run
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
        e.preventDefault();
        void handleRunTool();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  });

  const { properties: schemaProperties, required: schemaRequired } = useMemo(
    () => getSchemaProperties(activeTool?.inputSchema),
    [activeTool],
  );
  const hasSchemaProperties = schemaProperties.length > 0;
  const effectiveArgMode: ArgumentEditorMode = hasSchemaProperties ? argMode : "json";

  const parsedArguments = useMemo<Record<string, unknown> | null>(() => {
    try {
      const parsed: unknown = JSON.parse(argumentsText);
      if (typeof parsed === "object" && parsed !== null && !Array.isArray(parsed)) {
        return parsed as Record<string, unknown>;
      }
      return null;
    } catch {
      return null;
    }
  }, [argumentsText]);

  const updateArgumentField = (key: string, nextValue: unknown) => {
    const base = parsedArguments ? { ...parsedArguments } : {};
    if (nextValue === undefined) {
      delete base[key];
    } else {
      base[key] = nextValue;
    }
    setArgumentsText(JSON.stringify(base, null, 2));
    setJsonError(null);
  };

  const filteredTools = useMemo(() => {
    return tools.filter((tool) => {
      if (selectedProviderId !== "all" && tool.sourceId !== selectedProviderId) {
        return false;
      }
      if (!searchQuery.trim()) return true;
      const query = searchQuery.toLowerCase();
      return (
        tool.name.toLowerCase().includes(query) ||
        (tool.title && tool.title.toLowerCase().includes(query)) ||
        (tool.description && tool.description.toLowerCase().includes(query))
      );
    });
  }, [tools, selectedProviderId, searchQuery]);

  return (
    <div
      data-slot="tool-playground"
      className="flex min-h-[520px] w-full min-w-0 flex-col gap-3 lg:h-[calc(100dvh-148px)]"
    >
      {/* Top action header */}
      <div className="flex shrink-0 flex-wrap items-center justify-between gap-3 rounded-xl border border-border/70 bg-card p-3 shadow-2xs">
        <div className="flex items-center gap-2.5">
          <TerminalIcon className="size-5 text-primary" aria-hidden="true" />
          <div>
            <h2 className="text-sm font-semibold tracking-tight text-foreground">
              {t("Tool Playground")}
            </h2>
            <p className="text-xs text-muted-foreground">
              {t("Directly test and invoke tools with live schema and responses.")}
            </p>
          </div>
        </div>

        <div className="flex items-center gap-2">
          {activeTool ? (
            <Button
              size="sm"
              onClick={() => void handleRunTool()}
              disabled={busy === "invoke" || jsonError !== null}
              className="gap-1.5 font-medium shadow-xs"
            >
              <PlayIcon className="size-3.5 fill-current" aria-hidden="true" />
              <span>{t(busy === "invoke" ? "Running…" : "Run Tool")}</span>
              <kbd className="ml-1 hidden rounded bg-primary-foreground/20 px-1 py-0.5 text-[10px] font-mono sm:inline">
                ⌘↵
              </kbd>
            </Button>
          ) : null}
          <Button
            size="icon-sm"
            variant="outline"
            aria-label={t("Refresh")}
            onClick={() => {
              setBusy("load");
              setInventoryRevision((r) => r + 1);
            }}
            disabled={busy !== ""}
          >
            <RefreshCwIcon className="size-3.5" aria-hidden="true" />
          </Button>
        </div>
      </div>

      {error ? (
        <div className="rounded-lg border border-destructive/20 bg-destructive/10 px-3 py-2 text-xs text-destructive">
          {error}
        </div>
      ) : null}

      {/* 3-Pane Studio Layout */}
      <div className="grid flex-1 min-h-0 min-w-0 grid-cols-1 gap-3 lg:grid-cols-12">
        {/* Pane 1: Tool Picker Sidebar (col 3) */}
        <Card className="flex flex-col min-h-0 overflow-hidden lg:col-span-3">
          <div className="space-y-2 border-b border-border/70 p-2.5">
            <div className="relative">
              <SearchIcon className="pointer-events-none absolute top-2.5 left-2.5 size-3.5 text-muted-foreground" />
              <Input
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder={t("Filter tools…")}
                aria-label={t("Filter tools…")}
                className="h-8 pl-8 text-xs"
              />
            </div>
            {providers.length > 0 ? (
              <Select
                value={selectedProviderId}
                onValueChange={(next) => setSelectedProviderId(String(next))}
              >
                <SelectTrigger
                  size="sm"
                  aria-label={t("Filter by provider")}
                  className="h-7 w-full text-[11px]"
                >
                  <SelectValue>
                    {selectedProviderId === "all"
                      ? t("All providers")
                      : toolSourceLabel(
                          providers.find((p) => `provider:${p.providerId}` === selectedProviderId),
                        )}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">{t("All providers")}</SelectItem>
                  {providers.map((p) => (
                    <SelectItem key={p.providerId} value={`provider:${p.providerId}`}>
                      {toolSourceLabel(p)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : null}
          </div>

          <div
            role="listbox"
            aria-label={t("Available tools")}
            className="flex-1 overflow-y-auto p-1.5 space-y-1 [scrollbar-width:thin]"
          >
            {filteredTools.map((tool) => {
              const isSelected = tool.toolId === selectedToolId;
              return (
                <button
                  key={tool.toolId}
                  type="button"
                  role="option"
                  aria-selected={isSelected}
                  onClick={() => {
                    setSelectedToolId(tool.toolId);
                    setSearchParams({ toolId: tool.toolId });
                  }}
                  className={[
                    "flex w-full flex-col items-start gap-1 rounded-lg px-2.5 py-2 text-left text-xs transition-colors",
                    isSelected
                      ? "bg-primary/10 text-primary font-medium border border-primary/25"
                      : "text-foreground hover:bg-muted/70",
                  ].join(" ")}
                >
                  <div className="flex w-full items-center justify-between gap-1">
                    <span className="truncate">{tool.title || tool.name}</span>
                    <Badge variant="outline" className="text-[9px] px-1 py-0 shrink-0">
                      #{tool.rank}
                    </Badge>
                  </div>
                  {tool.description ? (
                    <span className="line-clamp-1 text-[11px] text-muted-foreground font-normal">
                      {tool.description}
                    </span>
                  ) : null}
                </button>
              );
            })}
            {filteredTools.length === 0 ? (
              <p className="p-4 text-center text-xs text-muted-foreground">
                {t("No tools match filter")}
              </p>
            ) : null}
          </div>
        </Card>

        {/* Pane 2: Editor & Parameters (col 5) */}
        <Card className="flex flex-col min-h-0 overflow-hidden lg:col-span-5">
          {activeTool ? (
            <>
              {/* Tool Header */}
              <div className="border-b border-border/70 p-3">
                <div className="flex items-center justify-between gap-2">
                  <h3 className="truncate font-semibold text-sm text-foreground">
                    {activeTool.title || activeTool.name}
                  </h3>
                  <Badge variant="secondary" className="font-mono text-[10px]">
                    {activeTool.sourceKey}
                  </Badge>
                </div>
                <p className="mt-1 font-mono text-[11px] text-muted-foreground truncate">
                  {activeTool.toolId}
                </p>

                {/* Tabs */}
                <div className="mt-2.5 flex items-center gap-1 border-t border-border/60 pt-2">
                  <button
                    type="button"
                    onClick={() => setActiveTab("arguments")}
                    className={[
                      "flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium transition-colors",
                      activeTab === "arguments"
                        ? "bg-muted text-foreground"
                        : "text-muted-foreground hover:text-foreground",
                    ].join(" ")}
                  >
                    <FileCode2Icon className="size-3.5" aria-hidden="true" />
                    <span>{t("Arguments")}</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => setActiveTab("schema")}
                    className={[
                      "flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium transition-colors",
                      activeTab === "schema"
                        ? "bg-muted text-foreground"
                        : "text-muted-foreground hover:text-foreground",
                    ].join(" ")}
                  >
                    <WrenchIcon className="size-3.5" aria-hidden="true" />
                    <span>{t("Input schema")}</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => setActiveTab("description")}
                    className={[
                      "flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium transition-colors",
                      activeTab === "description"
                        ? "bg-muted text-foreground"
                        : "text-muted-foreground hover:text-foreground",
                    ].join(" ")}
                  >
                    <InfoIcon className="size-3.5" aria-hidden="true" />
                    <span>{t("Description")}</span>
                  </button>
                </div>
              </div>

              {/* Tab Contents */}
              <div className="flex-1 min-h-0 overflow-y-auto p-3">
                {activeTab === "arguments" ? (
                  <div className="flex h-full flex-col gap-2.5">
                    <div className="flex items-center justify-between gap-2 text-[11px]">
                      <div className="flex items-center gap-2">
                        {hasSchemaProperties ? (
                          <div
                            role="group"
                            aria-label={t("Arguments")}
                            className="inline-flex rounded-md border border-border/80 bg-muted/40 p-0.5"
                          >
                            <button
                              type="button"
                              aria-pressed={effectiveArgMode === "form"}
                              disabled={jsonError !== null}
                              onClick={() => setArgMode("form")}
                              className={[
                                "rounded px-2 py-0.5 text-[11px] font-medium transition-colors disabled:opacity-50",
                                effectiveArgMode === "form"
                                  ? "bg-background text-foreground shadow-2xs"
                                  : "text-muted-foreground hover:text-foreground",
                              ].join(" ")}
                            >
                              {t("Form")}
                            </button>
                            <button
                              type="button"
                              aria-pressed={effectiveArgMode === "json"}
                              onClick={() => setArgMode("json")}
                              className={[
                                "rounded px-2 py-0.5 text-[11px] font-medium transition-colors",
                                effectiveArgMode === "json"
                                  ? "bg-background text-foreground shadow-2xs"
                                  : "text-muted-foreground hover:text-foreground",
                              ].join(" ")}
                            >
                              {t("JSON")}
                            </button>
                          </div>
                        ) : null}
                        <span className={jsonError ? "text-destructive font-medium" : "text-teal font-medium"}>
                          {jsonError ? t("Invalid JSON") : t("Valid JSON")}
                        </span>
                      </div>
                      {effectiveArgMode === "json" ? (
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          onClick={handleFormatJson}
                          className="h-6 text-[11px] px-2 text-muted-foreground"
                        >
                          {t("Format JSON")}
                        </Button>
                      ) : null}
                    </div>

                    {effectiveArgMode === "form" ? (
                      <div className="space-y-3 pb-2">
                        {schemaProperties.map(([key, prop]) => {
                          const fieldId = `playground-arg-${key}`;
                          const propType = resolveSchemaType(prop.type);
                          const isRequired = schemaRequired.has(key);
                          const currentValue = parsedArguments?.[key];
                          const enumValues = Array.isArray(prop.enum) && prop.enum.length > 0 ? prop.enum : null;

                          if (propType === "boolean" && !enumValues) {
                            return (
                              <label
                                key={key}
                                htmlFor={fieldId}
                                className="flex cursor-pointer items-center justify-between gap-3 rounded-lg border border-border/70 bg-muted/20 px-3 py-2 text-xs"
                              >
                                <div className="min-w-0">
                                  <span className="font-medium text-foreground">
                                    {prop.title || key}
                                    {isRequired ? <span className="ml-0.5 text-destructive">*</span> : null}
                                  </span>
                                  {prop.description ? (
                                    <p className="mt-0.5 text-[11px] text-muted-foreground">{prop.description}</p>
                                  ) : null}
                                </div>
                                <input
                                  id={fieldId}
                                  type="checkbox"
                                  checked={Boolean(currentValue)}
                                  onChange={(e) => updateArgumentField(key, e.target.checked)}
                                  className="size-4 shrink-0 rounded border-input accent-primary"
                                />
                              </label>
                            );
                          }

                          return (
                            <div key={key} className="space-y-1">
                              <label
                                htmlFor={fieldId}
                                className="flex items-center gap-1.5 text-xs font-medium text-foreground"
                              >
                                <span>{prop.title || key}</span>
                                {isRequired ? <span className="text-destructive">*</span> : null}
                                <span className="ml-auto font-mono text-[10px] font-normal text-muted-foreground">
                                  {enumValues ? "enum" : propType}
                                </span>
                              </label>
                              {enumValues ? (
                                (() => {
                                  const selectedIndex = enumValues.findIndex((item) => item === currentValue);
                                  return (
                                    <Select
                                      value={selectedIndex >= 0 ? String(selectedIndex) : ""}
                                      onValueChange={(raw) => {
                                        const idx = Number(raw);
                                        if (Number.isInteger(idx) && idx >= 0 && idx < enumValues.length) {
                                          updateArgumentField(key, enumValues[idx]);
                                        }
                                      }}
                                    >
                                      <SelectTrigger
                                        id={fieldId}
                                        size="sm"
                                        aria-label={prop.title || key}
                                        className="w-full text-xs"
                                      >
                                        <SelectValue placeholder={t("Select a value…")}>
                                          {selectedIndex >= 0
                                            ? String(enumValues[selectedIndex])
                                            : t("Select a value…")}
                                        </SelectValue>
                                      </SelectTrigger>
                                      <SelectContent>
                                        {enumValues.map((option, idx) => (
                                          <SelectItem key={idx} value={String(idx)}>
                                            {String(option)}
                                          </SelectItem>
                                        ))}
                                      </SelectContent>
                                    </Select>
                                  );
                                })()
                              ) : propType === "number" || propType === "integer" ? (
                                <Input
                                  id={fieldId}
                                  type="number"
                                  step={propType === "integer" ? 1 : "any"}
                                  value={typeof currentValue === "number" && Number.isFinite(currentValue) ? String(currentValue) : ""}
                                  onChange={(e) => {
                                    const raw = e.target.value;
                                    if (raw === "") {
                                      updateArgumentField(key, undefined);
                                      return;
                                    }
                                    const parsedNum = Number(raw);
                                    if (Number.isFinite(parsedNum)) {
                                      updateArgumentField(key, parsedNum);
                                    }
                                  }}
                                  className="h-8 text-xs"
                                />
                              ) : propType === "object" || propType === "array" ? (
                                <Textarea
                                  id={fieldId}
                                  value={JSON.stringify(
                                    currentValue ?? (propType === "array" ? [] : {}),
                                    null,
                                    2,
                                  )}
                                  onChange={(e) => {
                                    try {
                                      updateArgumentField(key, JSON.parse(e.target.value));
                                    } catch {
                                      // Ignore incomplete nested JSON until valid
                                    }
                                  }}
                                  className="min-h-20 font-mono text-xs"
                                />
                              ) : (
                                <Input
                                  id={fieldId}
                                  type="text"
                                  value={
                                    typeof currentValue === "string"
                                      ? currentValue
                                      : typeof currentValue === "number" || typeof currentValue === "boolean"
                                        ? String(currentValue)
                                        : ""
                                  }
                                  onChange={(e) => updateArgumentField(key, e.target.value)}
                                  className="h-8 text-xs"
                                />
                              )}
                              {prop.description ? (
                                <p className="text-[11px] text-muted-foreground">{prop.description}</p>
                              ) : null}
                            </div>
                          );
                        })}
                      </div>
                    ) : (
                      <Textarea
                        value={argumentsText}
                        onChange={(e) => handleArgumentsChange(e.target.value)}
                        aria-label={t("Arguments (JSON)")}
                        className="flex-1 font-mono text-xs resize-none bg-muted/20 border-border/80"
                      />
                    )}
                  </div>
                ) : null}

                {activeTab === "schema" ? (
                  <div className="space-y-3 font-mono text-xs">
                    <pre className="overflow-x-auto rounded-lg bg-muted/40 p-3 text-foreground leading-relaxed [scrollbar-width:thin]">
                      {JSON.stringify(activeTool.inputSchema ?? {}, null, 2)}
                    </pre>
                  </div>
                ) : null}

                {activeTab === "description" ? (
                  <div className="space-y-3 text-xs leading-6 text-foreground">
                    <p className="whitespace-pre-wrap">{activeTool.description || t("No description")}</p>
                    {activeTool.tags && activeTool.tags.length > 0 ? (
                      <div className="flex flex-wrap gap-1.5 pt-2 border-t border-border/60">
                        {activeTool.tags.map((tag) => (
                          <Badge key={tag} variant="secondary" className="text-[10px]">
                            {tag}
                          </Badge>
                        ))}
                      </div>
                    ) : null}
                  </div>
                ) : null}
              </div>
            </>
          ) : (
            <div className="flex h-full items-center justify-center p-6 text-center text-xs text-muted-foreground">
              {t("Select a tool on the left to begin inspecting and running.")}
            </div>
          )}
        </Card>

        {/* Pane 3: Results & History (col 4) */}
        <Card className="flex flex-col min-h-0 overflow-hidden lg:col-span-4">
          <div className="flex items-center justify-between border-b border-border/70 p-3">
            <div className="flex items-center gap-2">
              <span className="text-xs font-semibold text-foreground">{t("Execution Output")}</span>
              {executionMs !== null ? (
                <span className="font-mono text-[10px] text-muted-foreground">
                  {t("{{count}} ms", { count: executionMs })}
                </span>
              ) : null}
            </div>
            {invocation ? (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={handleCopyResult}
                className="h-6 text-[11px] gap-1 px-1.5 text-muted-foreground hover:text-foreground"
              >
                {copied ? <CheckIcon className="size-3 text-teal" /> : <CopyIcon className="size-3" />}
                <span>{t(copied ? "Copied!" : "Copy response")}</span>
              </Button>
            ) : null}
          </div>

          <div className="flex-1 min-h-0 overflow-y-auto p-3 [scrollbar-width:thin]">
            {invocation ? (
              <div className="space-y-3">
                <div className="flex items-center gap-2">
                  <Badge
                    variant={
                      invocation.kind === "complete"
                        ? "default"
                        : invocation.kind === "failed" || invocation.kind === "rejected"
                          ? "destructive"
                          : "secondary"
                    }
                    className="capitalize text-xs"
                  >
                    {invocation.kind}
                  </Badge>
                  {invocation.code ? (
                    <span className="font-mono text-[11px] text-muted-foreground">
                      {invocation.code}
                    </span>
                  ) : null}
                </div>

                {invocation.message ? (
                  <p className="rounded-md bg-muted/60 p-2 text-xs text-foreground">
                    {invocation.message}
                  </p>
                ) : null}

                <pre className="overflow-x-auto rounded-lg bg-muted/30 p-3 font-mono text-[11px] text-foreground leading-relaxed border border-border/60 [scrollbar-width:thin]">
                  {JSON.stringify(invocation.content ?? invocation, null, 2)}
                </pre>
              </div>
            ) : busy === "invoke" ? (
              <div className="flex h-40 items-center justify-center text-xs text-muted-foreground">
                <RefreshCwIcon className="mr-2 size-4 animate-spin text-primary" />
                <span>{t("Running…")}</span>
              </div>
            ) : (
              <p className="py-12 text-center text-xs text-muted-foreground">
                {t("Run a tool to inspect invocation output and telemetry.")}
              </p>
            )}

            {/* Recent History Drawer */}
            {history.length > 0 ? (
              <div className="mt-4 border-t border-border/70 pt-3">
                <p className="mb-2 text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">
                  {t("Recent Runs")}
                </p>
                <div className="space-y-1.5">
                  {history.slice(0, 5).map((item) => (
                    <button
                      key={item.id}
                      type="button"
                      onClick={() => {
                        setSelectedToolId(item.toolId);
                        setArgumentsText(JSON.stringify(item.args, null, 2));
                        setInvocation(item.result);
                        setExecutionMs(item.durationMs);
                      }}
                      className="flex w-full items-center justify-between rounded-md border border-border/50 bg-background/50 px-2 py-1.5 text-left text-[11px] hover:bg-muted"
                    >
                      <span className="truncate font-medium">{item.toolName}</span>
                      <div className="flex items-center gap-1.5 shrink-0">
                        <span className="font-mono text-[10px] text-muted-foreground">
                          {t("{{count}} ms", { count: item.durationMs })}
                        </span>
                        <Badge
                          variant={item.result.kind === "complete" ? "default" : "destructive"}
                          className="text-[9px] px-1 py-0 capitalize"
                        >
                          {item.result.kind}
                        </Badge>
                      </div>
                    </button>
                  ))}
                </div>
              </div>
            ) : null}
          </div>
        </Card>
      </div>
    </div>
  );
}
