import { useEffect, useState } from "react";

import type {
  InvocationResult,
  MenderClient,
  Profile,
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
import { Input } from "@mender/ui/components/input";
import { Textarea } from "@mender/ui/components/textarea";

import { EmptyState, ErrorNotice, PageHeader } from "../../shared/page";

export function CatalogPage({ client }: { client: MenderClient }) {
  const [query, setQuery] = useState("");
  const [profile, setProfile] = useState("");
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [results, setResults] = useState<ToolSummary[]>([]);
  const [selected, setSelected] = useState<ToolDetail | null>(null);
  const [argumentsText, setArgumentsText] = useState("{}");
  const [invocation, setInvocation] = useState<InvocationResult | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let active = true;
    void client
      .listProfiles()
      .then((value) => {
        if (active) {
          setProfiles(value.profiles);
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

  async function search() {
    if (!query.trim()) {
      return;
    }
    setBusy(true);
    try {
      const response = await client.searchTools({
        query,
        limit: 30,
        ...(profile ? { profile } : {}),
      });
      setResults(response.tools);
      setSelected(null);
      setInvocation(null);
      setError("");
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  async function selectTool(tool: ToolSummary) {
    setBusy(true);
    try {
      const detail = await client.getTool(tool.toolId);
      setSelected(detail);
      setArgumentsText("{}");
      setInvocation(null);
      setError("");
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  async function callTool() {
    if (!selected) {
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

    setBusy(true);
    try {
      const result = await client.callTool(
        selected.toolId,
        argumentsValue as Record<string, unknown>,
      );
      setInvocation(result);
      setError("");
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <PageHeader
        title="Catalog"
        description="Search compact Tool metadata first, then load schemas only for the selected Tool."
      />
      {error ? <ErrorNotice message={error} /> : null}

      <Card>
        <CardContent className="pt-6">
          <div className="grid grid-cols-[minmax(0,1fr)_220px_auto] gap-3">
            <Input
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
              value={profile}
              onChange={(event) => setProfile(event.target.value)}
              className="h-8 rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40"
            >
              <option value="">All catalog tools</option>
              {profiles.map((item) => (
                <option key={item.profileId} value={item.slug}>
                  Profile: {item.slug}
                </option>
              ))}
            </select>
            <Button onClick={() => void search()} disabled={busy || !query.trim()}>
              Search
            </Button>
          </div>
        </CardContent>
      </Card>

      <div className="mt-6 grid gap-5 xl:grid-cols-[minmax(340px,0.85fr)_minmax(0,1.4fr)]">
        <Card>
          <CardHeader>
            <CardTitle>Results</CardTitle>
            <CardDescription>
              {results.length} compact match{results.length === 1 ? "" : "es"}
            </CardDescription>
          </CardHeader>
          <CardContent>
            {results.length ? (
              <div className="space-y-2">
                {results.map((tool) => (
                  <button
                    type="button"
                    key={tool.toolId}
                    onClick={() => void selectTool(tool)}
                    className={[
                      "w-full rounded-lg border p-3 text-left transition-colors",
                      selected?.toolId === tool.toolId
                        ? "border-primary bg-primary/5"
                        : "border-border hover:bg-muted",
                    ].join(" ")}
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div>
                        <p className="font-medium">{tool.title || tool.name}</p>
                        <p className="mt-1 line-clamp-2 text-sm text-muted-foreground">
                          {tool.description || "No description"}
                        </p>
                      </div>
                      <Badge variant="outline">#{tool.rank}</Badge>
                    </div>
                    <p className="mt-2 truncate font-mono text-[11px] text-muted-foreground">
                      {tool.toolId}
                    </p>
                  </button>
                ))}
              </div>
            ) : (
              <EmptyState
                title="Search the Catalog"
                description="Schemas are not loaded until you choose a Tool."
              />
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>
              {selected ? selected.title || selected.name : "Tool details"}
            </CardTitle>
            <CardDescription>
              {selected
                ? selected.toolId
                : "Select a search result to load its canonical schema."}
            </CardDescription>
          </CardHeader>
          <CardContent>
            {selected ? (
              <div className="space-y-5">
                <div>
                  <p className="text-sm text-muted-foreground">
                    {selected.description || "No description"}
                  </p>
                  <div className="mt-3 flex flex-wrap gap-2">
                    {selected.tags.map((tag) => (
                      <Badge key={tag} variant="secondary">
                        {tag}
                      </Badge>
                    ))}
                  </div>
                </div>

                <SchemaBlock title="Input schema" value={selected.inputSchema} />
                {selected.outputSchema !== undefined ? (
                  <SchemaBlock title="Output schema" value={selected.outputSchema} />
                ) : null}

                <div>
                  <label className="mb-2 block text-sm font-medium">
                    Invocation arguments
                  </label>
                  <Textarea
                    className="min-h-36 font-mono text-xs"
                    value={argumentsText}
                    onChange={(event) => setArgumentsText(event.target.value)}
                  />
                  <Button
                    className="mt-3"
                    onClick={() => void callTool()}
                    disabled={busy}
                  >
                    Invoke through Mender
                  </Button>
                </div>

                {invocation ? (
                  <SchemaBlock title={`Result · ${invocation.kind}`} value={invocation} />
                ) : null}
              </div>
            ) : (
              <EmptyState
                title="Progressive disclosure"
                description="Search returns metadata only. Full schemas appear here after selection."
              />
            )}
          </CardContent>
        </Card>
      </div>
    </>
  );
}

function SchemaBlock({ title, value }: { title: string; value: unknown }) {
  return (
    <div>
      <p className="mb-2 text-sm font-medium">{title}</p>
      <pre className="max-h-80 overflow-auto rounded-lg border border-border bg-muted/60 p-3 text-xs leading-5">
        {JSON.stringify(value, null, 2)}
      </pre>
    </div>
  );
}

function errorMessage(reason: unknown) {
  return reason instanceof Error ? reason.message : "Unexpected error";
}
