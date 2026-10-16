import { useCallback, useEffect, useMemo, useState } from "react";
import {
  KeyRoundIcon,
  PresentationIcon,
  Settings2Icon,
  ShieldCheckIcon,
  Trash2Icon,
} from "lucide-react";

import type {
  Credential,
  MenderClient,
  Profile,
  Provider,
  ToolSummary,
  Tunnel,
} from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
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

import { errorMessage } from "../../shared/errors";
import { toolSourceLabel } from "../tools/tool-source-label";
import { canonicalizeOwnedIdentifier, isCanonicalOwnedSlug, ownedIdentifierCreateError } from "../../shared/form-identifiers";
import { IconButton } from "../../shared/icon-button";
import { EmptyState, ErrorNotice } from "../../shared/page";
import { PageTab, PageTabs, PageToolbar, ResultPanel } from "../../shared/page-toolbar";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import { EmptyPanelState, SearchField } from "../../shared/workbench";

import { formatNumber, t, tCount } from "../../i18n/i18n";

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

const TOOL_ALIAS_PATTERN = /^[A-Za-z0-9](?:[A-Za-z0-9_.-]*[A-Za-z0-9])?$/;

type GovernanceForm = Pick<
  ProfileToolBinding,
  "policyConstraint" | "concurrencyConstraint"
>;

export function ProfilesPage({ client }: { client: MenderClient }) {
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [tunnels, setTunnels] = useState<Tunnel[]>([]);
  const [catalogTools, setCatalogTools] = useState<ToolSummary[]>([]);
  const [initialBrowseTools,setInitialBrowseTools] = useState<ToolSummary[]>([]);
  const [catalogNextCursor,setCatalogNextCursor] = useState<string|null>(null);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [inventoryQuery, setInventoryQuery] = useState("");

  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] = useState(emptyCreateForm);
  const [createError, setCreateError] = useState("");

  const [manageSlug, setManageSlug] = useState("");
  const [manageView, setManageView] = useState<"overview" | "tools">("tools");
  const [manageQuery, setManageQuery] = useState("");
  const [manageExposure, setManageExposure] = useState<"all" | "direct" | "dynamic">("all");
  const [pickerSlug, setPickerSlug] = useState("");
  const [pickerQuery, setPickerQuery] = useState("");
  const [pickerProvider, setPickerProvider] = useState("");
  const [pickerTools, setPickerTools] = useState<ToolSummary[]>([]);
  const [pickerNextCursor,setPickerNextCursor] = useState<string|null>(null);
  const [pickerAppliedQuery,setPickerAppliedQuery] = useState("");
  const [pickerAppliedProvider,setPickerAppliedProvider] = useState("");
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
  const [presentationError, setPresentationError] = useState("");
  const [governanceSlug, setGovernanceSlug] = useState("");
  const [governanceToolId, setGovernanceToolId] = useState("");
  const [governanceForm, setGovernanceForm] = useState<GovernanceForm>({
    policyConstraint: "inherit",
    concurrencyConstraint: "inherit",
  });
  const [credentialSlug, setCredentialSlug] = useState("");
  const [credentialToolId, setCredentialToolId] = useState("");
  const [credentialSelected, setCredentialSelected] = useState<Set<string>>(
    new Set(),
  );

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
      client.listCredentials(),
      client.listTunnels(),
      client.browseTools({ limit: 100 }),
    ])
      .then(([
        profileResult,
        providerResult,
        credentialResult,
        tunnelResult,
        toolResult,
      ]) => {
        if (active) {
          setProfiles(profileResult.profiles);
          setProviders(providerResult.providers);
          setCredentials(credentialResult.credentials);
          setTunnels(tunnelResult.tunnels);
          setCatalogTools(toolResult.tools);
          setInitialBrowseTools(toolResult.tools);
          setCatalogNextCursor(toolResult.nextCursor??null);
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
  const credentialProfile = useMemo(
    () => profiles.find((profile) => profile.slug === credentialSlug) ?? null,
    [profiles, credentialSlug],
  );
  const credentialBinding = useMemo(
    () =>
      credentialProfile?.tools.find(
        (binding) => binding.toolId === credentialToolId,
      ) ?? null,
    [credentialProfile, credentialToolId],
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

  const visibleManagedTools = useMemo(() => {
    if (managedProfile === null) return [];
    const needle = manageQuery.trim().toLocaleLowerCase();
    return managedProfile.tools.filter((selection) => {
      if (manageExposure !== "all" &&
        (selection.direct ? "direct" : "dynamic") !== manageExposure) {
        return false;
      }
      if (!needle) return true;
      const tool = knownTools.get(selection.toolId);
      const provider = tool ? providerById.get(tool.providerId) : undefined;
      return [
        selection.alias, selection.titleOverride, selection.descriptionOverride,
        tool?.title, tool?.name, tool?.description,
        provider?.displayName, provider?.namespace,
      ].some((value) => value?.toLocaleLowerCase().includes(needle));
    });
  }, [managedProfile, manageQuery, manageExposure, knownTools, providerById]);
  const managedToolPagination = usePagination(visibleManagedTools);

  const visibleProfiles = useMemo(() => {
    const needle = inventoryQuery.trim().toLocaleLowerCase();
    return needle ? profiles.filter((profile) =>
      [profile.displayName, profile.slug, profile.mode]
        .some((value) => value.toLocaleLowerCase().includes(needle))
    ) : profiles;
  }, [inventoryQuery, profiles]);
  const profilePagination = usePagination(visibleProfiles);

  const createSlug = canonicalizeOwnedIdentifier(createForm.slug);
  const createSlugValid = isCanonicalOwnedSlug(createSlug);
  const createDisplayNameValid = createForm.displayName.trim() !== "" &&
    new TextEncoder().encode(createForm.displayName.trim()).length <= 160;
  const createValid = createSlugValid && createDisplayNameValid;

  async function createProfile() {
    if (!createValid || busy) return;
    setBusy(true);
    setCreateError("");
    try {
      const profile = await client.createProfile({
        ...createForm,
        slug: createSlug,
        displayName: createForm.displayName.trim(),
      });
      setCreateOpen(false);
      setCreateForm(emptyCreateForm);
      await refreshProfiles();
      setManageSlug(profile.slug);
    } catch (reason) {
      setCreateError(ownedIdentifierCreateError(reason, "profile"));
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
    setPickerTools(initialBrowseTools);
    setPickerNextCursor(catalogNextCursor);
    setPickerAppliedQuery("");
    setPickerAppliedProvider("");

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
      const query=pickerQuery.trim();
      const providerId=pickerProvider;
      const response=query
        ? await client.searchTools({query,limit:100,...(providerId?{providerId}:{})})
        : await client.browseTools({limit:100,...(providerId?{providerId}:{})});
      setPickerTools(response.tools);
      setPickerNextCursor(response.nextCursor??null);
      setPickerAppliedQuery(query);
      setPickerAppliedProvider(providerId);
      setCatalogTools(current=>mergeTools(current,response.tools));
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  async function loadMorePicker() {
    if (!pickerNextCursor || busy) return;
    setBusy(true);
    setError("");
    try {
      const response=pickerAppliedQuery
        ? await client.searchTools({query:pickerAppliedQuery,limit:100,cursor:pickerNextCursor,
            ...(pickerAppliedProvider?{providerId:pickerAppliedProvider}:{})})
        : await client.browseTools({limit:100,cursor:pickerNextCursor,
            ...(pickerAppliedProvider?{providerId:pickerAppliedProvider}:{})});
      setPickerTools(current=>mergeTools(current,response.tools));
      setCatalogTools(current=>mergeTools(current,response.tools));
      setPickerNextCursor(response.nextCursor??null);
    } catch(reason) {
      setPickerNextCursor(null);
      setError(errorMessage(reason));
    } finally {setBusy(false);}
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
    setPresentationError("");
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
    setPresentationError("");
    const slug = presentationSlug;
    setPresentationSlug("");
    setPresentationToolId("");
    if (slug) {
      window.setTimeout(() => setManageSlug(slug), 0);
    }
  }

  const trimmedAlias = presentationForm.alias.trim();
  const aliasValid = trimmedAlias === "" || (trimmedAlias.length <= 128 && TOOL_ALIAS_PATTERN.test(trimmedAlias));

  async function savePresentation() {
    if (presentationProfile === null || presentationBinding === null || !aliasValid) {
      return;
    }
    const direct =
      presentationProfile.mode === "direct"
        ? true
        : presentationProfile.mode === "dynamic"
          ? false
          : presentationForm.direct;
    setBusy(true);
    setPresentationError("");
    try {
      const profile = await client.replaceProfileTools(presentationProfile.slug, {
        tools: presentationProfile.tools.map((binding) =>
          binding.toolId === presentationBinding.toolId
            ? { ...binding, ...presentationForm, alias: trimmedAlias, direct }
            : binding,
        ),
      });
      setProfiles((current) =>
        current.map((item) => (item.slug === profile.slug ? profile : item)),
      );
      setPresentationSlug("");
      setPresentationToolId("");
      window.setTimeout(() => setManageSlug(profile.slug), 0);
    } catch {
      setPresentationError(t("Could not save Tool presentation. Review the fields and try again."));
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

  function openCredentials(binding: ProfileToolBinding) {
    if (managedProfile === null) {
      return;
    }
    const slug = managedProfile.slug;
    setCredentialSelected(new Set(binding.credentialRefs));
    setManageSlug("");
    window.setTimeout(() => {
      setCredentialSlug(slug);
      setCredentialToolId(binding.toolId);
    }, 0);
  }

  function closeCredentials() {
    const slug = credentialSlug;
    setCredentialSlug("");
    setCredentialToolId("");
    setCredentialSelected(new Set());
    if (slug) {
      window.setTimeout(() => setManageSlug(slug), 0);
    }
  }

  async function saveCredentials() {
    if (credentialProfile === null || credentialBinding === null) {
      return;
    }
    setBusy(true);
    setError("");
    try {
      const profile = await client.replaceProfileTools(credentialProfile.slug, {
        tools: credentialProfile.tools.map((binding) =>
          binding.toolId === credentialBinding.toolId
            ? { ...binding, credentialRefs: [...credentialSelected] }
            : binding,
        ),
      });
      setProfiles((current) =>
        current.map((item) => (item.slug === profile.slug ? profile : item)),
      );
      setCredentialSlug("");
      setCredentialToolId("");
      setCredentialSelected(new Set());
      window.setTimeout(() => setManageSlug(profile.slug), 0);
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  async function setProfileTunnel(tunnelId: string) {
    if (managedProfile === null) {
      return;
    }

    setBusy(true);
    setError("");
    try {
      const profile = await client.setProfileTunnel(managedProfile.slug, {
        tunnelId,
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
      <PageToolbar
        tabs={
          <span className="text-xs text-muted-foreground">
            {tCount(profiles.length, "{{count}} profile configured", "{{count}} profiles configured")}
          </span>
        }
        filters={
          <div className="w-full min-w-0 sm:w-56 sm:shrink-0">
            <SearchField label="Search profiles" placeholder="Search profiles…" value={inventoryQuery} onChange={setInventoryQuery} />
          </div>
        }
        actions={
          <>
            <Button variant="outline" size="sm" disabled={busy} onClick={() => void refreshProfiles()}>{t("Refresh")}</Button>
            <Button size="sm" onClick={() => { setCreateError(""); setCreateOpen(true); }}>{t("Create profile")}</Button>
          </>
        }
      />
      {error ? <ErrorNotice message={error} /> : null}
      <ResultPanel title="Profile inventory">
          {!loaded ? (
            <div className="px-4 py-12 text-center text-sm text-muted-foreground">
              {t("Loading profiles…")}
            </div>
          ) : visibleProfiles.length ? (
            <>
              <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("Profile")}</TableHead>
                  <TableHead>{t("Mode")}</TableHead>
                  <TableHead>{t("Tools")}</TableHead>
                  <TableHead>{t("Tunnel")}</TableHead>
                  <TableHead className="text-right">{t("Actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {profilePagination.items.map((profile) => (
                  <TableRow key={profile.profileId}>
                    <TableCell className="font-medium">
                      {profile.displayName || profile.slug}
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline">{t(profile.mode)}</Badge>
                    </TableCell>
                    <TableCell>{formatNumber(profile.tools.length)}</TableCell>
                    <TableCell>
                      {profile.tunnelId ? (
                        <Badge variant="outline">
                          {tunnels.find(
                            (tunnel) => tunnel.tunnelId === profile.tunnelId,
                          )?.displayName ?? t("Configured")}
                        </Badge>
                      ) : (
                        <span className="text-sm text-muted-foreground">
                          {t("Local only")}
                        </span>
                      )}
                    </TableCell>
                    <TableCell className="text-right">
                      <IconButton
                        variant="outline"
                        label={t("Manage {{profile}}", {
                          profile: profile.displayName || profile.slug,
                        })}
                        onClick={() => { setManageView("tools"); setManageQuery(""); setManageExposure("all"); managedToolPagination.reset(); setManageSlug(profile.slug); }}
                      >
                        <Settings2Icon />
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
                title={profiles.length ? t("No matching profiles") : t("No profiles")}
                description={profiles.length ? t("Try a different search or filter.") : t("Create a Profile before exposing Tools through MCP.")}
              />
            </EmptyPanelState>
          )}
      </ResultPanel>

      <TablePagination
        label="Profiles"
        pagination={profilePagination}
      />

      <Dialog
        open={createOpen}
        onOpenChange={(open) => {
          setCreateOpen(open);
          if (!open) {
            setCreateForm(emptyCreateForm);
            setCreateError("");
          }
        }}
      >
        <DialogContent className="max-h-[calc(100dvh-2rem)] grid-rows-[auto_minmax(0,1fr)_auto] sm:max-w-lg">
          <div className="grid gap-3">
            <DialogHeader>
              <DialogTitle>{t("Create profile")}</DialogTitle>
              <DialogDescription>
                {t("Choose an exposure mode, then select Tools from the Catalog.")}
              </DialogDescription>
            </DialogHeader>
            {createError ? (
              <div role="alert" className="rounded-xl border border-destructive/20 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                {createError}
              </div>
            ) : null}
          </div>

          <div className="min-h-0 space-y-4 overflow-y-auto pr-1">
            <Field label="Slug">
              <Input
                aria-label={t("Profile slug")}
                placeholder={t("default")}
                value={createForm.slug}
                aria-invalid={createForm.slug.trim() !== "" && !createSlugValid}
                onChange={(event) => {
                  setCreateError("");
                  setCreateForm((current) => ({ ...current, slug: event.target.value }));
                }}
              />
              <p className="mt-1 text-xs text-muted-foreground">
                {t("Uppercase letters and spaces become a lowercase hyphenated identifier.")}
              </p>
              {createForm.slug.trim() !== "" ? (
                <p className={createSlugValid ? "mt-1 break-all font-mono text-xs" : "mt-1 text-xs text-destructive"}>
                  {createSlugValid
                    ? t("Result: {{value}}", { value: createSlug })
                    : t("Use 1–63 characters, starting with a letter and ending with a letter or digit; only lowercase letters, digits and hyphens are allowed.")}
                </p>
              ) : null}
            </Field>
            <Field label="Display name">
              <Input
                aria-label={t("Profile display name")}
                value={createForm.displayName}
                aria-invalid={createForm.displayName.trim() !== "" && !createDisplayNameValid}
                onChange={(event) => {
                  setCreateError("");
                  setCreateForm((current) => ({ ...current, displayName: event.target.value }));
                }}
              />
              {createForm.displayName.trim() !== "" && !createDisplayNameValid ? (
                <p className="mt-1 text-xs text-destructive">{t("Display name is too long (maximum 160 bytes).")}</p>
              ) : null}
            </Field>
            <Field label="Exposure mode">
              <select
                aria-label={t("Exposure mode")}
                value={createForm.mode}
                onChange={(event) =>
                  setCreateForm((current) => ({
                    ...current,
                    mode: event.target.value as Profile["mode"],
                  }))
                }
                className="h-8 w-full rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40"
              >
                <option value="direct">{t("direct")}</option>
                <option value="dynamic">{t("dynamic")}</option>
                <option value="hybrid">{t("hybrid")}</option>
              </select>
            </Field>
            <p className="text-xs leading-5 text-muted-foreground">
              {t("Direct exports selected Tools. Dynamic exposes discovery meta-tools. Hybrid supports both and lets selected Tools opt into direct export.")}
            </p>
          </div>

          <DialogFooter>
            <DialogClose render={<Button variant="outline" />}>
              {t("Cancel")}
            </DialogClose>
            <Button
              disabled={busy || !createValid}
              onClick={() => void createProfile()}
            >
              {t(busy ? "Creating…" : "Create profile")}
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
        <DialogContent className="h-[min(760px,calc(100dvh-2rem))] grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden sm:max-w-3xl">
          <div className="space-y-3">
            <DialogHeader>
              <DialogTitle>
                {managedProfile?.displayName ?? t("Manage profile")}
              </DialogTitle>
              <DialogDescription>
                {t("Choose which Catalog Tools this Profile may expose.")}
              </DialogDescription>
            </DialogHeader>
            <PageTabs label="Manage Profile sections">
              <PageTab label="Tools" count={managedProfile?.tools.length ?? 0}
                active={manageView === "tools"} onClick={() => setManageView("tools")} />
              <PageTab label="Overview" active={manageView === "overview"}
                onClick={() => setManageView("overview")} />
            </PageTabs>
          </div>

          {managedProfile !== null ? (
            <div className="min-h-0 flex flex-col gap-3">
              {manageView === "overview" ? (
                <div className="min-h-0 space-y-5 overflow-y-auto overscroll-contain pr-1">
              <div className="grid grid-cols-2 gap-3">
                <ReadOnlyField label="Profile" value={managedProfile.slug} />
                <ReadOnlyField label="Mode" value={managedProfile.mode} />
              </div>

              <div className="rounded-lg border border-border bg-muted/30 p-4">
                <Field label="Tunnel">
                  <select
                    aria-label={t("Profile tunnel")}
                    value={managedProfile.tunnelId ?? ""}
                    disabled={busy}
                    onChange={(event) =>
                      void setProfileTunnel(event.target.value)
                    }
                    className="h-9 w-full rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40"
                  >
                    <option value="">{t("Local only (no Tunnel)")}</option>
                    {tunnels.map((tunnel) => (
                      <option key={tunnel.tunnelId} value={tunnel.tunnelId}>
                        {tunnel.displayName} · {t("OpenAI")}
                      </option>
                    ))}
                  </select>
                  <p className="mt-1 text-xs leading-5 text-muted-foreground">
                    {t(
                      "Select a Tunnel to make this Profile's managed MCP server reachable through that provider. Mender still binds the MCP server to loopback.",
                    )}
                  </p>
                </Field>
              </div>

                </div>
              ) : (
                <>
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <div className="font-medium">{t("Selected Tools")}</div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t("Tool identity is managed internally; select by name and source.")}
                  </p>
                </div>
                <Button
                  disabled={busy}
                  onClick={openToolPicker}
                >
                  {t("Choose tools")}
                </Button>
              </div>

              <div className="grid shrink-0 gap-2 sm:grid-cols-[minmax(0,1fr)_180px]">
                <Input
                  aria-label={t("Search selected tools")}
                  placeholder={t("Search selected tools…")}
                  value={manageQuery}
                  onChange={(event) => {
                    setManageQuery(event.target.value);
                    managedToolPagination.reset();
                  }}
                />
                <select
                  aria-label={t("Exposure filter")}
                  value={manageExposure}
                  onChange={(event) => {
                    setManageExposure(event.target.value as typeof manageExposure);
                    managedToolPagination.reset();
                  }}
                  className="h-9 min-w-0 rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40"
                >
                  <option value="all">{t("All exposures")}</option>
                  <option value="direct">{t("Direct")}</option>
                  <option value="dynamic">{t("Dynamic")}</option>
                </select>
              </div>
              {visibleManagedTools.length ? (
                <div data-slot="profile-selected-tools-list" className="min-h-0 flex-1 divide-y divide-border overflow-y-auto overscroll-contain rounded-lg border border-border">
                  {managedToolPagination.items.map((selection) => {
                    const tool = knownTools.get(selection.toolId);
                    const provider =
                      tool === undefined
                        ? undefined
                        : providerById.get(tool.providerId);
                    const title =
                      selection.titleOverride ||
                      tool?.title ||
                      tool?.name ||
                      t("Unavailable Tool");
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
                              t("Unknown Provider")}
                            {description ? " · " + description : ""}
                          </p>
                          <div className="mt-2 flex flex-wrap gap-1.5">
                            <Badge variant="outline">
                              {t(selection.direct ? "Direct" : "Dynamic")}
                            </Badge>
                            {selection.alias ? (
                              <Badge variant="secondary">
                                {t("Alias:")} {selection.alias}
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
                                {t(
                                  selection.policyConstraint === "deny"
                                    ? "Denied"
                                    : "Approval required",
                                )}
                              </Badge>
                            ) : null}
                            {selection.concurrencyConstraint === "serial" ? (
                              <Badge variant="secondary">{t("Serial")}</Badge>
                            ) : null}
                            {selection.credentialRefs.length ? (
                              <Badge variant="secondary">
                                {tCount(
                                  selection.credentialRefs.length,
                                  "{{count}} credential",
                                  "{{count}} credentials",
                                )}
                              </Badge>
                            ) : null}
                          </div>
                        </div>
                        <div className="flex shrink-0 flex-wrap justify-end gap-2">
                          <IconButton
                            variant="outline"
                            label={t("Presentation for {{tool}}", { tool: title })}
                            disabled={busy}
                            onClick={() => openPresentation(selection)}
                          >
                            <PresentationIcon />
                          </IconButton>
                          <IconButton
                            variant="outline"
                            label={t("Governance for {{tool}}", { tool: title })}
                            disabled={busy}
                            onClick={() => openGovernance(selection)}
                          >
                            <ShieldCheckIcon />
                          </IconButton>
                          <IconButton
                            variant="outline"
                            label={t("Credentials for {{tool}}", { tool: title })}
                            disabled={busy}
                            onClick={() => openCredentials(selection)}
                          >
                            <KeyRoundIcon />
                          </IconButton>
                          <IconButton
                            variant="outline"
                            label={t("Remove {{tool}}", { tool: title })}
                            disabled={busy}
                            onClick={() => void removeTool(selection.toolId)}
                          >
                            <Trash2Icon />
                          </IconButton>
                        </div>
                      </div>
                    );
                  })}
                </div>
              ) : (
                <div className="min-h-0 flex-1 overflow-y-auto rounded-lg border border-border">
                  {managedProfile.tools.length ? (
                    <EmptyState title="No matching Tools." description="Try a different search or filter." />
                  ) : (
                    <EmptyState title="No tools selected" description="Choose Tools from the Catalog to define this Profile's exposure boundary." />
                  )}
                </div>
              )}
              <div className="shrink-0">
                <TablePagination label="Selected Tools" pagination={managedToolPagination} />
              </div>
                </>
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
            <DialogTitle>{t("Tool presentation")}</DialogTitle>
            <DialogDescription>
              {t("Configure Profile-specific model-facing presentation without changing the Catalog Tool.")}
            </DialogDescription>
          </DialogHeader>

          {presentationError ? (
            <div role="alert" className="rounded-xl border border-destructive/20 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {presentationError}
            </div>
          ) : null}

          {presentationBinding !== null && presentationProfile !== null ? (
            <div className="space-y-4">
              <Field label="Alias">
                <Input
                  aria-label={t("Tool binding alias")}
                  placeholder={t("Inherit canonical Tool name")}
                  value={presentationForm.alias}
                  aria-invalid={!aliasValid}
                  onChange={(event) => {
                    setPresentationError("");
                    setPresentationForm((current) => ({
                      ...current,
                      alias: event.target.value,
                    }));
                  }}
                />
                {!aliasValid ? (
                  <p className="mt-1 text-xs text-destructive">
                    {t("Tool aliases preserve case and allow letters, digits, underscores, dots and hyphens (maximum 128 characters). Spaces are not allowed.")}
                  </p>
                ) : null}
              </Field>
              <Field label="Title override">
                <Input
                  aria-label={t("Tool binding title override")}
                  placeholder={t("Inherit canonical title")}
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
                  aria-label={t("Tool binding description override")}
                  className="min-h-28 w-full rounded-lg border border-input bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring/40"
                  placeholder={t("Inherit canonical description")}
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
                  {t("Direct export")}
                </label>
              ) : (
                <p className="text-xs leading-5 text-muted-foreground">
                  {t("Direct export is controlled by this Profile's {{mode}} exposure mode.", {
                    mode: t(presentationProfile.mode),
                  })}
                </p>
              )}
            </div>
          ) : null}

          <DialogFooter>
            <Button variant="outline" onClick={closePresentation}>
              {t("Cancel")}
            </Button>
            <Button disabled={busy || !aliasValid} onClick={() => void savePresentation()}>
              {t(busy ? "Saving…" : "Save presentation")}
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
            <DialogTitle>{t("Tool governance")}</DialogTitle>
            <DialogDescription>
              {t("Tighten execution behavior for this Profile binding. Binding policy can never relax the global Policy decision.")}
            </DialogDescription>
          </DialogHeader>

          {governanceBinding !== null ? (
            <div className="space-y-5">
              <Field label="Policy constraint">
                <select
                  aria-label={t("Binding policy constraint")}
                  value={governanceForm.policyConstraint}
                  onChange={(event) =>
                    setGovernanceForm((current) => ({
                      ...current,
                      policyConstraint: event.target.value as GovernanceForm["policyConstraint"],
                    }))
                  }
                  className="h-9 w-full rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40"
                >
                  <option value="inherit">{t("inherit")}</option>
                  <option value="require_approval">{t("require approval")}</option>
                  <option value="deny">{t("deny")}</option>
                </select>
              </Field>
              <p className="text-xs leading-5 text-muted-foreground">
                {t("Inherit keeps normal Runtime behavior. Require approval uses the existing Approval gate and currently fails closed when no approval adapter is configured. Deny keeps the Tool selected but rejects invocation.")}
              </p>
              <Field label="Concurrency constraint">
                <select
                  aria-label={t("Binding concurrency constraint")}
                  value={governanceForm.concurrencyConstraint}
                  onChange={(event) =>
                    setGovernanceForm((current) => ({
                      ...current,
                      concurrencyConstraint: event.target.value as GovernanceForm["concurrencyConstraint"],
                    }))
                  }
                  className="h-9 w-full rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40"
                >
                  <option value="inherit">{t("inherit")}</option>
                  <option value="serial">{t("serial")}</option>
                </select>
              </Field>
              <p className="text-xs leading-5 text-muted-foreground">
                {t("Serial prevents overlapping calls for this Profile and Tool binding only.")}
              </p>
            </div>
          ) : null}

          <DialogFooter>
            <Button variant="outline" onClick={closeGovernance}>
              {t("Cancel")}
            </Button>
            <Button disabled={busy} onClick={() => void saveGovernance()}>
              {t(busy ? "Saving…" : "Save governance")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={credentialBinding !== null}
        onOpenChange={(open) => {
          if (!open) {
            closeCredentials();
          }
        }}
      >
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{t("Tool credentials")}</DialogTitle>
            <DialogDescription>
              {t("Allow this Profile binding to resolve selected inventory Credentials at execution time. Secret values remain outside Profile state.")}
            </DialogDescription>
          </DialogHeader>

          {credentialBinding !== null ? (
            credentials.length ? (
              <div className="max-h-80 space-y-2 overflow-y-auto pr-1">
                {credentials.map((item) => {
                  const checked = credentialSelected.has(item.reference);
                  return (
                    <label
                      key={item.reference}
                      className="flex cursor-pointer items-start gap-3 rounded-lg border border-border px-3 py-3"
                    >
                      <input
                        type="checkbox"
                        className="mt-1"
                        checked={checked}
                        onChange={(event) =>
                          setCredentialSelected((current) => {
                            const next = new Set(current);
                            if (event.target.checked) {
                              next.add(item.reference);
                            } else {
                              next.delete(item.reference);
                            }
                            return next;
                          })
                        }
                      />
                      <span className="min-w-0 flex-1">
                        <span className="block text-sm font-medium">
                          {item.displayName}
                        </span>
                        <span className="mt-1 block truncate font-mono text-xs text-muted-foreground">
                          {item.reference}
                        </span>
                        <span className="mt-1 block text-xs text-muted-foreground">
                          {item.source === "environment"
                            ? t("Environment · {{name}}", {
                                name: item.environmentVariable,
                              })
                            : t("Native OS credential store")}
                        </span>
                      </span>
                    </label>
                  );
                })}
              </div>
            ) : (
              <EmptyState
                title="No Credentials in inventory"
                description="Create a Credential first, then return here to grant this binding access."
              />
            )
          ) : null}

          <DialogFooter>
            <Button variant="outline" onClick={closeCredentials}>
              {t("Cancel")}
            </Button>
            <Button disabled={busy} onClick={() => void saveCredentials()}>
              {t(busy ? "Saving…" : "Save credentials")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ToolPickerDialog
        open={pickerProfile !== null}
        profile={pickerProfile}
        providers={providers}
        tools={pickerTools}
        hasMore={pickerNextCursor!==null && pickerAppliedQuery===pickerQuery.trim() && pickerAppliedProvider===pickerProvider}
        onLoadMore={() => void loadMorePicker()}
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
  hasMore,
  onLoadMore,
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
  hasMore: boolean;
  onLoadMore: () => void;
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
  const pickerPagination = usePagination(tools);

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) {
          onClose();
        }
      }}
    >
      <DialogContent className="h-[min(760px,calc(100dvh-2rem))] grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>{t("Choose tools")}</DialogTitle>
          <DialogDescription>
            {t("Browse or search the Catalog, then select the Tools this Profile may expose.")}
          </DialogDescription>
        </DialogHeader>

        <div className="min-h-0 flex flex-col gap-3">
        <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,210px)_auto] sm:items-center">
          <Input
            aria-label={t("Profile tool search")}
            placeholder={t("Search tools")}
            value={query}
            onChange={(event) => {
              onQueryChange(event.target.value);
              pickerPagination.reset();
            }}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                onApply();
              }
            }}
          />
          <select
            aria-label={t("Provider filter")}
            value={providerId}
            onChange={(event) => {
              onProviderChange(event.target.value);
              pickerPagination.reset();
            }}
            className="h-8 rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40"
          >
            <option value="">{t("All Providers")}</option>
            {providers.map((provider) => (
              <option key={provider.providerId} value={provider.providerId}>
                {toolSourceLabel(provider)}
              </option>
            ))}
          </select>
          <Button variant="outline" disabled={busy} onClick={onApply}>
            {t(busy ? "Loading…" : "Apply")}
          </Button>
        </div>

        <div className="flex items-center justify-between text-xs text-muted-foreground">
          <span>
            {tCount(tools.length, "{{count}} visible Tool", "{{count}} visible Tools")}
          </span>
          <span>{tCount(selected.size, "{{count}} selected", "{{count}} selected")}</span>
        </div>

        <div data-slot="profile-tool-picker-list" className="min-h-0 flex-1 divide-y divide-border overflow-y-auto overscroll-contain rounded-lg border border-border">
          {tools.length ? (
            pickerPagination.items.map((tool) => {
              const isSelected = selected.has(tool.toolId);
              const provider = providerById.get(tool.providerId);
              return (
                <div
                  key={tool.toolId}
                  className="flex items-start gap-3 px-4 py-3"
                >
                  <input
                    aria-label={t("Select {{tool}}", { tool: tool.title || tool.name })}
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
                      {toolSourceLabel(provider)}
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
                      {t("Direct export")}
                    </label>
                  ) : null}
                </div>
              );
            })
          ) : (
            <div className="p-8 text-center text-sm text-muted-foreground">
              {t("No matching Tools.")}
            </div>
          )}
        </div>

        <div className="shrink-0">
          <TablePagination label="Catalog Tools" pagination={pickerPagination} />
          {hasMore?<div className="flex justify-center py-2">
            <Button type="button" size="sm" variant="outline" disabled={busy} onClick={onLoadMore}>
              {t("Load more tools")}
            </Button>
          </div>:null}
        </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("Cancel")}
          </Button>
          <Button disabled={busy} onClick={onSave}>
            {t(busy ? "Saving…" : "Save selection")}
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
      <span className="text-sm font-medium">{t(label)}</span>
      {children}
    </label>
  );
}

function ReadOnlyField({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="mb-1.5 text-sm font-medium">{t(label)}</div>
      <div className="rounded-lg border border-border bg-muted/40 px-3 py-2 text-sm text-muted-foreground">
        {value}
      </div>
    </div>
  );
}
