import { useCallback, useEffect, useMemo, useState } from "react";
import {
  EyeIcon,
  PackagePlusIcon,
  PowerIcon,
  PowerOffIcon,
  Trash2Icon,
} from "lucide-react";

import type { MenderClient } from "@mender/sdk";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@mender/ui/components/table";

import { formatDateTime, t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { IconButton } from "../../shared/icon-button";
import { EmptyState, ErrorNotice, PageHeader } from "../../shared/page";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import {
  DataPanel,
  EmptyPanelState,
  FilterSelect,
  SearchField,
  WorkbenchToolbar,
} from "../../shared/workbench";
import {
  type MarketplaceInstaller,
  type MarketplacePlugin,
  type MarketplacePluginStatus,
  loadMarketplacePlugins,
  removeMarketplacePlugin,
  setMarketplacePluginEnabled,
} from "./marketplace";

type SourceFilter = string;
type StatusFilter = "all" | MarketplacePluginStatus;
type KindFilter = string;
type PluginDetailView = "overview" | "technical";

export function PluginsPage({
  client,
  installers = [],
}: {
  client: MenderClient;
  installers?: MarketplaceInstaller[];
}) {
  const [plugins, setPlugins] = useState<MarketplacePlugin[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [query, setQuery] = useState("");
  const [kindFilter, setKindFilter] = useState<KindFilter>("all");
  const [sourceFilter, setSourceFilter] = useState<SourceFilter>("all");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [selected, setSelected] = useState<MarketplacePlugin | null>(null);
  const [detailView, setDetailView] =
    useState<PluginDetailView>("overview");

  const refresh = useCallback(async () => {
    try {
      const marketplace = await loadMarketplacePlugins(client);
      setPlugins(marketplace);
      setSelected((current) => {
        if (current === null) {
          return null;
        }
        return marketplace.find((item) => item.key === current.key) ?? null;
      });
      setError("");
    } catch (reason) {
      setError(errorMessage(reason));
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

  const kindOptions = useMemo(() => {
    const labelsByKind = new Map<string, string>();
    for (const plugin of plugins) {
      labelsByKind.set(plugin.kind, plugin.kindLabel);
    }
    return [
      { value: "all", label: "All plugin types" },
      ...Array.from(labelsByKind, ([value, label]) => ({ value, label })),
    ];
  }, [plugins]);

  const sourceOptions = useMemo(() => {
    const labelsBySource = new Map<string, string>();
    for (const plugin of plugins) {
      labelsBySource.set(plugin.source, plugin.sourceLabel);
    }
    return [
      { value: "all", label: "All plugin sources" },
      ...Array.from(labelsBySource, ([value, label]) => ({ value, label })),
    ];
  }, [plugins]);

  const visiblePlugins = useMemo(() => {
    const normalized = query.trim().toLocaleLowerCase();
    return plugins.filter((plugin) => {
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
  }, [kindFilter, plugins, query, sourceFilter, statusFilter]);

  const pagination = usePagination(visiblePlugins, {
    initialPageSize: 10,
    pageSizeOptions: [10, 25, 50],
  });

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

  async function setEnabled(plugin: MarketplacePlugin, enabled: boolean) {
    setBusy(plugin.key);
    setError("");
    try {
      await setMarketplacePluginEnabled(client, plugin, enabled);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function remove(plugin: MarketplacePlugin) {
    if (
      !window.confirm(
        t(
          'Remove installed plugin "{{plugin}}" {{version}}? Configured Providers will block removal.',
          {
            plugin: plugin.displayName,
            version: plugin.version,
          },
        ),
      )
    ) {
      return;
    }

    setBusy(plugin.key);
    setError("");
    try {
      await removeMarketplacePlugin(client, plugin);
      if (selected?.key === plugin.key) {
        setSelected(null);
      }
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  function openPlugin(plugin: MarketplacePlugin) {
    setSelected(plugin);
    setDetailView("overview");
  }

  return (
    <>
      <PageHeader
        actions={
          installers.length ? (
            <div className="flex flex-wrap gap-2">
              {installers.map((installer) => (
                <Button
                  key={installer.key}
                  type="button"
                  disabled={busy !== ""}
                  onClick={() => void installPlugin(installer)}
                >
                  <PackagePlusIcon data-icon="inline-start" />
                  {t(
                    busy === "install:" + installer.key
                      ? installer.pendingLabel
                      : installer.label,
                  )}
                </Button>
              ))}
            </div>
          ) : undefined
        }
      />

      {error ? <ErrorNotice message={error} /> : null}

      <DataPanel
        title="Plugin catalog"
        description="Browse built-in and installed plugin packages available to Mender."
        toolbar={
          <WorkbenchToolbar>
            <SearchField
              value={query}
              onChange={setQuery}
              label="Search plugins"
              placeholder="Search plugins…"
              className="md:min-w-64"
            />
            <FilterSelect
              value={kindFilter}
              onValueChange={setKindFilter}
              label="Plugin type filter"
              options={kindOptions}
            />
            <FilterSelect
              value={sourceFilter}
              onValueChange={setSourceFilter}
              label="Plugin source filter"
              options={sourceOptions}
            />
            <FilterSelect
              value={statusFilter}
              onValueChange={(value) =>
                setStatusFilter(value as StatusFilter)
              }
              label="Plugin status filter"
              options={[
                { value: "all", label: "All plugin statuses" },
                { value: "enabled", label: "Enabled" },
                { value: "disabled", label: "Disabled" },
                { value: "degraded", label: "Degraded" },
                { value: "invalid", label: "Invalid" },
              ]}
            />
          </WorkbenchToolbar>
        }
      >
        {!loaded ? (
          <div className="px-4 py-12 text-center text-sm text-muted-foreground">
            {t("Loading plugins…")}
          </div>
        ) : visiblePlugins.length ? (
          <>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("Plugin")}</TableHead>
                  <TableHead className="hidden md:table-cell">
                    {t("Type")}
                  </TableHead>
                  <TableHead className="hidden md:table-cell">
                    {t("Source")}
                  </TableHead>
                  <TableHead className="hidden md:table-cell">
                    {t("Version")}
                  </TableHead>
                  <TableHead>{t("Status")}</TableHead>
                  <TableHead className="text-right">{t("Actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {pagination.items.map((plugin) => (
                  <TableRow key={plugin.key}>
                    <TableCell className="font-medium">
                      {plugin.displayName}
                    </TableCell>
                    <TableCell className="hidden md:table-cell">
                      {pluginKindLabel(plugin)}
                    </TableCell>
                    <TableCell className="hidden md:table-cell">
                      {pluginSourceLabel(plugin)}
                    </TableCell>
                    <TableCell className="hidden md:table-cell">
                      {plugin.version}
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={
                          plugin.status === "enabled"
                            ? "outline"
                            : "secondary"
                        }
                      >
                        {pluginStatusLabel(plugin)}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
                        <IconButton
                          variant="ghost"
                          label={t("View {{plugin}}", {
                            plugin: plugin.displayName,
                          })}
                          onClick={() => openPlugin(plugin)}
                        >
                          <EyeIcon />
                        </IconButton>

                        {plugin.actions.disable ? (
                          <IconButton
                            variant="outline"
                            label={t("Disable {{plugin}}", {
                              plugin: plugin.displayName,
                            })}
                            disabled={busy !== ""}
                            onClick={() => void setEnabled(plugin, false)}
                          >
                            <PowerOffIcon />
                          </IconButton>
                        ) : plugin.actions.enable ? (
                          <IconButton
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

                        {plugin.actions.remove ? (
                          <IconButton
                            variant="destructive"
                            label={t("Remove {{plugin}}", {
                              plugin: plugin.displayName,
                            })}
                            disabled={busy !== ""}
                            onClick={() => void remove(plugin)}
                          >
                            <Trash2Icon />
                          </IconButton>
                        ) : null}
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            <div className="px-4 pb-3">
              <TablePagination
                label="Plugin Marketplace"
                pagination={pagination}
              />
            </div>
          </>
        ) : (
          <EmptyPanelState>
            <EmptyState
              title="No matching plugins"
              description="Try a different search or filter."
            />
          </EmptyPanelState>
        )}
      </DataPanel>

      <Dialog
        open={selected !== null}
        onOpenChange={(open) => {
          if (!open) {
            setSelected(null);
            setDetailView("overview");
          }
        }}
      >
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {detailView === "overview"
                ? t("Plugin details")
                : t("Technical details")}
            </DialogTitle>
            <DialogDescription>
              {selected === null
                ? t("Plugin package")
                : selected.displayName}
            </DialogDescription>
          </DialogHeader>

          {selected !== null && detailView === "overview" ? (
            <div className="flex flex-col gap-5">
              <div className="rounded-xl border border-border/70 bg-muted/15 p-4">
                <div className="flex flex-wrap items-center gap-2">
                  <p className="text-base font-semibold">
                    {selected.displayName}
                  </p>
                  <Badge variant="outline">
                    {pluginKindLabel(selected)}
                  </Badge>
                  <Badge
                    variant={
                      selected.status === "enabled"
                        ? "outline"
                        : "secondary"
                    }
                  >
                    {pluginStatusLabel(selected)}
                  </Badge>
                </div>
                <p className="mt-2 text-sm leading-5 text-muted-foreground">
                  {selected.description || t("No plugin description.")}
                </p>
              </div>

              <div className="grid gap-3 sm:grid-cols-2">
                <DetailField
                  label="Type"
                  value={pluginKindLabel(selected)}
                />
                <DetailField
                  label="Source"
                  value={pluginSourceLabel(selected)}
                />
                <DetailField label="Version" value={selected.version} />
                <DetailField
                  label="Status"
                  value={pluginStatusLabel(selected)}
                />
              </div>

              <div>
                <p className="mb-2 text-sm font-medium">{t("Capabilities")}</p>
                {selected.capabilities.length ? (
                  <div className="flex flex-wrap gap-2">
                    {selected.capabilities.map((capability) => (
                      <Badge key={capability} variant="outline">
                        {capability}
                      </Badge>
                    ))}
                  </div>
                ) : (
                  <p className="text-sm text-muted-foreground">
                    {t("No declared capabilities.")}
                  </p>
                )}
              </div>

              {selected.issue !== undefined ? (
                <ErrorNotice
                  message={t("Plugin issue: {{message}}", {
                    message: selected.issue.message,
                  })}
                />
              ) : null}

              <Button
                type="button"
                variant="outline"
                onClick={() => setDetailView("technical")}
              >
                {t("View technical details")}
              </Button>
            </div>
          ) : null}

          {selected !== null && detailView === "technical" ? (
            <div className="grid gap-3 sm:grid-cols-2">
              {selected.technicalDetails.map((detail) => (
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
          ) : null}

          <DialogFooter showCloseButton>
            {detailView === "technical" ? (
              <Button
                type="button"
                variant="outline"
                onClick={() => setDetailView("overview")}
              >
                {t("Back")}
              </Button>
            ) : null}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
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
    <div className="rounded-xl border border-border/70 bg-muted/20 p-3">
      <p className="text-xs font-medium text-muted-foreground">{t(label)}</p>
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

function pluginKindLabel(plugin: MarketplacePlugin) {
  return t(plugin.kindLabel);
}

function pluginSourceLabel(plugin: MarketplacePlugin) {
  switch (plugin.source) {
    case "builtin":
      return t("Built-in");
    case "local":
      return t("Local package");
    default:
      return plugin.sourceLabel;
  }
}

function pluginStatusLabel(plugin: MarketplacePlugin) {
  switch (plugin.status) {
    case "enabled":
      return t("Enabled");
    case "disabled":
      return t("Disabled");
    case "invalid":
      return t("Invalid");
    case "degraded":
      return t("Degraded");
  }
}
