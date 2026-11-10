import { useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useNavigate, useParams } from "react-router";
import {
  ChevronLeftIcon,
  ChevronRightIcon,
  EllipsisIcon,
  KeyRoundIcon,
  PresentationIcon,
  ShieldCheckIcon,
  Trash2Icon,
} from "lucide-react";

import type {
  Agent,
  Credential,
  ManagementClient,
  Profile,
  Provider,
  ToolSummary,
} from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@mender/ui/components/dialog";
import { Input } from "@mender/ui/components/input";
import { Popover, PopoverContent, PopoverTrigger } from "@mender/ui/components/popover";

import { errorMessage } from "../../shared/errors";
import { FormHeading } from "../../shared/form-info";
import { ProfileToolSourcePicker } from "./profile-tool-source-picker";
import { profileToolSources } from "./profile-source-label";
import { EmptyState, ErrorNotice } from "../../shared/page";
import { DetailWorkspace } from "../../shared/detail-workspace";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";

import { t, tCount } from "../../i18n/i18n";
import { usePageHeading } from "../../shell/page-chrome";

type ProfileToolBinding = Profile["tools"][number];

type PresentationForm = Pick<
  ProfileToolBinding,
  "alias" | "titleOverride" | "descriptionOverride"
>;

const TOOL_ALIAS_PATTERN = /^[A-Za-z0-9](?:[A-Za-z0-9_.-]*[A-Za-z0-9])?$/;

type GovernanceForm = Pick<
  ProfileToolBinding,
  "policyConstraint" | "concurrencyConstraint"
>;

export function ProfileEditPage({ client }: { client: ManagementClient }) {
  const { slug } = useParams();
  return <ProfileEditor key={slug} client={client} slug={slug} />;
}

function ProfileEditor({ client, slug }: { client: ManagementClient; slug: string | undefined }) {
  const navigate = useNavigate();
  const location = useLocation();
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [agents,setAgents] = useState<Agent[]>([]);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [catalogTools, setCatalogTools] = useState<ToolSummary[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [hasSnapshot, setHasSnapshot] = useState(false);
  const [retryVersion, setRetryVersion] = useState(0);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [toolActionsId, setToolActionsId] = useState<string | null>(null);
  const [pendingRemoval, setPendingRemoval] = useState<{ toolId: string; title: string } | null>(null);
  const [removeError, setRemoveError] = useState("");

  const [manageSlug, setManageSlug] = useState(slug ?? "");
  const [manageView, setManageView] = useState<"overview" | "tools">(() => {
    const state: unknown = location.state;
    return typeof state === "object" && state !== null && "initialSection" in state && state.initialSection === "tools"
      ? "tools" : "overview";
  });
  const [manageQuery, setManageQuery] = useState("");
  const [manageSourceId, setManageSourceId] = useState("");
  const [pickerSlug, setPickerSlug] = useState("");
  const [pickerQuery, setPickerQuery] = useState("");
  const [pickerSourceQuery, setPickerSourceQuery] = useState("");
  const [pickerSourceId, setPickerSourceId] = useState("");
  const [pickerTools, setPickerTools] = useState<ToolSummary[]>([]);
  const [pickerNextCursor,setPickerNextCursor] = useState<string|null>(null);
  const [pickerAppliedQuery,setPickerAppliedQuery] = useState("");
  const pickerRequestRef = useRef(0);
  const attemptedToolDetails = useRef(new Set<string>());
  const [pickerSelected, setPickerSelected] = useState<Set<string>>(new Set());

  const [presentationSlug, setPresentationSlug] = useState("");
  const [presentationToolId, setPresentationToolId] = useState("");
  const [presentationForm, setPresentationForm] = useState<PresentationForm>({
    alias: "",
    titleOverride: "",
    descriptionOverride: "",
  });
  const [presentationError, setPresentationError] = useState("");
  const [governanceSlug, setGovernanceSlug] = useState("");
  const [governanceError, setGovernanceError] = useState("");
  const [governanceToolId, setGovernanceToolId] = useState("");
  const [governanceForm, setGovernanceForm] = useState<GovernanceForm>({
    policyConstraint: "inherit",
    concurrencyConstraint: "inherit",
  });
  const [credentialSlug, setCredentialSlug] = useState("");
  const [credentialError, setCredentialError] = useState("");
  const [credentialToolId, setCredentialToolId] = useState("");
  const [credentialSelected, setCredentialSelected] = useState<Set<string>>(
    new Set(),
  );

  useEffect(() => {
    let active = true;
    void Promise.all([
      client.listProfiles(),
      client.listProviders(),
      client.listAgents(),
      client.listCredentials(),
      client.browseTools({ limit: 100 }),
    ])
      .then(([
        profileResult,
        providerResult,
        agentResult,
        credentialResult,
        toolResult,
      ]) => {
        if (active) {
          setProfiles(profileResult.profiles);
          setProviders(providerResult.providers);
          setAgents(agentResult.agents);
          setCredentials(credentialResult.credentials);
          setCatalogTools(toolResult.tools);
          setHasSnapshot(true);
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
  }, [client, retryVersion]);

  const managedProfile = useMemo(
    () => profiles.find((profile) => profile.slug === manageSlug) ?? null,
    [profiles, manageSlug],
  );
  usePageHeading(managedProfile?.displayName);
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

  const sourceEntries = useMemo(() => profileToolSources(providers,agents,catalogTools), [providers,agents,catalogTools]);
  const sourceById = useMemo(() => new Map(sourceEntries.map((source)=>[source.sourceId,source])),[sourceEntries]);

  // Profile ToolBindings persist only canonical ToolIDs. Hydrate missing
  // selected Tool metadata by exact ID so a second+ Catalog page cannot
  // silently hide or misgroup existing grants. Failed reads stay manageable.
  useEffect(() => {
    if (managedProfile === null) return;
    const missing = managedProfile.tools.map((binding) => binding.toolId)
      .filter((id) => !knownTools.has(id) && !attemptedToolDetails.current.has(id));
    if (!missing.length) return;
    for (const id of missing) attemptedToolDetails.current.add(id);
    void (async () => {
      const loaded: ToolSummary[] = [];
      for (let index = 0; index < missing.length; index += 8) {
        const chunk = await Promise.allSettled(
          missing.slice(index, index + 8).map((id) => client.getTool(id)),
        );
        for (const result of chunk) {
          if (result.status === "fulfilled") {
            loaded.push({ rank: 0, toolId: result.value.toolId,
              sourceId: result.value.sourceId, sourceKind: result.value.sourceKind,
              ...(result.value.providerId ? {providerId: result.value.providerId} : {}),
              name: result.value.name,
              title: result.value.title, description: result.value.description });
          }
        }
      }
      if (loaded.length) setCatalogTools((current) => mergeTools(current, loaded));
    })();
  }, [client, managedProfile, knownTools]);

  // Source IDs are exact typed identities; a missing Tool remains in the
  // management-only Unknown source bucket, never in the Add chooser.
  const managedSourceGroups = useMemo(() => {
    if (managedProfile === null) return [];
    const counts = new Map<string, number>();
    for (const binding of managedProfile.tools) {
      const id = knownTools.get(binding.toolId)?.sourceId ?? "__unknown__";
      counts.set(id, (counts.get(id) ?? 0) + 1);
    }
    return [...counts].map(([id, count]) => {
      const source=sourceById.get(id);
      return {id,count,source,label:source?.label ?? t("Unknown source")};
    }).sort((a, b) => a.label.localeCompare(b.label) || a.id.localeCompare(b.id));
  }, [managedProfile, knownTools, sourceById]);

  const visibleManagedSources = useMemo(() => {
    const needle = manageQuery.trim().toLocaleLowerCase();
    return needle
      ? managedSourceGroups.filter((group) => group.label.toLocaleLowerCase().includes(needle))
      : managedSourceGroups;
  }, [managedSourceGroups, manageQuery]);
  const managedSourcePagination = usePagination(visibleManagedSources);

  const visibleManagedTools = useMemo(() => {
    if (managedProfile === null || !manageSourceId) return [];
    const needle = manageQuery.trim().toLocaleLowerCase();
    return managedProfile.tools.filter((selection) => {
      if ((knownTools.get(selection.toolId)?.sourceId ?? "__unknown__") !== manageSourceId) {
        return false;
      }
      if (!needle) return true;
      const tool = knownTools.get(selection.toolId);
      return [
        selection.alias, selection.titleOverride, selection.descriptionOverride,
        tool?.title, tool?.name, tool?.description,
      ].some((value) => value?.toLocaleLowerCase().includes(needle));
    });
  }, [managedProfile, manageSourceId, manageQuery, knownTools]);
  const managedToolPagination = usePagination(visibleManagedTools);

  const pickerSelectedCounts = useMemo(() => {
    const counts = new Map<string, number>();
    for (const id of pickerSelected) {
      const sourceId = knownTools.get(id)?.sourceId;
      if (sourceId) counts.set(sourceId, (counts.get(sourceId) ?? 0) + 1);
    }
    return counts;
  }, [pickerSelected, knownTools]);

  // A no-op Save would unnecessarily revise Profile and refresh managed MCP.
  const pickerDirty = useMemo(() => {
    if (!pickerProfile) return false;
    if (pickerSelected.size !== pickerProfile.tools.length) return true;
    for (const binding of pickerProfile.tools) {
      if (!pickerSelected.has(binding.toolId)) return true;
    }
    return false;
  }, [pickerProfile, pickerSelected]);

  function openToolPicker() {
    if (managedProfile === null) return;
    const slug = managedProfile.slug;
    pickerRequestRef.current += 1;
    setPickerSourceQuery("");
    setPickerQuery("");
    setPickerSourceId("");
    setPickerTools([]);
    setPickerNextCursor(null);
    setPickerAppliedQuery("");
    setError("");
    setPickerSelected(new Set(managedProfile.tools.map((selection) => selection.toolId)));
    // Refresh only source metadata; the chooser starts at sources, not a
    // preloaded flat Catalog Tool list.
    void Promise.all([
      client.listProviders(), client.listAgents(),
      client.browseTools({sourceId:"native:workspace",limit:100}),
      client.browseTools({sourceId:"native:jobs",limit:100}),
    ]).then(([providerResult,agentResult,workspaceResult,jobsResult])=>{
      setProviders(providerResult.providers);
      setAgents(agentResult.agents);
      setCatalogTools((current)=>mergeTools(mergeTools(current,workspaceResult.tools.filter(t=>t.sourceId==="native:workspace")),jobsResult.tools.filter(t=>t.sourceId==="native:jobs")));
    }).catch((reason:unknown)=>setError(errorMessage(reason)));

    // Full-page selection replaces the editor. When already managing an available
    // source, continue in that source instead of making the user choose it twice.
    setManageSlug("");
    setPickerSlug(slug);
    const currentSource=sourceById.get(manageSourceId);
    if(currentSource?.selectable && currentSource.toolCount>0){void openPickerSource(manageSourceId);}
  }

  async function openPickerSource(sourceId: string) {
    if (busy || !sourceById.get(sourceId)?.selectable) return;
    const requestId = ++pickerRequestRef.current;
    setPickerSourceId(sourceId);
    setPickerQuery("");
    setPickerAppliedQuery("");
    setPickerTools([]);
    setPickerNextCursor(null);
    setBusy(true);
    setError("");
    try {
      const response = await client.browseTools({ sourceId, limit: 100 });
      if (requestId !== pickerRequestRef.current) return;
      setPickerTools(response.tools);
      setPickerNextCursor(response.nextCursor ?? null);
      setCatalogTools((current) => mergeTools(current, response.tools));
    } catch (reason) {
      if (requestId === pickerRequestRef.current) setError(errorMessage(reason));
    } finally {
      if (requestId === pickerRequestRef.current) setBusy(false);
    }
  }

  function backToPickerSources() {
    pickerRequestRef.current += 1;
    setPickerSourceId("");
    setPickerQuery("");
    setPickerAppliedQuery("");
    setPickerTools([]);
    setPickerNextCursor(null);
    setBusy(false);
  }

  async function searchPicker() {
    if (!pickerSourceId || busy) return;
    const requestId = ++pickerRequestRef.current;
    const query = pickerQuery.trim();
    const sourceId = pickerSourceId;
    setBusy(true);
    setError("");
    setPickerNextCursor(null);
    try {
      const response = query
        ? await client.searchTools({ query, limit: 100, sourceId })
        : await client.browseTools({ limit: 100, sourceId });
      if (requestId !== pickerRequestRef.current) return;
      setPickerTools(response.tools);
      setPickerNextCursor(response.nextCursor ?? null);
      setPickerAppliedQuery(query);
      setCatalogTools((current) => mergeTools(current, response.tools));
    } catch (reason) {
      if (requestId === pickerRequestRef.current) setError(errorMessage(reason));
    } finally {
      if (requestId === pickerRequestRef.current) setBusy(false);
    }
  }

  async function loadMorePicker() {
    if (!pickerSourceId || !pickerNextCursor || busy) return;
    const requestId = ++pickerRequestRef.current;
    const sourceId = pickerSourceId;
    const cursor = pickerNextCursor;
    setBusy(true);
    setError("");
    try {
      const response = pickerAppliedQuery
        ? await client.searchTools({ query: pickerAppliedQuery, limit: 100, cursor, sourceId })
        : await client.browseTools({ limit: 100, cursor, sourceId });
      if (requestId !== pickerRequestRef.current) return;
      setPickerTools((current) => mergeTools(current, response.tools));
      setCatalogTools((current) => mergeTools(current, response.tools));
      setPickerNextCursor(response.nextCursor ?? null);
    } catch (reason) {
      if (requestId === pickerRequestRef.current) {
        setPickerNextCursor(null);
        setError(errorMessage(reason));
      }
    } finally {
      if (requestId === pickerRequestRef.current) setBusy(false);
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
      return existing === undefined ? { toolId } : existing;
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
    pickerRequestRef.current += 1;
    setBusy(false);
    setPickerSlug("");
    setPickerSourceQuery("");
    setPickerQuery("");
    setPickerSourceId("");
    setPickerTools([]);
    setPickerNextCursor(null);
    setError("");
    if (slug) setManageSlug(slug);
  }

  function openPresentation(binding: ProfileToolBinding) {
    setPresentationError("");
    if (managedProfile === null) {
      return;
    }
    const slug = managedProfile.slug;
    setPresentationForm({
      alias: binding.alias,
      titleOverride: binding.titleOverride,
      descriptionOverride: binding.descriptionOverride,
    });
    setPresentationSlug(slug);
    setPresentationToolId(binding.toolId);
  }

  function closePresentation() {
    setPresentationError("");
    const slug = presentationSlug;
    setPresentationSlug("");
    setPresentationToolId("");
    if (slug) setManageSlug(slug);
  }

  const trimmedAlias = presentationForm.alias.trim();
  const aliasValid = trimmedAlias === "" || (trimmedAlias.length <= 128 && TOOL_ALIAS_PATTERN.test(trimmedAlias));

  async function savePresentation() {
    if (presentationProfile === null || presentationBinding === null || !aliasValid) {
      return;
    }
    setBusy(true);
    setPresentationError("");
    try {
      const profile = await client.replaceProfileTools(presentationProfile.slug, {
        tools: presentationProfile.tools.map((binding) =>
          binding.toolId === presentationBinding.toolId
            ? { ...binding, ...presentationForm, alias: trimmedAlias }
            : binding,
        ),
      });
      setProfiles((current) =>
        current.map((item) => (item.slug === profile.slug ? profile : item)),
      );
      setPresentationSlug("");
      setPresentationToolId("");

    } catch {
      setPresentationError(t("Could not save Tool presentation. Review the fields and try again."));
    } finally {
      setBusy(false);
    }
  }

  function openGovernance(binding: ProfileToolBinding) {
    setGovernanceError("");
    if (managedProfile === null) {
      return;
    }
    const slug = managedProfile.slug;
    setGovernanceForm({
      policyConstraint: binding.policyConstraint,
      concurrencyConstraint: binding.concurrencyConstraint,
    });
    setGovernanceSlug(slug);
    setGovernanceToolId(binding.toolId);
  }

  function closeGovernance() {
    setGovernanceError("");
    const slug = governanceSlug;
    setGovernanceSlug("");
    setGovernanceToolId("");
    if (slug) setManageSlug(slug);
  }

  async function saveGovernance() {
    if (governanceProfile === null || governanceBinding === null) return;
    setBusy(true);
    setGovernanceError("");
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
    } catch (reason) {
      setGovernanceError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  function openCredentials(binding: ProfileToolBinding) {
    setCredentialError("");
    if (managedProfile === null) {
      return;
    }
    const slug = managedProfile.slug;
    setCredentialSelected(new Set(binding.credentialRefs));
    setCredentialSlug(slug);
    setCredentialToolId(binding.toolId);
  }

  function closeCredentials() {
    setCredentialError("");
    const slug = credentialSlug;
    setCredentialSlug("");
    setCredentialToolId("");
    setCredentialSelected(new Set());
    if (slug) setManageSlug(slug);
  }

  async function saveCredentials() {
    if (credentialProfile === null || credentialBinding === null) return;
    setBusy(true);
    setCredentialError("");
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
    } catch (reason) {
      setCredentialError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  async function removeTool() {
    if (managedProfile === null || pendingRemoval === null || busy) return;
    setBusy(true);
    setRemoveError("");
    try {
      const profile = await client.replaceProfileTools(managedProfile.slug, {
        tools: managedProfile.tools.filter((binding) => binding.toolId !== pendingRemoval.toolId),
      });
      setProfiles((current) => current.map((item) => item.slug === profile.slug ? profile : item));
      setPendingRemoval(null);
    } catch (reason) {
      setRemoveError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }


  return (
    <>
      {error && !pickerProfile ? <ErrorNotice message={error} onRetry={() => setRetryVersion((v) => v + 1)} /> : null}
      {!loaded ? <p className="text-sm text-muted-foreground">{t("Loading profiles…")}</p> : null}
      {loaded && hasSnapshot && !managedProfile && !pickerProfile ?
        <ErrorNotice message={t("The requested item was not found.")} /> : null}
      {managedProfile && !pickerProfile ? (
        <section data-slot="profile-editor" aria-label={t("Manage profile")} className="flex flex-col gap-5 rounded-xl border border-border bg-card p-5">
          <h2 className="min-w-0 truncate text-base font-semibold">{managedProfile.displayName || t("Manage profile")}</h2>
          <DetailWorkspace<"overview" | "tools"> label="Manage Profile sections"
            sections={[
              { value: "overview", label: "Overview" },
              { value: "tools", label: "Tools", count: managedProfile.tools.length },
            ]}
            active={manageView} onChange={setManageView}>
          {managedProfile !== null ? (
            <div className="flex flex-col gap-4">
              {manageView === "overview" ? (
                <div className="space-y-5 overscroll-contain pr-1">
              <dl className="grid gap-x-8 gap-y-5 sm:grid-cols-2">
                <ReadOnlyField label="Slug" value={managedProfile.slug} />
                <ReadOnlyField label="Display name" value={managedProfile.displayName} />
              </dl>

              <Button size="sm" variant="outline" onClick={() =>
                void navigate("/profiles/" + encodeURIComponent(managedProfile.slug) + "/deliveries/mcp")}>
                {t("Manage MCP delivery")}
              </Button>

                </div>
              ) : (
                <>
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="flex min-w-0 items-center gap-2">
                  {manageSourceId ? (
                    <>
                      <Button variant="ghost" size="sm" className="min-w-0 gap-1.5 px-2"
                        aria-label={t("Back to sources")}
                        onClick={() => { setManageSourceId(""); setManageQuery(""); managedSourcePagination.reset(); }}>
                        <ChevronLeftIcon className="size-4 shrink-0" />
                        {t("Selected sources")}
                      </Button>
                      <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                      <span className="min-w-0 truncate text-sm font-medium">
                        {managedSourceGroups.find((group) => group.id === manageSourceId)?.label ?? t("Unknown source")}
                      </span>
                    </>
                  ) : (
                    <h3 className="text-sm font-medium">{t("Selected sources")}</h3>
                  )}
                </div>
                <Button disabled={busy} onClick={openToolPicker}>
                  {t("Add tools")}
                </Button>
              </div>

              {manageSourceId ? (
                <div className="shrink-0">
                  <Input
                    aria-label={t("Search selected tools")}
                    placeholder={t("Search selected tools…")}
                    value={manageQuery}
                    onChange={(event) => { setManageQuery(event.target.value); managedToolPagination.reset(); }}
                  />
                </div>
              ) : (
                <Input
                  aria-label={t("Search selected sources")}
                  placeholder={t("Search sources")}
                  value={manageQuery}
                  onChange={(event) => { setManageQuery(event.target.value); managedSourcePagination.reset(); }}
                />
              )}
              {manageSourceId ? (
                <>
              {visibleManagedTools.length ? (
                <div data-slot="profile-selected-tools-list" className="min-h-0 divide-y divide-border rounded-lg border border-border">
                  {managedToolPagination.items.map((selection) => {
                    const tool = knownTools.get(selection.toolId);
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
                          {description ? (
                            <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">{description}</p>
                          ) : null}
                          <div className="mt-2 flex flex-wrap gap-1.5">
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
                        <Popover open={toolActionsId === selection.toolId}
                          onOpenChange={(open) => setToolActionsId(open ? selection.toolId : null)}>
                          <PopoverTrigger render={<Button size="sm" variant="outline" type="button"
                            aria-label={t("Actions for {{tool}}", { tool: title })} disabled={busy} />}>
                            <EllipsisIcon data-icon="inline-start" />
                            {t("Actions")}
                          </PopoverTrigger>
                          <PopoverContent align="end" className="w-52 p-2">
                            <div className="grid gap-1">
                              <Button size="sm" variant="ghost" className="justify-start"
                                aria-label={t("Presentation for {{tool}}", { tool: title })}
                                onClick={() => { setToolActionsId(null); openPresentation(selection); }}>
                                <PresentationIcon data-icon="inline-start" />{t("Tool presentation")}
                              </Button>
                              <Button size="sm" variant="ghost" className="justify-start"
                                aria-label={t("Governance for {{tool}}", { tool: title })}
                                onClick={() => { setToolActionsId(null); openGovernance(selection); }}>
                                <ShieldCheckIcon data-icon="inline-start" />{t("Tool governance")}
                              </Button>
                              <Button size="sm" variant="ghost" className="justify-start"
                                aria-label={t("Credentials for {{tool}}", { tool: title })}
                                onClick={() => { setToolActionsId(null); openCredentials(selection); }}>
                                <KeyRoundIcon data-icon="inline-start" />{t("Tool credentials")}
                              </Button>
                              <Button size="sm" variant="ghost" className="justify-start text-destructive hover:text-destructive"
                                aria-label={t("Remove {{tool}}", { tool: title })}
                                onClick={() => { setToolActionsId(null); setRemoveError(""); setPendingRemoval({ toolId: selection.toolId, title }); }}>
                                <Trash2Icon data-icon="inline-start" />{t("Remove")}
                              </Button>
                            </div>
                          </PopoverContent>
                        </Popover>
                      </div>
                    );
                  })}
                </div>
              ) : (
                <div className="min-h-0 rounded-lg border border-border">
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
              ) : (
                <>
                  <div data-slot="profile-selected-sources-list" className="min-h-0 divide-y divide-border rounded-lg border border-border">
                    {managedSourcePagination.items.length ? managedSourcePagination.items.map((group) => (
                      <button
                        key={group.id}
                        type="button"
                        aria-label={t("Manage source {{source}}", { source: group.label })}
                        className="flex w-full items-center gap-3 px-4 py-4 text-left hover:bg-muted/40 focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-ring"
                        onClick={() => { setManageSourceId(group.id); setManageQuery(""); managedToolPagination.reset(); }}
                      >
                        <span className="min-w-0 flex-1">
                          <span className="block truncate text-sm font-medium">{group.label}</span>
                          <span className="mt-1 block text-xs text-muted-foreground">
                            {tCount(group.count, "{{count}} selected Tool", "{{count}} selected Tools")}
                          </span>
                        </span>
                        {!group.source?.selectable ? (
                          <Badge variant="outline">{t("Unavailable")}</Badge>
                        ) : null}
                        <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground" />
                      </button>
                    )) : managedProfile.tools.length ? (
                      <EmptyState title="No matching sources" description="Try another source name." />
                    ) : (
                      <EmptyState title="No tools selected" description="Choose sources, then select their individual tools." />
                    )}
                  </div>
                  <div className="shrink-0">
                    <TablePagination label="Selected sources" pagination={managedSourcePagination} />
                  </div>
                </>
              )}
                </>
              )}
            </div>
          ) : null}
          </DetailWorkspace>
        </section>
      ) : null}
      <ProfileToolSourcePicker
        open={pickerProfile !== null}
        sources={sourceEntries}
        tools={pickerTools}
        hasMore={pickerNextCursor !== null && pickerAppliedQuery === pickerQuery.trim()}
        onLoadMore={() => void loadMorePicker()}
        query={pickerQuery}
        sourceQuery={pickerSourceQuery}
        sourceId={pickerSourceId}
        selected={pickerSelected}
        selectedCounts={pickerSelectedCounts}
        dirty={pickerDirty}
        busy={busy}
        error={error}
        onSourceQueryChange={setPickerSourceQuery}
        onOpenSource={(sourceId) => void openPickerSource(sourceId)}
        onBackToSources={backToPickerSources}
        onQueryChange={setPickerQuery}
        onSelectedChange={setPickerSelected}
        onApply={() => void searchPicker()}
        onSave={() => void saveToolSelection()}
        onClose={closeToolPicker}
      />
      <Dialog open={pendingRemoval !== null} onOpenChange={(open) => {
        if (!open && !busy) { setPendingRemoval(null); setRemoveError(""); }
      }}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{pendingRemoval ? t("Remove {{tool}}", { tool: pendingRemoval.title }) : t("Remove tool")}</DialogTitle>
            <DialogDescription>
              {t("This will stop exposing the Tool through this Profile. The Tool remains in the Catalog.")}
            </DialogDescription>
          </DialogHeader>
          {removeError ? <ErrorNotice message={removeError} /> : null}
          <DialogFooter>
            <Button variant="outline" disabled={busy} onClick={() => { setPendingRemoval(null); setRemoveError(""); }}>
              {t("Cancel")}
            </Button>
            <Button variant="destructive" disabled={busy} onClick={() => void removeTool()}>
              {t(busy ? "Saving…" : "Remove tool")}
            </Button>
          </DialogFooter>
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
            <DialogDescription className="sr-only">
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
            <DialogDescription className="sr-only">
              {t("Tighten execution behavior for this Profile binding. Binding policy can never relax the global Policy decision.")}
            </DialogDescription>
          </DialogHeader>
          {governanceError ? <ErrorNotice message={governanceError} /> : null}

          {governanceBinding !== null ? (
            <div className="space-y-5">
              <p role="note" className="text-xs text-muted-foreground">{t("Binding settings cannot weaken global Policy.")}</p>
              <Field label="Policy constraint" help={t("Inherit keeps normal Runtime behavior. Require approval uses the existing Approval gate and currently fails closed when no approval adapter is configured. Deny keeps the Tool selected but rejects invocation.")}>
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
              {governanceForm.policyConstraint === "require_approval" ? (
                <p role="alert" className="text-xs text-foreground">
                  {t("Without an approval adapter, Tool calls requiring approval will be denied.")}
                </p>
              ) : null}
              <Field label="Concurrency constraint" help={t("Serial prevents overlapping calls for this Profile and Tool binding only.")}>
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
          {credentialError ? <ErrorNotice message={credentialError} /> : null}

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


    </>
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
  help,
  children,
}: {
  label: string;
  help?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <FormHeading label={t(label)} help={help} />
      {children}
    </div>
  );
}

function ReadOnlyField({ label, value }: { label: string; value: string }) {
  return <div className="min-w-0">
    <dt className="mb-1 text-xs text-muted-foreground">{t(label)}</dt>
    <dd className="break-words text-sm font-medium">{value}</dd>
  </div>;
}
