import { useCallback, useEffect, useMemo, useState } from "react";

import type { MenderClient, Profile } from "@mender/sdk";
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

import { EmptyState, ErrorNotice, PageHeader } from "../../shared/page";

export function ProfilesPage({ client }: { client: MenderClient }) {
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [selectedSlug, setSelectedSlug] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [createForm, setCreateForm] = useState({
    slug: "",
    displayName: "",
    mode: "hybrid" as Profile["mode"],
  });
  const [toolId, setToolId] = useState("");
  const [direct, setDirect] = useState(false);

  const refresh = useCallback(async () => {
    try {
      const result = await client.listProfiles();
      setProfiles(result.profiles);
      setSelectedSlug((current) => {
        if (current && result.profiles.some((item) => item.slug === current)) {
          return current;
        }
        return result.profiles[0]?.slug ?? "";
      });
      setError("");
    } catch (reason) {
      setError(errorMessage(reason));
    }
  }, [client]);

  useEffect(() => {
    let active = true;
    void client
      .listProfiles()
      .then((result) => {
        if (!active) {
          return;
        }
        setProfiles(result.profiles);
        setSelectedSlug(result.profiles[0]?.slug ?? "");
        setError("");
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

  const selected = useMemo(
    () => profiles.find((profile) => profile.slug === selectedSlug) ?? null,
    [profiles, selectedSlug],
  );

  async function createProfile() {
    setBusy(true);
    try {
      const profile = await client.createProfile(createForm);
      setCreateForm({ slug: "", displayName: "", mode: "hybrid" });
      await refresh();
      setSelectedSlug(profile.slug);
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  async function addTool() {
    if (!selected) {
      return;
    }
    setBusy(true);
    try {
      await client.addProfileTool(selected.slug, {
        toolId,
        ...(direct ? { direct: true } : {}),
      });
      setToolId("");
      setDirect(false);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  async function removeTool(id: string) {
    if (!selected) {
      return;
    }
    setBusy(true);
    try {
      await client.removeProfileTool(selected.slug, id);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <PageHeader
        title="Profiles"
        description="Profiles are Tool exposure and security boundaries for MCP serving."
      />
      {error ? <ErrorNotice message={error} /> : null}

      <Card>
        <CardHeader>
          <CardTitle>Create profile</CardTitle>
          <CardDescription>
            Direct exports every selected Tool; dynamic exposes discovery meta-tools; hybrid combines both.
          </CardDescription>
        </CardHeader>
        <CardContent className="grid grid-cols-[180px_minmax(220px,1fr)_180px_auto] gap-3">
          <Input
            placeholder="slug"
            value={createForm.slug}
            onChange={(event) =>
              setCreateForm((current) => ({
                ...current,
                slug: event.target.value,
              }))
            }
          />
          <Input
            placeholder="Display name"
            value={createForm.displayName}
            onChange={(event) =>
              setCreateForm((current) => ({
                ...current,
                displayName: event.target.value,
              }))
            }
          />
          <select
            value={createForm.mode}
            onChange={(event) =>
              setCreateForm((current) => ({
                ...current,
                mode: event.target.value as Profile["mode"],
              }))
            }
            className="h-8 rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40"
          >
            <option value="direct">direct</option>
            <option value="dynamic">dynamic</option>
            <option value="hybrid">hybrid</option>
          </select>
          <Button
            onClick={() => void createProfile()}
            disabled={busy || !createForm.slug || !createForm.displayName}
          >
            Create
          </Button>
        </CardContent>
      </Card>

      <div className="mt-6 grid gap-5 xl:grid-cols-[300px_minmax(0,1fr)]">
        <Card>
          <CardHeader>
            <CardTitle>Profiles</CardTitle>
          </CardHeader>
          <CardContent>
            {profiles.length ? (
              <div className="space-y-2">
                {profiles.map((profile) => (
                  <button
                    type="button"
                    key={profile.profileId}
                    onClick={() => setSelectedSlug(profile.slug)}
                    className={[
                      "w-full rounded-lg border p-3 text-left",
                      profile.slug === selectedSlug
                        ? "border-primary bg-primary/5"
                        : "border-border hover:bg-muted",
                    ].join(" ")}
                  >
                    <div className="flex items-center justify-between gap-3">
                      <p className="font-medium">{profile.displayName}</p>
                      <Badge variant="outline">{profile.mode}</Badge>
                    </div>
                    <p className="mt-1 font-mono text-xs text-muted-foreground">
                      {profile.slug} · {profile.tools.length} tools
                    </p>
                  </button>
                ))}
              </div>
            ) : (
              <EmptyState
                title="No profiles"
                description="Create a Profile before exposing Tools through MCP."
              />
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{selected?.displayName ?? "Profile membership"}</CardTitle>
            <CardDescription>
              {selected
                ? `${selected.slug} · ${selected.mode}`
                : "Select a Profile to manage Tool membership."}
            </CardDescription>
          </CardHeader>
          <CardContent>
            {selected ? (
              <div className="space-y-5">
                <div className="flex items-end gap-3">
                  <div className="min-w-0 flex-1">
                    <label className="mb-2 block text-sm font-medium">
                      Add ToolID
                    </label>
                    <Input
                      className="font-mono text-xs"
                      placeholder="tool_..."
                      value={toolId}
                      onChange={(event) => setToolId(event.target.value)}
                    />
                  </div>
                  <label className="flex h-8 items-center gap-2 text-sm text-muted-foreground">
                    <input
                      type="checkbox"
                      checked={direct}
                      onChange={(event) => setDirect(event.target.checked)}
                    />
                    Direct in hybrid
                  </label>
                  <Button
                    onClick={() => void addTool()}
                    disabled={busy || !toolId}
                  >
                    Add Tool
                  </Button>
                </div>

                {selected.tools.length ? (
                  <div className="divide-y divide-border rounded-lg border border-border">
                    {selected.tools.map((tool) => (
                      <div
                        key={tool.toolId}
                        className="flex items-center justify-between gap-4 px-4 py-3"
                      >
                        <div className="min-w-0">
                          <p className="truncate font-mono text-xs">{tool.toolId}</p>
                          <p className="mt-1 text-xs text-muted-foreground">
                            {tool.direct ? "Direct export" : "Dynamic only"}
                          </p>
                        </div>
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={busy}
                          onClick={() => void removeTool(tool.toolId)}
                        >
                          Remove
                        </Button>
                      </div>
                    ))}
                  </div>
                ) : (
                  <EmptyState
                    title="No tools selected"
                    description="Find a Tool in Catalog, then add its stable ToolID."
                  />
                )}
              </div>
            ) : (
              <EmptyState
                title="Select a Profile"
                description="Profile membership will appear here."
              />
            )}
          </CardContent>
        </Card>
      </div>
    </>
  );
}

function errorMessage(reason: unknown) {
  return reason instanceof Error ? reason.message : "Unexpected error";
}
