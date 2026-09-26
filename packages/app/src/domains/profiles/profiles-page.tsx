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

import { EmptyState, ErrorNotice, PageHeader } from "../../shared/page";

const emptyCreateForm = {
  slug: "",
  displayName: "",
  mode: "hybrid" as Profile["mode"],
};

export function ProfilesPage({ client }: { client: MenderClient }) {
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] = useState(emptyCreateForm);

  const [manageSlug, setManageSlug] = useState("");
  const [toolId, setToolId] = useState("");
  const [direct, setDirect] = useState(false);

  const refresh = useCallback(async () => {
    try {
      const result = await client.listProfiles();
      setProfiles(result.profiles);
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
      .listProfiles()
      .then((result) => {
        if (active) {
          setProfiles(result.profiles);
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

  const managedProfile = useMemo(
    () => profiles.find((profile) => profile.slug === manageSlug) ?? null,
    [profiles, manageSlug],
  );

  async function createProfile() {
    setBusy(true);
    setError("");
    try {
      const profile = await client.createProfile(createForm);
      setCreateOpen(false);
      setCreateForm(emptyCreateForm);
      await refresh();
      setManageSlug(profile.slug);
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  async function addTool() {
    if (managedProfile === null) {
      return;
    }
    setBusy(true);
    setError("");
    try {
      await client.addProfileTool(managedProfile.slug, {
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
    if (managedProfile === null) {
      return;
    }
    setBusy(true);
    setError("");
    try {
      await client.removeProfileTool(managedProfile.slug, id);
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
        description="Manage Tool exposure boundaries for MCP serving."
        actions={
          <Button onClick={() => setCreateOpen(true)}>Create profile</Button>
        }
      />
      {error ? <ErrorNotice message={error} /> : null}

      <Card>
        <CardHeader className="border-b">
          <CardTitle>Profiles</CardTitle>
          <CardDescription>
            {profiles.length} configured {profiles.length === 1 ? "profile" : "profiles"}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {!loaded ? (
            <div className="py-10 text-center text-sm text-muted-foreground">
              Loading profiles…
            </div>
          ) : profiles.length ? (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Profile</TableHead>
                  <TableHead>Mode</TableHead>
                  <TableHead>Tools</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {profiles.map((profile) => (
                  <TableRow key={profile.profileId}>
                    <TableCell>
                      <div className="font-medium">{profile.displayName}</div>
                      <div className="font-mono text-xs text-muted-foreground">
                        {profile.slug}
                      </div>
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline">{profile.mode}</Badge>
                    </TableCell>
                    <TableCell>{profile.tools.length}</TableCell>
                    <TableCell className="text-right">
                      <Button
                        variant="outline"
                        size="sm"
                        aria-label={`Manage ${profile.slug}`}
                        onClick={() => {
                          setToolId("");
                          setDirect(false);
                          setManageSlug(profile.slug);
                        }}
                      >
                        Manage
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <EmptyState
              title="No profiles"
              description="Create a Profile before exposing Tools through MCP."
            />
          )}
        </CardContent>
      </Card>

      <Dialog
        open={createOpen}
        onOpenChange={(open) => {
          setCreateOpen(open);
          if (!open) {
            setCreateForm(emptyCreateForm);
          }
        }}
      >
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Create profile</DialogTitle>
            <DialogDescription>
              Choose an exposure mode, then manage Tool membership separately.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <Field label="Slug">
              <Input
                aria-label="Profile slug"
                placeholder="default"
                value={createForm.slug}
                onChange={(event) =>
                  setCreateForm((current) => ({
                    ...current,
                    slug: event.target.value,
                  }))
                }
              />
            </Field>
            <Field label="Display name">
              <Input
                aria-label="Profile display name"
                value={createForm.displayName}
                onChange={(event) =>
                  setCreateForm((current) => ({
                    ...current,
                    displayName: event.target.value,
                  }))
                }
              />
            </Field>
            <Field label="Exposure mode">
              <select
                aria-label="Exposure mode"
                value={createForm.mode}
                onChange={(event) =>
                  setCreateForm((current) => ({
                    ...current,
                    mode: event.target.value as Profile["mode"],
                  }))
                }
                className="h-8 w-full rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40"
              >
                <option value="direct">direct</option>
                <option value="dynamic">dynamic</option>
                <option value="hybrid">hybrid</option>
              </select>
            </Field>
            <p className="text-xs leading-5 text-muted-foreground">
              Direct exports selected Tools. Dynamic exposes discovery meta-tools.
              Hybrid supports both and marks selected Tools for direct export.
            </p>
          </div>

          <DialogFooter>
            <DialogClose render={<Button variant="outline" />}>
              Cancel
            </DialogClose>
            <Button
              disabled={busy || !createForm.slug || !createForm.displayName}
              onClick={() => void createProfile()}
            >
              {busy ? "Creating…" : "Create profile"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={managedProfile !== null}
        onOpenChange={(open) => {
          if (!open) {
            setManageSlug("");
            setToolId("");
            setDirect(false);
          }
        }}
      >
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {managedProfile?.displayName ?? "Manage profile"}
            </DialogTitle>
            <DialogDescription>
              Manage Tool membership for this Profile. Profile identity and mode
              remain unchanged.
            </DialogDescription>
          </DialogHeader>

          {managedProfile !== null ? (
            <div className="space-y-5">
              <div className="grid grid-cols-2 gap-3">
                <ReadOnlyField label="Slug" value={managedProfile.slug} />
                <ReadOnlyField label="Mode" value={managedProfile.mode} />
              </div>

              <div className="rounded-lg border border-border p-4">
                <div className="mb-3">
                  <div className="font-medium">Add Tool</div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    Use the stable ToolID from Catalog.
                  </p>
                </div>
                <div className="flex items-end gap-3">
                  <Field label="ToolID" className="min-w-0 flex-1">
                    <Input
                      aria-label="ToolID"
                      className="font-mono text-xs"
                      placeholder="tool_..."
                      value={toolId}
                      onChange={(event) => setToolId(event.target.value)}
                    />
                  </Field>
                  {managedProfile.mode === "hybrid" ? (
                    <label className="flex h-8 items-center gap-2 text-sm text-muted-foreground">
                      <input
                        type="checkbox"
                        checked={direct}
                        onChange={(event) => setDirect(event.target.checked)}
                      />
                      Direct export
                    </label>
                  ) : null}
                  <Button
                    disabled={busy || !toolId}
                    onClick={() => void addTool()}
                  >
                    Add Tool
                  </Button>
                </div>
              </div>

              <div>
                <div className="mb-3 flex items-center justify-between">
                  <div className="font-medium">Selected Tools</div>
                  <Badge variant="outline">{managedProfile.tools.length}</Badge>
                </div>
                {managedProfile.tools.length ? (
                  <div className="divide-y divide-border rounded-lg border border-border">
                    {managedProfile.tools.map((tool) => (
                      <div
                        key={tool.toolId}
                        className="flex items-center justify-between gap-4 px-4 py-3"
                      >
                        <div className="min-w-0">
                          <p className="truncate font-mono text-xs">
                            {tool.toolId}
                          </p>
                          <p className="mt-1 text-xs text-muted-foreground">
                            {tool.direct ? "Direct export" : "Dynamic only"}
                          </p>
                        </div>
                        <Button
                          variant="outline"
                          size="sm"
                          aria-label={`Remove ${tool.toolId}`}
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
            </div>
          ) : null}

          <DialogFooter showCloseButton />
        </DialogContent>
      </Dialog>
    </>
  );
}

function Field({
  label,
  className = "",
  children,
}: {
  label: string;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <label className={["block space-y-1.5", className].join(" ")}>
      <span className="text-sm font-medium">{label}</span>
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

function errorMessage(reason: unknown) {
  return reason instanceof Error ? reason.message : "Unexpected error";
}
