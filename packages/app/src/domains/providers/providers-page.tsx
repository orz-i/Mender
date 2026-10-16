import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import {
  ArrowLeftIcon,
  EyeIcon,
  KeyRoundIcon,
  Link2Icon,
  PencilIcon,
  PlusIcon,
  RefreshCwIcon,
  Trash2Icon,
} from "lucide-react";

import type {
  Credential,
  Sandbox,
  MenderClient,
  Provider,
  ProviderPlugin,
  ProviderPluginIssue,
  IntegrationOffering,
} from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@mender/ui/components/card";
import {
  Dialog,
  DialogClose,
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
import { canonicalizeOwnedIdentifier, isCanonicalOwnedSlug, ownedIdentifierCreateError } from "../../shared/form-identifiers";
import { IconButton } from "../../shared/icon-button";
import { EmptyState, ErrorNotice, InsightBanner } from "../../shared/page";
import { PageToolbar, ResultPanel } from "../../shared/page-toolbar";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import {
  SearchField,
  EmptyPanelState,
} from "../../shared/workbench";
import {
  MCP_PLUGIN_ID,
  MODEL_TOOLS_PLUGIN_ID,
  OPENAPI_PLUGIN_ID,
  WORKSPACE_PLUGIN_ID,
  AGENT_DELEGATION_PLUGIN_ID,
  isInternalProvider,
  type ProviderFormState,
  providerRenderer,
  emptyProviderForm,
  formFromProvider,
  updateRequestFromForm,
  configFromForm,
  formatHeaders,
  parseMCPJSON,
  asRecord,
  modelToolsEngineLabel,
  modelToolsModelPlaceholder,
  modelToolsEngineSummary,
  modelToolsModelSummary,
  openAPISourceSummary,
  openAPIServerSummary,
  openAPIFilterSummary,
  openAPISecuritySummary,
  connectionSummary,
} from "./provider-form-model";
import { GenericProviderConfigForm } from "./generic-provider-config-form";
import {
  genericConfigDefaults,
  genericConfigValidationError,
} from "./generic-provider-config-schema";

import { formatDateTime, formatNumber, t, tCount } from "../../i18n/i18n";

function providerPluginSelectionKey(plugin: ProviderPlugin) {
  return plugin.installationId
    ? plugin.pluginId + "@" + plugin.installationId
    : plugin.pluginId + "@builtin";
}

function providerFormSelectionKey(form: ProviderFormState) {
  return form.installationId
    ? form.pluginId + "@" + form.installationId
    : form.pluginId + "@builtin";
}

function providerPluginMatchesForm(
  plugin: ProviderPlugin,
  form: ProviderFormState,
) {
  return (
    plugin.pluginId === form.pluginId &&
    (plugin.installationId ?? "") === form.installationId
  );
}

function providerPluginForForm(
  plugins: ProviderPlugin[],
  form: ProviderFormState,
) {
  return plugins.find((plugin) => providerPluginMatchesForm(plugin, form));
}

function providerPluginChoiceLabel(plugin: ProviderPlugin) {
  return plugin.installationId
    ? plugin.displayName + " · " + plugin.packageVersion
    : plugin.displayName;
}

function providerPluginForProvider(
  plugins: ProviderPlugin[],
  provider: Provider,
) {
  return plugins.find(
    (plugin) =>
      plugin.pluginId === provider.pluginId &&
      (plugin.installationId ?? "") === (provider.installationId ?? ""),
  );
}

function providerPluginLabelForProvider(
  plugins: ProviderPlugin[],
  provider: Provider,
) {
  const plugin = providerPluginForProvider(plugins, provider);
  if (plugin !== undefined) {
    return providerPluginChoiceLabel(plugin);
  }
  if (provider.pluginDisplayName) {
    return provider.pluginDisplayName;
  }
  switch (provider.pluginId) {
    case MCP_PLUGIN_ID:
      return t("MCP");
    case MODEL_TOOLS_PLUGIN_ID:
      return "Model Tools";
    case OPENAPI_PLUGIN_ID:
      return "OpenAPI";
    case WORKSPACE_PLUGIN_ID:
      return t("Workspace");
    default:
      return t("Unconfigured");
  }
}

function providerIsBuiltIn(provider: Provider, pluginId: string) {
  return provider.installationId === undefined && provider.pluginId === pluginId;
}

function providerRebindCandidates(
  plugins: ProviderPlugin[],
  provider: Provider,
) {
  if (!provider.installationId || !provider.pluginId) {
    return [];
  }
  return plugins.filter(
    (plugin) =>
      plugin.sourceKind === "external" &&
      plugin.pluginId === provider.pluginId &&
      plugin.installationId !== undefined &&
      plugin.installationId !== provider.installationId,
  );
}

function providerSupportsFirstPartyAuthentication(provider: Provider) {
  return (
    provider.installationId === undefined &&
    (provider.pluginId === MCP_PLUGIN_ID ||
      provider.pluginId === OPENAPI_PLUGIN_ID ||
      provider.pluginId === MODEL_TOOLS_PLUGIN_ID)
  );
}

function formForProviderPlugin(
  form: ProviderFormState,
  plugin: ProviderPlugin,
): ProviderFormState {
  const installationId = plugin.installationId ?? "";
  const next = {
    ...form,
    pluginId: plugin.pluginId,
    installationId,
  };
  return {
    ...next,
    genericConfig:
      providerRenderer(next) === "generic"
        ? genericConfigDefaults(plugin.configSchema)
        : {},
  };
}

export function ProvidersPage({
  client,
}: {
  client: MenderClient;
}) {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const [createRequest] = useState(() => {
    const remainingSearchParams = new URLSearchParams(searchParams);
    remainingSearchParams.delete("create");
    remainingSearchParams.delete("pluginId");
    remainingSearchParams.delete("installationId");
    remainingSearchParams.delete("agentId");
    remainingSearchParams.delete("offeringId");
    return {
      requested: searchParams.get("create") === "1",
      pluginId: searchParams.get("pluginId"),
      installationId: searchParams.get("installationId") ?? "",
      agentId: searchParams.get("agentId") ?? "",
      offeringId: searchParams.get("offeringId") ?? "",
      remainingSearchParams,
    };
  });
  const [providers, setProviders] = useState<Provider[]>([]);
  const [providerPlugins, setProviderPlugins] = useState<ProviderPlugin[]>([]);
  const [providerPluginIssues, setProviderPluginIssues] = useState<
    ProviderPluginIssue[]
  >([]);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [sandboxes, setSandboxes] = useState<Sandbox[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [inventoryQuery, setInventoryQuery] = useState("");
  const [showInternal, setShowInternal] = useState(() => searchParams.get("showInternal") === "1");

  const [pluginPickerOpen, setPluginPickerOpen] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [createCanReturnToPicker, setCreateCanReturnToPicker] = useState(false);
  const [createForm, setCreateForm] =
    useState<ProviderFormState>(emptyProviderForm);
  const [createError, setCreateError] = useState("");
  const [connectionOffering, setConnectionOffering] = useState<IntegrationOffering|null>(null);
  const [offeringName, setOfferingName] = useState("");
  const [offeringNamespace, setOfferingNamespace] = useState("");
  const [offeringCredentialRef, setOfferingCredentialRef] = useState("");
  const [offeringError, setOfferingError] = useState("");

  const [viewProvider, setViewProvider] = useState<Provider | null>(null);
  const [editProvider, setEditProvider] = useState<Provider | null>(null);
  const [editForm, setEditForm] = useState<ProviderFormState | null>(null);
  const [rebindProvider, setRebindProvider] = useState<Provider | null>(null);
  const [rebindPlugin, setRebindPlugin] = useState<ProviderPlugin | null>(null);
  const [rebindConfig, setRebindConfig] = useState<Record<string, unknown>>({});
  const [authenticationProvider, setAuthenticationProvider] =
    useState<Provider | null>(null);
  const [authenticationForm, setAuthenticationForm] =
    useState<ProviderFormState | null>(null);
  const [authenticationReturnsToCreate, setAuthenticationReturnsToCreate] =
    useState(false);
  const [authenticationOriginForm, setAuthenticationOriginForm] =
    useState<ProviderFormState | null>(null);

  const refreshProviderPlugins = useCallback(async (): Promise<void> => {
    try {
      const result = await client.listProviderPlugins();
      setProviderPlugins(result.plugins);
      setProviderPluginIssues(result.issues);
      setError("");
    } catch (reason) {
      setError(errorMessage(reason));
    }
  }, [client]);

  const refresh = useCallback(async (): Promise<Provider[] | null> => {
    try {
      const result = await client.listProviders();
      setProviders(result.providers);
      setError("");
      return result.providers;
    } catch (reason) {
      setError(errorMessage(reason));
      return null;
    } finally {
      setLoaded(true);
    }
  }, [client]);

  useEffect(() => {
    let active = true;

    void Promise.all([
      client.listProviderPlugins(),
      client.listProviders(),
      client.listCredentials(),
      client.listSandboxes(),
      createRequest.offeringId ? client.listIntegrationOfferings() : Promise.resolve({integrations:[] as IntegrationOffering[]}),
    ])
      .then(([pluginResult, providerResult, credentialResult, sandboxResult, offeringResult]) => {
        if (active) {
          setProviderPlugins(pluginResult.plugins);
          setProviderPluginIssues(pluginResult.issues);
          setProviders(providerResult.providers);
          setCredentials(credentialResult.credentials);
          setSandboxes(sandboxResult.sandboxes);

          if (createRequest.requested && createRequest.offeringId) {
            const offering=offeringResult.integrations.find(item=>item.offeringId===createRequest.offeringId);
            if (offering===undefined || offering.kind!=="remote-mcp" ||
              offering.driverPluginId!==MCP_PLUGIN_ID || offering.transport!=="streamable-http" ||
              offering.authentication!=="optional-bearer" || !offering.endpoint.startsWith("https://")) {
              setError(t("The requested item was not found."));
            } else {
              setConnectionOffering(offering);
              setOfferingName(offering.displayName);
              setOfferingNamespace(offering.defaultNamespace);
              setOfferingCredentialRef("");
              setOfferingError("");
              setError("");
            }
            setSearchParams(createRequest.remainingSearchParams, {replace:true});
          } else if (createRequest.requested) {
            const plugin =
              createRequest.pluginId === null
                ? undefined
                : pluginResult.plugins.find(
                    (candidate) =>
                      candidate.pluginId === createRequest.pluginId &&
                      (candidate.installationId ?? "") ===
                        createRequest.installationId,
                  );
            if (plugin === undefined) {
              setError(t("The requested item was not found."));
            } else {
              setError("");
              const base=formForProviderPlugin(emptyProviderForm, plugin);
              const fromAgent=plugin.pluginId===AGENT_DELEGATION_PLUGIN_ID &&
                /^agent_[0-9a-f]{32}$/.test(createRequest.agentId);
              setCreateForm(fromAgent?{
                ...base,namespace:"agent-"+createRequest.agentId.slice(-8),
                displayName:"Agent Delegation",
                genericConfig:{agentId:createRequest.agentId},
              }:base);
              setPluginPickerOpen(false);
              setCreateCanReturnToPicker(false);
              setCreateOpen(true);
            }
            setSearchParams(createRequest.remainingSearchParams, {
              replace: true,
            });
          } else {
            setError("");
          }
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
  }, [client, createRequest, setSearchParams]);

  const visibleProviders = useMemo(() => {
    const needle = inventoryQuery.trim().toLocaleLowerCase();
    const normal = showInternal ? providers : providers.filter(provider => !isInternalProvider(provider.pluginId));
    return needle ? normal.filter((provider) =>
      [provider.displayName, provider.namespace, provider.pluginId ?? ""]
        .some((value) => value.toLocaleLowerCase().includes(needle))
    ) : normal;
  }, [inventoryQuery, providers, showInternal]);
  const providerPagination = usePagination(visibleProviders);


  async function connectOffering() {
    const offering=connectionOffering;
    const namespace=canonicalizeOwnedIdentifier(offeringNamespace);
    if(!offering || busy!=="" || !isCanonicalOwnedSlug(namespace) ||
      offeringName.trim()==="" || new TextEncoder().encode(offeringName.trim()).length>160)return;
    // Every trusted parameter derives from the reviewed host catalogue.
    // No model/user-supplied Endpoint, executable or header template enters.
    if(offering.kind!=="remote-mcp" || offering.driverPluginId!==MCP_PLUGIN_ID ||
      offering.transport!=="streamable-http" || offering.authentication!=="optional-bearer" ||
      !offering.endpoint.startsWith("https://"))return;
    setBusy("integration");
    setOfferingError("");
    try{
      await client.createProvider({
        namespace,displayName:offeringName.trim(),pluginId:offering.driverPluginId,
        config:{transport:{
          kind:"streamable-http",endpoint:offering.endpoint,headers:offering.headers,
          ...(offeringCredentialRef?{credentialHeaders:[{
            name:"Authorization",scheme:"Bearer",credentialRef:offeringCredentialRef,
          }]}:{}),
        }},
      });
      setConnectionOffering(null);
      await refresh();
    }catch(reason){
      setOfferingError(ownedIdentifierCreateError(reason,"provider"));
      await refresh();
    }finally{
      setBusy("");
    }
  }

  async function addProvider() {
    const namespace = canonicalizeOwnedIdentifier(createForm.namespace);
    if (!isCanonicalOwnedSlug(namespace) || busy !== "") return;
    setBusy("create");
    setCreateError("");
    setError("");
    try {
      await client.createProvider({
        namespace,
        displayName: createForm.displayName.trim(),
        pluginId: createForm.pluginId,
        ...(createForm.installationId
          ? { installationId: createForm.installationId }
          : {}),
        config: configFromForm(createForm),
        ...(createForm.description
          ? { description: createForm.description }
          : {}),
      });

      setCreateOpen(false);
      setCreateCanReturnToPicker(false);
      setCreateForm(emptyProviderForm);
      await refresh();
    } catch (reason) {
      // The inventory may contain an identically named provider created in
      // another window. An uncertain response is never evidence of success.
      setCreateError(ownedIdentifierCreateError(reason, "provider"));
      await refresh();
    } finally {
      setBusy("");
    }
  }

  function beginEdit(provider: Provider) {
    if (!provider.pluginId) {
      return;
    }
    setError("");
    setEditProvider(provider);
    setEditForm(formFromProvider(provider));
  }

  async function saveEdit() {
    if (editProvider === null || editForm === null) {
      return;
    }

    setBusy(editProvider.providerId);
    setError("");
    try {
      await client.updateProvider(
        editProvider.providerId,
        updateRequestFromForm(editForm),
      );
      setEditProvider(null);
      setEditForm(null);
      await refresh();
    } catch (reason) {
      const message = errorMessage(reason);
      await refresh();
      setError(message);
    } finally {
      setBusy("");
    }
  }

  function beginRebind(provider: Provider) {
    const target = providerRebindCandidates(providerPlugins, provider)[0];
    if (target === undefined) {
      return;
    }
    const currentConfig = asRecord(provider.config);
    setError("");
    setRebindProvider(provider);
    setRebindPlugin(target);
    setRebindConfig(
      currentConfig === null
        ? genericConfigDefaults(target.configSchema)
        : JSON.parse(JSON.stringify(currentConfig)) as Record<string, unknown>,
    );
  }

  function closeRebind() {
    setRebindProvider(null);
    setRebindPlugin(null);
    setRebindConfig({});
  }

  async function saveRebind() {
    if (
      rebindProvider === null ||
      rebindPlugin === null ||
      rebindPlugin.installationId === undefined
    ) {
      return;
    }

    setBusy(rebindProvider.providerId);
    setError("");
    try {
      await client.rebindProvider(rebindProvider.providerId, {
        installationId: rebindPlugin.installationId,
        config: JSON.parse(JSON.stringify(rebindConfig)) as Record<string, unknown>,
      });
      closeRebind();
      await refresh();
      await refreshProviderPlugins();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function syncProvider(provider: Provider) {
    setBusy(provider.providerId);
    setError("");
    try {
      await client.syncProvider(provider.providerId);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function deleteProvider(provider: Provider) {
    if (
      !window.confirm(
        t('Delete provider "{{name}}"? Profile references will block deletion.', {
          name: provider.namespace,
        }),
      )
    ) {
      return;
    }
    setBusy(provider.providerId);
    setError("");
    try {
      await client.deleteProvider(provider.providerId);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  function openCreateAuthentication() {
    setAuthenticationOriginForm(createForm);
    setCreateOpen(false);
    setAuthenticationReturnsToCreate(true);
    window.setTimeout(() => {
      setAuthenticationProvider(null);
      setAuthenticationForm({ ...createForm });
    }, 0);
  }

  function openProviderAuthentication(provider: Provider) {
    if (!providerSupportsFirstPartyAuthentication(provider)) {
      return;
    }
    setError("");
    setAuthenticationReturnsToCreate(false);
    setAuthenticationProvider(provider);
    setAuthenticationForm(formFromProvider(provider));
  }

  function closeAuthentication() {
    const returnToCreate = authenticationReturnsToCreate;
    const origin = authenticationOriginForm;
    setAuthenticationProvider(null);
    setAuthenticationForm(null);
    setAuthenticationReturnsToCreate(false);
    setAuthenticationOriginForm(null);
    if (returnToCreate) {
      if (origin !== null) {
        setCreateForm(origin);
      }
      window.setTimeout(() => setCreateOpen(true), 0);
    }
  }

  async function saveAuthentication() {
    if (authenticationForm === null) {
      return;
    }
    if (authenticationProvider === null) {
      setCreateForm(authenticationForm);
      setAuthenticationForm(null);
      setAuthenticationReturnsToCreate(false);
      setAuthenticationOriginForm(null);
      window.setTimeout(() => setCreateOpen(true), 0);
      return;
    }

    setBusy(authenticationProvider.providerId);
    setError("");
    try {
      await client.updateProvider(
        authenticationProvider.providerId,
        updateRequestFromForm(authenticationForm),
      );
      setAuthenticationProvider(null);
      setAuthenticationForm(null);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  function openCreatePluginPicker() {
    setError("");
    setCreateError("");
    setCreateOpen(false);
    setCreateCanReturnToPicker(false);
    setCreateForm(emptyProviderForm);
    setPluginPickerOpen(true);
  }

  function chooseCreatePlugin(plugin: ProviderPlugin) {
    setError("");
    setCreateError("");
    setCreateForm(formForProviderPlugin(emptyProviderForm, plugin));
    setPluginPickerOpen(false);
    setCreateCanReturnToPicker(true);
    setCreateOpen(true);
  }

  function returnToCreatePluginPicker() {
    setCreateError("");
    setCreateOpen(false);
    setCreateCanReturnToPicker(false);
    setCreateForm(emptyProviderForm);
    window.setTimeout(() => setPluginPickerOpen(true), 0);
  }

  function setCreateDialogOpen(open: boolean) {
    setCreateOpen(open);
    if (!open) {
      setCreateError("");
      setCreateCanReturnToPicker(false);
      setCreateForm(emptyProviderForm);
    }
  }

  return (
    <>
      <PageToolbar
        tabs={
          <span className="text-xs text-muted-foreground">
            {tCount(showInternal ? providers.length : providers.filter(p=>!isInternalProvider(p.pluginId)).length, "{{count}} configured source", "{{count}} configured sources")}
          </span>
        }
        filters={
          <div className="w-full min-w-0 sm:w-56 sm:shrink-0">
            <SearchField label="Search providers" placeholder="Search providers…" value={inventoryQuery} onChange={setInventoryQuery} />
          </div>
        }
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => setShowInternal(value => !value)}>{t(showInternal ? "Hide internal providers" : "Show internal providers")}</Button>
            <Button variant="outline" size="sm" disabled={busy !== ""} onClick={() => void refresh()}>{t("Refresh")}</Button>
            <Button size="sm" onClick={openCreatePluginPicker}>{t("Add provider")}</Button>
          </>
        }
      />
      {error ? <ErrorNotice message={error} /> : null}
      {providerPluginIssues.length ? (
        <InsightBanner
          title="Some Provider plugins need attention. Review Plugin Marketplace for installation details."
          actions={
            <Button
              size="sm"
              variant="primary"
              onClick={() => void navigate("/plugins")}
            >
              {t("Review Marketplace")}
            </Button>
          }
        />
      ) : null}
      <ResultPanel title="Provider inventory">
        {!loaded ? (
          <div className="px-4 py-12 text-center text-sm text-muted-foreground">
            {t("Loading providers…")}
          </div>
        ) : visibleProviders.length ? (
          <>
            <Table aria-label={t("Provider inventory")} className="max-md:block max-md:w-full">
              <TableHeader className="max-md:sr-only">
                <TableRow>
                  <TableHead>{t("Provider")}</TableHead>
                  <TableHead>{t("Plugin")}</TableHead>
                  <TableHead>{t("Connection")}</TableHead>
                  <TableHead>{t("Tools")}</TableHead>
                  <TableHead className="text-right">{t("Actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody className="max-md:block">
                {providerPagination.items.map((provider) => (
                  <TableRow
                    key={provider.providerId}
                    className="md:[&>td]:py-3 max-md:grid max-md:grid-cols-[minmax(0,1fr)_auto] max-md:items-start max-md:gap-x-3 max-md:gap-y-1.5 max-md:px-4 max-md:py-3.5 max-md:border-b max-md:last:border-b-0"
                  >
                    <TableCell className="font-medium max-md:col-start-1 max-md:row-start-1 max-md:block max-md:min-w-0 max-md:p-0">
                      <span className="block truncate font-medium text-sm text-foreground" title={provider.displayName || provider.namespace}>
                        {provider.displayName || provider.namespace}
                      </span>
                    </TableCell>
                    <TableCell className="max-md:col-start-2 max-md:row-start-1 max-md:block max-md:p-0 max-md:justify-self-end">
                      <Badge variant="outline" className="max-md:text-[11px] max-md:px-2 max-md:py-0.5">
                        {providerPluginLabelForProvider(
                          providerPlugins,
                          provider,
                        )}
                      </Badge>
                    </TableCell>
                    <TableCell className="max-w-[360px] truncate font-mono text-xs max-md:col-start-1 max-md:row-start-2 max-md:block max-md:min-w-0 max-md:p-0">
                      <span className="block truncate font-mono text-xs text-muted-foreground max-md:text-[11px]" title={connectionSummary(provider)}>
                        {connectionSummary(provider)}
                      </span>
                    </TableCell>
                    <TableCell className="max-md:col-start-2 max-md:row-start-2 max-md:block max-md:p-0 max-md:justify-self-end">
                      <Badge
                        variant={provider.toolCount ? "outline" : "secondary"}
                        className="max-md:text-[11px] max-md:px-2 max-md:py-0.5"
                      >
                        {tCount(
                          provider.toolCount,
                          "{{count}} synced Tool",
                          "{{count}} synced Tools",
                        )}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right max-md:col-span-2 max-md:row-start-3 max-md:block max-md:p-0 max-md:pt-2.5 max-md:mt-1 max-md:border-t max-md:border-border/40">
                      <div className="flex justify-end gap-1 max-md:justify-end">
                        <IconButton
                          variant="ghost"
                          label={t("View {{provider}}", {
                            provider: provider.displayName || provider.namespace,
                          })}
                          onClick={() => setViewProvider(provider)}
                        >
                          <EyeIcon />
                        </IconButton>
                        <IconButton
                          variant="ghost"
                          label={t("Edit {{provider}}", {
                            provider: provider.displayName || provider.namespace,
                          })}
                          disabled={busy !== "" || !provider.pluginId}
                          onClick={() => beginEdit(provider)}
                        >
                          <PencilIcon />
                        </IconButton>
                        <IconButton
                          variant="outline"
                          label={t("Authentication {{provider}}", {
                            provider: provider.displayName || provider.namespace,
                          })}
                          disabled={
                            busy !== "" ||
                            !providerSupportsFirstPartyAuthentication(provider)
                          }
                          onClick={() => openProviderAuthentication(provider)}
                        >
                          <KeyRoundIcon />
                        </IconButton>
                        {providerRebindCandidates(
                          providerPlugins,
                          provider,
                        ).length ? (
                          <IconButton
                            variant="outline"
                            label={t("Rebind {{provider}}", {
                              provider: provider.displayName || provider.namespace,
                            })}
                            disabled={busy !== ""}
                            onClick={() => beginRebind(provider)}
                          >
                            <Link2Icon />
                          </IconButton>
                        ) : null}
                        <IconButton
                          variant="outline"
                          label={t("Sync {{provider}}", {
                            provider: provider.displayName || provider.namespace,
                          })}
                          disabled={busy !== ""}
                          onClick={() => void syncProvider(provider)}
                        >
                          <RefreshCwIcon />
                        </IconButton>
                        <IconButton
                          variant="destructive"
                          label={t("Delete {{provider}}", {
                            provider: provider.displayName || provider.namespace,
                          })}
                          disabled={busy !== ""}
                          onClick={() => void deleteProvider(provider)}
                        >
                          <Trash2Icon />
                        </IconButton>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </>
        ) : (
          <EmptyPanelState>
            <EmptyState
              title={providers.length ? t("No matching providers") : t("No providers")}
              description={providers.length ? t("Try a different search or filter.") : t("Add a Provider plugin and configure its connection to populate the Catalog.")}
            />
          </EmptyPanelState>
        )}
      </ResultPanel>

      <TablePagination
        label="Providers"
        pagination={providerPagination}
      />

      {connectionOffering!==null?(
        <Dialog open onOpenChange={open=>{
          if(!open&&busy!=="integration"){setConnectionOffering(null);setOfferingError("");}
        }}>
          <DialogContent className="max-h-[calc(100dvh-2rem)] grid-rows-[auto_minmax(0,1fr)_auto] sm:max-w-xl">
            <div className="grid gap-2">
              <DialogHeader>
                <DialogTitle>{t("Connect {{integration}}",{integration:connectionOffering.displayName})}</DialogTitle>
                <DialogDescription>{t("Configure this reviewed Remote MCP integration. No Profile access is granted automatically.")}</DialogDescription>
              </DialogHeader>
              {offeringError?<div role="alert" className="rounded-lg border border-destructive/20 p-3 text-sm text-destructive">{offeringError}</div>:null}
            </div>
            <div className="min-h-0 space-y-4 overflow-y-auto pr-1">
              <Field label="Display name">
                <Input aria-label={t("Integration name")} value={offeringName}
                  onChange={event=>setOfferingName(event.target.value)}/>
              </Field>
              <div className="rounded-lg border border-border p-3">
                <p className="mb-1 text-xs text-muted-foreground">{t("Reviewed endpoint · Streamable HTTP")}</p>
                <p className="break-all font-mono text-xs">{connectionOffering.endpoint}</p>
                <p className="mt-2 text-xs text-muted-foreground">{t("The endpoint and client header come from the reviewed integration definition.")}</p>
              </div>
              <Field label="API Credential" optional>
                <select aria-label={t("AnySearch API Credential")} value={offeringCredentialRef}
                  onChange={event=>setOfferingCredentialRef(event.target.value)}
                  className="h-9 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus:ring-2 focus:ring-ring/40">
                  <option value="">{t("Anonymous connection (lower rate limit)")}</option>
                  {credentials.map(item=><option key={item.reference} value={item.reference}>
                    {item.displayName} · {item.reference}
                  </option>)}
                </select>
              </Field>
              <details className="rounded-lg border border-border">
                <summary className="cursor-pointer px-3 py-2 text-sm font-medium">{t("Advanced settings")}</summary>
                <div className="border-t border-border p-3">
                  <Field label="Namespace">
                    <Input aria-label={t("Integration namespace")} value={offeringNamespace}
                      onChange={event=>setOfferingNamespace(event.target.value)}/>
                  </Field>
                </div>
              </details>
            </div>
            <DialogFooter>
              <Button variant="outline" disabled={busy==="integration"} onClick={()=>setConnectionOffering(null)}>{t("Cancel")}</Button>
              <Button disabled={busy!=="" || !isCanonicalOwnedSlug(canonicalizeOwnedIdentifier(offeringNamespace)) ||
                !offeringName.trim() || new TextEncoder().encode(offeringName.trim()).length>160}
                onClick={()=>void connectOffering()}>{t("Connect")}</Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      ):null}

      <ProviderPluginPickerDialog
        open={pluginPickerOpen}
        plugins={showInternal ? providerPlugins : providerPlugins.filter(plugin => !isInternalProvider(plugin.pluginId))}
        onOpenChange={setPluginPickerOpen}
        onSelect={chooseCreatePlugin}
      />

      <ProviderFormDialog
        open={createOpen}
        mode="create"
        form={createForm}
        plugins={providerPlugins}
        credentials={credentials}
        sandboxes={sandboxes}
        busy={busy === "create"}
        createError={createError}
        onOpenChange={setCreateDialogOpen}
        onChange={(next) => {
          setCreateError("");
          setCreateForm(next);
        }}
        onAuthentication={openCreateAuthentication}
        {...(createCanReturnToPicker
          ? { onBack: returnToCreatePluginPicker }
          : {})}
        onSubmit={() => void addProvider()}
      />

      {authenticationForm !== null ? (
        <ProviderAuthenticationDialog
          open
          form={authenticationForm}
          credentials={credentials}
          busy={busy !== ""}
          onOpenChange={(open) => {
            if (!open) {
              closeAuthentication();
            }
          }}
          onChange={setAuthenticationForm}
          onSubmit={() => void saveAuthentication()}
          submitLabel={
            authenticationProvider === null
              ? "Use authentication"
              : "Save authentication"
          }
        />
      ) : null}

      <ProviderDetailsDialog
        provider={viewProvider}
        plugins={providerPlugins}
        onOpenChange={(open) => {
          if (!open) {
            setViewProvider(null);
          }
        }}
      />

      {editForm !== null ? (
        <ProviderFormDialog
          open={editProvider !== null}
          mode="edit"
          form={editForm}
          plugins={providerPlugins}
          credentials={credentials}
          sandboxes={sandboxes}
          busy={
            editProvider !== null && busy === editProvider.providerId
          }
          onOpenChange={(open) => {
            if (!open) {
              setEditProvider(null);
              setEditForm(null);
            }
          }}
          onChange={setEditForm}
          onSubmit={() => void saveEdit()}
        />
      ) : null}

      {rebindProvider !== null && rebindPlugin !== null ? (
        <ProviderRebindDialog
          open
          provider={rebindProvider}
          candidates={providerRebindCandidates(
            providerPlugins,
            rebindProvider,
          )}
          plugin={rebindPlugin}
          config={rebindConfig}
          credentials={credentials}
          busy={busy === rebindProvider.providerId}
          onOpenChange={(open) => {
            if (!open) {
              closeRebind();
            }
          }}
          onPluginChange={(plugin) => {
            setRebindPlugin(plugin);
          }}
          onConfigChange={setRebindConfig}
          onSubmit={() => void saveRebind()}
        />
      ) : null}
    </>
  );
}

function ProviderRebindDialog({
  open,
  provider,
  candidates,
  plugin,
  config,
  credentials,
  busy,
  onOpenChange,
  onPluginChange,
  onConfigChange,
  onSubmit,
}: {
  open: boolean;
  provider: Provider;
  candidates: ProviderPlugin[];
  plugin: ProviderPlugin;
  config: Record<string, unknown>;
  credentials: Credential[];
  busy: boolean;
  onOpenChange: (open: boolean) => void;
  onPluginChange: (plugin: ProviderPlugin) => void;
  onConfigChange: (value: Record<string, unknown>) => void;
  onSubmit: () => void;
}) {
  const validationError = genericConfigValidationError(
    plugin.configSchema,
    config,
  );
  const selectedInstallationId = plugin.installationId ?? "";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t("Rebind Provider installation")}</DialogTitle>
          <DialogDescription>
            {t("Move this Provider to another exact installation of the same plugin. Provider identity and SourceKey-stable Tool identities remain unchanged.")}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="grid grid-cols-2 gap-3">
            <ReadOnlyField label="Provider" value={provider.namespace} />
            <ReadOnlyField
              label="Current installation"
              value={provider.installationId ?? t("Unavailable")}
            />
          </div>

          <Field label="Target installation">
            <select
              aria-label={t("Target Provider plugin installation")}
              value={selectedInstallationId}
              onChange={(event) => {
                const selected = candidates.find(
                  (candidate) =>
                    candidate.installationId === event.target.value,
                );
                if (selected !== undefined) {
                  onPluginChange(selected);
                }
              }}
              className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus:ring-2 focus:ring-ring/40"
            >
              {candidates.map((candidate) => (
                <option
                  key={providerPluginSelectionKey(candidate)}
                  value={candidate.installationId}
                >
                  {candidate.packageVersion + " · " + candidate.installationId}
                </option>
              ))}
            </select>
          </Field>

          <div className="grid grid-cols-2 gap-3">
            <ReadOnlyField label="Plugin" value={plugin.pluginId} />
            <ReadOnlyField
              label="Target package version"
              value={plugin.packageVersion}
            />
          </div>

          <div className="rounded-lg border border-border bg-muted/30 p-3">
            <div className="text-sm font-medium">{t("Target configuration")}</div>
            <p className="mt-1 text-xs leading-5 text-muted-foreground">
              {t("The current configuration is carried forward for review. Update it when the target package requires a different configuration contract.")}
            </p>
          </div>

          <GenericProviderConfigForm
            key={providerPluginSelectionKey(plugin)}
            schema={plugin.configSchema}
            uiSchema={plugin.uiSchema}
            value={config}
            credentials={credentials}
            onChange={onConfigChange}
          />
          {validationError ? (
            <p className="text-xs text-destructive">{validationError}</p>
          ) : null}
        </div>

        <DialogFooter>
          <DialogClose render={<Button variant="outline" />}>
            {t("Cancel")}
          </DialogClose>
          <Button
            disabled={busy || validationError !== null}
            onClick={onSubmit}
          >
            {t(busy ? "Rebinding…" : "Rebind installation")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ProviderPluginPickerDialog({
  open,
  plugins,
  onOpenChange,
  onSelect,
}: {
  open: boolean;
  plugins: ProviderPlugin[];
  onOpenChange: (open: boolean) => void;
  onSelect: (plugin: ProviderPlugin) => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("Choose Provider plugin")}</DialogTitle>
          <DialogDescription>
            {t(
              "Select a Provider plugin first. Configuration opens after you choose a plugin.",
            )}
          </DialogDescription>
        </DialogHeader>

        {plugins.length ? (
          <div className="flex max-h-[60vh] flex-col gap-3 overflow-y-auto pr-1">
            {plugins.map((plugin) => (
              <Card key={providerPluginSelectionKey(plugin)} size="sm">
                <CardHeader>
                  <CardTitle>{plugin.displayName}</CardTitle>
                  <CardAction>
                    <Button
                      type="button"
                      size="sm"
                      aria-label={t("Add {{plugin}} provider", {
                        plugin: providerPluginChoiceLabel(plugin),
                      })}
                      onClick={() => onSelect(plugin)}
                    >
                      <PlusIcon data-icon="inline-start" />
                      {t("Add")}
                    </Button>
                  </CardAction>
                  <CardDescription className="line-clamp-2">
                    {plugin.description || t("No plugin description.")}
                  </CardDescription>
                </CardHeader>
                <CardContent className="flex flex-wrap gap-2">
                  <Badge variant="outline">
                    {t(plugin.sourceKind === "builtin" ? "Built-in" : "Local package")}
                  </Badge>
                  <Badge variant="secondary">{plugin.packageVersion}</Badge>
                </CardContent>
              </Card>
            ))}
          </div>
        ) : (
          <EmptyState
            title="No Provider plugins"
            description="Install or enable a Provider plugin before creating a Provider."
          />
        )}

        <DialogFooter>
          <DialogClose render={<Button variant="outline" />}>
            {t("Cancel")}
          </DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ProviderFormDialog({
  open,
  mode,
  form,
  plugins,
  credentials,
  sandboxes,
  busy,
  createError,
  onOpenChange,
  onChange,
  onAuthentication,
  onBack,
  onSubmit,
}: {
  open: boolean;
  mode: "create" | "edit";
  form: ProviderFormState;
  plugins: ProviderPlugin[];
  credentials: Credential[];
  sandboxes: Sandbox[];
  busy: boolean;
  createError?: string;
  onOpenChange: (open: boolean) => void;
  onChange: (value: ProviderFormState) => void;
  onAuthentication?: () => void;
  onBack?: () => void;
  onSubmit: () => void;
}) {
  const [importError, setImportError] = useState("");
  const creating = mode === "create";
  const selectedPlugin = providerPluginForForm(plugins, form);
  const renderer = providerRenderer(form);
  const canonicalNamespace = canonicalizeOwnedIdentifier(form.namespace);
  const namespaceValid = isCanonicalOwnedSlug(canonicalNamespace);
  const genericError =
    renderer === "generic"
      ? genericConfigValidationError(
          selectedPlugin?.configSchema,
          form.genericConfig,
        )
      : null;
  const valid =
    form.displayName.trim() !== "" &&
    new TextEncoder().encode(form.displayName.trim()).length <= 160 &&
    new TextEncoder().encode(form.description.trim()).length <= 4096 &&
    (!creating || namespaceValid) &&
    (renderer === "mcp"
      ? form.transport === "http"
        ? form.endpoint.trim() !== ""
        : form.executable.trim() !== ""
      : renderer === "openapi"
        ? form.openapiSourceKind === "url"
          ? form.openapiSpecUrl.trim() !== ""
          : form.openapiSpecFile.trim() !== ""
        : renderer === "model-tools"
          ? form.modelToolsModel.trim() !== "" &&
            form.modelToolsCredentialRef.trim() !== ""
          : selectedPlugin !== undefined && genericError === null);

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) {
          setImportError("");
        }
        onOpenChange(nextOpen);
      }}
    >
      <DialogContent className="max-h-[calc(100dvh-2rem)] grid-rows-[auto_minmax(0,1fr)_auto] sm:max-w-xl">
        <div className="grid gap-3">
          <DialogHeader>
          <DialogTitle>
            {creating
              ? selectedPlugin !== undefined
                ? t("Add {{plugin}} provider", {
                    plugin: providerPluginChoiceLabel(selectedPlugin),
                  })
                : t("Add provider")
              : t("Edit provider")}
          </DialogTitle>
          <DialogDescription>
            {creating
              ? selectedPlugin !== undefined
                ? t(
                    "Configure {{plugin}}. This form is scoped to the selected Provider plugin.",
                    {
                      plugin: providerPluginChoiceLabel(selectedPlugin),
                    },
                  )
                : t("The requested item was not found.")
              : t(
                  "Update mutable metadata and plugin connection settings. Provider identity and plugin remain stable.",
                )}
          </DialogDescription>
          </DialogHeader>
          {creating && createError ? (
            <div role="alert" className="rounded-xl border border-destructive/20 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {createError}
            </div>
          ) : null}
        </div>

        <div className="min-h-0 space-y-4 overflow-y-auto pr-1">
          {!creating ? (
            <div className="grid grid-cols-2 gap-3">
              <ReadOnlyField label="Namespace" value={form.namespace} />
              <ReadOnlyField
                label="Provider plugin"
                value={
                  selectedPlugin !== undefined
                    ? providerPluginChoiceLabel(selectedPlugin)
                    : form.pluginId
                }
              />
            </div>
          ) : null}

          {renderer === "mcp" ? (
            <div>
              <div className="mb-2 text-sm font-medium">{t("MCP connection")}</div>
              <div className="grid grid-cols-2 gap-2">
                <Button
                  type="button"
                  variant={form.transport === "http" ? "default" : "outline"}
                  aria-pressed={form.transport === "http"}
                  onClick={() => onChange({ ...form, transport: "http" })}
                >
                  {t("Remote endpoint")}
                </Button>
                <Button
                  type="button"
                  variant={form.transport === "stdio" ? "default" : "outline"}
                  aria-pressed={form.transport === "stdio"}
                  onClick={() => onChange({ ...form, transport: "stdio" })}
                >
                  {t("Local command")}
                </Button>
              </div>
            </div>
          ) : renderer === "model-tools" ? (
            <div>
              <div className="mb-2 text-sm font-medium">{t("Model tool engine")}</div>
              <div className="grid grid-cols-3 gap-2">
                {(["openai", "anthropic", "gemini"] as const).map(
                  (engine) => (
                    <Button
                      key={engine}
                      type="button"
                      variant={
                        form.modelToolsEngine === engine ? "default" : "outline"
                      }
                      aria-pressed={form.modelToolsEngine === engine}
                      onClick={() =>
                        onChange({ ...form, modelToolsEngine: engine })
                      }
                    >
                      {modelToolsEngineLabel(engine)}
                    </Button>
                  ),
                )}
              </div>
            </div>
          ) : renderer === "openapi" ? (
            <div>
              <div className="mb-2 text-sm font-medium">{t("OpenAPI description")}</div>
              <div className="grid grid-cols-2 gap-2">
                <Button
                  type="button"
                  variant={
                    form.openapiSourceKind === "url" ? "default" : "outline"
                  }
                  aria-pressed={form.openapiSourceKind === "url"}
                  onClick={() =>
                    onChange({ ...form, openapiSourceKind: "url" })
                  }
                >
                  {t("Remote URL")}
                </Button>
                <Button
                  type="button"
                  variant={
                    form.openapiSourceKind === "file" ? "default" : "outline"
                  }
                  aria-pressed={form.openapiSourceKind === "file"}
                  onClick={() =>
                    onChange({ ...form, openapiSourceKind: "file" })
                  }
                >
                  {t("Local file")}
                </Button>
              </div>
            </div>
          ) : (
            <div className="rounded-lg border border-border bg-muted/30 p-3 text-sm text-muted-foreground">
              <div className="font-medium text-foreground">{t("Schema-driven configuration")}</div>
              <div className="mt-1 text-xs leading-5">
                {t(
                  selectedPlugin?.sourceKind === "external"
                    ? "This Provider is pinned to an installed external plugin package."
                    : "This plugin uses Mender's generic configuration renderer.",
                )}
              </div>
            </div>
          )}

          {creating ? (
            <Field label="Namespace">
              <Input
                aria-label={t("Namespace")}
                placeholder={t("namespace")}
                value={form.namespace}
                aria-invalid={form.namespace.trim() !== "" && !namespaceValid}
                onChange={(event) =>
                  onChange({ ...form, namespace: event.target.value })
                }
              />
              <p className="mt-1 text-xs text-muted-foreground">
                {t("Uppercase letters and spaces become a lowercase hyphenated identifier.")}
              </p>
              {form.namespace.trim() !== "" ? (
                <p className={namespaceValid ? "mt-1 break-all font-mono text-xs" : "mt-1 text-xs text-destructive"}>
                  {namespaceValid
                    ? t("Result: {{value}}", { value: canonicalNamespace })
                    : t("Use 1–63 characters, starting with a letter and ending with a letter or digit; only lowercase letters, digits and hyphens are allowed.")}
                </p>
              ) : null}
            </Field>
          ) : null}

          <Field label="Display name">
            <Input
              aria-label={t("Display name")}
              value={form.displayName}
              aria-invalid={form.displayName.trim() !== "" && new TextEncoder().encode(form.displayName.trim()).length > 160}
              onChange={(event) =>
                onChange({ ...form, displayName: event.target.value })
              }
            />
            {form.displayName.trim() !== "" && new TextEncoder().encode(form.displayName.trim()).length > 160 ? (
              <p className="mt-1 text-xs text-destructive">{t("Display name is too long (maximum 160 bytes).")}</p>
            ) : null}
          </Field>

          <Field label="Description" optional>
            <Input
              aria-label={t("Description")}
              value={form.description}
              aria-invalid={new TextEncoder().encode(form.description.trim()).length > 4096}
              onChange={(event) =>
                onChange({ ...form, description: event.target.value })
              }
            />
            {new TextEncoder().encode(form.description.trim()).length > 4096 ? (
              <p className="mt-1 text-xs text-destructive">{t("Description is too long (maximum 4096 bytes).")}</p>
            ) : null}
          </Field>

          {renderer === "generic" ? (
            <div className="space-y-3">
              {form.installationId ? (
                <div className="grid grid-cols-2 gap-3">
                  <ReadOnlyField
                    label="Installation"
                    value={form.installationId}
                  />
                  <ReadOnlyField
                    label="Package version"
                    value={selectedPlugin?.packageVersion ?? t("Unavailable")}
                  />
                </div>
              ) : null}
              <GenericProviderConfigForm
                key={providerFormSelectionKey(form)}
                schema={selectedPlugin?.configSchema}
                uiSchema={selectedPlugin?.uiSchema}
                value={form.genericConfig}
                credentials={credentials}
                sandboxes={sandboxes}
                pluginId={selectedPlugin?.pluginId}
                onChange={(genericConfig) =>
                  onChange({ ...form, genericConfig })
                }
              />
              {genericError ? (
                <p className="text-xs text-destructive">{genericError}</p>
              ) : null}
            </div>
          ) : null}

          {renderer === "model-tools" ? (
            <>
              <Field label="Model">
                <Input
                  aria-label={t("Model Tools model")}
                  placeholder={modelToolsModelPlaceholder(form.modelToolsEngine)}
                  value={form.modelToolsModel}
                  onChange={(event) =>
                    onChange({ ...form, modelToolsModel: event.target.value })
                  }
                />
              </Field>
              <ReadOnlyField label="Capability" value={t("Web search")} />
              <p className="text-xs leading-5 text-muted-foreground">
                {t("Mender exposes one stable web_search Tool. The selected model must execute the provider-hosted search tool; model-only answers are rejected.")}
              </p>
            </>
          ) : null}

          {renderer === "mcp" ? (
            form.transport === "http" ? (
            <>
              <Field label="Endpoint">
                <Input
                  aria-label={t("Endpoint")}
                  placeholder={t("https://example.com/mcp")}
                  value={form.endpoint}
                  onChange={(event) =>
                    onChange({ ...form, endpoint: event.target.value })
                  }
                />
                <p className="mt-1 text-xs text-muted-foreground">
                  {t("Non-loopback endpoints require HTTPS.")}
                </p>
              </Field>

              <details className="rounded-lg border border-border">
                <summary className="cursor-pointer select-none px-4 py-3 text-sm font-medium">
                  {t("Advanced settings")}
                </summary>
                <div className="space-y-4 border-t border-border px-4 py-4">
                  <ReadOnlyField
                    label="MCP type"
                    value="streamable-http"
                  />

                  <Field label="Legacy HTTP header templates" optional>
                    <Textarea
                      aria-label={t("HTTP header templates")}
                      className="min-h-24 font-mono text-xs"
                      placeholder={
                        "Authorization: Bearer ${ANYSEARCH_API_KEY}\nX-Anysearch-Client: mcp/1.0.0"
                      }
                      value={form.headers}
                      onChange={(event) =>
                        onChange({ ...form, headers: event.target.value })
                      }
                    />
                    <p className="mt-1 text-xs leading-5 text-muted-foreground">
                      {t("Compatibility path only. Sensitive headers must use environment references such as")} <code>{"${ANYSEARCH_API_KEY}"}</code>{t(". Prefer the separate Authentication workflow for inventory-backed, Profile-scoped Credentials.")}
                    </p>
                  </Field>

                  {creating ? (
                    <div className="rounded-lg bg-muted/40 p-3">
                      <Field label="Import MCP JSON" optional>
                        <Textarea
                          aria-label={t("MCP JSON")}
                          className="min-h-28 font-mono text-xs"
                          placeholder={
                            '{"mcpServers":{"anysearch":{"type":"streamable-http","url":"https://api.anysearch.com/mcp","headers":{"Authorization":"Bearer ${ANYSEARCH_API_KEY}"}}}}'
                          }
                          value={form.importJson}
                          onChange={(event) =>
                            onChange({ ...form, importJson: event.target.value })
                          }
                        />
                      </Field>
                      {importError ? (
                        <p className="mt-2 text-xs text-destructive">
                          {importError}
                        </p>
                      ) : null}
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        className="mt-3"
                        disabled={!form.importJson.trim()}
                        onClick={() => {
                          try {
                            const imported = parseMCPJSON(form.importJson);
                            onChange({
                              ...form,
                              transport: "http",
                              namespace: imported.namespace,
                              displayName:
                                form.displayName || imported.namespace,
                              endpoint: imported.endpoint,
                              headers: formatHeaders(imported.headers),
                            });
                            setImportError("");
                          } catch (reason) {
                            setImportError(errorMessage(reason));
                          }
                        }}
                      >
                        {t("Import configuration")}
                      </Button>
                    </div>
                  ) : null}
                </div>
              </details>
            </>
          ) : (
            <>
              <Field label="Executable">
                <Input
                  aria-label={t("Executable")}
                  placeholder={t("/path/to/mcp-server")}
                  value={form.executable}
                  onChange={(event) =>
                    onChange({ ...form, executable: event.target.value })
                  }
                />
              </Field>
              <Field label="Arguments" optional>
                <Textarea
                  aria-label={t("Arguments")}
                  placeholder={"One argument per line\n--stdio"}
                  value={form.args}
                  onChange={(event) =>
                    onChange({ ...form, args: event.target.value })
                  }
                />
              </Field>
              <Field label="Working directory" optional>
                <Input
                  aria-label={t("Working directory")}
                  value={form.directory}
                  onChange={(event) =>
                    onChange({ ...form, directory: event.target.value })
                  }
                />
              </Field>
              <p className="text-xs text-muted-foreground">
                {t("Mender does not persist environment values or secrets for stdio Providers.")}
              </p>
            </>
            )
          ) : null}

          {renderer === "openapi" ? (
            <>
              {form.openapiSourceKind === "url" ? (
                <Field label="Description URL">
                  <Input
                    aria-label={t("OpenAPI description URL")}
                    placeholder={t("https://example.com/openapi.yaml")}
                    value={form.openapiSpecUrl}
                    onChange={(event) =>
                      onChange({ ...form, openapiSpecUrl: event.target.value })
                    }
                  />
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t("Non-loopback URLs require HTTPS. External $ref retrieval is disabled.")}
                  </p>
                </Field>
              ) : (
                <Field label="Description file">
                  <Input
                    aria-label={t("OpenAPI description file")}
                    placeholder={t("/absolute/path/to/openapi.yaml")}
                    value={form.openapiSpecFile}
                    onChange={(event) =>
                      onChange({ ...form, openapiSpecFile: event.target.value })
                    }
                  />
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t("Use an absolute local path. External file references are disabled.")}
                  </p>
                </Field>
              )}

              <Field label="API server override" optional>
                <Input
                  aria-label={t("OpenAPI server override")}
                  placeholder={t("https://api.example.com")}
                  value={form.openapiServerUrl}
                  onChange={(event) =>
                    onChange({ ...form, openapiServerUrl: event.target.value })
                  }
                />
              </Field>

              <details className="rounded-lg border border-border">
                <summary className="cursor-pointer select-none px-4 py-3 text-sm font-medium">
                  {t("Operations")}
                </summary>
                <div className="space-y-4 border-t border-border px-4 py-4">
                  <p className="text-xs leading-5 text-muted-foreground">
                    {t("Filters use stable OpenAPI operation SourceKeys. Use operationId when present; otherwise use values such as")} <code>get /users/{"{id}"}</code>.
                  </p>
                  <Field label="Include SourceKeys" optional>
                    <Textarea
                      aria-label={t("OpenAPI include SourceKeys")}
                      className="min-h-24 font-mono text-xs"
                      placeholder={"One SourceKey per line\ngetUser"}
                      value={form.openapiInclude}
                      onChange={(event) =>
                        onChange({ ...form, openapiInclude: event.target.value })
                      }
                    />
                  </Field>
                  <Field label="Exclude SourceKeys" optional>
                    <Textarea
                      aria-label={t("OpenAPI exclude SourceKeys")}
                      className="min-h-24 font-mono text-xs"
                      placeholder={"One SourceKey per line\ndeleteUser"}
                      value={form.openapiExclude}
                      onChange={(event) =>
                        onChange({ ...form, openapiExclude: event.target.value })
                      }
                    />
                  </Field>
                </div>
              </details>
            </>
          ) : null}
        </div>

        <DialogFooter>
          {creating && onBack !== undefined ? (
            <Button
              type="button"
              variant="outline"
              disabled={busy}
              onClick={onBack}
            >
              <ArrowLeftIcon data-icon="inline-start" />
              {t("Back to plugins")}
            </Button>
          ) : null}
          <DialogClose render={<Button variant="outline" />}>
            {t("Cancel")}
          </DialogClose>
          {onAuthentication !== undefined && renderer !== "generic" ? (
            <Button
              type="button"
              variant="outline"
              disabled={busy}
              onClick={onAuthentication}
            >
              {t("Configure authentication")}
            </Button>
          ) : null}
          <Button disabled={busy || !valid} onClick={onSubmit}>
            {t(
              busy
                ? creating
                  ? "Adding…"
                  : "Saving…"
                : creating
                  ? "Add provider"
                  : "Save changes",
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ProviderAuthenticationDialog({
  open,
  form,
  credentials,
  busy,
  onOpenChange,
  onChange,
  onSubmit,
  submitLabel,
}: {
  open: boolean;
  form: ProviderFormState;
  credentials: Credential[];
  busy: boolean;
  onOpenChange: (open: boolean) => void;
  onChange: (value: ProviderFormState) => void;
  onSubmit: () => void;
  submitLabel: string;
}) {
  const firstCredential = credentials[0]?.reference ?? "";

  function addHTTPBinding() {
    if (!firstCredential) {
      return;
    }
    onChange({
      ...form,
      credentialHeaders: [
        ...form.credentialHeaders,
        {
          name: "Authorization",
          scheme: "Bearer",
          credentialRef: firstCredential,
        },
      ],
    });
  }

  function addStdioBinding() {
    if (!firstCredential) {
      return;
    }
    onChange({
      ...form,
      credentialEnvironment: [
        ...form.credentialEnvironment,
        {
          name: "TOKEN",
          credentialRef: firstCredential,
        },
      ],
    });
  }

  function addOpenAPIBinding() {
    if (!firstCredential) {
      return;
    }
    onChange({
      ...form,
      openapiCredentials: [
        ...form.openapiCredentials,
        {
          schemeName: "",
          credentialRef: firstCredential,
        },
      ],
    });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("Provider authentication")}</DialogTitle>
          <DialogDescription>
            {t("Bind Provider authentication to inventory Credentials. Mender stores only Credential References and resolves secret values at connection and execution boundaries.")}
          </DialogDescription>
        </DialogHeader>

        <DialogBody>
        {credentials.length ? (
          <div className="space-y-4">
            {form.pluginId === MODEL_TOOLS_PLUGIN_ID ? (
              <>
                <p className="text-xs leading-5 text-muted-foreground">
                  {t("Select the inventory Credential used to authenticate the configured model API. The secret is resolved only during governed Tool execution and remains subject to Profile Credential scope.")}
                </p>
                <Field label="Model API Credential">
                  <select
                    aria-label={t("Model Tools API credential")}
                    value={form.modelToolsCredentialRef}
                    onChange={(event) =>
                      onChange({
                        ...form,
                        modelToolsCredentialRef: event.target.value,
                      })
                    }
                    className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus:ring-2 focus:ring-ring/40"
                  >
                    <option value="" disabled>
                      {t("Choose a Credential")}
                    </option>
                    {credentials.map((item) => (
                      <option key={item.reference} value={item.reference}>
                        {item.displayName} · {item.reference}
                      </option>
                    ))}
                  </select>
                </Field>
              </>
            ) : form.pluginId === OPENAPI_PLUGIN_ID ? (
              <>
                <p className="text-xs leading-5 text-muted-foreground">
                  {t("Bind OpenAPI Security Scheme names to inventory Credentials. The scheme name must match the description exactly; Mender keeps the Credential Reference in Provider config and resolves the secret only at execution.")}
                </p>
                <div className="space-y-3">
                  {form.openapiCredentials.map((binding, index) => (
                    <div
                      key={index}
                      className="grid gap-3 rounded-lg border border-border p-3 sm:grid-cols-[1fr_1.4fr_auto]"
                    >
                      <Field label="Security Scheme">
                        <Input
                          aria-label={t("OpenAPI security scheme {{index}}", {
                            index: index + 1,
                          })}
                          placeholder={t("BearerAuth")}
                          value={binding.schemeName}
                          onChange={(event) =>
                            onChange({
                              ...form,
                              openapiCredentials: form.openapiCredentials.map(
                                (item, itemIndex) =>
                                  itemIndex === index
                                    ? {
                                        ...item,
                                        schemeName: event.target.value,
                                      }
                                    : item,
                              ),
                            })
                          }
                        />
                      </Field>
                      <Field label="Credential">
                        <select
                          aria-label={t("OpenAPI security credential {{index}}", {
                            index: index + 1,
                          })}
                          value={binding.credentialRef}
                          onChange={(event) =>
                            onChange({
                              ...form,
                              openapiCredentials: form.openapiCredentials.map(
                                (item, itemIndex) =>
                                  itemIndex === index
                                    ? {
                                        ...item,
                                        credentialRef: event.target.value,
                                      }
                                    : item,
                              ),
                            })
                          }
                          className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus:ring-2 focus:ring-ring/40"
                        >
                          {credentials.map((item) => (
                            <option key={item.reference} value={item.reference}>
                              {item.displayName} · {item.reference}
                            </option>
                          ))}
                        </select>
                      </Field>
                      <div className="flex items-end">
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={() =>
                            onChange({
                              ...form,
                              openapiCredentials: form.openapiCredentials.filter(
                                (_, itemIndex) => itemIndex !== index,
                              ),
                            })
                          }
                        >
                          {t("Remove")}
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={addOpenAPIBinding}
                >
                  {t("Add Security Scheme binding")}
                </Button>
              </>
            ) : form.transport === "http" ? (
              <>
                {form.headers.trim() ? (
                  <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-xs leading-5 text-muted-foreground">
                    {t("Legacy HTTP header templates are still configured. They remain compatible but are not governed by Profile Credential scope. Prefer inventory-backed bindings below.")}
                  </div>
                ) : null}

                <div className="space-y-3">
                  {form.credentialHeaders.map((binding, index) => (
                    <div
                      key={index}
                      className="grid gap-3 rounded-lg border border-border p-3 sm:grid-cols-[1fr_120px_1.4fr_auto]"
                    >
                      <Field label="Header">
                        <Input
                          aria-label={t("Credential header name {{index}}", {
                            index: index + 1,
                          })}
                          value={binding.name}
                          onChange={(event) =>
                            onChange({
                              ...form,
                              credentialHeaders: form.credentialHeaders.map(
                                (item, itemIndex) =>
                                  itemIndex === index
                                    ? { ...item, name: event.target.value }
                                    : item,
                              ),
                            })
                          }
                        />
                      </Field>
                      <Field label="Scheme">
                        <Input
                          aria-label={t("Credential header scheme {{index}}", {
                            index: index + 1,
                          })}
                          placeholder={t("Bearer")}
                          value={binding.scheme}
                          onChange={(event) =>
                            onChange({
                              ...form,
                              credentialHeaders: form.credentialHeaders.map(
                                (item, itemIndex) =>
                                  itemIndex === index
                                    ? { ...item, scheme: event.target.value }
                                    : item,
                              ),
                            })
                          }
                        />
                      </Field>
                      <Field label="Credential">
                        <select
                          aria-label={t("Credential header credential {{index}}", {
                            index: index + 1,
                          })}
                          value={binding.credentialRef}
                          onChange={(event) =>
                            onChange({
                              ...form,
                              credentialHeaders: form.credentialHeaders.map(
                                (item, itemIndex) =>
                                  itemIndex === index
                                    ? {
                                        ...item,
                                        credentialRef: event.target.value,
                                      }
                                    : item,
                              ),
                            })
                          }
                          className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus:ring-2 focus:ring-ring/40"
                        >
                          {credentials.map((item) => (
                            <option key={item.reference} value={item.reference}>
                              {item.displayName} · {item.reference}
                            </option>
                          ))}
                        </select>
                      </Field>
                      <div className="flex items-end">
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={() =>
                            onChange({
                              ...form,
                              credentialHeaders: form.credentialHeaders.filter(
                                (_, itemIndex) => itemIndex !== index,
                              ),
                            })
                          }
                        >
                          {t("Remove")}
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={addHTTPBinding}
                >
                  {t("Add credential header")}
                </Button>
              </>
            ) : (
              <>
                <div className="space-y-3">
                  {form.credentialEnvironment.map((binding, index) => (
                    <div
                      key={index}
                      className="grid gap-3 rounded-lg border border-border p-3 sm:grid-cols-[1fr_1.4fr_auto]"
                    >
                      <Field label="Environment name">
                        <Input
                          aria-label={t("Credential environment name {{index}}", {
                            index: index + 1,
                          })}
                          value={binding.name}
                          onChange={(event) =>
                            onChange({
                              ...form,
                              credentialEnvironment:
                                form.credentialEnvironment.map(
                                  (item, itemIndex) =>
                                    itemIndex === index
                                      ? { ...item, name: event.target.value }
                                      : item,
                                ),
                            })
                          }
                        />
                      </Field>
                      <Field label="Credential">
                        <select
                          aria-label={t("Credential environment credential {{index}}", {
                            index: index + 1,
                          })}
                          value={binding.credentialRef}
                          onChange={(event) =>
                            onChange({
                              ...form,
                              credentialEnvironment:
                                form.credentialEnvironment.map(
                                  (item, itemIndex) =>
                                    itemIndex === index
                                      ? {
                                          ...item,
                                          credentialRef: event.target.value,
                                        }
                                      : item,
                                ),
                            })
                          }
                          className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus:ring-2 focus:ring-ring/40"
                        >
                          {credentials.map((item) => (
                            <option key={item.reference} value={item.reference}>
                              {item.displayName} · {item.reference}
                            </option>
                          ))}
                        </select>
                      </Field>
                      <div className="flex items-end">
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={() =>
                            onChange({
                              ...form,
                              credentialEnvironment:
                                form.credentialEnvironment.filter(
                                  (_, itemIndex) => itemIndex !== index,
                                ),
                            })
                          }
                        >
                          {t("Remove")}
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={addStdioBinding}
                >
                  {t("Add credential environment")}
                </Button>
              </>
            )}
          </div>
        ) : (
          <EmptyState
            title="No Credentials in inventory"
            description="Create a Credential first, then return here to configure Provider authentication."
          />
        )}
        </DialogBody>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("Cancel")}
          </Button>
          <Button disabled={busy} onClick={onSubmit}>
            {busy ? t("Saving…") : t(submitLabel)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ProviderDetailsDialog({
  provider,
  plugins,
  onOpenChange,
}: {
  provider: Provider | null;
  plugins: ProviderPlugin[];
  onOpenChange: (open: boolean) => void;
}) {
  const selectedPlugin =
    provider === null ? undefined : providerPluginForProvider(plugins, provider);
  return (
    <Dialog open={provider !== null} onOpenChange={onOpenChange}>
      <DialogContent className="grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t("Provider details")}</DialogTitle>
          <DialogDescription>
            {t("Identity, connection configuration, and timestamps for this Provider.")}
          </DialogDescription>
        </DialogHeader>

        <DialogBody>
        {provider !== null ? (
          <div className="grid gap-3">
            <DetailRow label="Display name" value={provider.displayName} />
            <DetailRow label="Namespace" value={provider.namespace} mono />
            <DetailRow label="Provider ID" value={provider.providerId} mono />
            <DetailRow
              label="Provider plugin"
              value={providerPluginLabelForProvider(plugins, provider)}
            />
            {provider.pluginId ? (
              <DetailRow label="Plugin ID" value={provider.pluginId} mono />
            ) : null}
            {provider.installationId ? (
              <>
                <DetailRow
                  label="Installation"
                  value={provider.installationId}
                  mono
                />
                <DetailRow
                  label="Package version"
                  value={
                    selectedPlugin?.packageVersion ??
                    provider.pluginVersion ??
                    t("Unavailable")
                  }
                  mono
                />
                <DetailRow
                  label="Configuration"
                  value={JSON.stringify(provider.config ?? {}, null, 2)}
                  mono
                  preserve
                />
              </>
            ) : null}
            {providerIsBuiltIn(provider, MCP_PLUGIN_ID) ? (
              <DetailRow label="MCP transport" value={provider.transport} />
            ) : null}
            {providerIsBuiltIn(provider, MODEL_TOOLS_PLUGIN_ID) ? (
              <>
                <DetailRow
                  label="Engine"
                  value={modelToolsEngineSummary(provider)}
                />
                <DetailRow
                  label="Model"
                  value={modelToolsModelSummary(provider)}
                  mono
                />
                <DetailRow label="Capability" value={t("Web search")} />
              </>
            ) : null}
            {providerIsBuiltIn(provider, OPENAPI_PLUGIN_ID) ? (
              <>
                <DetailRow
                  label="Description source"
                  value={openAPISourceSummary(provider)}
                  mono
                />
                <DetailRow
                  label="Server override"
                  value={openAPIServerSummary(provider)}
                  mono
                />
                <DetailRow
                  label="Operation filters"
                  value={openAPIFilterSummary(provider)}
                  mono
                  preserve
                />
                <DetailRow
                  label="Security Schemes"
                  value={openAPISecuritySummary(provider)}
                  mono
                  preserve
                />
              </>
            ) : null}
            <DetailRow label="Synced Tools" value={formatNumber(provider.toolCount)} />
            <DetailRow
              label="Connection"
              value={connectionSummary(provider)}
              mono
            />
            {provider.transport === "http" &&
            provider.headers !== undefined &&
            Object.keys(provider.headers).length ? (
              <DetailRow
                label="Header templates"
                value={formatHeaders(provider.headers)}
                mono
                preserve
              />
            ) : null}
            {provider.transport === "stdio" && provider.args?.length ? (
              <DetailRow
                label="Arguments"
                value={provider.args.join("\n")}
                mono
                preserve
              />
            ) : null}
            {provider.transport === "stdio" && provider.directory ? (
              <DetailRow
                label="Working directory"
                value={provider.directory}
                mono
              />
            ) : null}
            <DetailRow
              label="Description"
              value={provider.description || "—"}
            />
            <DetailRow
              label="Created"
              value={formatTimestamp(provider.createdAt)}
            />
            <DetailRow
              label="Updated"
              value={formatTimestamp(provider.updatedAt)}
            />
          </div>
        ) : null}
        </DialogBody>

        <DialogFooter showCloseButton />
      </DialogContent>
    </Dialog>
  );
}

function Field({
  label,
  optional = false,
  children,
}: {
  label: string;
  optional?: boolean;
  children: React.ReactNode;
}) {
  return (
    <label className="block space-y-1.5">
      <span className="text-sm font-medium">
        {t(label)}
        {optional ? (
          <span className="ml-1 font-normal text-muted-foreground">
            {t("(optional)")}
          </span>
        ) : null}
      </span>
      {children}
    </label>
  );
}

function ReadOnlyField({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="mb-1.5 text-sm font-medium">{t(label)}</div>
      <div className="rounded-lg border border-border bg-muted/40 px-3 py-2 font-mono text-sm text-muted-foreground">
        {value}
      </div>
    </div>
  );
}

function DetailRow({
  label,
  value,
  mono = false,
  preserve = false,
}: {
  label: string;
  value: string;
  mono?: boolean;
  preserve?: boolean;
}) {
  return (
    <div className="grid grid-cols-[130px_minmax(0,1fr)] gap-4 border-b border-border/60 pb-3 last:border-0 last:pb-0">
      <div className="text-sm text-muted-foreground">{t(label)}</div>
      <div
        className={[
          "min-w-0 break-words text-sm",
          mono ? "font-mono text-xs" : "",
          preserve ? "whitespace-pre-wrap" : "",
        ].join(" ")}
      >
        {value}
      </div>
    </div>
  );
}

function formatTimestamp(value: string) {
  return formatDateTime(value);
}
