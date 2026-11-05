import { useEffect, useState } from "react";
import {
  BlocksIcon,
  BellIcon,
  BotIcon,
  BookOpenIcon,
  BoxIcon,
  CableIcon,
  ChevronDownIcon,
  GaugeIcon,
  KeyRoundIcon,
  LanguagesIcon,
  Layers3Icon,
  LayoutDashboardIcon,
  MenuIcon,
  PanelLeftCloseIcon,
  PanelLeftOpenIcon,
  ScrollTextIcon,
  SearchIcon,
  ShieldCheckIcon,
  WrenchIcon,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { motion, useReducedMotion } from "motion/react";
import { NavLink, Outlet, useLocation, useNavigate } from "react-router";

import type { AgentApprovalCapability, ManagementClient } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@mender/ui/components/dialog";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@mender/ui/components/tooltip";

import { BrandMark } from "../brand/brand-mark";
import { setLocale, t, useLocale } from "../i18n/i18n";

const SIDEBAR_STORAGE_KEY = "mender.sidebar.collapsed";

type NavigationItem = {
  to: string;
  label: string;
  icon: LucideIcon;
};

type NavigationGroup = {
  label: string;
  items: NavigationItem[];
};

const navigation: NavigationGroup[] = [
  {
    label: "Workspace",
    items: [
      { to: "/", label: "Overview", icon: LayoutDashboardIcon },
      { to: "/plugins", label: "Plugin Marketplace", icon: BlocksIcon },
      { to: "/tools", label: "Tools", icon: WrenchIcon },
      { to: "/skills", label: "Skills", icon: BookOpenIcon },
      { to: "/agents", label: "Agents", icon: BotIcon },
      { to: "/sandboxes", label: "Sandboxes", icon: BoxIcon },
    ],
  },
  {
    label: "Connections",
    items: [
      { to: "/profiles", label: "Profiles", icon: Layers3Icon },
      { to: "/tunnels", label: "Tunnels", icon: CableIcon },
    ],
  },
  {
    label: "Advanced",
    items: [
      { to: "/runtime", label: "Runtime", icon: GaugeIcon },
      { to: "/policy", label: "Policy", icon: ShieldCheckIcon },
      { to: "/credentials", label: "Credentials", icon: KeyRoundIcon },
      { to: "/audit", label: "Audit", icon: ScrollTextIcon },
      { to: "/approvals", label: "Approvals", icon: BellIcon },
    ],
  },
];

const flatNavigation = navigation.flatMap((group) => group.items);

export function ManagementShell({ client, approvals }: {
  client: ManagementClient;
  approvals?: AgentApprovalCapability | undefined;
}) {
  const locale = useLocale();
  const location = useLocation();
  const navigate = useNavigate();
  const reduceMotion = useReducedMotion();
  const [collapsed, setCollapsed] = useState(() =>
    typeof window !== "undefined" &&
    window.localStorage.getItem(SIDEBAR_STORAGE_KEY) === "true",
  );
  const [runtimeReady, setRuntimeReady] = useState<boolean | null>(null);
  const [commandOpen, setCommandOpen] = useState(false);
  const [mobileNavigationOpen, setMobileNavigationOpen] = useState(false);
  const [expandedGroups, setExpandedGroups] = useState<Record<string, { pathname: string; open: boolean }>>({});
  const [query, setQuery] = useState("");
  const [pendingApprovalCount, setPendingApprovalCount] = useState(0);

  useEffect(() => {
    window.localStorage.setItem(SIDEBAR_STORAGE_KEY, String(collapsed));
  }, [collapsed]);

  useEffect(() => {
    let active = true;
    void Promise.resolve()
      .then(() => client.getSystemStatus())
      .then((status) => {
        if (active) setRuntimeReady(status.ready);
      })
      .catch(() => {
        if (active) setRuntimeReady(false);
      });
    return () => { active = false; };
  }, [client]);

  useEffect(() => {
    if (!approvals) return;
    let live=true;
    const poll=()=>{
      void approvals.listPendingAgentApprovals().then(({approvals})=>{
        if(live)setPendingApprovalCount(approvals.length);
      }).catch(()=>{if(live)setPendingApprovalCount(0);});
    };
    poll();
    const timer=setInterval(poll,2500);
    return()=>{live=false;clearInterval(timer);};
  },[approvals]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setCommandOpen((open) => !open);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  const activeItem =
    flatNavigation.find((item) =>
      item.to === "/"
        ? location.pathname === "/"
        : location.pathname.startsWith(item.to),
    ) ?? flatNavigation[0]!;
  const normalizedQuery = query.trim().toLocaleLowerCase();
  const visibleNavigation=flatNavigation.filter(item=>item.to!=="/approvals"||approvals !== undefined);
  const filteredNavigation = normalizedQuery
    ? visibleNavigation.filter((item) =>
        t(item.label).toLocaleLowerCase().includes(normalizedQuery),
      )
    : visibleNavigation;

  function groupExpanded(group: NavigationGroup) {
    const override = expandedGroups[group.label];
    if (override?.pathname === location.pathname) return override.open;
    return group.label === "Workspace" || group.items.some((item) =>
      item.to === location.pathname || (item.to !== "/" && location.pathname.startsWith(item.to + "/")),
    );
  }

  function goTo(to: string) {
    void navigate(to);
    setMobileNavigationOpen(false);
    setCommandOpen(false);
    setQuery("");
  }

  function switchLocale() {
    setLocale(locale === "en" ? "zh-CN" : "en");
  }

  return (
    <div className="flex min-h-screen bg-background text-foreground">
      <aside
        className={[
          "sticky top-0 z-30 hidden h-dvh shrink-0 flex-col border-r border-border bg-sidebar transition-[width] duration-200 ease-out lg:flex",
          collapsed ? "w-[64px]" : "w-[228px]",
        ].join(" ")}
      >
        <div
          data-slot="sidebar-brand"
          className={[
            "flex h-[56px] shrink-0 items-center",
            collapsed
              ? "justify-center px-2"
              : "justify-center px-3 lg:justify-start lg:gap-2 lg:px-4",
          ].join(" ")}
        >
          {collapsed ? (
            <div
              data-slot="sidebar-brand-logo"
              className="group/brand relative flex size-8 shrink-0 items-center justify-center rounded-lg"
            >
              <BrandMark className="size-6 transition-opacity duration-150 group-hover/brand:opacity-0" aria-hidden="true" />
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      aria-label={t("Expand sidebar")}
                      onClick={() => setCollapsed(false)}
                      className="absolute inset-0 size-8 rounded-lg text-foreground opacity-0 transition-opacity duration-150 hover:bg-muted focus-visible:opacity-100 focus-visible:outline-2 focus-visible:outline-ring group-hover/brand:opacity-100"
                    />
                  }
                >
                  <PanelLeftOpenIcon className="size-[19px]" />
                </TooltipTrigger>
                <TooltipContent side="right">
                  {t("Expand sidebar")}
                </TooltipContent>
              </Tooltip>
            </div>
          ) : (
            <>
              <div className="flex size-8 shrink-0 items-center justify-center rounded-lg">
                <BrandMark className="size-6" aria-hidden="true" />
              </div>
              <div className="hidden min-w-0 flex-1 lg:block">
                <p className="brand-wordmark truncate text-[17px] leading-tight text-foreground">Mender</p>
              </div>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      aria-label={t("Collapse sidebar")}
                      onClick={() => setCollapsed(true)}
                      className="hidden size-7 shrink-0 text-muted-foreground hover:text-foreground lg:inline-flex"
                    />
                  }
                >
                  <PanelLeftCloseIcon />
                </TooltipTrigger>
                <TooltipContent side="right">
                  {t("Collapse sidebar")}
                </TooltipContent>
              </Tooltip>
            </>
          )}
        </div>

        <nav
          className={["min-h-0 flex-1 overflow-y-auto pb-5", collapsed ? "px-2" : "px-2 lg:px-3"].join(" ")}
          aria-label={t("Primary navigation")}
        >
          {navigation.map((group, groupIndex) => (
            <div key={group.label} className={groupIndex === 0 ? "mt-1" : "mt-3"}>
              {!collapsed ? (
                <button
                  type="button"
                  aria-expanded={groupExpanded(group)}
                  aria-controls={`nav-group-${groupIndex}`}
                  onClick={() => setExpandedGroups((current) => ({
                    ...current,
                    [group.label]: { pathname: location.pathname, open: !groupExpanded(group) },
                  }))}
                  className="mb-1 flex h-7 w-full items-center justify-between rounded-lg px-3 text-left text-[10px] font-semibold tracking-[0.06em] uppercase text-muted-foreground hover:bg-muted/70 hover:text-foreground focus-visible:outline-2 focus-visible:outline-ring"
                >
                  {t(group.label)}
                  <ChevronDownIcon className={["size-3.5 transition-transform", groupExpanded(group) ? "rotate-180" : ""].join(" ")} aria-hidden="true" />
                </button>
              ) : groupIndex > 0 ? (
                <div className="mx-2 mb-3 border-t border-border/80" />
              ) : null}
              <div id={`nav-group-${groupIndex}`} className="flex flex-col gap-0.5">
                {(collapsed || groupExpanded(group) ? group.items.filter(item=>item.to!=="/approvals"||approvals !== undefined) : []).map((item) => {
                  const Icon = item.icon;
                  const link = (
                    <NavLink
                      key={item.to}
                      to={item.to}
                      end={item.to === "/"}
                      aria-label={t(item.label)}
                      title={t(item.label)}
                      className={({ isActive }) => [
                        "group flex h-9 min-w-0 items-center rounded-lg text-[13px] font-medium transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring",
                        collapsed ? "justify-center px-0" : "justify-start gap-2.5 px-3",
                        isActive
                          ? "bg-accent text-foreground"
                          : "text-muted-foreground hover:bg-muted/75 hover:text-foreground",
                      ].join(" ")}
                    >
                      {({ isActive }) => (
                        <>
                          <Icon
                            className={["size-[17px] shrink-0", isActive ? "text-primary-ink" : "text-muted-foreground group-hover:text-foreground"].join(" ")}
                            aria-hidden="true"
                          />
                          {!collapsed ? <span className="min-w-0 truncate">{t(item.label)}</span> : null}
                        </>
                      )}
                    </NavLink>
                  );
                  return collapsed ? (
                    <Tooltip key={item.to}>
                      <TooltipTrigger render={link} />
                      <TooltipContent side="right">{t(item.label)}</TooltipContent>
                    </Tooltip>
                  ) : link;
                })}
              </div>
            </div>
          ))}
        </nav>

      </aside>

      <Dialog open={mobileNavigationOpen} onOpenChange={setMobileNavigationOpen}>
        <DialogContent className="top-0 left-0 flex h-dvh max-h-dvh w-[min(336px,calc(100vw-1rem))] max-w-none flex-col gap-0 overflow-hidden rounded-none rounded-r-2xl p-0 translate-x-0 translate-y-0 sm:max-w-none lg:hidden">
          <DialogHeader className="flex h-[76px] shrink-0 flex-row items-center gap-3 border-b border-border/70 px-5">
            <BrandMark className="size-8 shrink-0" aria-hidden="true" />
            <DialogTitle className="brand-wordmark text-lg">Mender</DialogTitle>
            <DialogDescription className="sr-only">{t("Primary navigation")}</DialogDescription>
          </DialogHeader>
          <nav className="min-h-0 flex-1 overflow-y-auto px-3 py-3" aria-label={t("Primary navigation")}>
            {navigation.map((group) => (
              <div key={group.label} className="mb-4">
                <p className="px-3 py-2 text-xs font-semibold text-muted-foreground">{t(group.label)}</p>
                <div className="grid gap-1">
                  {group.items.filter(item => item.to !== "/approvals" || approvals !== undefined).map((item) => {
                    const Icon = item.icon;
                    return (
                      <NavLink
                        key={item.to}
                        to={item.to}
                        end={item.to === "/"}
                        onClick={() => setMobileNavigationOpen(false)}
                        className={({ isActive }) => [
                          "flex min-h-10 items-center gap-2.5 rounded-lg px-3 text-[13px] font-medium focus-visible:outline-2 focus-visible:outline-ring",
                          isActive ? "bg-accent text-foreground" : "text-foreground hover:bg-muted",
                        ].join(" ")}
                      >
                        <Icon className="size-[17px] shrink-0" aria-hidden="true" />
                        {t(item.label)}
                      </NavLink>
                    );
                  })}
                </div>
              </div>
            ))}
          </nav>
        </DialogContent>
      </Dialog>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-20 flex h-[54px] shrink-0 items-center justify-between gap-2 border-b border-border bg-background/95 px-4 backdrop-blur-sm sm:px-5 lg:px-6">
          <div className="flex min-w-0 items-center gap-2">
            <Button
              type="button"
              variant="ghost"
              size="icon-lg"
              aria-label={t("Open navigation")}
              onClick={() => setMobileNavigationOpen(true)}
              className="size-8 shrink-0 rounded-lg lg:hidden"
            >
              <MenuIcon className="size-5" />
            </Button>
            <h1 className="truncate text-[16px] leading-tight font-semibold tracking-[-0.02em] sm:text-[17px]">
              {t(activeItem.label)}
            </h1>
          </div>

          <div className="flex min-w-0 shrink-0 items-center justify-end gap-1 sm:gap-2">
            <button
              type="button"
              onClick={() => setCommandOpen(true)}
              aria-label={t("Search pages…")}
              className="flex h-8 min-w-8 items-center justify-center gap-2 rounded-lg border border-border bg-card px-2.5 text-left text-[13px] text-muted-foreground transition-colors hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring sm:w-[178px] sm:justify-start xl:w-[210px]"
            >
              <SearchIcon className="size-4 shrink-0" aria-hidden="true" />
              <span className="hidden truncate sm:inline">{t("Search pages…")}</span>
              <span className="ml-auto hidden rounded-md bg-muted px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground xl:inline">
                {t("⌘K")}
              </span>
            </button>
            {approvals !== undefined?<Tooltip>
              <TooltipTrigger render={<Button type="button" variant="ghost" size="icon-lg"
                aria-label={t("Pending approvals")} onClick={()=>void navigate("/approvals")}
                className="relative size-8 rounded-lg text-muted-foreground hover:bg-muted hover:text-foreground"/>}>
                <BellIcon className="size-4"/>
                {pendingApprovalCount>0?<span aria-label={t("Pending approvals")+": "+pendingApprovalCount}
                  className="absolute -right-1 -top-1 flex min-w-4 items-center justify-center rounded-full bg-foreground px-1 text-[10px] text-background">
                  {pendingApprovalCount>99?"99+":pendingApprovalCount}
                </span>:null}
              </TooltipTrigger>
              <TooltipContent side="bottom">{t("Pending approvals")} ({pendingApprovalCount})</TooltipContent>
            </Tooltip>:null}
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-lg"
                    aria-label={t("Language")}
                    onClick={switchLocale}
                    className="size-8 rounded-lg text-muted-foreground hover:bg-muted hover:text-foreground"
                  />
                }
              >
                <LanguagesIcon />
              </TooltipTrigger>
              <TooltipContent side="bottom">
                {t("Language")}: {locale === "en" ? t("English") : t("Simplified Chinese")}
                {" → "}
                {locale === "en" ? t("Simplified Chinese") : t("English")}
              </TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger
                render={
                  <div
                    role="status"
                    aria-label={runtimeReady === true ? t("Connected") : runtimeReady === false ? t("Unavailable") : t("Loading")}
                    className="flex size-7 items-center justify-center cursor-default"
                  >
                    <span
                      className={[
                        "size-2 rounded-full transition-colors",
                        runtimeReady === true
                          ? "bg-teal"
                          : runtimeReady === false
                            ? "bg-destructive"
                            : "bg-muted-foreground animate-pulse",
                      ].join(" ")}
                      aria-hidden="true"
                    />
                    <span className="sr-only">
                      {runtimeReady === true ? t("Connected") : runtimeReady === false ? t("Unavailable") : t("Loading")}
                    </span>
                  </div>
                }
              />
              <TooltipContent side="bottom">
                {runtimeReady === true ? t("Connected") : runtimeReady === false ? t("Unavailable") : t("Loading")}
              </TooltipContent>
            </Tooltip>
          </div>
        </header>

        <main className="flex min-w-0 flex-1 flex-col">
          <motion.div
            key={location.pathname}
            initial={reduceMotion ? false : { opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ duration: 0.12, ease: "easeOut" }}
            className="mx-auto flex w-full max-w-[1560px] flex-1 flex-col px-4 pt-4 pb-7 sm:px-5 lg:px-6"
          >
            <Outlet />
          </motion.div>
        </main>
      </div>

      <Dialog
        open={commandOpen}
        onOpenChange={(open) => {
          setCommandOpen(open);
          if (!open) setQuery("");
        }}
      >
        <DialogContent className="gap-0 p-0 sm:max-w-lg">
          <DialogHeader className="border-b border-border/70 px-5 py-4">
            <DialogTitle className="text-base">{t("Quick navigation")}</DialogTitle>
            <DialogDescription className="sr-only">
              {t("Search pages and jump to a management area.")}
            </DialogDescription>
          </DialogHeader>
          <div className="border-b border-border/70 p-3">
            <input
              autoFocus
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder={t("Search pages…")}
              aria-label={t("Search pages")}
              className="h-9 w-full rounded-lg border border-input bg-card px-3 text-sm outline-none placeholder:text-muted-foreground focus:border-ring focus:ring-2 focus:ring-ring/20"
            />
          </div>
          <div className="max-h-[360px] overflow-y-auto p-2">
            {filteredNavigation.length ? (
              filteredNavigation.map((item) => {
                const Icon = item.icon;
                return (
                  <button
                    key={item.to}
                    type="button"
                    onClick={() => goTo(item.to)}
                    className="flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left text-sm transition-colors hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring"
                  >
                    <span className="flex size-7 items-center justify-center rounded-lg bg-muted text-muted-foreground">
                      <Icon className="size-4" aria-hidden="true" />
                    </span>
                    <span className="font-medium">{t(item.label)}</span>
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
