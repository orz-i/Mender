import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useLocation, useNavigate, useSearchParams } from "react-router";
import {
  ArrowRightIcon,
  EllipsisIcon,
  EyeIcon,
  KeyRoundIcon,
  Link2Icon,
  PencilIcon,
  RefreshCwIcon,
  Trash2Icon,
} from "lucide-react";

import type {
  Credential,
  IntegrationOffering,
  ManagementClient,
  Provider,
  ProviderPlugin,
  ProviderPluginIssue,
} from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@mender/ui/components/card";
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

import { t, tCount, useLocale } from "../../../i18n/i18n";
import { errorMessage } from "../../../shared/errors";
import { canonicalizeOwnedIdentifier, isCanonicalOwnedSlug, ownedIdentifierCreateError } from "../../../shared/form-identifiers";
import { FormHeading } from "../../../shared/form-info";
import { EmptyState, ErrorNotice, InsightBanner } from "../../../shared/page";
import { PageToolbar, ResultPanel } from "../../../shared/page-toolbar";
import { usePagination } from "../../../shared/pagination";
import { TablePagination } from "../../../shared/table-pagination";
import { EmptyPanelState, SearchField } from "../../../shared/workbench";
import { toolSourceLabel } from "../tool-source-label";
import { ProviderAuthenticationDialog } from "./provider-authentication-dialog";
import { ProviderConfigurationDialog } from "./provider-configuration-dialog";
import { useProviderCreation } from "./provider-create";
import { ProviderDetailsDialog } from "./provider-details-dialog";
import { ProviderEditDialog } from "./provider-edit-dialog";
import { Field } from "./provider-fields";
import {
  MCP_PLUGIN_ID,
  type ProviderFormState,
  emptyProviderForm,
  formFromProvider,
  isInternalProvider,
  updateRequestFromForm,
} from "./provider-form-model";
import {
  formForProviderPlugin,
  providerRebindCandidates,
  providerSupportsFirstPartyAuthentication,
} from "./provider-plugin-selection";
import { ProviderRebindDialog } from "./provider-rebind-dialog";
import { toolServiceDescription } from "./tool-service-description";

export function ToolsPage({ client }: { client: ManagementClient }) {
  const navigate = useNavigate();
  const location = useLocation();
  const locale = useLocale();
  const [searchParams, setSearchParams] = useSearchParams();
  const createHandledRef = useRef(false);
  const [createRequest] = useState(() => {
    const remainingSearchParams = new URLSearchParams(searchParams);
    remainingSearchParams.delete("create");
    remainingSearchParams.delete("pluginId");
    remainingSearchParams.delete("installationId");
    remainingSearchParams.delete("offeringId");
    const requested = searchParams.get("create") === "1";
    const origin = (location.state as { marketplaceReturnTo?: unknown } | null)?.marketplaceReturnTo;
    // Restore only an in-app plugin detail route, never a caller-supplied URL.
    const marketplaceReturnTo = requested && typeof origin === "string" &&
      /^\/plugins\/[^/?#]+(?:\?view=(?:discover|installed))?$/.test(origin) ? origin : null;
    return {
      requested,
      marketplaceReturnTo,
      pluginId: searchParams.get("pluginId"),
      installationId: searchParams.get("installationId") ?? "",
      offeringId: searchParams.get("offeringId") ?? "",
      remainingSearchParams,
    };
  });
  const [marketplaceReturnTo, setMarketplaceReturnTo] = useState(createRequest.marketplaceReturnTo);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [providerPlugins, setProviderPlugins] = useState<ProviderPlugin[]>([]);
  const [providerPluginIssues, setProviderPluginIssues] = useState<
    ProviderPluginIssue[]
  >([]);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [hasSnapshot, setHasSnapshot] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [inventoryQuery, setInventoryQuery] = useState("");
  const [openActionsProviderId, setOpenActionsProviderId] = useState<string | null>(null);

  const [connectionOffering, setConnectionOffering] = useState<IntegrationOffering|null>(null);
  const [offeringName, setOfferingName] = useState("");
  const [offeringNamespace, setOfferingNamespace] = useState("");
  const [offeringCredentialRef, setOfferingCredentialRef] = useState("");
  const [offeringError, setOfferingError] = useState("");

  const [viewProvider, setViewProvider] = useState<Provider | null>(null);
  const [editProvider, setEditProvider] = useState<Provider | null>(null);
  const [rebindRequest, setRebindRequest] = useState<{ provider: Provider; target: ProviderPlugin } | null>(null);
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
      setHasSnapshot(true);
      setError("");
      return result.providers;
    } catch (reason) {
      setError(errorMessage(reason));
      return null;
    } finally {
      setLoaded(true);
    }
  }, [client]);

  function onBusyChange(operation: string) {
    setBusy(operation);
    if (operation !== "") setError("");
  }

  const creation = useProviderCreation({
    client,
    plugins: providerPlugins,
    busy,
    onBusyChange,
    onCreated: () => setMarketplaceReturnTo(null),
    refresh,
  });
  const { setOpen: setCreateOpen, setForm: setCreateForm } = creation;

  useEffect(() => {
    let active = true;

    void Promise.all([
      client.listProviderPlugins(),
      client.listProviders(),
      client.listCredentials(),
      createRequest.offeringId ? client.listIntegrationOfferings() : Promise.resolve({integrations:[] as IntegrationOffering[]}),
    ])
      .then(([pluginResult, providerResult, credentialResult, offeringResult]) => {
        if (active) {
          setProviderPlugins(pluginResult.plugins);
          setProviderPluginIssues(pluginResult.issues);
          setProviders(providerResult.providers);
          setHasSnapshot(true);
          setCredentials(credentialResult.credentials);

          if (createRequest.requested && !createHandledRef.current && createRequest.offeringId) {
            createHandledRef.current = true;
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
          } else if (createRequest.requested && !createHandledRef.current) {
            createHandledRef.current = true;
            const plugin =
              createRequest.pluginId === null
                ? undefined
                : pluginResult.plugins.find(
                    (candidate) =>
                      candidate.pluginId === createRequest.pluginId &&
                      (candidate.installationId ?? "") ===
                        createRequest.installationId,
                  );
            if (plugin === undefined || isInternalProvider(plugin.pluginId)) {
              // Resource-owned Tool Suites must be prepared on their Agent or
              // Sandbox; a generic Provider deep link is never that authority.
              setError(t("The requested item was not found."));
            } else {
              setError("");
              setCreateForm(formForProviderPlugin(emptyProviderForm, plugin));
              setCreateOpen(true);
            }
            setSearchParams(createRequest.remainingSearchParams, {
              replace: true,
            });
          } else if (!createRequest.requested) {
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
  }, [client, createRequest, setCreateForm, setCreateOpen, setSearchParams]);

  const visibleProviders = useMemo(() => {
    const needle = inventoryQuery.trim().toLocaleLowerCase();
    return providers.filter(provider =>
      !needle || [toolSourceLabel(provider), provider.description,
        locale === "zh-CN" ? toolServiceDescription(provider) : provider.description,
        provider.displayName, provider.namespace, provider.toolSourceName ?? ""]
        .some(value => value.toLocaleLowerCase().includes(needle))
    );
  }, [inventoryQuery, locale, providers]);
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
      setMarketplaceReturnTo(null);
      await refresh();
    }catch(reason){
      setOfferingError(ownedIdentifierCreateError(reason,"provider"));
      await refresh();
    }finally{
      setBusy("");
    }
  }

  function beginEdit(provider: Provider) {
    if (!provider.pluginId) return;
    setError("");
    setEditProvider(provider);
  }

  function beginRebind(provider: Provider) {
    const target = providerRebindCandidates(providerPlugins, provider)[0];
    if (target === undefined) return;
    setError("");
    setRebindRequest({ provider, target });
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
    setAuthenticationOriginForm(creation.form);
    setCreateOpen(false);
    setAuthenticationReturnsToCreate(true);
    window.setTimeout(() => {
      setAuthenticationProvider(null);
      setAuthenticationForm({ ...creation.form });
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

  function returnToMarketplaceOnCancel() {
    const returnTo = marketplaceReturnTo;
    setMarketplaceReturnTo(null);
    if (returnTo) void navigate(returnTo, { replace: true });
  }

  function setCreateDialogOpen(open: boolean) {
    if (!open && busy !== "") return;
    setCreateOpen(open);
    if (!open) {
      creation.reset();
      returnToMarketplaceOnCancel();
    }
  }

  function dismissConnectionOffering() {
    setConnectionOffering(null);
    setOfferingError("");
    returnToMarketplaceOnCancel();
  }

  return (
    <>
      <PageToolbar
        filters={
          <div className="w-full min-w-0 sm:w-56 md:w-64 sm:shrink-0">
            <SearchField label="Search services" placeholder="Search services…" value={inventoryQuery} onChange={setInventoryQuery} />
          </div>
        }
        actions={
          <>
            <Button variant="outline" size="sm" disabled={busy !== ""} onClick={() => void refresh()}>{t("Refresh")}</Button>
            <Button size="sm" onClick={() => void navigate("/plugins?intent=source")}>{t("Add tool service")}</Button>
          </>
        }
      />
      {error ? <ErrorNotice message={error} onRetry={!hasSnapshot ? () => { void refresh(); } : undefined} /> : null}
      {hasSnapshot && providerPluginIssues.length ? (
        <InsightBanner
          title="Some Provider plugins need attention. Review Plugin Marketplace for installation details."
          actions={
            <Button
              size="sm"
              variant="primary"
              onClick={() => void navigate("/plugins?view=installed")}
            >
              {t("Review Marketplace")}
            </Button>
          }
        />
      ) : null}
      {error && !hasSnapshot ? null : <ResultPanel title="Tool services" variant="plain" empty={visibleProviders.length === 0} className="w-full">
        {!loaded ? (
          <div className="px-4 py-12 text-center text-sm text-muted-foreground">
            {t("Loading services…")}
          </div>
        ) : visibleProviders.length ? (
          <div className="grid w-full min-w-0 gap-3 sm:grid-cols-2 2xl:grid-cols-3">
            {providerPagination.items.map(provider => {
              const name=toolSourceLabel(provider);
              const managed=provider.toolSourceKind==="agent" || provider.toolSourceKind==="sandbox" ||
                isInternalProvider(provider.pluginId);
              const ownerPath=provider.toolSourceKind==="agent" && provider.toolSourceId
                ? "/agents/"+encodeURIComponent(provider.toolSourceId)
                : provider.toolSourceKind==="sandbox" && provider.toolSourceId
                  ? "/sandboxes/"+encodeURIComponent(provider.toolSourceId)
                  : null;
              const hasTools=provider.toolCount>0;
              const description=toolServiceDescription(provider).trim() ||
                (!hasTools ? t("No tools have been discovered for this service yet.") : "");
              return <Card key={provider.providerId} size="sm" className="h-full min-w-0 border-border/60 transition-[border-color,box-shadow] hover:border-primary/35 hover:shadow-sm">
                <CardHeader className="min-w-0">
                  <CardTitle className="min-w-0">
                    <Link className="block truncate hover:text-primary-ink hover:underline focus-visible:rounded focus-visible:outline-2 focus-visible:outline-ring"
                      title={name} to={"/tools/"+encodeURIComponent(provider.providerId)}>{name}</Link>
                  </CardTitle>
                  {description ? <CardDescription className="line-clamp-2">
                    {description}
                  </CardDescription> : null}
                  <CardAction>
                    <Badge variant="secondary">
                      {tCount(provider.toolCount,"{{count}} tool","{{count}} tools")}
                    </Badge>
                  </CardAction>
                </CardHeader>
                <CardContent className="mt-auto flex min-w-0 flex-wrap items-center gap-2 border-t border-border/55 pt-3">
                  {hasTools ? (
                    <Button size="sm" onClick={() => void navigate("/tools/"+encodeURIComponent(provider.providerId))}>
                      {t("View tools")} <ArrowRightIcon data-icon="inline-end" />
                    </Button>
                  ) : ownerPath ? (
                    <Button size="sm" onClick={() => void navigate(ownerPath)}>
                      {t("Manage")}
                    </Button>
                  ) : (
                    <Button size="sm" onClick={() => setViewProvider(provider)}>
                      {t("Details")}
                    </Button>
                  )}
                  {hasTools && ownerPath ? (
                    <Button size="sm" variant="outline" onClick={() => void navigate(ownerPath)}>
                      {t("Manage")}
                    </Button>
                  ) : hasTools && managed ? (
                    <Button size="sm" variant="outline" onClick={() => setViewProvider(provider)}>
                      {t("Details")}
                    </Button>
                  ) : !managed ? (
                    <Popover open={openActionsProviderId === provider.providerId}
                      onOpenChange={(open) => setOpenActionsProviderId(open ? provider.providerId : null)}>
                      <PopoverTrigger render={<Button size="sm" variant="outline" type="button" aria-label={t("Actions") + ": " + name} />}>
                        <EllipsisIcon className="size-4" aria-hidden="true" />
                        {t("Actions")}
                      </PopoverTrigger>
                      <PopoverContent align="end" className="w-56 p-2">
                        <div className="grid gap-1" aria-label={t("Actions")}>
                          {hasTools ? (
                            <Button size="sm" variant="ghost" className="justify-start" aria-label={t("View {{provider}}", {provider:name})}
                              onClick={() => { setOpenActionsProviderId(null); setViewProvider(provider); }}>
                              <EyeIcon data-icon="inline-start" />{t("Details")}
                            </Button>
                          ) : null}
                          <Button size="sm" variant="ghost" className="justify-start" aria-label={t("Edit {{provider}}", {provider:name})}
                            disabled={busy!=="" || !provider.pluginId}
                            onClick={() => { setOpenActionsProviderId(null); beginEdit(provider); }}>
                            <PencilIcon data-icon="inline-start" />{t("Edit")}
                          </Button>
                          {providerSupportsFirstPartyAuthentication(provider) ? (
                            <Button size="sm" variant="ghost" className="justify-start" aria-label={t("Authentication {{provider}}", {provider:name})}
                              disabled={busy!==""}
                              onClick={() => { setOpenActionsProviderId(null); openProviderAuthentication(provider); }}>
                              <KeyRoundIcon data-icon="inline-start" />{t("Authentication")}
                            </Button>
                          ) : null}
                          {providerRebindCandidates(providerPlugins,provider).length ? (
                            <Button size="sm" variant="ghost" className="justify-start" aria-label={t("Rebind {{provider}}", {provider:name})}
                              disabled={busy!==""}
                              onClick={() => { setOpenActionsProviderId(null); beginRebind(provider); }}>
                              <Link2Icon data-icon="inline-start" />{t("Rebind")}
                            </Button>
                          ) : null}
                          <Button size="sm" variant="ghost" className="justify-start" aria-label={t("Sync {{provider}}", {provider:name})}
                            disabled={busy!==""}
                            onClick={() => { setOpenActionsProviderId(null); void syncProvider(provider); }}>
                            <RefreshCwIcon data-icon="inline-start" />{t("Sync")}
                          </Button>
                          <Button size="sm" variant="ghost" className="justify-start text-destructive hover:text-destructive"
                            aria-label={t("Delete {{provider}}", {provider:name})}
                            disabled={busy!==""}
                            onClick={() => { setOpenActionsProviderId(null); void deleteProvider(provider); }}>
                            <Trash2Icon data-icon="inline-start" />{t("Delete")}
                          </Button>
                        </div>
                      </PopoverContent>
                    </Popover>
                  ) : null}
                </CardContent>
              </Card>;
            })}
          </div>
        ) : (
          <EmptyPanelState>
            <EmptyState title={providers.length?t("No matching services"):t("No tool services")}
              description={providers.length?t("Try a different search or filter."):t("Add a service from Plugin Marketplace to discover its tools.")}/>
          </EmptyPanelState>
        )}
      </ResultPanel>}
      {hasSnapshot ? <TablePagination label="Tool services" pagination={providerPagination} /> : null}

      {connectionOffering!==null?(
        <Dialog open onOpenChange={open=>{
          if(!open&&busy!=="integration")dismissConnectionOffering();
        }}>
          <DialogContent className="max-h-[calc(100dvh-2rem)] grid-rows-[auto_minmax(0,1fr)_auto] sm:max-w-xl">
            <div className="grid gap-2">
              <DialogHeader>
                <DialogTitle>{t("Connect {{integration}}",{integration:connectionOffering.displayName})}</DialogTitle>
                <DialogDescription className="sr-only">{t("Configure this reviewed Remote MCP integration. No Profile access is granted automatically.")}</DialogDescription>
              </DialogHeader>
              {offeringError?<div role="alert" className="rounded-lg border border-destructive/20 p-3 text-sm text-destructive">{offeringError}</div>:null}
            </div>
            <div className="min-h-0 space-y-4 overflow-y-auto pr-1">
              <Field label="Display name">
                <Input aria-label={t("Integration name")} value={offeringName}
                  onChange={event=>setOfferingName(event.target.value)}/>
              </Field>
              <div className="min-w-0">
                <FormHeading label={t("Reviewed endpoint · Streamable HTTP")} help={t("The endpoint and client header come from the reviewed integration definition.")} />
                <p className="break-all font-mono text-xs text-muted-foreground">{connectionOffering.endpoint}</p>
              </div>
              <p role="note" className="text-xs text-muted-foreground">{t("Connecting does not grant Profile access.")}</p>
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
              <Button variant="outline" disabled={busy==="integration"} onClick={dismissConnectionOffering}>{t("Cancel")}</Button>
              <Button disabled={busy!=="" || !isCanonicalOwnedSlug(canonicalizeOwnedIdentifier(offeringNamespace)) ||
                !offeringName.trim() || new TextEncoder().encode(offeringName.trim()).length>160}
                onClick={()=>void connectOffering()}>{t("Connect")}</Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      ):null}

      <ProviderConfigurationDialog
        open={creation.open}
        mode="create"
        form={creation.form}
        plugins={providerPlugins}
        credentials={credentials}
        busy={busy === "create"}
        createError={creation.submitError}
        draftError={creation.draftError}
        onDraftErrorChange={creation.setDraftError}
        onOpenChange={setCreateDialogOpen}
        onChange={creation.change}
        onAuthentication={openCreateAuthentication}
        onSubmit={() => void creation.submit()}
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
              ? t("Use authentication")
              : t("Save authentication")
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

      {editProvider !== null ? (
        <ProviderEditDialog
          key={editProvider.providerId}
          client={client}
          provider={editProvider}
          plugins={providerPlugins}
          credentials={credentials}
          busy={busy}
          onBusyChange={onBusyChange}
          onClose={() => setEditProvider(null)}
          refresh={refresh}
        />
      ) : null}

      {rebindRequest !== null ? (
        <ProviderRebindDialog
          client={client}
          provider={rebindRequest.provider}
          candidates={providerRebindCandidates(providerPlugins, rebindRequest.provider)}
          initialPlugin={rebindRequest.target}
          credentials={credentials}
          busy={busy}
          onBusyChange={onBusyChange}
          onClose={() => setRebindRequest(null)}
          onSave={async () => {
            setRebindRequest(null);
            await refresh();
            await refreshProviderPlugins();
          }}
        />
      ) : null}
    </>
  );
}
