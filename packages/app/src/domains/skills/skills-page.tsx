import {
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import {
  BookOpenIcon,
  FileCode2Icon,
  EyeIcon,
  HardDriveIcon,
  PackageOpenIcon,
  PlusIcon,
  PowerIcon,
  PowerOffIcon,
  RefreshCwIcon,
  ShieldCheckIcon,
  ShieldOffIcon,
  Trash2Icon,
  UploadIcon,
} from "lucide-react";

import type {
  MenderClient,
  SkillDeployment,
  SkillInspection,
  SkillInstallation,
} from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import {
  Dialog,
  DialogContent,
  DialogBody,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@mender/ui/components/dialog";

import { formatDateTime, formatNumber, t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { IconButton } from "../../shared/icon-button";
import { EmptyState, ErrorNotice } from "../../shared/page";
import { PageTab, PageTabs, PageToolbar, ResultPanel } from "../../shared/page-toolbar";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import { FilterPopover, FilterSelectField } from "../../shared/filter-popover";
import {
  EmptyPanelState,
  SearchField,
} from "../../shared/workbench";

type DeploySkill = (
  installationId: string,
  target: SkillDeployment["target"],
) => Promise<SkillDeployment | null>;

type SkillFilter = "all" | "builtin" | "installed" | "enabled";
type SkillSourceFilter = "all" | "builtin" | "local";
type SkillStatusFilter = "all" | "enabled" | "disabled";
type SkillSort = "name" | "version";
type SkillView = "overview" | "resources" | "deployments";

const deploymentTargets: Array<{
  value: SkillDeployment["target"];
  label: string;
}> = [
  { value: "agents-project", label: "Agents project" },
  { value: "github-project", label: "GitHub project" },
  { value: "claude-project", label: "Claude project" },
  { value: "agents-user", label: "Agents user" },
  { value: "copilot-user", label: "Copilot user" },
];

export function SkillsPage({
  client,
  installSkill,
  deploySkill,
}: {
  client: MenderClient;
  installSkill?: () => Promise<SkillInstallation | null>;
  deploySkill?: DeploySkill;
}) {
  const [installations, setInstallations] = useState<SkillInstallation[]>([]);
  const [deployments, setDeployments] = useState<SkillDeployment[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [hasSnapshot, setHasSnapshot] = useState(false);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");

  const [selected, setSelected] = useState<SkillInstallation | null>(null);
  const [inspection, setInspection] = useState<SkillInspection | null>(null);
  const [skillView, setSkillView] = useState<SkillView>("overview");
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<SkillFilter>("all");
  const [sourceFilter, setSourceFilter] =
    useState<SkillSourceFilter>("all");
  const [statusFilter, setStatusFilter] =
    useState<SkillStatusFilter>("all");
  const [sort, setSort] = useState<SkillSort>("name");
  const [deploying, setDeploying] = useState<SkillInstallation | null>(null);
  const [deploymentTarget, setDeploymentTarget] =
    useState<SkillDeployment["target"]>("agents-project");

  const refresh = useCallback(async () => {
    setLoaded(false);
    setError("");
    try {
      const [skillResult, deploymentResult] = await Promise.all([
        client.listSkills(),
        client.listSkillDeployments(),
      ]);
      setInstallations(skillResult.installations);
      setDeployments(deploymentResult.deployments);
      setHasSnapshot(true);
      setSelected((current) => {
        if (current === null) {
          return null;
        }
        return (
          skillResult.installations.find(
            (item) => item.installationId === current.installationId,
          ) ?? null
        );
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
    void Promise.all([
      client.listSkills(),
      client.listSkillDeployments(),
    ])
      .then(([skillResult, deploymentResult]) => {
        if (!active) {
          return;
        }
        setInstallations(skillResult.installations);
        setDeployments(deploymentResult.deployments);
        setHasSnapshot(true);
        setError("");
      })
      .catch((reason: unknown) => {
        if (active) {
          setError(errorMessage(reason));
        }
      })
      .finally(() => {
        if (active) {
          setLoaded(true);
        }
      });
    return () => {
      active = false;
    };
  }, [client]);

  const visibleInstallations = useMemo(() => {
    const normalizedQuery = query.trim().toLocaleLowerCase();
    return installations.filter((item) => {
      const matchesFilter =
        filter === "all" ||
        (filter === "builtin" && item.builtIn) ||
        (filter === "installed" && !item.builtIn) ||
        (filter === "enabled" && item.enabled);
      const matchesSource =
        sourceFilter === "all" ||
        (sourceFilter === "builtin" && item.builtIn) ||
        (sourceFilter === "local" && !item.builtIn);
      const matchesStatus =
        statusFilter === "all" ||
        (statusFilter === "enabled" && item.enabled) ||
        (statusFilter === "disabled" && !item.enabled);
      const matchesQuery =
        !normalizedQuery ||
        item.name.toLocaleLowerCase().includes(normalizedQuery) ||
        item.description.toLocaleLowerCase().includes(normalizedQuery);
      return matchesFilter && matchesSource && matchesStatus && matchesQuery;
    }).sort((a, b) => {
      if (a.builtIn !== b.builtIn) {
        return a.builtIn ? -1 : 1;
      }
      if (sort === "version") {
        return (a.versionHint || "").localeCompare(b.versionHint || "");
      }
      return a.name.localeCompare(b.name);
    });
  }, [
    filter,
    installations,
    query,
    sort,
    sourceFilter,
    statusFilter,
  ]);

  useEffect(() => {
    if (selected === null) {
      return;
    }

    let active = true;
    void client
      .getSkillInstallation(selected.installationId)
      .then((item) => {
        if (active) {
          setInspection(item);
          setError("");
        }
      })
      .catch((reason: unknown) => {
        if (active) {
          setInspection(null);
          setError(errorMessage(reason));
        }
      });

    return () => {
      active = false;
    };
  }, [client, selected]);

  const skillPagination = usePagination(visibleInstallations, {
    initialPageSize: 10,
    pageSizeOptions: [10, 25, 50],
  });

  const selectedDeployments =
    selected === null
      ? []
      : deployments.filter(
          (item) => item.installationId === selected.installationId,
        );
  const selectedInspection =
    selected !== null &&
    inspection?.installation.installationId === selected.installationId
      ? inspection
      : null;
  const mutating = busy !== "" && !busy.startsWith("view:");

  async function install() {
    if (installSkill === undefined) {
      return;
    }
    setBusy("install");
    setError("");
    try {
      const installed = await installSkill();
      if (installed !== null) {
        setSelected(installed);
        setInspection(null);
        setSkillView("overview");
        await refresh();
      }
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function toggleTrust(item: SkillInstallation) {
    setBusy("trust:" + item.installationId);
    setError("");
    try {
      if (item.trusted) {
        await client.untrustSkillInstallation(item.installationId);
      } else {
        await client.trustSkillInstallation(item.installationId);
      }
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function toggleEnabled(item: SkillInstallation) {
    setBusy("enabled:" + item.installationId);
    setError("");
    try {
      if (item.enabled) {
        await client.disableSkillInstallation(item.installationId);
      } else {
        await client.enableSkillInstallation(item.installationId);
      }
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function remove(item: SkillInstallation) {
    setBusy("remove:" + item.installationId);
    setError("");
    try {
      await client.deleteSkillInstallation(item.installationId);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function deploy() {
    if (deploying === null || deploySkill === undefined) {
      return;
    }
    setBusy("deploy:" + deploying.installationId);
    setError("");
    try {
      const deployed = await deploySkill(
        deploying.installationId,
        deploymentTarget,
      );
      if (deployed !== null) {
        setDeploying(null);
        await refresh();
      }
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function undeploy(item: SkillDeployment) {
    setBusy("undeploy:" + item.deploymentId);
    setError("");
    try {
      await client.undeploySkill(item.deploymentId);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  function openSkill(item: SkillInstallation) {
    setSelected(item);
    setInspection(null);
    setSkillView("overview");
  }

  function closeSkill() {
    setSelected(null);
    setInspection(null);
    setSkillView("overview");
  }

  return (
    <>
      {hasSnapshot ? <PageToolbar
        tabs={
          <PageTabs label="Skill Library">
            <PageTab label="All Skills" count={installations.length} active={filter === "all"} onClick={() => setFilter("all")} />
            <PageTab label="Built-in" count={installations.filter((item) => item.builtIn).length} active={filter === "builtin"} onClick={() => setFilter("builtin")} />
            <PageTab label="Installed Skills" count={installations.filter((item) => !item.builtIn).length} active={filter === "installed"} onClick={() => setFilter("installed")} />
            <PageTab label="Enabled" count={installations.filter((item) => item.enabled).length} active={filter === "enabled"} onClick={() => setFilter("enabled")} />
          </PageTabs>
        }
        filters={
          <>
            <div className="w-full min-w-0 sm:w-56 md:w-64 sm:shrink-0">
              <SearchField value={query} onChange={setQuery} placeholder="Search Skills…" label="Search Skills" />
            </div>
            <FilterPopover
              activeCount={
                (sourceFilter !== "all" ? 1 : 0) +
                (statusFilter !== "all" ? 1 : 0) +
                (sort !== "name" ? 1 : 0)
              }
              onReset={() => {
                setSourceFilter("all");
                setStatusFilter("all");
                setSort("name");
              }}
            >
              <FilterSelectField
                label="Skill source filter"
                value={sourceFilter}
                onChange={(value) => setSourceFilter(value as SkillSourceFilter)}
                options={[
                  { value: "all", label: "All Sources" },
                  { value: "builtin", label: "Built-in" },
                  { value: "local", label: "Local" },
                ]}
              />
              <FilterSelectField
                label="Skill status filter"
                value={statusFilter}
                onChange={(value) => setStatusFilter(value as SkillStatusFilter)}
                options={[
                  { value: "all", label: "All Status" },
                  { value: "enabled", label: "Enabled" },
                  { value: "disabled", label: "Disabled" },
                ]}
              />
              <FilterSelectField
                label="Skill sort"
                value={sort}
                onChange={(value) => setSort(value as SkillSort)}
                options={[
                  { value: "name", label: "Name" },
                  { value: "version", label: "Version" },
                ]}
              />
            </FilterPopover>
          </>
        }
        actions={
          <>
            <Button type="button" variant="outline" size="sm" onClick={() => void refresh()} disabled={mutating}>
              <RefreshCwIcon data-icon="inline-start" />
              {t("Refresh")}
            </Button>
            {installSkill !== undefined ? (
              <Button type="button" size="sm" onClick={() => void install()} disabled={mutating}>
                <PlusIcon data-icon="inline-start" />
                {busy === "install" ? t("Installing…") : t("Install Skill")}
              </Button>
            ) : null}
          </>
        }
      /> : null}

      {error ? <ErrorNotice message={error} onRetry={!hasSnapshot ? () => { void refresh(); } : undefined} /> : null}

      {error && !hasSnapshot ? null : <ResultPanel title="Skill Library">
        {!loaded ? (
          <p className="px-3 py-10 text-center text-sm text-muted-foreground">
            {t("Loading…")}
          </p>
        ) : visibleInstallations.length === 0 ? (
          <EmptyPanelState>
            <EmptyState
              title="No matching Skills"
              description="Try a different search or filter."
            />
          </EmptyPanelState>
        ) : (
          <div className="p-3">
            <div className="space-y-2">
                  {skillPagination.items.map((item) => (
                      <div
                        key={item.installationId}
                        className="group flex min-h-[88px] min-w-0 flex-col items-stretch gap-3 rounded-xl border border-border/70 bg-card p-3.5 transition-[border-color,box-shadow] hover:border-border hover:shadow-xs lg:flex-row lg:items-center dark:hover:border-border/80"
                      >
                        <button
                          type="button"
                          onClick={() => openSkill(item)}
                          className="flex w-full min-w-0 flex-1 items-start gap-3 rounded-lg text-left outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-card lg:items-center"
                        >
                          <span
                            className={[
                              "flex size-11 shrink-0 items-center justify-center rounded-xl",
                              item.builtIn
                                ? "bg-foreground text-background dark:bg-muted dark:text-foreground"
                                : "bg-muted text-muted-foreground",
                            ].join(" ")}
                          >
                            {item.builtIn ? (
                              <BookOpenIcon className="size-5" />
                            ) : (
                              <FileCode2Icon className="size-5" />
                            )}
                          </span>

                          <span className="min-w-0 flex-1">
                            <span className="flex min-w-0 flex-wrap items-center gap-1.5">
                              <span className="truncate text-sm font-semibold tracking-tight text-foreground">
                                {item.name}
                              </span>
                              <Badge
                                variant={item.builtIn ? "default" : "outline"}
                                className="shrink-0"
                              >
                                {item.builtIn ? t("Built-in") : t("Local")}
                              </Badge>
                              <span className="sm:hidden">
                                <Badge
                                  variant={item.enabled ? "default" : "secondary"}
                                  className="shrink-0"
                                >
                                  {item.enabled ? t("Enabled") : t("Disabled")}
                                </Badge>
                              </span>
                            </span>
                            <span className="mt-1 block max-h-[3.75rem] max-w-2xl overflow-hidden text-sm leading-5 text-muted-foreground" title={item.description}>
                              {item.description}
                            </span>
                          </span>

                          <span className="hidden w-28 shrink-0 text-right sm:block">
                            <span className="block font-mono text-xs text-muted-foreground">
                              {item.versionHint || t("Unversioned")}
                            </span>
                            <Badge
                              variant={item.enabled ? "default" : "secondary"}
                              className="mt-1 shrink-0"
                            >
                              {item.enabled ? t("Enabled") : t("Disabled")}
                            </Badge>
                          </span>
                        </button>

                        <div className="flex w-full shrink-0 items-center justify-end gap-1 border-t border-border/65 pt-2 lg:w-auto lg:border-t-0 lg:pt-0">
                          <IconButton
                            type="button"
                            variant="ghost"
                            size="icon-lg"
                            label={t("View {{skill}}", {
                              skill: item.name,
                            })}
                            disabled={mutating}
                            onClick={() => openSkill(item)}
                          >
                            <EyeIcon />
                          </IconButton>
                          {deploySkill !== undefined && item.enabled ? (
                            <IconButton
                              type="button"
                              variant="ghost"
                              size="icon-lg"
                              label={t("Deploy {{skill}}", {
                                skill: item.name,
                              })}
                              disabled={mutating}
                              onClick={() => setDeploying(item)}
                            >
                              <UploadIcon />
                            </IconButton>
                          ) : null}
                          {!item.builtIn ? (
                            <>
                              <IconButton
                                type="button"
                                variant="ghost"
                                size="icon-lg"
                                label={
                                  item.trusted
                                    ? t("Untrust {{skill}}", {
                                        skill: item.name,
                                      })
                                    : t("Trust {{skill}}", {
                                        skill: item.name,
                                      })
                                }
                                disabled={mutating}
                                onClick={() => void toggleTrust(item)}
                              >
                                {item.trusted ? (
                                  <ShieldOffIcon />
                                ) : (
                                  <ShieldCheckIcon />
                                )}
                              </IconButton>
                              <IconButton
                                type="button"
                                variant="ghost"
                                size="icon-lg"
                                label={
                                  item.enabled
                                    ? t("Disable {{skill}}", {
                                        skill: item.name,
                                      })
                                    : t("Enable {{skill}}", {
                                        skill: item.name,
                                      })
                                }
                                disabled={mutating || !item.trusted}
                                onClick={() => void toggleEnabled(item)}
                              >
                                {item.enabled ? (
                                  <PowerOffIcon />
                                ) : (
                                  <PowerIcon />
                                )}
                              </IconButton>
                              <IconButton
                                type="button"
                                variant="ghost"
                                size="icon-lg"
                                label={t("Delete {{skill}}", {
                                  skill: item.name,
                                })}
                                disabled={mutating}
                                onClick={() => void remove(item)}
                              >
                                <Trash2Icon />
                              </IconButton>
                            </>
                          ) : null}
                        </div>
                      </div>
                  ))}
                </div>
              </div>
            )}
      </ResultPanel>}

      {hasSnapshot && visibleInstallations.length ? (
        <TablePagination label="Skills" pagination={skillPagination} />
      ) : null}

      <Dialog
        open={selected !== null}
        onOpenChange={(open) => {
          if (!open) {
            closeSkill();
          }
        }}
      >
        <DialogContent className="grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {skillView === "overview"
                ? t("Skill details")
                : skillView === "resources"
                  ? t("Skill resources")
                  : t("Skill deployments")}
            </DialogTitle>
            <DialogDescription>
              {selected === null
                ? t("Exact Skill installation")
                : t("Exact installation {{installation}} of {{skill}}.", {
                    installation: selected.installationId,
                    skill: selected.name,
                  })}
            </DialogDescription>
          </DialogHeader>

          <DialogBody>
          {selected !== null && skillView === "overview" ? (
            <div className="space-y-5">
              <div className="flex items-start gap-3 rounded-xl border border-border/70 bg-muted/20 p-4">
                <span className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-foreground text-background dark:bg-muted dark:text-foreground">
                  {selected.builtIn ? (
                    <BookOpenIcon className="size-5" />
                  ) : (
                    <FileCode2Icon className="size-5" />
                  )}
                </span>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <p className="text-base font-semibold tracking-tight text-foreground">
                      {selected.name}
                    </p>
                    <Badge variant={selected.builtIn ? "default" : "outline"}>
                      {selected.builtIn ? t("Built-in") : t("Local")}
                    </Badge>
                    <Badge variant={selected.enabled ? "default" : "secondary"}>
                      {selected.enabled ? t("Enabled") : t("Disabled")}
                    </Badge>
                  </div>
                  <p className="mt-1 text-sm leading-5 text-muted-foreground">
                    {selected.description}
                  </p>
                </div>
              </div>

              <div className="grid gap-3 sm:grid-cols-2">
                <SkillMeta
                  label="Installation"
                  value={selected.installationId}
                  mono
                />
                <SkillMeta label="Skill ID" value={selected.skillId} mono />
                <SkillMeta
                  label="Version"
                  value={selected.versionHint || t("Unversioned")}
                />
                <SkillMeta
                  label="Source"
                  value={selected.builtIn ? t("Built-in") : t("Local")}
                />
                <SkillMeta
                  label="Digest"
                  value={shortDigest(selected.digest)}
                  mono
                />
                <SkillMeta
                  label="Installed"
                  value={
                    selected.installedAt
                      ? formatDateTime(selected.installedAt)
                      : t("Built-in")
                  }
                />
                <SkillMeta
                  label="Resources"
                  value={
                    selectedInspection === null
                      ? t("Loading…")
                      : formatNumber(selectedInspection.resources.length)
                  }
                />
                <SkillMeta
                  label="Size"
                  value={
                    selectedInspection === null
                      ? t("Loading…")
                      : t("{{count}} bytes", {
                          count: formatNumber(selectedInspection.totalBytes),
                        })
                  }
                />
              </div>

              {selectedInspection?.metadata.allowedTools ? (
                <div className="rounded-xl border border-border/70 p-4">
                  <p className="text-sm font-medium">
                    {t("Allowed tools metadata")}
                  </p>
                  <p className="mt-2 truncate font-mono text-xs text-muted-foreground">
                    {selectedInspection.metadata.allowedTools}
                  </p>
                  <p className="mt-2 text-xs leading-5 text-muted-foreground">
                    {t(
                      "This metadata is informational only. Mender Policy and Profile governance remain authoritative.",
                    )}
                  </p>
                </div>
              ) : null}

              <div className="grid gap-3 sm:grid-cols-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setSkillView("resources")}
                >
                  <PackageOpenIcon data-icon="inline-start" />
                  {t("View resources")}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setSkillView("deployments")}
                >
                  <HardDriveIcon data-icon="inline-start" />
                  {t("View deployments")}
                </Button>
              </div>
            </div>
          ) : null}

          {selected !== null && skillView === "resources" ? (
            <div className="max-h-[58vh] overflow-y-auto rounded-xl border border-border/70">
              {selectedInspection === null ? (
                <p className="px-4 py-10 text-center text-sm text-muted-foreground">
                  {t("Loading…")}
                </p>
              ) : selectedInspection.resources.length ? (
                <div className="divide-y divide-border/60">
                  {selectedInspection.resources.map((resource) => (
                    <div
                      key={resource.path}
                      className="flex items-start gap-3 px-4 py-3"
                    >
                      <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
                        <FileCode2Icon className="size-4" />
                      </span>
                      <div className="min-w-0 flex-1">
                        <p className="break-all font-mono text-xs">
                          {resource.path}
                        </p>
                        <p className="mt-1 text-xs text-muted-foreground">
                          {resourceKindLabel(resource.kind)} ·{" "}
                          {t("{{count}} bytes", {
                            count: formatNumber(resource.size),
                          })}
                        </p>
                      </div>
                      {resource.executable ? (
                        <Badge
                          variant="outline"
                          className="shrink-0"
                        >
                          {t("Executable")}
                        </Badge>
                      ) : null}
                    </div>
                  ))}
                </div>
              ) : (
                <p className="px-4 py-10 text-center text-sm text-muted-foreground">
                  {t("No resources in this installation.")}
                </p>
              )}
            </div>
          ) : null}

          {selected !== null && skillView === "deployments" ? (
            <div className="space-y-3">
              <div className="max-h-[52vh] overflow-y-auto space-y-2">
                {selectedDeployments.length ? (
                  selectedDeployments.map((item) => (
                    <div
                      key={item.deploymentId}
                      className="flex items-start gap-3 rounded-xl border border-border/70 p-4"
                    >
                      <HardDriveIcon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <p className="text-sm font-medium">
                            {deploymentTargetLabel(item.target)}
                          </p>
                          <Badge
                            variant={
                              item.drift === "current"
                                ? "default"
                                : item.drift === "missing"
                                  ? "destructive"
                                  : "secondary"
                            }
                          >
                            {deploymentDriftLabel(item.drift)}
                          </Badge>
                        </div>
                        <p
                          className="mt-1 break-all font-mono text-xs text-muted-foreground"
                          title={item.destination}
                        >
                          {item.destination}
                        </p>
                        <p className="mt-2 text-xs text-muted-foreground">
                          {formatDateTime(item.deployedAt)}
                        </p>
                      </div>
                      <IconButton
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        label={t("Undeploy {{skill}}", {
                          skill: selected.name,
                        })}
                        disabled={mutating}
                        onClick={() => void undeploy(item)}
                      >
                        <Trash2Icon />
                      </IconButton>
                    </div>
                  ))
                ) : (
                  <div className="rounded-xl border border-dashed border-border/80 p-8 text-center">
                    <HardDriveIcon className="mx-auto size-5 text-muted-foreground" />
                    <p className="mt-2 text-sm font-medium">
                      {t("No deployments")}
                    </p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {t("No deployments for this exact installation.")}
                    </p>
                  </div>
                )}
              </div>

              {deploySkill !== undefined && selected.enabled ? (
                <Button
                  type="button"
                  className="w-full"
                  onClick={() => {
                    setDeploying(selected);
                    closeSkill();
                  }}
                  disabled={mutating}
                >
                  <UploadIcon data-icon="inline-start" />
                  {t("Deploy Skill")}
                </Button>
              ) : null}
            </div>
          ) : null}

          </DialogBody>
          <DialogFooter showCloseButton>
            {skillView !== "overview" ? (
              <Button
                type="button"
                variant="outline"
                onClick={() => setSkillView("overview")}
              >
                {t("Back")}
              </Button>
            ) : null}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={deploying !== null}
        onOpenChange={(open) => {
          if (!open) {
            setDeploying(null);
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("Deploy Skill")}</DialogTitle>
            <DialogDescription>
              {deploying === null
                ? ""
                : t("Deploy exact installation {{installation}} of {{skill}}.", {
                    installation: deploying.installationId,
                    skill: deploying.name,
                  })}
            </DialogDescription>
          </DialogHeader>

          <label className="grid gap-2">
            <span className="text-sm font-medium">{t("Target")}</span>
            <select
              aria-label={t("Skill deployment target")}
              value={deploymentTarget}
              onChange={(event) =>
                setDeploymentTarget(
                  event.target.value as SkillDeployment["target"],
                )
              }
              className="h-10 rounded-xl border border-input bg-background px-3 text-sm text-foreground outline-none focus:border-ring focus:ring-2 focus:ring-ring/20"
            >
              {deploymentTargets.map((item) => (
                <option key={item.value} value={item.value}>
                  {t(item.label)}
                </option>
              ))}
            </select>
          </label>

          <DialogFooter showCloseButton>
            <Button
              type="button"
              onClick={() => void deploy()}
              disabled={
                deploying === null ||
                busy === "deploy:" + deploying.installationId
              }
            >
              {deploying !== null &&
              busy === "deploy:" + deploying.installationId
                ? t("Deploying…")
                : t("Deploy")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function SkillMeta({
  label,
  value,
  mono = false,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div className="min-w-0 rounded-xl border border-border/70 bg-muted/20 p-3">
      <p className="text-xs font-medium text-muted-foreground">{t(label)}</p>
      <p
        className={[
          "mt-1 break-all text-sm text-foreground",
          mono ? "font-mono text-xs" : "",
        ].join(" ")}
      >
        {value}
      </p>
    </div>
  );
}

function shortDigest(value: string) {
  if (value.length <= 20) {
    return value;
  }
  return value.slice(0, 14) + "…" + value.slice(-6);
}

function deploymentTargetLabel(target: SkillDeployment["target"]) {
  switch (target) {
    case "agents-project":
      return t("Agents project");
    case "github-project":
      return t("GitHub project");
    case "claude-project":
      return t("Claude project");
    case "agents-user":
      return t("Agents user");
    case "copilot-user":
      return t("Copilot user");
  }
}

function deploymentDriftLabel(drift: SkillDeployment["drift"]) {
  switch (drift) {
    case "current":
      return t("Current");
    case "drifted":
      return t("Drifted");
    case "missing":
      return t("Missing");
  }
}

function resourceKindLabel(
  kind: SkillInspection["resources"][number]["kind"],
) {
  switch (kind) {
    case "manifest":
      return t("Manifest");
    case "script":
      return t("Script");
    case "reference":
      return t("Reference");
    case "asset":
      return t("Asset");
    case "other":
      return t("Other");
  }
}
