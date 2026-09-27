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

type ProfileToolBinding = Profile["tools"][number];

type PresentationForm = Pick<
  ProfileToolBinding,
  "direct" | "alias" | "titleOverride" | "descriptionOverride"
>;

type GovernanceForm = Pick<
  ProfileToolBinding,
  "policyConstraint" | "concurrencyConstraint"
>;

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

  const [presentationSlug, setPresentationSlug] = useState("");
  const [presentationToolId, setPresentationToolId] = useState("");
  const [presentationForm, setPresentationForm] = useState<PresentationForm>({
    direct: false,
    alias: "",
    titleOverride: "",
    descriptionOverride: "",
  });
  const [governanceSlug, setGovernanceSlug] = useState("");
  const [governanceToolId, setGovernanceToolId] = useState("");
  const [governanceForm, setGovernanceForm] = useState<GovernanceForm>({
    policyConstraint: "inherit",
    concurrencyConstraint: "inherit",
  });

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
  const presentationProfile = useMemo(
    () => profiles.find((profile) => profile.slug === presentationSlug) ?? null,
    [profiles, presentationSlug],
  );
  const presentationBinding = useMemo(
    () =>
      presentationProfile?.tools.find(
        (binding) => binding.toolId === presentationToolId,
      ) ?? null,
    [presentationProfile, presentationToolId],
  );
  const governanceProfile = useMemo(
    () => profiles.find((profile) => profile.slug === governanceSlug) ?? null,
    [profiles, governanceSlug],
  );
  const governanceBinding = useMemo(
    () =>
      governanceProfile?.tools.find(
        (binding) => binding.toolId === governanceToolId,
      ) ?? null,
    [governanceProfile, governanceToolId],
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

    const existingById = new Map(
      pickerProfile.tools.map((binding) => [binding.toolId, binding]),
    );
    const tools = [...pickerSelected].map((toolId) => {
      const existing = existingById.get(toolId);
      const direct =
        pickerProfile.mode === "direct"
          ? true
          : pickerProfile.mode === "dynamic"
            ? false
            : (pickerDirect[toolId] ?? false);
      return existing === undefined
        ? { toolId, direct }
        : { ...existing, direct };
    });

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

  function openPresentation(binding: ProfileToolBinding) {
    if (managedProfile === null) {
      return;
    }
    const slug = managedProfile.slug;
    setPresentationForm({
      direct: binding.direct,
      alias: binding.alias,
      titleOverride: binding.titleOverride,
      descriptionOverride: binding.descriptionOverride,
    });
    setManageSlug("");
    window.setTimeout(() => {
      setPresentationSlug(slug);
      setPresentationToolId(binding.toolId);
    }, 0);
  }

  function closePresentation() {
    const slug = presentationSlug;
    setPresentationSlug("");
    setPresentationToolId("");
    if (slug) {
      window.setTimeout(() => setManageSlug(slug), 0);
    }
  }

  async function savePresentation() {
    if (presentationProfile === null || presentationBinding === null) {
      return;
    }
    const direct =
      presentationProfile.mode === "direct"
        ? true
        : presentationProfile.mode === "dynamic"
          ? false
          : presentationForm.direct;
    setBusy(true);
    setError("");
    try {
      const profile = await client.replaceProfileTools(presentationProfile.slug, {
        tools: presentationProfile.tools.map((binding) =>
          binding.toolId === presentationBinding.toolId
            ? { ...binding, ...presentationForm, direct }
            : binding,
        ),
      });
      setProfiles((current) =>
        current.map((item) => (item.slug === profile.slug ? profile : item)),
      );
      setPresentationSlug("");
      setPresentationToolId("");
      window.setTimeout(() => setManageSlug(profile.slug), 0);
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  function openGovernance(binding: ProfileToolBinding) {
    if (managedProfile === null) {
      return;
    }
    const slug = managedProfile.slug;
    setGovernanceForm({
      policyConstraint: binding.policyConstraint,
      concurrencyConstraint: binding.concurrencyConstraint,
    });
    setManageSlug("");
    window.setTimeout(() => {
      setGovernanceSlug(slug);
      setGovernanceToolId(binding.toolId);
    }, 0);
  }

  function closeGovernance() {
    const slug = governanceSlug;
    setGovernanceSlug("");
    setGovernanceToolId("");
    if (slug) {
      window.setTimeout(() => setManageSlug(slug), 0);
    }
  }

  async function saveGovernance() {
    if (governanceProfile === null || governanceBinding === null) {
      return;
    }
    setBusy(true);
    setError("");
    try {
      const profile = await client.replaceProfileTools(governanceProfile.slug, {
        tools: governanceProfile.tools.map((binding) =>
          binding.toolId === governanceBinding.toolId
            ? { ...binding, ...governanceForm }
            : binding,
        ),
      });
      setProfiles((current) =>
        current.map((item) => (item.slug === profile.slug ? profile : item)),
      );
      setGovernanceSlug("");
      setGovernanceToolId("");
      window.setTimeout(() => setManageSlug(profile.slug), 0);
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
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
        tools: managedProfile.tools.filter(
          (selection) => selection.toolId !== toolId,
        ),
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
                    const title =
                      selection.titleOverride ||
                      tool?.title ||
                      tool?.name ||
                      "Unavailable Tool";
                    const description =
                      selection.descriptionOverride || tool?.description || "";
                    return (
                      <div
                        key={selection.toolId}
                        className="flex items-start justify-between gap-4 px-4 py-3"
                      >
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-sm font-medium">{title}</p>
                          <p className="mt-1 truncate text-xs text-muted-foreground">
                            {provider?.displayName ||
                              provider?.namespace ||
                              "Unknown Provider"}
                            {description ? " · " + description : ""}
                          </p>
                          <div className="mt-2 flex flex-wrap gap-1.5">
                            <Badge variant="outline">
                              {selection.direct ? "Direct" : "Dynamic"}
                            </Badge>
                            {selection.alias ? (
                              <Badge variant="secondary">
                                Alias: {selection.alias}
                              </Badge>
                            ) : null}
                            {selection.policyConstraint !== "inherit" ? (
                              <Badge
                                variant={
                                  selection.policyConstraint === "deny"
                                    ? "destructive"
                                    : "secondary"
                                }
                              >
                                {selection.policyConstraint === "deny"
                                  ? "Denied"
                                  : "Approval required"}
                              </Badge>
                            ) : null}
                            {selection.concurrencyConstraint === "serial" ? (
                              <Badge variant="secondary">Serial</Badge>
                            ) : null}
                          </div>
                        </div>
                        <div className="flex shrink-0 flex-wrap justify-end gap-2">
                          <Button
                            variant="outline"
                            size="sm"
                            aria-label={"Presentation for " + title}
                            disabled={busy}
                            onClick={() => openPresentation(selection)}
                          >
                            Presentation
                          </Button>
                          <Button
                            variant="outline"
                            size="sm"
                            aria-label={"Governance for " + title}
                            disabled={busy}
                            onClick={() => openGovernance(selection)}
                          >
                            Governance
                          </Button>
                          <Button
                            variant="outline"
                            size="sm"
                            aria-label={"Remove " + title}
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

      <Dialog
        open={presentationBinding !== null}
        onOpenChange={(open) => {
          if (!open) {
            closePresentation();
          }
        }}
      >
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>Tool presentation</DialogTitle>
            <DialogDescription>
              Configure Profile-specific model-facing presentation without changing the Catalog Tool.
            </DialogDescription>
          </DialogHeader>

          {presentationBinding !== null && presentationProfile !== null ? (
            <div className="space-y-4">
              <Field label="Alias">
                <Input
                  aria-label="Tool binding alias"
                  placeholder="Inherit canonical Tool name"
                  value={presentationForm.alias}
                  onChange={(event) =>
                    setPresentationForm((current) => ({
                      ...current,
                      alias: event.target.value,
                    }))
                  }
                />
              </Field>
              <Field label="Title override">
                <Input
                  aria-label="Tool binding title override"
                  placeholder="Inherit canonical title"
                  value={presentationForm.titleOverride}
                  onChange={(event) =>
                    setPresentationForm((current) => ({
                      ...current,
                      titleOverride: event.target.value,
                    }))
                  }
                />
              </Field>
              <Field label="Description override">
                <textarea
                  aria-label="Tool binding description override"
                  className="min-h-28 w-full rounded-lg border border-input bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring/40"
                  placeholder="Inherit canonical description"
                  value={presentationForm.descriptionOverride}
                  onChange={(event) =>
                    setPresentationForm((current) => ({
                      ...current,
                      descriptionOverride: event.target.value,
                    }))
                  }
                />
              </Field>
              {presentationProfile.mode === "hybrid" ? (
                <label className="flex items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    checked={presentationForm.direct}
                    onChange={(event) =>
                      setPresentationForm((current) => ({
                        ...current,
                        direct: event.target.checked,
                      }))
                    }
                  />
                  Direct export
                </label>
              ) : (
                <p className="text-xs leading-5 text-muted-foreground">
                  Direct export is controlled by this Profile&apos;s {presentationProfile.mode} exposure mode.
                </p>
              )}
            </div>
          ) : null}

          <DialogFooter>
            <Button variant="outline" onClick={closePresentation}>
              Cancel
            </Button>
            <Button disabled={busy} onClick={() => void savePresentation()}>
              {busy ? "Saving…" : "Save presentation"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={governanceBinding !== null}
        onOpenChange={(open) => {
          if (!open) {
            closeGovernance();
          }
        }}
      >
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>Tool governance</DialogTitle>
            <DialogDescription>
              Tighten execution behavior for this Profile binding. Binding policy can never relax the global Policy decision.
            </DialogDescription>
          </DialogHeader>

          {governanceBinding !== null ? (
            <div className="space-y-5">
              <Field label="Policy constraint">
                <select
                  aria-label="Binding policy constraint"
                  value={governanceForm.policyConstraint}
                  onChange={(event) =>
                    setGovernanceForm((current) => ({
                      ...current,
                      policyConstraint: event.target.value as GovernanceForm["policyConstraint"],
                    }))
                  }
                  className="h-9 w-full rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40"
                >
                  <option value="inherit">inherit</option>
                  <option value="require_approval">require approval</option>
                  <option value="deny">deny</option>
                </select>
              </Field>
              <p className="text-xs leading-5 text-muted-foreground">
                Inherit keeps normal Runtime behavior. Require approval uses the existing Approval gate and currently fails closed when no approval adapter is configured. Deny keeps the Tool selected but rejects invocation.
              </p>
              <Field label="Concurrency constraint">
                <select
                  aria-label="Binding concurrency constraint"
                  value={governanceForm.concurrencyConstraint}
                  onChange={(event) =>
                    setGovernanceForm((current) => ({
                      ...current,
                      concurrencyConstraint: event.target.value as GovernanceForm["concurrencyConstraint"],
                    }))
                  }
                  className="h-9 w-full rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40"
                >
                  <option value="inherit">inherit</option>
                  <option value="serial">serial</option>
                </select>
              </Field>
              <p className="text-xs leading-5 text-muted-foreground">
                Serial prevents overlapping calls for this Profile and Tool binding only.
              </p>
            </div>
          ) : null}

          <DialogFooter>
            <Button variant="outline" onClick={closeGovernance}>
              Cancel
            </Button>
            <Button disabled={busy} onClick={() => void saveGovernance()}>
              {busy ? "Saving…" : "Save governance"}
            </Button>
          </DialogFooter>
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
