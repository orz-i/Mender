import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ArrowLeftIcon,
  BlocksIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  PackagePlusIcon,
  RefreshCwIcon,
  PlusIcon,
  PowerIcon,
  PowerOffIcon,
  Trash2Icon,
} from "lucide-react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";

import type { MenderClient } from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader,
  DialogTitle,
} from "@mender/ui/components/dialog";
import { Button } from "@mender/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@mender/ui/components/card";

import { formatDateTime, formatNumber, t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { dockerStatusText } from "../../shared/docker-status";
import { IconButton } from "../../shared/icon-button";
import { EmptyState, ErrorNotice } from "../../shared/page";
import { FocusSwitch } from "../../shared/focus-switch";
import { PageToolbar, ResultPanel } from "../../shared/page-toolbar";
import { FilterPopover, FilterSelectField } from "../../shared/filter-popover";
import {
  DataPanel,
  EmptyPanelState,
  SearchField,
} from "../../shared/workbench";
import {
  type MarketplaceInstaller,
  type MarketplacePlugin,
  type MarketplacePluginStatus,
  loadMarketplacePlugins,
  marketplaceCursorPage,
  removeMarketplacePlugin,
  setMarketplacePluginEnabled,
} from "./marketplace";

type SourceFilter = string;
type StatusFilter = "all" | MarketplacePluginStatus;
type KindFilter = string;

const MARKETPLACE_PAGE_SIZE = 12;

export function PluginsPage({
  client,
  installers = [],
}: {
  client: MenderClient;
  installers?: MarketplaceInstaller[];
}) {
  const { plugins, loaded, error, setError, refresh } =
    useMarketplacePlugins(client);
  const [searchParams] = useSearchParams();
  const [view,setView] = useState<"discover"|"installed">(
    ()=>searchParams.get("view")==="installed"?"installed":"discover"
  );
  const [busy, setBusy] = useState("");
  const [query, setQuery] = useState("");
  const [kindFilter, setKindFilter] = useState<KindFilter>("all");
  const [sourceFilter, setSourceFilter] = useState<SourceFilter>("all");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [cursorHistory, setCursorHistory] = useState<Array<string | null>>([
    null,
  ]);

  const sourceIntent = searchParams.get("intent") === "source";
  const scopedPlugins = useMemo(() => plugins.filter(plugin =>
    view==="discover"
      ? plugin.createAction!==undefined && (!sourceIntent || ["provider","integration"].includes(plugin.kind))
      : plugin.actionRef!==undefined
  ),[plugins,view,sourceIntent]);

  const kindOptions = useMemo(() => {
    const labelsByKind = new Map<string, string>();
    for (const plugin of scopedPlugins) {
      labelsByKind.set(plugin.kind, plugin.kindLabel);
    }
    return [
      { value: "all", label: "All plugin types" },
      ...Array.from(labelsByKind, ([value, label]) => ({ value, label })),
    ];
  }, [scopedPlugins]);

  const sourceOptions = useMemo(() => {
    const labelsBySource = new Map<string, string>();
    for (const plugin of scopedPlugins) {
      labelsBySource.set(plugin.source, plugin.sourceLabel);
    }
    return [
      { value: "all", label: "All plugin sources" },
      ...Array.from(labelsBySource, ([value, label]) => ({ value, label })),
    ];
  }, [scopedPlugins]);

  const visiblePlugins = useMemo(() => {
    const normalized = query.trim().toLocaleLowerCase();
    return scopedPlugins.filter((plugin) => {
      const matchesQuery =
        normalized === "" ||
        plugin.displayName.toLocaleLowerCase().includes(normalized) ||
        plugin.description.toLocaleLowerCase().includes(normalized);
      const matchesKind =
        kindFilter === "all" || plugin.kind === kindFilter;
      const matchesSource =
        sourceFilter === "all" || plugin.source === sourceFilter;
      const matchesStatus =
        statusFilter === "all" || plugin.status === statusFilter;
      return matchesQuery && matchesKind && matchesSource && matchesStatus;
    });
  }, [kindFilter, scopedPlugins, query, sourceFilter, statusFilter]);

  const cursor = cursorHistory.at(-1) ?? null;
  const page = useMemo(
    () =>
      marketplaceCursorPage(
        visiblePlugins,
        cursor,
        MARKETPLACE_PAGE_SIZE,
      ),
    [cursor, visiblePlugins],
  );

  async function installPlugin(installer: MarketplaceInstaller) {
    setBusy("install:" + installer.key);
    setError("");
    try {
      if (await installer.install()) {
        await refresh();
      }
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  return (
    <>
      <PageToolbar
        tabs={<FocusSwitch
          label="Marketplace view"
          value={view}
          onChange={(next)=>{
            setView(next);
            setKindFilter("all");setSourceFilter("all");setStatusFilter("all");
            setQuery("");setCursorHistory([null]);
          }}
          items={[
            {value:"discover",label:"Discover"},
            {value:"installed",label:"Installed plugins"},
          ]}
        />}
        filters={
          <>
            <div className="w-full min-w-0 sm:w-48 sm:shrink-0">
              <SearchField
                value={query}
                onChange={(value) => { setQuery(value); setCursorHistory([null]); }}
                label="Search plugins"
                placeholder="Search plugins…"
              />
            </div>
            <FilterPopover
              activeCount={
                (kindFilter !== "all" ? 1 : 0) +
                (sourceFilter !== "all" ? 1 : 0) +
                (statusFilter !== "all" ? 1 : 0)
              }
              onReset={() => {
                setKindFilter("all");
                setSourceFilter("all");
                setStatusFilter("all");
                setCursorHistory([null]);
              }}
            >
              <FilterSelectField
                label="Plugin type filter"
                value={kindFilter}
                onChange={(value) => {
                  setKindFilter(value);
                  setCursorHistory([null]);
                }}
                options={kindOptions}
              />
              <FilterSelectField
                label="Plugin source filter"
                value={sourceFilter}
                onChange={(value) => {
                  setSourceFilter(value);
                  setCursorHistory([null]);
                }}
                options={sourceOptions}
              />
              <FilterSelectField
                label="Plugin status filter"
                value={statusFilter}
                onChange={(value) => {
                  setStatusFilter(value as StatusFilter);
                  setCursorHistory([null]);
                }}
                options={[
                  { value: "all", label: "All plugin statuses" },
                  { value: "enabled", label: "Enabled" },
                  { value: "available", label: "Available" },
                  { value: "disabled", label: "Disabled" },
                  { value: "unavailable", label: "Unavailable" },
                  { value: "degraded", label: "Degraded" },
                  { value: "invalid", label: "Invalid" },
                ]}
              />
            </FilterPopover>
          </>
        }
        actions={
          <>
            <Button type="button" variant="outline" size="sm" disabled={busy !== ""} onClick={() => void refresh()}>
              <RefreshCwIcon data-icon="inline-start" />
              {t("Refresh")}
            </Button>
            {view==="installed" && installers.map((installer) => (
              <Button key={installer.key} type="button" size="sm" disabled={busy !== ""} onClick={() => void installPlugin(installer)}>
                <PackagePlusIcon data-icon="inline-start" />
                {t(busy === "install:" + installer.key ? installer.pendingLabel : installer.label)}
              </Button>
            ))}
          </>
        }
      />

      {error ? <ErrorNotice message={error} /> : null}

      <ResultPanel title={view==="discover"?"Plugin catalog":"Installed plugins"}>
        {!loaded ? (
          <div className="px-4 py-12 text-center text-sm text-muted-foreground">
            {t("Loading plugins…")}
          </div>
        ) : visiblePlugins.length ? (
          <div className="p-4">
            <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
              {page.items.map((plugin) => (
                <PluginCard key={plugin.key} plugin={plugin} view={view} />
              ))}
            </div>
          </div>
        ) : (
          <EmptyPanelState>
            <EmptyState
              title="No matching plugins"
              description="Try a different search or filter."
            />
          </EmptyPanelState>
        )}
      </ResultPanel>

      {visiblePlugins.length ? (
        <MarketplaceCursorPagination
          page={page}
          canGoPrevious={cursorHistory.length > 1}
          onPrevious={() =>
            setCursorHistory((history) =>
              history.length > 1 ? history.slice(0, -1) : history,
            )
          }
          onNext={() => {
            if (page.nextCursor !== null) {
              setCursorHistory((history) => [
                ...history,
                page.nextCursor,
              ]);
            }
          }}
        />
      ) : null}
    </>
  );
}

export function PluginDetailPage({ client }: { client: MenderClient }) {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const backPath=searchParams.get("view")==="installed"?"/plugins?view=installed":"/plugins";
  const { pluginKey = "" } = useParams();
  const { plugins, loaded, error, setError, refresh } =
    useMarketplacePlugins(client);
  const [busy, setBusy] = useState("");
  const [removeTarget,setRemoveTarget]=useState<MarketplacePlugin|null>(null);
  const [section, setSection] = useState<"overview" | "capabilities" | "technical">("overview");
  const plugin = plugins.find((item) => item.key === pluginKey) ?? null;
  const createAction = plugin?.createAction;
  async function setEnabled(
    current: MarketplacePlugin,
    enabled: boolean,
  ) {
    setBusy(current.key);
    setError("");
    try {
      await setMarketplacePluginEnabled(client, current, enabled);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function remove(current:MarketplacePlugin){
    if(busy!=="" || !current.actions.remove)return;
    setBusy(current.key);
    setError("");
    try {
      await removeMarketplacePlugin(client, current);
      setRemoveTarget(null);
      void navigate("/plugins?view=installed", { replace: true });
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  return (
    <>
      <PageToolbar
        tabs={plugin ? (
          <FocusSwitch
            label="Plugin details"
            value={section}
            onChange={setSection}
            items={[
              { value: "overview", label: "Overview" },
              { value: "capabilities", label: "Capabilities" },
              { value: "technical", label: "Technical details" },
            ]}
          />
        ) : undefined}
        actions={
          <div className="flex flex-wrap items-center justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => void navigate(backPath)}
            >
              <ArrowLeftIcon data-icon="inline-start" />
              {t("Back")}
            </Button>
            {createAction !== undefined ? (
              <Button
                type="button"
                disabled={busy !== ""}
                onClick={() => void navigate(createAction.href)}
              >
                <PlusIcon data-icon="inline-start" />
                {t(createAction.label)}
              </Button>
            ) : null}
            {plugin?.actions.disable ? (
              <IconButton
                type="button"
                variant="outline"
                label={t("Disable {{plugin}}", {
                  plugin: plugin.displayName,
                })}
                disabled={busy !== ""}
                onClick={() => void setEnabled(plugin, false)}
              >
                <PowerOffIcon />
              </IconButton>
            ) : plugin?.actions.enable ? (
              <IconButton
                type="button"
                variant="outline"
                label={t("Enable {{plugin}}", {
                  plugin: plugin.displayName,
                })}
                disabled={busy !== ""}
                onClick={() => void setEnabled(plugin, true)}
              >
                <PowerIcon />
              </IconButton>
            ) : null}
            {plugin?.actions.remove ? (
              <IconButton
                type="button"
                variant="destructive"
                label={t("Remove {{plugin}}", {
                  plugin: plugin.displayName,
                })}
                disabled={busy !== ""}
                onClick={() => setRemoveTarget(plugin)}
              >
                <Trash2Icon />
              </IconButton>
            ) : null}
          </div>
        }
      />

      <Dialog open={removeTarget!==null} onOpenChange={(open)=>{
        if(!open && busy==="")setRemoveTarget(null);
      }}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("Confirm plugin removal")}</DialogTitle>
            <DialogDescription>
              {removeTarget?t('Remove installed plugin "{{plugin}}" {{version}}? Referenced resources will block removal.',{
                plugin:removeTarget.displayName,version:removeTarget.version,
              }):""}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" disabled={busy!==""}
              onClick={()=>setRemoveTarget(null)}>{t("Cancel")}</Button>
            <Button type="button" variant="destructive"
              disabled={busy!=="" || removeTarget===null}
              onClick={()=>{if(removeTarget)void remove(removeTarget);}}>
              {t("Remove")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {error ? <ErrorNotice message={error} /> : null}

      {!loaded ? (
        <DataPanel title="Plugin details" description="Plugin package">
          <div className="px-4 py-12 text-center text-sm text-muted-foreground">
            {t("Loading plugins…")}
          </div>
        </DataPanel>
      ) : plugin === null ? (
        <DataPanel title="Plugin details" description="Plugin package">
          <EmptyPanelState>
            <EmptyState
              title="No matching plugins"
              description="The requested item was not found."
            />
          </EmptyPanelState>
        </DataPanel>
      ) : (
        <div className="flex flex-col gap-4">
          {plugin.issue !== undefined ? (
            <ErrorNotice
              message={plugin.status === "unavailable" && plugin.key === "io.mender.sandbox.docker@builtin"
                ? dockerStatusText(plugin.issue.message) + " " + t("Mender Desktop automatically discovers local Docker engines. Start Docker Desktop or Colima to connect. For headless runtimes, configure MENDER_DOCKER_SOCKET.")
                : plugin.key === "io.mender.sandbox.daytona@builtin"
                  ? t("Experimental Daytona disabled; local operator must set MENDER_DAYTONA_EXPERIMENTAL=enabled before Runtime startup")
                  : t("Plugin issue: {{message}}", { message: plugin.issue.message })}
            />
          ) : null}
          {section === "overview" ? (
          <DataPanel
            title={plugin.displayName}
            description={pluginDescription(plugin) || t("No plugin description.")}
          >
            <div className="grid gap-3 p-4 sm:grid-cols-2 xl:grid-cols-4">
              <DetailField
                label="Type"
                value={pluginKindLabel(plugin)}
              />
              <DetailField
                label="Source"
                value={pluginSourceLabel(plugin)}
              />
              <DetailField label="Version" value={pluginVersionLabel(plugin)} />
              <DetailField
                label="Status"
                value={pluginStatusLabel(plugin)}
              />
            </div>
          </DataPanel>

          ) : section === "capabilities" ? (
          <DataPanel
            title="Capabilities"
            description={
              plugin.capabilities.length
                ? undefined
                : "No declared capabilities."
            }
          >
            <div className="flex min-h-20 flex-wrap items-start gap-2 p-4">
              {plugin.capabilities.length ? (
                plugin.capabilities.map((capability) => (
                  <Badge key={capability} variant="outline">
                    {capability}
                  </Badge>
                ))
              ) : (
                <span className="text-sm text-muted-foreground">
                  {t("No declared capabilities.")}
                </span>
              )}
            </div>
          </DataPanel>

          ) : (
          <DataPanel title="Technical details">
            <div className="grid gap-3 p-4 sm:grid-cols-2 xl:grid-cols-3">
              {plugin.technicalDetails.map((detail) => (
                <DetailField
                  key={detail.label}
                  label={detail.label}
                  value={
                    detail.format === "dateTime"
                      ? formatDateTime(detail.value)
                      : t(detail.value)
                  }
                  {...(detail.mono ? { mono: true } : {})}
                />
              ))}
            </div>
          </DataPanel>
          )}
        </div>
      )}
    </>
  );
}

function useMarketplacePlugins(client: MenderClient) {
  const [plugins, setPlugins] = useState<MarketplacePlugin[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");

  const refresh = useCallback(async () => {
    try {
      const marketplace = await loadMarketplacePlugins(client);
      setPlugins(marketplace);
      setError("");
      return marketplace;
    } catch (reason) {
      setError(errorMessage(reason));
      return null;
    } finally {
      setLoaded(true);
    }
  }, [client]);

  useEffect(() => {
    let active = true;
    void loadMarketplacePlugins(client)
      .then((marketplace) => {
        if (!active) {
          return;
        }
        setPlugins(marketplace);
        setError("");
        setLoaded(true);
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

  // The native host monitors the same pinned Engine after first connection.
  // Refresh both ready and unavailable statuses so daemon shutdown and
  // recovery are visible without manual interaction or daemon I/O from UI.
  const dockerPresent = plugins.some(
    (item) => item.key === "io.mender.sandbox.docker@builtin",
  );
  useEffect(() => {
    if (!dockerPresent) return;
    let pending = false;
    const timer = window.setInterval(() => {
      if (pending) return;
      pending = true;
      void refresh().finally(() => { pending = false; });
    }, 5000);
    return () => window.clearInterval(timer);
  }, [dockerPresent, refresh]);

  return {
    plugins,
    loaded,
    error,
    setError,
    refresh,
  };
}

function PluginCard({ plugin,view }: { plugin: MarketplacePlugin; view:"discover"|"installed" }) {
  return (
    <Link
      to={`/plugins/${encodeURIComponent(plugin.key)}?view=${view}`}
      aria-label={t("View {{plugin}}", {
        plugin: plugin.displayName,
      })}
      className="group block rounded-[22px] outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <Card className="h-full transition-[transform,box-shadow] duration-200 group-hover:-translate-y-0.5 group-hover:shadow-[0_16px_32px_-16px_rgb(18_18_18/0.24)]">
        <CardHeader className="gap-3">
          <div className="flex min-w-0 items-start gap-3">
            <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-foreground text-background dark:bg-muted dark:text-foreground">
              <BlocksIcon className="size-[19px]" aria-hidden="true" />
            </span>
            <div className="min-w-0 flex-1">
              <CardTitle className="truncate text-base" title={plugin.displayName}>
                {plugin.displayName}
              </CardTitle>
              <p className="mt-1 text-xs text-muted-foreground">{pluginKindLabel(plugin)}</p>
            </div>
            <Badge variant={plugin.status === "enabled" ? "default" : "secondary"}>
              {pluginStatusLabel(plugin)}
            </Badge>
          </div>
          <CardDescription className="min-h-[3.75rem] line-clamp-3">
            {pluginDescription(plugin) || t("No plugin description.")}
          </CardDescription>
        </CardHeader>

        <CardContent className="flex flex-col gap-3">
          <div className="flex flex-wrap gap-2">
            <Badge variant="secondary">{pluginKindLabel(plugin)}</Badge>
            <Badge variant="outline">{pluginSourceLabel(plugin)}</Badge>
          </div>
          {plugin.capabilities.length ? (
            <div className="flex min-h-6 flex-wrap items-center gap-1.5">
              {plugin.capabilities.slice(0, 3).map((capability) => (
                <Badge key={capability} variant="ghost">
                  {capability}
                </Badge>
              ))}
              {plugin.capabilities.length > 3 ? (
                <span className="text-xs text-muted-foreground">
                  +{formatNumber(plugin.capabilities.length - 3)}
                </span>
              ) : null}
            </div>
          ) : (
            <div className="min-h-6 text-xs text-muted-foreground">
              {t("No declared capabilities.")}
            </div>
          )}
        </CardContent>

        <CardFooter className="justify-between gap-3">
          <span className="text-xs text-muted-foreground">
            {t("Version")}
          </span>
          <span className="truncate font-mono text-xs">
            {pluginVersionLabel(plugin)}
          </span>
        </CardFooter>
      </Card>
    </Link>
  );
}

function MarketplaceCursorPagination({
  page,
  canGoPrevious,
  onPrevious,
  onNext,
}: {
  page: ReturnType<typeof marketplaceCursorPage>;
  canGoPrevious: boolean;
  onPrevious: () => void;
  onNext: () => void;
}) {
  return (
    <nav
      aria-label={t("{{label}} pagination", {
        label: t("Plugin Marketplace"),
      })}
      className="mt-4 flex flex-col gap-3 px-1 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between"
    >
      <p aria-live="polite">
        {t("Plugins {{from}}–{{to}} of {{total}}", {
          from: formatNumber(page.from),
          to: formatNumber(page.to),
          total: formatNumber(page.total),
        })}
      </p>
      <div className="flex items-center gap-2">
        <IconButton
          type="button"
          variant="outline"
          label={t("{{label}} previous page", {
            label: t("Plugin Marketplace"),
          })}
          disabled={!canGoPrevious}
          onClick={onPrevious}
        >
          <ChevronLeftIcon />
        </IconButton>
        <IconButton
          type="button"
          variant="outline"
          label={t("{{label}} next page", {
            label: t("Plugin Marketplace"),
          })}
          disabled={page.nextCursor === null}
          onClick={onNext}
        >
          <ChevronRightIcon />
        </IconButton>
      </div>
    </nav>
  );
}

function DetailField({
  label,
  value,
  mono = false,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div className="rounded-xl border border-border/70 bg-background/80 p-3">
      <p className="text-xs font-medium text-muted-foreground">
        {t(label)}
      </p>
      <p
        className={[
          "mt-1 break-all text-sm",
          mono ? "font-mono text-xs" : "",
        ].join(" ")}
      >
        {value}
      </p>
    </div>
  );
}

function pluginKindLabel(plugin:MarketplacePlugin){
  switch(plugin.kind){
    case "integration":return t("Remote MCP integration");
    case "provider":return t("Provider plugin");
    case "sandbox":return t("Sandbox backend");
    case "tunnel":return t("Tunnel transport");
    case "agent":return t("Agent backend");
    default:return t(plugin.kindLabel);
  }
}

function pluginSourceLabel(plugin:MarketplacePlugin){
  switch(plugin.source){
    case "curated":return t("Curated");
    case "builtin":return t("Built-in");
    case "local":return t("Local package");
    default:return t(plugin.sourceLabel);
  }
}

function pluginStatusLabel(plugin: MarketplacePlugin) {
  switch (plugin.status) {
    case "available":
      return t("Available");
    case "enabled":
      return t("Enabled");
    case "disabled":
      return t("Disabled");
    case "invalid":
      return t("Invalid");
    case "degraded":
      return t("Degraded");
    case "unavailable":
      return t("Unavailable");
  }
}

function pluginVersionLabel(plugin: MarketplacePlugin) {
  return plugin.version === "Reviewed preset" ? t("Reviewed preset") : plugin.version;
}

function pluginDescription(plugin: MarketplacePlugin) {
  return plugin.description;
}
