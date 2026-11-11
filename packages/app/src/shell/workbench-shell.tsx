import { useCallback, useEffect, useState } from "react";
import {
  BellIcon,
  CheckCircle2Icon,
  ChevronRightIcon,
  CommandIcon,
  LanguagesIcon,
  MinusIcon,
  RefreshCwIcon,
  SearchIcon,
  Settings2Icon,
  SquareIcon,
  TerminalIcon,
  Trash2Icon,
  XIcon,
} from "lucide-react";
import { Link, NavLink, Outlet, useLocation, useNavigate } from "react-router";

import type { AgentApprovalCapability, ManagementClient, PendingAgentApproval } from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@mender/ui/components/dialog";
import { Popover, PopoverContent, PopoverTrigger } from "@mender/ui/components/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@mender/ui/components/tooltip";

import { BrandMark } from "../brand/brand-mark";
import { setLocale, t, useLocale } from "../i18n/i18n";
import { useDesktopHost, type DesktopWindowControls } from "./desktop-host-context";
import {
  APPEARANCE_STORAGE_KEY,
  flatNavigation,
  readAppearance,
  secondaryNavigation,
  workbenchNavigation,
} from "./navigation";
import type { Appearance } from "./navigation";
import { PageHeadingContext, getPageTrail } from "./page-chrome";

interface ConsoleLogItem {
  id: string;
  time: string;
  level: "info" | "success" | "warn";
  message: string;
}

export function WorkbenchShell({
  client,
  approvals,
  hasLocalOwner,
  windowControls,
}: {
  client: ManagementClient;
  approvals?: AgentApprovalCapability | undefined;
  hasLocalOwner?: boolean;
  windowControls?: DesktopWindowControls | undefined;
}) {
  const host = useDesktopHost();
  const controls = windowControls ?? host.windowControls;
  const locale = useLocale();
  const location = useLocation();
  const navigate = useNavigate();
  const [pageHeading, setPageHeading] = useState<{ path: string; value: string } | null>(null);
  const [runtimeReady, setRuntimeReady] = useState<boolean | null>(null);
  const [commandOpen, setCommandOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [pendingApprovalCount, setPendingApprovalCount] = useState(0);
  const [pendingApprovalsList, setPendingApprovalsList] = useState<PendingAgentApproval[]>([]);
  const [appearance, setAppearance] = useState<Appearance>(readAppearance);

  // Console Drawer state
  const [consoleOpen, setConsoleOpen] = useState(false);
  const [consoleTab, setConsoleTab] = useState<"logs" | "approvals">("logs");
  const [consoleLogs, setConsoleLogs] = useState<ConsoleLogItem[]>([
    {
      id: "init",
      time: new Date().toLocaleTimeString(),
      level: "info",
      message: t("Workbench initialized"),
    },
  ]);

  const addLog = useCallback((message: string, level: "info" | "success" | "warn" = "info") => {
    setConsoleLogs((prev) => [
      {
        id: String(Date.now()) + Math.random().toString(36).slice(2, 6),
        time: new Date().toLocaleTimeString(),
        level,
        message,
      },
      ...prev.slice(0, 99),
    ]);
  }, []);

  const updatePageHeading = useCallback((path: string, value?: string) => {
    setPageHeading((current) => (value ? { path, value } : current?.path === path ? null : current));
  }, []);

  useEffect(() => {
    document.documentElement.dataset.platform = "desktop";
    return () => {
      delete document.documentElement.dataset.platform;
    };
  }, []);

  useEffect(() => {
    window.localStorage.setItem(APPEARANCE_STORAGE_KEY, appearance);
    if (typeof window.matchMedia !== "function") {
      document.documentElement.classList.toggle("dark", appearance === "dark");
      return;
    }
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const apply = () =>
      document.documentElement.classList.toggle(
        "dark",
        appearance === "dark" || (appearance === "system" && media.matches),
      );
    apply();
    if (appearance === "system") {
      media.addEventListener("change", apply);
      return () => media.removeEventListener("change", apply);
    }
    return undefined;
  }, [appearance]);

  useEffect(() => {
    let active = true;
    void Promise.resolve()
      .then(() => client.getSystemStatus())
      .then((status) => {
        if (active) {
          setRuntimeReady(status.ready);
          addLog(status.ready ? t("Runtime: Connected") : t("Runtime: Unavailable"), status.ready ? "success" : "warn");
        }
      })
      .catch(() => {
        if (active) {
          setRuntimeReady(false);
          addLog(t("Runtime: Unavailable"), "warn");
        }
      });
    return () => {
      active = false;
    };
  }, [client, addLog]);

  useEffect(() => {
    if (!approvals) return;
    let active = true;
    const refresh = () => {
      void approvals
        .listPendingAgentApprovals()
        .then(({ approvals: pending }) => {
          if (active) {
            setPendingApprovalCount(pending.length);
            setPendingApprovalsList(pending);
          }
        })
        .catch(() => {
          if (active) {
            setPendingApprovalCount(0);
            setPendingApprovalsList([]);
          }
        });
    };
    refresh();
    const timer = setInterval(refresh, 2500);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [approvals]);

  // Keyboard shortcut listener:
  // Cmd/Ctrl+K: Command Palette
  // Cmd/Ctrl+`: Console Drawer
  // Cmd/Ctrl+Shift+P: Quick open Tool Playground
  // Esc: Close Command Palette
  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      const isCmdOrCtrl = event.metaKey || event.ctrlKey;
      if (isCmdOrCtrl && event.shiftKey && event.key.toLowerCase() === "p") {
        event.preventDefault();
        void navigate("/playground");
        return;
      }
      if (isCmdOrCtrl && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setCommandOpen((open) => !open);
        return;
      }
      if (isCmdOrCtrl && event.key === "`") {
        event.preventDefault();
        setConsoleOpen((open) => !open);
        return;
      }
      if (event.key === "Escape") {
        setCommandOpen(false);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [navigate]);

  const activeItem =
    flatNavigation.find((item) =>
      item.to === "/"
        ? location.pathname === "/"
        : location.pathname === item.to || location.pathname.startsWith(item.to + "/"),
    ) ?? workbenchNavigation[0]!;
  const trail = getPageTrail(location.pathname, activeItem);
  const currentHeading = pageHeading?.path === location.pathname ? pageHeading.value : undefined;
  if (currentHeading && trail.length > 1) {
    trail[1] = { ...trail[1]!, label: currentHeading };
  }
  const visibleNavigation = flatNavigation.filter(
    (item) => item.to !== "/approvals" || approvals !== undefined,
  );
  const needle = query.trim().toLocaleLowerCase();
  const results = needle
    ? visibleNavigation.filter((item) => t(item.label).toLocaleLowerCase().includes(needle))
    : visibleNavigation;

  function goTo(to: string) {
    void navigate(to);
    setCommandOpen(false);
    setSettingsOpen(false);
    setQuery("");
  }

  const handleResolveApproval = async (approvalId: string, choice: "allow_once" | "reject") => {
    if (!approvals) return;
    try {
      await approvals.decideAgentApproval(approvalId, choice);
      addLog(t("Approval resolved: {{decision}}", { decision: choice }), "info");
      const { approvals: pending } = await approvals.listPendingAgentApprovals();
      setPendingApprovalCount(pending.length);
      setPendingApprovalsList(pending);
    } catch {
      addLog(t("Approval resolution failed"), "warn");
    }
  };

  return (
    <div
      data-slot="workbench-layout"
      data-layout="workbench"
      className="flex h-screen w-screen min-h-0 min-w-0 flex-col overflow-hidden bg-background text-foreground select-none"
    >
      {/* Desktop Custom Titlebar */}
      <header
        data-slot="workbench-titlebar"
        className="app-drag-region flex h-[38px] shrink-0 items-center justify-between border-b border-border/75 bg-card/75 px-3 text-xs backdrop-blur"
      >
        <div className="app-no-drag flex items-center gap-2">
          <Link
            to="/"
            className="inline-flex items-center gap-2 rounded text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring/40"
          >
            <BrandMark className="size-4 shrink-0" aria-hidden="true" />
            <span className="brand-wordmark text-[13px] font-semibold tracking-tight">Mender</span>
          </Link>
          <span className="rounded bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium tracking-wide text-primary">
            {t("Workbench")}
          </span>
        </div>

        {/* Center: Command Palette Trigger */}
        <div className="app-no-drag flex items-center">
          <button
            type="button"
            onClick={() => setCommandOpen(true)}
            className="flex h-7 w-[260px] sm:w-[320px] items-center justify-between rounded-md border border-border/80 bg-background/80 px-2.5 text-xs text-muted-foreground shadow-2xs transition-colors hover:border-border hover:bg-background hover:text-foreground focus-visible:outline-2 focus-visible:outline-ring"
            aria-label={t("Search pages…")}
          >
            <span className="flex items-center gap-1.5 truncate">
              <SearchIcon className="size-3.5 shrink-0" aria-hidden="true" />
              <span className="truncate">{t("Search tools, sandboxes, agents…")}</span>
            </span>
            <kbd className="rounded border border-border bg-muted px-1.5 py-0.5 text-[10px] font-mono text-muted-foreground">
              {t("⌘K")}
            </kbd>
          </button>
        </div>

        {/* Right Action Icons */}
        <div className="app-no-drag flex items-center gap-1">
          {approvals !== undefined ? (
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    type="button"
                    size="icon-sm"
                    variant="ghost"
                    className="relative size-7 text-muted-foreground hover:text-foreground"
                    aria-label={t("Pending approvals")}
                    onClick={() => {
                      setConsoleOpen(true);
                      setConsoleTab("approvals");
                    }}
                  />
                }
              >
                <BellIcon className="size-3.5" aria-hidden="true" />
                {pendingApprovalCount > 0 ? (
                  <span
                    aria-label={t("Pending approvals") + ": " + pendingApprovalCount}
                    className="absolute -right-0.5 -top-0.5 min-w-3.5 rounded-full bg-destructive px-1 text-center text-[9px] leading-3.5 text-white"
                  >
                    {pendingApprovalCount > 99 ? "99+" : pendingApprovalCount}
                  </span>
                ) : null}
              </TooltipTrigger>
              <TooltipContent side="bottom">
                {t("Pending approvals")} ({pendingApprovalCount})
              </TooltipContent>
            </Tooltip>
          ) : null}

          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  className="size-7 text-muted-foreground hover:text-foreground"
                  aria-label={t("Refresh")}
                  onClick={() => {
                    void navigate(0);
                  }}
                />
              }
            >
              <RefreshCwIcon className="size-3.5" aria-hidden="true" />
            </TooltipTrigger>
            <TooltipContent side="bottom">{t("Refresh")}</TooltipContent>
          </Tooltip>

          <Popover open={settingsOpen} onOpenChange={setSettingsOpen}>
            <PopoverTrigger
              render={
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  className="size-7 text-muted-foreground hover:text-foreground"
                  aria-label={t("Settings")}
                />
              }
            >
              <Settings2Icon className="size-3.5" aria-hidden="true" />
            </PopoverTrigger>
            <PopoverContent
              side="bottom"
              align="end"
              sideOffset={6}
              className="w-[240px] rounded-lg border-border bg-popover p-2 shadow-lg"
            >
              <p className="px-2 py-1 text-[11px] font-semibold text-foreground">{t("Settings")}</p>
              {secondaryNavigation.map((group) => (
                <div key={group.label} className="border-t border-border/75 py-1">
                  <p className="px-2 py-1 text-[10px] font-semibold tracking-wider text-muted-foreground uppercase">
                    {t(group.label)}
                  </p>
                  {group.items
                    .filter((item) => item.to !== "/approvals" || approvals !== undefined)
                    .map((item) => {
                      const Icon = item.icon;
                      return (
                        <button
                          key={item.to}
                          type="button"
                          onClick={() => goTo(item.to)}
                          className="flex h-8 w-full items-center gap-2 rounded-md px-2 text-left text-xs text-foreground hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring"
                        >
                          <Icon className="size-3.5 text-muted-foreground" aria-hidden="true" />
                          <span className="flex-1">{t(item.label)}</span>
                          {item.to === "/approvals" && pendingApprovalCount > 0 ? (
                            <span className="rounded bg-destructive/10 px-1 text-[10px] text-destructive">
                              {pendingApprovalCount}
                            </span>
                          ) : null}
                        </button>
                      );
                    })}
                </div>
              ))}
              <div className="border-t border-border/75 px-2 pt-1.5 pb-0.5">
                <p className="mb-1.5 text-[10px] font-medium text-muted-foreground">{t("Appearance")}</p>
                <div role="group" aria-label={t("Appearance")} className="flex gap-1 rounded bg-secondary p-0.5">
                  {(["light", "dark", "system"] as const).map((option) => (
                    <button
                      key={option}
                      type="button"
                      aria-pressed={appearance === option}
                      onClick={() => setAppearance(option)}
                      className={[
                        "h-6 flex-1 rounded px-1 text-[10px] focus-visible:outline-2 focus-visible:outline-ring",
                        appearance === option
                          ? "bg-card font-semibold text-foreground shadow-2xs"
                          : "text-muted-foreground hover:text-foreground",
                      ].join(" ")}
                    >
                      {t(option === "light" ? "Light" : option === "dark" ? "Dark" : "System")}
                    </button>
                  ))}
                </div>
                <button
                  type="button"
                  onClick={() => {
                    setLocale(locale === "en" ? "zh-CN" : "en");
                  }}
                  className="mt-1.5 flex h-7 w-full items-center gap-2 rounded px-1 text-left text-[11px] text-muted-foreground hover:bg-muted hover:text-foreground"
                >
                  <LanguagesIcon className="size-3.5" aria-hidden="true" />
                  {t("Language")}: {locale === "en" ? t("Simplified Chinese") : t("English")}
                </button>
              </div>
            </PopoverContent>
          </Popover>

          {controls ? (
            <div className="ml-1.5 flex items-center border-l border-border/75 pl-1.5 gap-0.5">
              {controls.minimise ? (
                <button
                  type="button"
                  aria-label={t("Minimise")}
                  title={t("Minimise")}
                  onClick={() => void controls.minimise?.()}
                  className="inline-flex size-6 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                >
                  <MinusIcon className="size-3" aria-hidden="true" />
                </button>
              ) : null}
              {controls.toggleMaximise ? (
                <button
                  type="button"
                  aria-label={t("Toggle maximise")}
                  title={t("Toggle maximise")}
                  onClick={() => void controls.toggleMaximise?.()}
                  className="inline-flex size-6 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                >
                  <SquareIcon className="size-2.5" aria-hidden="true" />
                </button>
              ) : null}
              {controls.close ? (
                <button
                  type="button"
                  aria-label={t("Close window")}
                  title={t("Close window")}
                  onClick={() => void controls.close?.()}
                  className="inline-flex size-6 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-destructive hover:text-white"
                >
                  <XIcon className="size-3" aria-hidden="true" />
                </button>
              ) : null}
            </div>
          ) : null}
        </div>
        <h1 className="sr-only">{t(trail[trail.length - 1]!.label)}</h1>
      </header>

      {/* Main Body: Activity Bar + Canvas + Drawer */}
      <div className="flex flex-1 min-h-0 min-w-0 overflow-hidden">
        {/* Left Activity Bar Rail */}
        <nav
          aria-label={t("Primary navigation")}
          data-slot="workbench-activity-bar"
          className="flex w-[48px] shrink-0 flex-col items-center border-r border-border/75 bg-sidebar py-2"
        >
          <div className="flex flex-col items-center gap-1.5">
            {workbenchNavigation.map((item) => {
              const Icon = item.icon;
              const isExact = item.to === "/";
              const isActive = isExact
                ? location.pathname === "/"
                : location.pathname === item.to || location.pathname.startsWith(item.to + "/");
              return (
                <Tooltip key={item.to}>
                  <TooltipTrigger
                    render={
                      <NavLink
                        to={item.to}
                        end={isExact}
                        aria-label={t(item.label)}
                        className={[
                          "relative flex size-9 items-center justify-center rounded-lg transition-colors focus-visible:outline-2 focus-visible:outline-ring",
                          isActive
                            ? "bg-primary/10 text-primary font-medium"
                            : "text-muted-foreground hover:bg-muted hover:text-foreground",
                        ].join(" ")}
                      />
                    }
                  >
                    <Icon className="size-4" aria-hidden="true" />
                    {isActive ? (
                      <span
                        className="absolute left-0 top-1.5 bottom-1.5 w-[3px] rounded-r bg-primary"
                        aria-hidden="true"
                      />
                    ) : null}
                  </TooltipTrigger>
                  <TooltipContent side="right" sideOffset={8}>
                    {t(item.label)}
                  </TooltipContent>
                </Tooltip>
              );
            })}
          </div>

          {approvals !== undefined ? (
            <div className="mt-auto flex flex-col items-center gap-1.5 pt-2">
              <Tooltip>
                <TooltipTrigger
                  render={
                    <button
                      type="button"
                      onClick={() => {
                        setConsoleOpen(true);
                        setConsoleTab("approvals");
                      }}
                      aria-label={t("Approvals")}
                      className="relative flex size-9 items-center justify-center rounded-lg text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:outline-2 focus-visible:outline-ring"
                    />
                  }
                >
                  <BellIcon className="size-4" aria-hidden="true" />
                  {pendingApprovalCount > 0 ? (
                    <span className="absolute top-1.5 right-1.5 size-2 rounded-full bg-destructive" />
                  ) : null}
                </TooltipTrigger>
                <TooltipContent side="right" sideOffset={8}>
                  {t("Approvals")}{pendingApprovalCount > 0 ? ` (${pendingApprovalCount})` : ""}
                </TooltipContent>
              </Tooltip>
            </div>
          ) : null}
        </nav>

        {/* Viewport: Breadcrumbs + Main Content + Collapsible Bottom Drawer */}
        <div
          data-slot="workbench-viewport"
          className="flex flex-1 min-h-0 min-w-0 flex-col overflow-hidden bg-background"
        >
          {/* Subheader Breadcrumb */}
          <div className="flex h-8 shrink-0 items-center justify-between border-b border-border/60 bg-muted/25 px-4 text-xs">
            <nav
              aria-label={t("Breadcrumb")}
              data-slot="page-breadcrumb"
              className="flex min-w-0 items-center gap-1 text-[11px] text-muted-foreground"
            >
              {trail.slice(0, -1).map((crumb) => (
                <span key={crumb.to} className="inline-flex min-w-0 items-center gap-1">
                  <Link to={crumb.to} className="max-w-44 truncate hover:text-foreground">
                    {t(crumb.label)}
                  </Link>
                  <ChevronRightIcon className="size-3 text-muted-foreground/50" aria-hidden="true" />
                </span>
              ))}
              <span className="truncate font-medium text-foreground">
                {t(trail[trail.length - 1]!.label)}
              </span>
            </nav>

            <div className="flex items-center gap-2 text-[11px] text-muted-foreground">
              <span className="rounded bg-muted px-1.5 py-0.5 font-mono text-[10px]">
                {location.pathname}
              </span>
            </div>
          </div>

          {/* Main Scrollable Canvas */}
          <main
            data-slot="app-workspace"
            className="flex-1 min-h-0 min-w-0 overflow-y-auto p-4 sm:p-5 [scrollbar-width:thin]"
          >
            <PageHeadingContext.Provider value={updatePageHeading}>
              <Outlet />
            </PageHeadingContext.Provider>
          </main>

          {/* Collapsible Console Drawer */}
          {consoleOpen ? (
            <div
              data-slot="workbench-console"
              className="flex h-[210px] shrink-0 flex-col border-t border-border/80 bg-card/95 backdrop-blur overflow-hidden"
            >
              {/* Drawer Tab Header */}
              <div className="flex h-7 shrink-0 items-center justify-between border-b border-border/60 bg-muted/40 px-3 text-xs">
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={() => setConsoleTab("logs")}
                    className={[
                      "flex items-center gap-1.5 rounded px-2 py-0.5 text-xs font-medium transition-colors",
                      consoleTab === "logs"
                        ? "bg-background text-foreground shadow-2xs font-semibold"
                        : "text-muted-foreground hover:text-foreground",
                    ].join(" ")}
                  >
                    <TerminalIcon className="size-3" aria-hidden="true" />
                    <span>{t("Console Logs")}</span>
                  </button>

                  {approvals !== undefined ? (
                    <button
                      type="button"
                      onClick={() => setConsoleTab("approvals")}
                      className={[
                        "flex items-center gap-1.5 rounded px-2 py-0.5 text-xs font-medium transition-colors",
                        consoleTab === "approvals"
                          ? "bg-background text-foreground shadow-2xs font-semibold"
                          : "text-muted-foreground hover:text-foreground",
                      ].join(" ")}
                    >
                      <BellIcon className="size-3" aria-hidden="true" />
                      <span>{t("Pending Approvals")}</span>
                      {pendingApprovalCount > 0 ? (
                        <span className="rounded-full bg-destructive px-1 text-[9px] text-white">
                          {pendingApprovalCount}
                        </span>
                      ) : null}
                    </button>
                  ) : null}
                </div>

                <div className="flex items-center gap-1">
                  {consoleTab === "logs" ? (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => setConsoleLogs([])}
                      className="h-5 text-[10px] gap-1 px-1.5 text-muted-foreground hover:text-foreground"
                    >
                      <Trash2Icon className="size-3" />
                      <span>{t("Clear")}</span>
                    </Button>
                  ) : null}
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    onClick={() => setConsoleOpen(false)}
                    aria-label={t("Close console")}
                    className="size-5 text-muted-foreground hover:text-foreground"
                  >
                    <XIcon className="size-3.5" />
                  </Button>
                </div>
              </div>

              {/* Drawer Content */}
              <div className="flex-1 min-h-0 overflow-y-auto p-2 font-mono text-xs [scrollbar-width:thin]">
                {consoleTab === "logs" ? (
                  <div className="space-y-1">
                    {consoleLogs.map((log) => (
                      <div key={log.id} className="flex items-start gap-2 text-[11px] leading-5">
                        <span className="text-muted-foreground shrink-0">{log.time}</span>
                        <span
                          className={[
                            "size-1.5 rounded-full mt-1.5 shrink-0",
                            log.level === "success"
                              ? "bg-teal"
                              : log.level === "warn"
                                ? "bg-destructive"
                                : "bg-primary",
                          ].join(" ")}
                        />
                        <span className="text-foreground flex-1 break-all">{log.message}</span>
                      </div>
                    ))}
                    {consoleLogs.length === 0 ? (
                      <p className="p-4 text-center text-muted-foreground">
                        {t("No console messages recorded yet.")}
                      </p>
                    ) : null}
                  </div>
                ) : (
                  <div className="space-y-2">
                    {pendingApprovalsList.map((appr) => (
                      <div
                        key={appr.approval.approvalId}
                        className="flex items-center justify-between gap-3 rounded-md border border-border/70 bg-background p-2 text-xs"
                      >
                        <div className="min-w-0">
                          <div className="flex items-center gap-2">
                            <span className="font-semibold text-foreground truncate">
                              {appr.agentName}
                            </span>
                            <Badge variant="outline" className="text-[10px]">
                              {appr.approval.kind}
                            </Badge>
                          </div>
                          {appr.approval.target ? (
                            <p className="mt-0.5 text-[11px] text-muted-foreground truncate font-mono">
                              {appr.approval.target}
                            </p>
                          ) : null}
                        </div>
                        <div className="flex items-center gap-1.5 shrink-0">
                          {appr.approval.canAllow ? (
                            <Button
                              size="sm"
                              className="h-6 text-xs px-2"
                              onClick={() => void handleResolveApproval(appr.approval.approvalId, "allow_once")}
                            >
                              <CheckCircle2Icon className="mr-1 size-3" />
                              {t("Approve")}
                            </Button>
                          ) : null}
                          <Button
                            size="sm"
                            variant="outline"
                            className="h-6 text-xs px-2 text-destructive"
                            onClick={() => void handleResolveApproval(appr.approval.approvalId, "reject")}
                          >
                            {t("Reject")}
                          </Button>
                        </div>
                      </div>
                    ))}
                    {pendingApprovalsList.length === 0 ? (
                      <p className="p-4 text-center text-muted-foreground">
                        {t("No pending agent approvals.")}
                      </p>
                    ) : null}
                  </div>
                )}
              </div>
            </div>
          ) : null}
        </div>
      </div>

      {/* Desktop Status Bar */}
      <footer
        data-slot="workbench-statusbar"
        className="flex h-[26px] shrink-0 items-center justify-between border-t border-border/75 bg-sidebar px-3 text-[11px] font-mono text-muted-foreground"
      >
        <div className="flex items-center gap-3">
          <span className="inline-flex items-center gap-1.5">
            <span
              className={[
                "size-1.5 rounded-full",
                runtimeReady === true
                  ? "bg-teal"
                  : runtimeReady === false
                    ? "bg-destructive"
                    : "animate-pulse bg-muted-foreground",
              ].join(" ")}
              aria-hidden="true"
            />
            <span>
              {runtimeReady === true
                ? t("Runtime: Connected")
                : runtimeReady === false
                  ? t("Runtime: Unavailable")
                  : t("Loading")}
            </span>
          </span>
          <span className="text-border">|</span>
          <span className="text-muted-foreground/80">
            {hasLocalOwner ? t("Local Owner: Active") : t("Web Remote")}
          </span>
        </div>

        <div className="flex items-center gap-3">
          <button
            type="button"
            onClick={() => setConsoleOpen((open) => !open)}
            aria-label={t("Toggle Console")}
            className="flex items-center gap-1.5 hover:text-foreground transition-colors text-[11px]"
          >
            <TerminalIcon className="size-3.5" aria-hidden="true" />
            <span>{t("Console")}</span>
            {pendingApprovalCount > 0 ? (
              <span className="rounded-full bg-destructive size-1.5" />
            ) : null}
          </button>
          <span className="text-border">|</span>
          <span className="hidden sm:inline text-[10px] text-muted-foreground/60">
            {t("⌘K Palette · ⌘` Console · ⌘⇧P Playground")}
          </span>
        </div>

        <div className="flex items-center gap-3">
          {pendingApprovalCount > 0 ? (
            <button
              type="button"
              onClick={() => {
                setConsoleOpen(true);
                setConsoleTab("approvals");
              }}
              className="inline-flex items-center gap-1 text-destructive hover:underline"
            >
              <span>⚠️</span>
              <span>
                {pendingApprovalCount} {t("Pending approvals")}
              </span>
            </button>
          ) : null}
          <span className="text-border">|</span>
          <span className="uppercase text-[10px]">{locale === "en" ? "EN" : "ZH"}</span>
          <span className="uppercase text-[10px]">{appearance}</span>
        </div>
      </footer>

      {/* Command Navigation Dialog */}
      <Dialog
        open={commandOpen}
        onOpenChange={(open) => {
          setCommandOpen(open);
          if (!open) setQuery("");
        }}
      >
        <DialogContent className="gap-0 p-0 sm:max-w-lg">
          <DialogHeader className="border-b border-border px-5 py-4">
            <DialogTitle className="text-base">{t("Quick navigation")}</DialogTitle>
            <DialogDescription className="sr-only">
              {t("Search pages and jump to a management area.")}
            </DialogDescription>
          </DialogHeader>
          <div className="border-b border-border p-3">
            <div className="relative">
              <SearchIcon className="pointer-events-none absolute top-2.5 left-3 size-4 text-muted-foreground" />
              <input
                autoFocus
                aria-label={t("Search pages")}
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={t("Search pages…")}
                className="h-9 w-full rounded-lg border border-input bg-card pr-3 pl-9 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring/40"
              />
            </div>
          </div>
          <div className="max-h-[min(64vh,440px)] space-y-0.5 overflow-y-auto p-2">
            {results.length ? (
              results.map((item) => {
                const Icon = item.icon;
                return (
                  <button
                    key={item.to}
                    type="button"
                    onClick={() => goTo(item.to)}
                    className="flex h-10 w-full items-center gap-3 rounded-lg px-3 text-left text-sm transition-colors hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring"
                  >
                    <Icon className="size-4 text-muted-foreground" aria-hidden="true" />
                    <span className="flex-1">{t(item.label)}</span>
                    <CommandIcon className="size-3.5 text-muted-foreground/40" aria-hidden="true" />
                  </button>
                );
              })
            ) : (
              <p className="px-3 py-8 text-center text-sm text-muted-foreground">
                {t("No matching pages")}
              </p>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
