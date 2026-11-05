import { useCallback, useEffect, useState } from "react";
import {
  BellIcon,
  BlocksIcon,
  BotIcon,
  BookOpenIcon,
  BoxIcon,
  CableIcon,
  ChevronLeftIcon,
  CommandIcon,
  GaugeIcon,
  KeyRoundIcon,
  LanguagesIcon,
  Layers3Icon,
  LayoutDashboardIcon,
  MenuIcon,
  RefreshCwIcon,
  ScrollTextIcon,
  SearchIcon,
  Settings2Icon,
  ShieldCheckIcon,
  WrenchIcon,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { Link, NavLink, Outlet, useLocation, useNavigate } from "react-router";

import type { AgentApprovalCapability, ManagementClient } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import {
  Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle,
} from "@mender/ui/components/dialog";
import { Popover, PopoverContent, PopoverTrigger } from "@mender/ui/components/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@mender/ui/components/tooltip";

import { BrandMark } from "../brand/brand-mark";
import { setLocale, t, useLocale } from "../i18n/i18n";
import { PageHeadingContext, getPageTrail } from "./page-chrome";

type NavigationItem = { to: string; label: string; icon: LucideIcon };

const primaryNavigation: NavigationItem[] = [
  { to: "/", label: "Overview", icon: LayoutDashboardIcon },
  { to: "/tools", label: "Tools", icon: WrenchIcon },
  { to: "/profiles", label: "Profiles", icon: Layers3Icon },
  { to: "/agents", label: "Agents", icon: BotIcon },
  { to: "/sandboxes", label: "Sandboxes", icon: BoxIcon },
  { to: "/skills", label: "Skills", icon: BookOpenIcon },
  { to: "/plugins", label: "Plugin Marketplace", icon: BlocksIcon },
  { to: "/audit", label: "Audit", icon: ScrollTextIcon },
];

const secondaryNavigation: Array<{ label: string; items: NavigationItem[] }> = [
  {
    label: "Connections",
    items: [
      { to: "/tunnels", label: "Tunnels", icon: CableIcon },
      { to: "/credentials", label: "Credentials", icon: KeyRoundIcon },
    ],
  },
  {
    label: "Advanced",
    items: [
      { to: "/approvals", label: "Approvals", icon: BellIcon },
      { to: "/policy", label: "Policy", icon: ShieldCheckIcon },
      { to: "/runtime", label: "Runtime", icon: GaugeIcon },
    ],
  },
];
const flatNavigation = [...primaryNavigation, ...secondaryNavigation.flatMap((group) => group.items)];

const APPEARANCE_STORAGE_KEY = "mender.appearance";
type Appearance = "light" | "dark" | "system";

function readAppearance(): Appearance {
  if (typeof window === "undefined") return "light";
  const stored = window.localStorage.getItem(APPEARANCE_STORAGE_KEY);
  return stored === "dark" || stored === "system" ? stored : "light";
}

export function ManagementShell({ client, approvals }: {
  client: ManagementClient;
  approvals?: AgentApprovalCapability | undefined;
}) {
  const locale = useLocale();
  const location = useLocation();
  const navigate = useNavigate();
  const [pageHeading, setPageHeading] = useState<{ path: string; value: string } | null>(null);
  const [runtimeReady, setRuntimeReady] = useState<boolean | null>(null);
  const [commandOpen, setCommandOpen] = useState(false);
  const [navigationOpen, setNavigationOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [pendingApprovalCount, setPendingApprovalCount] = useState(0);
  const [appearance, setAppearance] = useState<Appearance>(readAppearance);

  const updatePageHeading = useCallback((path: string, value?: string) => {
    setPageHeading((current) => value ? { path, value } : current?.path === path ? null : current);
  }, []);

  useEffect(() => {
    window.localStorage.setItem(APPEARANCE_STORAGE_KEY, appearance);
    if (typeof window.matchMedia !== "function") {
      document.documentElement.classList.toggle("dark", appearance === "dark");
      return;
    }
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const apply = () => document.documentElement.classList.toggle("dark",
      appearance === "dark" || (appearance === "system" && media.matches));
    apply();
    if (appearance === "system") {
      media.addEventListener("change", apply);
      return () => media.removeEventListener("change", apply);
    }
    return undefined;
  }, [appearance]);

  useEffect(() => {
    let active = true;
    void Promise.resolve().then(() => client.getSystemStatus())
      .then((status) => { if (active) setRuntimeReady(status.ready); })
      .catch(() => { if (active) setRuntimeReady(false); });
    return () => { active = false; };
  }, [client]);

  useEffect(() => {
    if (!approvals) return;
    let active = true;
    const refresh = () => {
      void approvals.listPendingAgentApprovals()
        .then(({ approvals: pending }) => {
          if (active) setPendingApprovalCount(pending.length);
        })
        .catch(() => { if (active) setPendingApprovalCount(0); });
    };
    refresh();
    const timer = setInterval(refresh, 2500);
    return () => { active = false; clearInterval(timer); };
  }, [approvals]);

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setCommandOpen((open) => !open);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, []);

  const activeItem = flatNavigation.find((item) => item.to === "/"
    ? location.pathname === "/"
    : location.pathname === item.to || location.pathname.startsWith(item.to + "/")) ?? primaryNavigation[0]!;
  const trail = getPageTrail(location.pathname, activeItem);
  const currentHeading = pageHeading?.path === location.pathname ? pageHeading.value : undefined;
  if (currentHeading && trail.length > 1) {
    trail[1] = { ...trail[1]!, label: currentHeading };
  }
  const visibleNavigation = flatNavigation.filter((item) => item.to !== "/approvals" || approvals !== undefined);
  const needle = query.trim().toLocaleLowerCase();
  const results = needle
    ? visibleNavigation.filter((item) => t(item.label).toLocaleLowerCase().includes(needle))
    : visibleNavigation;

  function goTo(to: string) {
    void navigate(to);
    setCommandOpen(false);
    setNavigationOpen(false);
    setSettingsOpen(false);
    setQuery("");
  }

  return (
    <div data-slot="management-layout" data-layout="top" className="flex min-h-screen min-w-0 flex-col bg-background text-foreground">
      <header data-slot="app-header" className="sticky top-0 z-30 flex h-[58px] shrink-0 items-center gap-3 border-b border-border/65 bg-background px-4 sm:px-6">
        <div className="flex min-w-[128px] shrink-0 items-center gap-2">
          <Button type="button" variant="ghost" size="icon-sm"
            aria-label={t("Open navigation")} onClick={() => setNavigationOpen(true)}
            className="size-8 lg:hidden">
            <MenuIcon className="size-[19px]" aria-hidden="true" />
          </Button>
          <Link to="/" className="inline-flex items-center gap-2 rounded-md text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring/40">
            <BrandMark className="size-6" aria-hidden="true" />
            <span className="brand-wordmark text-[17px] font-semibold tracking-[-0.035em]">Mender</span>
          </Link>
        </div>
        <nav aria-label={t("Primary navigation")} data-slot="top-navigation"
          className="hidden min-w-0 flex-1 items-center justify-center overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden lg:flex">
          <div className="flex w-max shrink-0 items-center gap-1 rounded-[11px] bg-secondary/70 p-1">
            {primaryNavigation.map((item) => (
              <NavLink key={item.to} to={item.to} end={item.to === "/"}
                aria-label={t(item.label)}
                className={({ isActive }) => [
                  "inline-flex h-[35px] shrink-0 items-center justify-center rounded-lg px-3.5 text-[13px] font-medium whitespace-nowrap transition-colors focus-visible:outline-2 focus-visible:outline-ring",
                  isActive ? "bg-card text-foreground shadow-[0_1px_3px_rgb(15_15_20/0.12)]" :
                    "text-muted-foreground hover:bg-card/65 hover:text-foreground",
                ].join(" ")}>
                {t(item.label)}
              </NavLink>
            ))}
          </div>
        </nav>
        <div className="ml-auto flex shrink-0 items-center gap-0.5 sm:gap-1">
          <Tooltip>
            <TooltipTrigger render={<Button type="button" size="icon-sm" variant="ghost" className="size-8 text-muted-foreground hover:text-foreground"
              aria-label={t("Search pages…")} onClick={() => setCommandOpen(true)} />}>
              <SearchIcon className="size-[17px]" aria-hidden="true" />
            </TooltipTrigger>
            <TooltipContent side="bottom">{t("Search pages…") + " (" + t("⌘K") + ")"}</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger render={<Button type="button" size="icon-sm" variant="ghost" className="size-8 text-muted-foreground hover:text-foreground"
              aria-label={t("Refresh")} onClick={() => { void navigate(0); }} />}>
              <RefreshCwIcon className="size-[17px]" aria-hidden="true" />
            </TooltipTrigger>
            <TooltipContent side="bottom">{t("Refresh")}</TooltipContent>
          </Tooltip>
          {approvals !== undefined ? (
            <Tooltip>
              <TooltipTrigger render={<Button type="button" size="icon-sm" variant="ghost"
                className="relative size-8 text-muted-foreground hover:text-foreground"
                aria-label={t("Pending approvals")} onClick={() => goTo("/approvals")} />}>
                <BellIcon className="size-[17px]" aria-hidden="true" />
                {pendingApprovalCount > 0 ? (
                  <span aria-label={t("Pending approvals") + ": " + pendingApprovalCount}
                    className="absolute -right-0.5 -top-0.5 min-w-4 rounded-full bg-destructive px-1 text-center text-[10px] leading-4 text-white">
                    {pendingApprovalCount > 99 ? "99+" : pendingApprovalCount}
                  </span>
                ) : null}
              </TooltipTrigger>
              <TooltipContent side="bottom">{t("Pending approvals")} ({pendingApprovalCount})</TooltipContent>
            </Tooltip>
          ) : null}
          <Popover open={settingsOpen} onOpenChange={setSettingsOpen}>
            <PopoverTrigger render={<Button type="button" size="icon-sm" variant="ghost"
              className="size-8 text-muted-foreground hover:text-foreground" aria-label={t("Settings")} />}>
              <Settings2Icon className="size-[18px]" aria-hidden="true" />
            </PopoverTrigger>
            <PopoverContent side="bottom" align="end" sideOffset={9}
              className="w-[256px] rounded-xl border-border bg-popover p-2 shadow-lg">
              <p className="px-2 py-2 text-[12px] font-semibold text-foreground">{t("Settings")}</p>
              {secondaryNavigation.map((group) => (
                <div key={group.label} className="border-t border-border/75 py-1.5">
                  <p className="px-2 py-1.5 text-[10px] font-semibold tracking-wider text-muted-foreground uppercase">{t(group.label)}</p>
                  {group.items.filter((item) => item.to !== "/approvals" || approvals !== undefined).map((item) => {
                    const Icon = item.icon;
                    return (
                      <button key={item.to} type="button" onClick={() => goTo(item.to)}
                        className="flex h-9 w-full items-center gap-2 rounded-lg px-2 text-left text-[13px] text-foreground hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring">
                        <Icon className="size-[16px] text-muted-foreground" aria-hidden="true" />
                        <span className="flex-1">{t(item.label)}</span>
                        {item.to === "/approvals" && pendingApprovalCount > 0 ? (
                          <span className="rounded-md bg-destructive/10 px-1.5 text-[11px] text-destructive">{pendingApprovalCount}</span>
                        ) : null}
                      </button>
                    );
                  })}
                </div>
              ))}
              <div className="border-t border-border/75 px-2 pt-2 pb-1">
                <p className="mb-2 text-[11px] font-medium text-muted-foreground">{t("Appearance")}</p>
                <div role="group" aria-label={t("Appearance")} className="flex gap-1 rounded-lg bg-secondary p-1">
                  {(["light", "dark", "system"] as const).map((option) => (
                    <button key={option} type="button" aria-pressed={appearance === option}
                      onClick={() => setAppearance(option)}
                      className={[
                        "h-7 flex-1 rounded-md px-1 text-[11px] focus-visible:outline-2 focus-visible:outline-ring",
                        appearance === option ? "bg-card font-semibold text-foreground shadow-sm" :
                          "text-muted-foreground hover:text-foreground",
                      ].join(" ")}>
                      {t(option === "light" ? "Light" : option === "dark" ? "Dark" : "System")}
                    </button>
                  ))}
                </div>
                <button type="button" onClick={() => { setLocale(locale === "en" ? "zh-CN" : "en"); }}
                  className="mt-2 flex h-8 w-full items-center gap-2 rounded-lg px-1 text-left text-[12px] text-muted-foreground hover:bg-muted hover:text-foreground">
                  <LanguagesIcon className="size-4" aria-hidden="true" />
                  {t("Language")}: {locale === "en" ? t("Simplified Chinese") : t("English")}
                </button>
              </div>
            </PopoverContent>
          </Popover>
          <Tooltip>
            <TooltipTrigger render={<div role="status" aria-label={
              runtimeReady === true ? t("Connected") : runtimeReady === false ? t("Unavailable") : t("Loading")
            } className="ml-1 flex size-5 cursor-default items-center justify-center" />}>
              <span className={[
                "size-2 rounded-full", runtimeReady === true ? "bg-teal" :
                  runtimeReady === false ? "bg-destructive" : "animate-pulse bg-muted-foreground",
              ].join(" ")} aria-hidden="true" />
            </TooltipTrigger>
            <TooltipContent side="bottom">
              {t(runtimeReady === true ? "Connected" : runtimeReady === false ? "Unavailable" : "Loading")}
            </TooltipContent>
          </Tooltip>
        </div>
        <h1 className="sr-only">{t(trail[trail.length - 1]!.label)}</h1>
      </header>

      <Dialog open={navigationOpen} onOpenChange={setNavigationOpen}>
        <DialogContent className="top-0 left-0 flex h-dvh max-h-dvh w-[min(330px,calc(100vw-1rem))] max-w-none flex-col gap-0 overflow-hidden rounded-none rounded-r-xl p-0 translate-x-0 translate-y-0 sm:max-w-none lg:hidden">
          <DialogHeader className="flex h-[60px] shrink-0 flex-row items-center gap-2 border-b border-border px-5">
            <BrandMark className="size-6 shrink-0" aria-hidden="true" />
            <DialogTitle className="brand-wordmark text-lg">Mender</DialogTitle>
            <DialogDescription className="sr-only">{t("Primary navigation")}</DialogDescription>
          </DialogHeader>
          <nav aria-label={t("Primary navigation")} className="min-h-0 flex-1 overflow-y-auto px-3 py-3">
            <p className="px-3 py-2 text-[11px] font-medium text-muted-foreground">{t("Primary navigation")}</p>
            {primaryNavigation.map((item) => {
              const Icon = item.icon;
              return <NavLink key={item.to} to={item.to} end={item.to === "/"}
                onClick={() => setNavigationOpen(false)}
                className={({ isActive }) => [
                  "flex h-10 items-center gap-3 rounded-lg px-3 text-[13px] font-medium",
                  isActive ? "bg-card text-foreground shadow-sm" : "text-muted-foreground hover:bg-muted hover:text-foreground",
                ].join(" ")}>
                <Icon className="size-4" aria-hidden="true" />
                {t(item.label)}
              </NavLink>;
            })}
            <p className="mt-4 border-t border-border px-3 pt-4 pb-2 text-[11px] font-medium text-muted-foreground">{t("Settings")}</p>
            {secondaryNavigation.flatMap((group) => group.items)
              .filter((item) => item.to !== "/approvals" || approvals !== undefined)
              .map((item) => {
                const Icon = item.icon;
                return <NavLink key={item.to} to={item.to} onClick={() => setNavigationOpen(false)}
                  className={({ isActive }) => [
                    "flex h-10 items-center gap-3 rounded-lg px-3 text-[13px] font-medium",
                    isActive ? "bg-card text-foreground shadow-sm" : "text-muted-foreground hover:bg-muted hover:text-foreground",
                  ].join(" ")}>
                  <Icon className="size-4" aria-hidden="true" />
                  {t(item.label)}
                </NavLink>;
              })}
          </nav>
        </DialogContent>
      </Dialog>

      <main className="mx-auto flex w-full max-w-[1680px] min-w-0 flex-1 flex-col px-4 pt-5 pb-8 sm:px-6 lg:px-7" data-slot="app-workspace">
        {trail.length > 1 ? (
          <nav aria-label={t("Breadcrumb")} data-slot="page-breadcrumb" className="mb-4 flex min-w-0 items-center gap-1.5 text-[12px] text-muted-foreground">
            {trail.length > 1 ? <Link to={trail[trail.length - 2]!.to}
              aria-label={t("Back") + ": " + t(trail[trail.length - 2]!.label)}
              className="flex items-center gap-1 rounded-md hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/40 sm:hidden">
              <ChevronLeftIcon className="size-4" />{t("Back")}
            </Link> : null}
            {trail.slice(0, -1).map((crumb) => (
              <span key={crumb.to} className="hidden min-w-0 items-center gap-1.5 sm:inline-flex">
                <Link to={crumb.to} className="max-w-44 truncate hover:text-foreground hover:underline">{t(crumb.label)}</Link>
                <span className="text-muted-foreground/45" aria-hidden="true">/</span>
              </span>
            ))}
            <span className="min-w-0 truncate font-medium text-foreground" title={t(trail[trail.length - 1]!.label)}>
              {t(trail[trail.length - 1]!.label)}
            </span>
          </nav>
        ) : null}
        <PageHeadingContext.Provider value={updatePageHeading}>
          <Outlet />
        </PageHeadingContext.Provider>
      </main>

      <Dialog open={commandOpen} onOpenChange={(open) => {
        setCommandOpen(open);
        if (!open) setQuery("");
      }}>
        <DialogContent className="gap-0 p-0 sm:max-w-lg">
          <DialogHeader className="border-b border-border px-5 py-4">
            <DialogTitle className="text-base">{t("Quick navigation")}</DialogTitle>
            <DialogDescription className="sr-only">{t("Search pages and jump to a management area.")}</DialogDescription>
          </DialogHeader>
          <div className="border-b border-border p-3">
            <div className="relative">
              <SearchIcon className="pointer-events-none absolute top-2.5 left-3 size-4 text-muted-foreground" />
              <input autoFocus aria-label={t("Search pages")} value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={t("Search pages…")}
                className="h-9 w-full rounded-lg border border-input bg-card pr-3 pl-9 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring/40" />
            </div>
          </div>
          <div className="max-h-[min(64vh,440px)] space-y-0.5 overflow-y-auto p-2">
            {results.length ? results.map((item) => {
              const Icon = item.icon;
              return (
                <button key={item.to} type="button" onClick={() => goTo(item.to)}
                  className="flex h-10 w-full items-center gap-3 rounded-lg px-3 text-left text-sm transition-colors hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring">
                  <Icon className="size-4 text-muted-foreground" aria-hidden="true" />
                  <span className="flex-1">{t(item.label)}</span>
                  <CommandIcon className="size-3.5 text-muted-foreground/40" aria-hidden="true" />
                </button>
              );
            }) : (
              <p className="px-3 py-8 text-center text-sm text-muted-foreground">{t("No matching pages")}</p>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
