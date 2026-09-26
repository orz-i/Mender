import { useCallback, useEffect, useMemo, useState } from "react";

import type {
  MenderClient,
  Profile,
  Provider,
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
  const [providers, setProviders] = useState<Provider[]>([]);
  const [catalogTools, setCatalogTools] = useState<ToolSummary[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] = useState(emptyCreateForm);

  const [manageSlug, setManageSlug] = useState("");
  const [pickerSlug, setPickerSlug] = useState("");
  const [pickerQuery, setPickerQuery] = useState("");
  const [pickerProvider, setPickerProvider] = useState("");
  const [pickerTools, setPickerTools] = useState<ToolSummary[]>([]);
  const [pickerSelected, setPickerSelected] = useState<Set<string>>(new Set());
  const [pickerDirect, setPickerDirect] = useState<Record<string, boolean>>({});

  const refreshProfiles = useCallback(async () => {
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
    void Promise.all([
      client.listProfiles(),
      client.listProviders(),
      client.browseTools({ limit: 100 }),
    ])
      .then(([profileResult, providerResult, toolResult]) => {
        if (active) {
          setProfiles(profileResult.profiles);
          setProviders(providerResult.providers);
          setCatalogTools(toolResult.tools);
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
  const pickerProfile = useMemo(
    () => profiles.find((profile) => profile.slug === pickerSlug) ?? null,
    [profiles, pickerSlug],
  );

  const knownTools = useMemo(() => {
    const result = new Map<string, ToolSummary>();
    for (const tool of [...catalogTools, ...pickerTools]) {
      result.set(tool.toolId, tool);
    }
    return result;
  }, [catalogTools, pickerTools]);

  const providerById = useMemo(
    () => new Map(providers.map((provider) => [provider.providerId, provider])),
    [providers],
  );

  async function createProfile() {
    setBusy(true);
    setError("");
    try {
      const profile = await client.createProfile(createForm);
      setCreateOpen(false);
      setCreateForm(emptyCreateForm);
      await refreshProfiles();
      setManageSlug(profile.slug);
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  function openToolPicker() {
    if (managedProfile === null) {
      return;
    }
    const slug = managedProfile.slug;
    setPickerQuery("");
    setPickerProvider("");
    setPickerSelected(
      new Set(managedProfile.tools.map((selection) => selection.toolId)),
    );
    setPickerDirect(
      Object.fromEntries(
        managedProfile.tools.map((selection) => [
          selection.toolId,
          selection.direct,
        ]),
      ),
    );
    setPickerTools(catalogTools);

    // Base UI modal dialogs must never overlap here. Close Manage first,
    // then open the independent Tool Picker on the next task after focus/inert
    // cleanup has completed in the WebView.
    setManageSlug("");
    window.setTimeout(() => setPickerSlug(slug), 0);
  }

  async function searchPicker() {
    setBusy(true);
    setError("");
    try {
      let tools: ToolSummary[];
      if (pickerQuery.trim()) {
        const result = await client.searchTools({
          query: pickerQuery,
          limit: 100,
        });
        tools = result.tools;
        if (pickerProvider) {
          tools = tools.filter((tool) => tool.providerId === pickerProvider);
        }
      } else {
        const result = await client.browseTools({
          limit: 100,
          ...(pickerProvider ? { providerId: pickerProvider } : {}),
        });
        tools = result.tools;
      }
      setPickerTools(tools);
      setCatalogTools((current) => mergeTools(current, tools));
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  async function saveToolSelection() {
    if (pickerProfile === null) {
      return;
    }

    const tools = [...pickerSelected].map((toolId) => ({
      toolId,
      direct:
        pickerProfile.mode === "direct"
          ? true
          : pickerProfile.mode === "dynamic"
            ? false
            : (pickerDirect[toolId] ?? false),
    }));

    setBusy(true);
    setError("");
    try {
      const profile = await client.replaceProfileTools(pickerProfile.slug, {
        tools,
      });
      setProfiles((current) =>
        current.map((item) => (item.slug === profile.slug ? profile : item)),
      );
      returnToManage(profile.slug);
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  function closeToolPicker() {
    returnToManage(pickerProfile?.slug ?? "");
  }

  function returnToManage(slug: string) {
    setPickerSlug("");
    setPickerQuery("");
    setPickerProvider("");
    if (slug) {
      window.setTimeout(() => setManageSlug(slug), 0);
    }
  }

  async function removeTool(toolId: string) {
    if (managedProfile === null) {
      return;
    }

    setBusy(true);
    setError("");
    try {
      const profile = await client.replaceProfileTools(managedProfile.slug, {
        tools: managedProfile.tools
          .filter((selection) => selection.toolId !== toolId)
          .map((selection) => ({
            toolId: selection.toolId,
            direct: selection.direct,
          })),
      });
      setProfiles((current) =>
        current.map((item) => (item.slug === profile.slug ? profile : item)),
      );
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  async function setDirect(toolId: string, direct: boolean) {
    if (managedProfile === null || managedProfile.mode !== "hybrid") {
      return;
    }

    setBusy(true);
    setError("");
    try {
      const profile = await client.replaceProfileTools(managedProfile.slug, {
        tools: managedProfile.tools.map((selection) => ({
          toolId: selection.toolId,
          direct: selection.toolId === toolId ? direct : selection.direct,
        })),
      });
      setProfiles((current) =>
        current.map((item) => (item.slug === profile.slug ? profile : item)),
      );
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
            {profiles.length} configured{" "}
            {profiles.length === 1 ? "profile" : "profiles"}
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
                      <div className="text-xs text-muted-foreground">
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
                        aria-label={"Manage " + profile.slug}
                        onClick={() => setManageSlug(profile.slug)}
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
              Choose an exposure mode, then select Tools from the Catalog.
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
              Hybrid supports both and lets selected Tools opt into direct export.
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
          }
        }}
      >
        <DialogContent className="sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>
              {managedProfile?.displayName ?? "Manage profile"}
            </DialogTitle>
            <DialogDescription>
              Choose which Catalog Tools this Profile may expose.
            </DialogDescription>
          </DialogHeader>

          {managedProfile !== null ? (
            <div className="space-y-5">
              <div className="grid grid-cols-2 gap-3">
                <ReadOnlyField label="Profile" value={managedProfile.slug} />
                <ReadOnlyField label="Mode" value={managedProfile.mode} />
              </div>

              <div className="flex items-center justify-between">
                <div>
                  <div className="font-medium">Selected Tools</div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    Tool identity is managed internally; select by name and source.
                  </p>
                </div>
                <Button
                  disabled={busy}
                  onClick={openToolPicker}
                >
                  Choose tools
                </Button>
              </div>

              {managedProfile.tools.length ? (
                <div className="divide-y divide-border rounded-lg border border-border">
                  {managedProfile.tools.map((selection) => {
                    const tool = knownTools.get(selection.toolId);
                    const provider =
                      tool === undefined
                        ? undefined
                        : providerById.get(tool.providerId);
                    return (
                      <div
                        key={selection.toolId}
                        className="flex items-center justify-between gap-4 px-4 py-3"
                      >
                        <div className="min-w-0">
                          <p className="truncate text-sm font-medium">
                            {tool?.title || tool?.name || "Unavailable Tool"}
                          </p>
                          <p className="mt-1 truncate text-xs text-muted-foreground">
                            {provider?.displayName ||
                              provider?.namespace ||
                              "Unknown Provider"}
                            {tool?.description
                              ? " · " + tool.description
                              : ""}
                          </p>
                        </div>
                        <div className="flex shrink-0 items-center gap-3">
                          {managedProfile.mode === "hybrid" ? (
                            <label className="flex items-center gap-2 text-xs text-muted-foreground">
                              <input
                                type="checkbox"
                                checked={selection.direct}
                                disabled={busy}
                                onChange={(event) =>
                                  void setDirect(
                                    selection.toolId,
                                    event.target.checked,
                                  )
                                }
                              />
                              Direct export
                            </label>
                          ) : (
                            <Badge variant="outline">
                              {managedProfile.mode === "direct"
                                ? "Direct"
                                : "Dynamic"}
                            </Badge>
                          )}
                          <Button
                            variant="outline"
                            size="sm"
                            aria-label={
                              "Remove " +
                              (tool?.title || tool?.name || "selected tool")
                            }
                            disabled={busy}
                            onClick={() => void removeTool(selection.toolId)}
                          >
                            Remove
                          </Button>
                        </div>
                      </div>
                    );
                  })}
                </div>
              ) : (
                <EmptyState
                  title="No tools selected"
                  description="Choose Tools from the Catalog to define this Profile's exposure boundary."
                />
              )}
            </div>
          ) : null}

          <DialogFooter showCloseButton />
        </DialogContent>
      </Dialog>

      <ToolPickerDialog
        open={pickerProfile !== null}
        profile={pickerProfile}
        providers={providers}
        tools={pickerTools}
        query={pickerQuery}
        providerId={pickerProvider}
        selected={pickerSelected}
        direct={pickerDirect}
        busy={busy}
        onQueryChange={setPickerQuery}
        onProviderChange={setPickerProvider}
        onSelectedChange={setPickerSelected}
        onDirectChange={setPickerDirect}
        onApply={() => void searchPicker()}
        onSave={() => void saveToolSelection()}
        onClose={closeToolPicker}
      />
    </>
  );
}

function ToolPickerDialog({
  open,
  profile,
  providers,
  tools,
  query,
  providerId,
  selected,
  direct,
  busy,
  onQueryChange,
  onProviderChange,
  onSelectedChange,
  onDirectChange,
  onApply,
  onSave,
  onClose,
}: {
  open: boolean;
  profile: Profile | null;
  providers: Provider[];
  tools: ToolSummary[];
  query: string;
  providerId: string;
  selected: Set<string>;
  direct: Record<string, boolean>;
  busy: boolean;
  onQueryChange: (value: string) => void;
  onProviderChange: (value: string) => void;
  onSelectedChange: (value: Set<string>) => void;
  onDirectChange: (value: Record<string, boolean>) => void;
  onApply: () => void;
  onSave: () => void;
  onClose: () => void;
}) {
  const providerById = new Map(
    providers.map((provider) => [provider.providerId, provider]),
  );

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) {
          onClose();
        }
      }}
    >
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>Choose tools</DialogTitle>
          <DialogDescription>
            Browse or search the Catalog, then select the Tools this Profile may
            expose.
          </DialogDescription>
        </DialogHeader>

        <div className="grid grid-cols-[minmax(0,1fr)_220px_auto] gap-3">
          <Input
            aria-label="Profile tool search"
            placeholder="Search tools"
            value={query}
            onChange={(event) => onQueryChange(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                onApply();
              }
            }}
          />
          <select
            aria-label="Provider filter"
            value={providerId}
            onChange={(event) => onProviderChange(event.target.value)}
            className="h-8 rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40"
          >
            <option value="">All Providers</option>
            {providers.map((provider) => (
              <option key={provider.providerId} value={provider.providerId}>
                {provider.displayName || provider.namespace}
              </option>
            ))}
          </select>
          <Button variant="outline" disabled={busy} onClick={onApply}>
            {busy ? "Loading…" : "Apply"}
          </Button>
        </div>

        <div className="flex items-center justify-between text-xs text-muted-foreground">
          <span>{tools.length} visible Tools</span>
          <span>{selected.size} selected</span>
        </div>

        <div className="max-h-[52vh] divide-y divide-border overflow-y-auto rounded-lg border border-border">
          {tools.length ? (
            tools.map((tool) => {
              const isSelected = selected.has(tool.toolId);
              const provider = providerById.get(tool.providerId);
              return (
                <div
                  key={tool.toolId}
                  className="flex items-start gap-3 px-4 py-3"
                >
                  <input
                    aria-label={"Select " + (tool.title || tool.name)}
                    type="checkbox"
                    className="mt-1"
                    checked={isSelected}
                    onChange={(event) => {
                      const next = new Set(selected);
                      if (event.target.checked) {
                        next.add(tool.toolId);
                      } else {
                        next.delete(tool.toolId);
                      }
                      onSelectedChange(next);
                    }}
                  />
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium">
                      {tool.title || tool.name}
                    </p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {provider?.displayName ||
                        provider?.namespace ||
                        "Unknown Provider"}
                    </p>
                    {tool.description ? (
                      <p className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
                        {tool.description}
                      </p>
                    ) : null}
                  </div>
                  {profile?.mode === "hybrid" && isSelected ? (
                    <label className="flex shrink-0 items-center gap-2 text-xs text-muted-foreground">
                      <input
                        type="checkbox"
                        checked={direct[tool.toolId] ?? false}
                        onChange={(event) =>
                          onDirectChange({
                            ...direct,
                            [tool.toolId]: event.target.checked,
                          })
                        }
                      />
                      Direct export
                    </label>
                  ) : null}
                </div>
              );
            })
          ) : (
            <div className="p-8 text-center text-sm text-muted-foreground">
              No matching Tools.
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button disabled={busy} onClick={onSave}>
            {busy ? "Saving…" : "Save selection"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function mergeTools(
  current: ToolSummary[],
  incoming: ToolSummary[],
): ToolSummary[] {
  const byId = new Map(current.map((tool) => [tool.toolId, tool]));
  for (const tool of incoming) {
    byId.set(tool.toolId, tool);
  }
  return [...byId.values()];
}

function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <label className="block space-y-1.5">
      <span className="text-sm font-medium">{label}</span>
      {children}
    </label>
  );
}

function ReadOnlyField({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="mb-1.5 text-sm font-medium">{label}</div>
      <div className="rounded-lg border border-border bg-muted/40 px-3 py-2 text-sm text-muted-foreground">
        {value}
      </div>
    </div>
  );
}

function errorMessage(reason: unknown) {
  return reason instanceof Error ? reason.message : "Unexpected error";
}
