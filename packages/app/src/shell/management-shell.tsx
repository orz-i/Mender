import { useEffect, useState } from "react";
import {
  BlocksIcon,
  BotIcon,
  BookOpenIcon,
  BoxIcon,
  CableIcon,
  GaugeIcon,
  KeyRoundIcon,
  LanguagesIcon,
  Layers3Icon,
  LayoutDashboardIcon,
  PackageSearchIcon,
  PanelLeftCloseIcon,
  PanelLeftOpenIcon,
  PlugIcon,
  ScrollTextIcon,
  SearchIcon,
  ServerCogIcon,
  ShieldCheckIcon,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { motion, useReducedMotion } from "motion/react";
import { NavLink, Outlet, useLocation, useNavigate } from "react-router";

import type { MenderClient } from "@mender/sdk";
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
      { to: "/providers", label: "Providers", icon: PlugIcon },
      { to: "/skills", label: "Skills", icon: BookOpenIcon },
      { to: "/catalog", label: "Catalog", icon: PackageSearchIcon },
    ],
  },
  {
    label: "Runtime",
    items: [
      { to: "/runtime", label: "Runtime", icon: GaugeIcon },
      { to: "/sandboxes", label: "Sandboxes", icon: BoxIcon },
      { to: "/agents", label: "Agents", icon: BotIcon },
      { to: "/profiles", label: "Profiles", icon: Layers3Icon },
      { to: "/tunnels", label: "Tunnels", icon: CableIcon },
      { to: "/servers", label: "Servers", icon: ServerCogIcon },
    ],
  },
  {
    label: "Governance",
    items: [
      { to: "/policy", label: "Policy", icon: ShieldCheckIcon },
      { to: "/credentials", label: "Credentials", icon: KeyRoundIcon },
      { to: "/audit", label: "Audit", icon: ScrollTextIcon },
    ],
  },
];

const flatNavigation = navigation.flatMap((group) => group.items);

export function ManagementShell({ client }: { client: MenderClient }) {
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
  const [query, setQuery] = useState("");

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
  const filteredNavigation = normalizedQuery
    ? flatNavigation.filter((item) =>
        t(item.label).toLocaleLowerCase().includes(normalizedQuery),
      )
    : flatNavigation;

  function goTo(to: string) {
    void navigate(to);
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
          "sticky top-0 z-30 flex h-dvh shrink-0 flex-col border-r border-border/70 bg-sidebar transition-[width] duration-200 ease-out",
          collapsed ? "w-[68px] lg:w-[80px]" : "w-[68px] lg:w-[258px]",
        ].join(" ")}
      >
        <div
          data-slot="sidebar-brand"
          className={[
            "flex h-[88px] shrink-0 items-center",
            collapsed
              ? "justify-center px-2 lg:justify-between lg:px-1"
              : "justify-center px-3 lg:justify-start lg:gap-3 lg:px-5",
          ].join(" ")}
        >
          <div className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-foreground text-background">
            <BlocksIcon className="size-[19px]" aria-hidden="true" />
          </div>
          {!collapsed ? (
            <div className="hidden min-w-0 flex-1 lg:block">
              <p className="truncate text-[23px] leading-6 font-semibold tracking-[-0.045em]">Mender</p>
            </div>
          ) : null}
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label={t(collapsed ? "Expand sidebar" : "Collapse sidebar")}
                  onClick={() => setCollapsed((current) => !current)}
                  className="hidden size-8 shrink-0 text-muted-foreground hover:text-foreground lg:inline-flex"
                />
              }
            >
              {collapsed ? <PanelLeftOpenIcon /> : <PanelLeftCloseIcon />}
            </TooltipTrigger>
            <TooltipContent side="right">
              {t(collapsed ? "Expand sidebar" : "Collapse sidebar")}
            </TooltipContent>
          </Tooltip>
        </div>

        <nav
          className={["min-h-0 flex-1 overflow-y-auto pb-5", collapsed ? "px-2" : "px-2 lg:px-4"].join(" ")}
          aria-label={t("Primary navigation")}
        >
          {navigation.map((group, groupIndex) => (
            <div key={group.label} className={groupIndex === 0 ? "mt-3" : "mt-7"}>
              {!collapsed ? (
                <p className="mb-2 hidden px-3 text-xs font-medium text-muted-foreground lg:block">
                  {t(group.label)}
                </p>
              ) : null}
              <div className={groupIndex === 0 ? "hidden" : "mx-2 mb-3 border-t border-border/80 lg:hidden"} />
              <div className="flex flex-col gap-1">
                {group.items.map((item) => {
                  const Icon = item.icon;
                  const link = (
                    <NavLink
                      key={item.to}
                      to={item.to}
                      end={item.to === "/"}
                      aria-label={t(item.label)}
                      title={t(item.label)}
                      className={({ isActive }) => [
                        "group flex h-10 min-w-0 items-center rounded-xl text-sm font-medium transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring",
                        collapsed ? "justify-center px-0" : "justify-center px-0 lg:justify-start lg:gap-3 lg:px-3",
                        isActive
                          ? "bg-primary text-primary-foreground"
                          : "text-muted-foreground hover:bg-card/90 hover:text-foreground",
                      ].join(" ")}
                    >
                      {({ isActive }) => (
                        <>
                          <Icon
                            className={["size-[19px] shrink-0", isActive ? "text-primary-foreground" : "text-muted-foreground group-hover:text-foreground"].join(" ")}
                            aria-hidden="true"
                          />
                          {!collapsed ? <span className="hidden min-w-0 truncate lg:block">{t(item.label)}</span> : null}
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

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-20 flex min-h-[88px] items-center justify-between gap-3 bg-background/95 px-4 backdrop-blur-xl sm:px-6 xl:px-8">
          <div className="min-w-0 py-4">
            <h1 className="truncate text-[23px] leading-[1.15] font-semibold tracking-[-0.045em] sm:text-[27px]">
              {t(activeItem.label)}
            </h1>
          </div>

          <div className="flex min-w-0 shrink-0 items-center justify-end gap-2 sm:gap-3">
            <button
              type="button"
              onClick={() => setCommandOpen(true)}
              aria-label={t("Search pages…")}
              className="flex h-10 min-w-10 items-center justify-center gap-2 rounded-2xl border border-border bg-card/70 px-3 text-left text-sm text-muted-foreground transition-colors hover:bg-card focus-visible:outline-2 focus-visible:outline-ring sm:w-[230px] sm:justify-start lg:w-[290px]"
            >
              <SearchIcon className="size-4 shrink-0" aria-hidden="true" />
              <span className="hidden truncate sm:inline">{t("Search pages…")}</span>
              <span className="ml-auto hidden rounded-lg bg-muted px-2 py-1 font-mono text-[11px] text-muted-foreground lg:inline">
                {t("⌘K")}
              </span>
            </button>
            <div role="status" className="flex h-9 items-center gap-2 rounded-full border border-border/80 bg-card px-3 text-xs">
              <span
                className={[
                  "size-2 rounded-full",
                  runtimeReady === true ? "bg-teal" : runtimeReady === false ? "bg-destructive" : "bg-muted-foreground",
                ].join(" ")}
                aria-hidden="true"
              />
              <span className="hidden font-medium text-foreground sm:inline">
                {runtimeReady === true ? t("Connected") : runtimeReady === false ? t("Unavailable") : t("Loading")}
              </span>
              <span className="sr-only sm:hidden">
                {runtimeReady === true ? t("Connected") : runtimeReady === false ? t("Unavailable") : t("Loading")}
              </span>
            </div>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-lg"
                    aria-label={t("Language")}
                    onClick={switchLocale}
                    className="size-9 rounded-xl text-muted-foreground hover:bg-card hover:text-foreground"
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
          </div>
        </header>

        <main className="min-w-0 flex-1">
          <motion.div
            key={location.pathname}
            initial={reduceMotion ? false : { opacity: 0, y: 5 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.17, ease: "easeOut" }}
            className="mx-auto w-full max-w-[1700px] px-4 pt-2 pb-9 sm:px-6 xl:px-8"
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
              className="h-10 w-full rounded-xl border border-input bg-card px-3 text-sm outline-none placeholder:text-muted-foreground focus:border-ring focus:ring-2 focus:ring-ring/20"
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
                    className="flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left text-sm transition-colors hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring"
                  >
                    <span className="flex size-8 items-center justify-center rounded-xl bg-foreground text-background dark:bg-muted dark:text-foreground">
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
