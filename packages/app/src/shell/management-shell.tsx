import { useEffect, useState } from "react";
import {
  BookOpenIcon,
  CommandIcon,
  KeyRoundIcon,
  LanguagesIcon,
  Layers3Icon,
  LayoutDashboardIcon,
  PackageSearchIcon,
  PanelLeftCloseIcon,
  PanelLeftOpenIcon,
  PlugIcon,
  ScrollTextIcon,
  ServerCogIcon,
  ShieldCheckIcon,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { motion } from "motion/react";
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

import {
  type Locale,
  setLocale,
  t,
  useLocale,
} from "../i18n/i18n";

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
      {
        to: "/",
        label: "Overview",
        icon: LayoutDashboardIcon,
      },
      {
        to: "/providers",
        label: "Providers",
        icon: PlugIcon,
      },
      {
        to: "/skills",
        label: "Skills",
        icon: BookOpenIcon,
      },
      {
        to: "/catalog",
        label: "Catalog",
        icon: PackageSearchIcon,
      },
    ],
  },
  {
    label: "Runtime",
    items: [
      {
        to: "/profiles",
        label: "Profiles",
        icon: Layers3Icon,
      },
      {
        to: "/servers",
        label: "Servers",
        icon: ServerCogIcon,
      },
    ],
  },
  {
    label: "Governance",
    items: [
      {
        to: "/policy",
        label: "Policy",
        icon: ShieldCheckIcon,
      },
      {
        to: "/credentials",
        label: "Credentials",
        icon: KeyRoundIcon,
      },
      {
        to: "/audit",
        label: "Audit",
        icon: ScrollTextIcon,
      },
    ],
  },
];

const flatNavigation = navigation.flatMap((group) => group.items);

export function ManagementShell({ client }: { client: MenderClient }) {
  const locale = useLocale();
  const location = useLocation();
  const navigate = useNavigate();
  const [collapsed, setCollapsed] = useState(() => {
    if (typeof window === "undefined") {
      return false;
    }
    return window.localStorage.getItem(SIDEBAR_STORAGE_KEY) === "true";
  });
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
        if (active) {
          setRuntimeReady(status.ready);
        }
      })
      .catch(() => {
        if (active) {
          setRuntimeReady(false);
        }
      });
    return () => {
      active = false;
    };
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

  const normalizedQuery = query.trim().toLocaleLowerCase();
  const filteredNavigation = normalizedQuery
    ? flatNavigation.filter((item) =>
        t(item.label).toLocaleLowerCase().includes(normalizedQuery),
      )
    : flatNavigation;

  const activeItem =
    flatNavigation.find((item) => {
      if (item.to === "/") {
        return location.pathname === "/";
      }
      return location.pathname.startsWith(item.to);
    }) ?? flatNavigation[0]!;

  function goTo(to: string) {
    void navigate(to);
    setCommandOpen(false);
    setQuery("");
  }

  return (
    <div className="min-h-screen bg-background text-foreground">
      <div className="flex min-h-screen">
        <aside
          className={[
            "sticky top-0 z-30 flex h-screen shrink-0 flex-col border-r border-border/70 bg-sidebar/95 transition-[width] duration-200 ease-out",
            collapsed ? "w-[72px]" : "w-[256px]",
          ].join(" ")}
        >
          <div
            className={[
              "flex h-16 items-center border-b border-border/60 px-3",
              collapsed ? "justify-center" : "gap-3",
            ].join(" ")}
          >
            <div className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-sm shadow-primary/15">
              <Layers3Icon className="size-[18px]" aria-hidden="true" />
            </div>

            {!collapsed ? (
              <div className="min-w-0 flex-1">
                <p className="truncate text-[15px] font-semibold tracking-tight">
                  Mender
                </p>
                <p className="truncate text-[11px] text-muted-foreground">
                  {t("Tool control plane")}
                </p>
              </div>
            ) : null}

            {!collapsed ? (
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label={t("Collapse sidebar")}
                onClick={() => setCollapsed(true)}
                className="text-muted-foreground hover:text-foreground"
              >
                <PanelLeftCloseIcon />
              </Button>
            ) : null}
          </div>

          {collapsed ? (
            <div className="flex justify-center border-b border-border/50 py-2">
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      aria-label={t("Expand sidebar")}
                      onClick={() => setCollapsed(false)}
                    />
                  }
                >
                  <PanelLeftOpenIcon />
                </TooltipTrigger>
                <TooltipContent side="right">
                  {t("Expand sidebar")}
                </TooltipContent>
              </Tooltip>
            </div>
          ) : null}

          <nav
            className={[
              "min-h-0 flex-1 overflow-y-auto py-4",
              collapsed ? "px-2" : "px-3",
            ].join(" ")}
            aria-label={t("Primary navigation")}
          >
            {navigation.map((group, groupIndex) => (
              <div
                key={group.label}
                className={groupIndex === 0 ? "" : "mt-5"}
              >
                {!collapsed ? (
                  <p className="mb-1.5 px-2 text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground/80">
                    {t(group.label)}
                  </p>
                ) : groupIndex !== 0 ? (
                  <div className="mx-2 mb-2 border-t border-border/60" />
                ) : null}

                <div className="space-y-1">
                  {group.items.map((item) => {
                    const Icon = item.icon;
                    const link = (
                      <NavLink
                        key={item.to}
                        to={item.to}
                        end={item.to === "/"}
                        aria-label={collapsed ? t(item.label) : undefined}
                        className={({ isActive }) =>
                          [
                            "group relative flex h-9 items-center rounded-xl text-sm font-medium transition-colors",
                            collapsed
                              ? "justify-center px-0"
                              : "gap-3 px-2.5",
                            isActive
                              ? "bg-primary/10 text-primary"
                              : "text-muted-foreground hover:bg-muted/70 hover:text-foreground",
                          ].join(" ")
                        }
                      >
                        {({ isActive }) => (
                          <>
                            <Icon
                              className={[
                                "size-[17px] shrink-0 transition-colors",
                                isActive
                                  ? "text-primary"
                                  : "text-muted-foreground group-hover:text-foreground",
                              ].join(" ")}
                              aria-hidden="true"
                            />
                            {!collapsed ? (
                              <span className="truncate">{t(item.label)}</span>
                            ) : null}
                            {isActive ? (
                              <span
                                aria-hidden="true"
                                className={[
                                  "absolute rounded-full bg-primary",
                                  collapsed
                                    ? "-left-2 h-5 w-0.5"
                                    : "-left-3 h-5 w-0.5",
                                ].join(" ")}
                              />
                            ) : null}
                          </>
                        )}
                      </NavLink>
                    );

                    if (!collapsed) {
                      return link;
                    }

                    return (
                      <Tooltip key={item.to}>
                        <TooltipTrigger render={link} />
                        <TooltipContent side="right">
                          {t(item.label)}
                        </TooltipContent>
                      </Tooltip>
                    );
                  })}
                </div>
              </div>
            ))}
          </nav>

          <div
            className={[
              "border-t border-border/60 py-3",
              collapsed ? "px-2" : "px-3",
            ].join(" ")}
          >
            {collapsed ? (
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      aria-label={t("Language")}
                      onClick={() =>
                        setLocale(locale === "en" ? "zh-CN" : "en")
                      }
                      className="w-full text-muted-foreground hover:text-foreground"
                    />
                  }
                >
                  <LanguagesIcon />
                </TooltipTrigger>
                <TooltipContent side="right">
                  {t("Language")}:{" "}
                  {locale === "en"
                    ? t("English")
                    : t("Simplified Chinese")}
                </TooltipContent>
              </Tooltip>
            ) : (
              <label className="grid gap-1.5 px-1 text-[11px] font-medium text-muted-foreground">
                <span className="flex items-center gap-2">
                  <LanguagesIcon className="size-3.5" aria-hidden="true" />
                  {t("Language")}
                </span>
                <select
                  aria-label={t("Language")}
                  value={locale}
                  onChange={(event) =>
                    setLocale(event.target.value as Locale)
                  }
                  className="h-8 w-full rounded-lg border border-input bg-background px-2 text-xs text-foreground outline-none transition-colors focus:border-ring focus:ring-2 focus:ring-ring/20"
                >
                  <option value="en">{t("English")}</option>
                  <option value="zh-CN">{t("Simplified Chinese")}</option>
                </select>
              </label>
            )}
          </div>
        </aside>

        <div className="min-w-0 flex-1">
          <header className="sticky top-0 z-20 flex h-[88px] items-center justify-between gap-4 border-b border-border/70 bg-background/92 px-5 backdrop-blur-xl md:px-6 xl:px-8">
            <button
              type="button"
              onClick={() => setCommandOpen(true)}
              className="flex h-10 w-full max-w-[620px] items-center gap-2 rounded-xl border border-border/70 bg-muted/35 px-3 text-left text-sm text-muted-foreground transition-colors hover:border-border hover:bg-muted/60"
            >
              <CommandIcon className="size-4 shrink-0" aria-hidden="true" />
              <span className="truncate">
                {t("Jump to a page")}
              </span>
              <span className="ml-auto hidden rounded-md border border-border/70 bg-background px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground sm:inline">
                {t("⌘K")}
              </span>
            </button>

            <div className="flex shrink-0 items-center gap-2">
              <div className="hidden items-center gap-2 rounded-xl border border-border/70 bg-card px-3 py-2 text-xs sm:flex">
                <span
                  className={[
                    "size-2 rounded-full",
                    runtimeReady === true
                      ? "bg-emerald-500"
                      : runtimeReady === false
                        ? "bg-destructive"
                        : "bg-muted-foreground/50",
                  ].join(" ")}
                  aria-hidden="true"
                />
                <span className="font-medium">
                  {runtimeReady === true
                    ? t("Connected")
                    : runtimeReady === false
                      ? t("Unavailable")
                      : t("Loading")}
                </span>
              </div>
              <div className="hidden rounded-xl border border-border/70 bg-card px-3 py-2 text-xs text-muted-foreground lg:block">
                {t(activeItem.label)}
              </div>
            </div>
          </header>

          <main className="min-w-0">
            <motion.div
              key={location.pathname}
              initial={{ opacity: 0, y: 4 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.16, ease: "easeOut" }}
              className="mx-auto w-full max-w-[1600px] px-5 py-6 md:px-6 xl:px-8 xl:py-7"
            >
              <Outlet />
            </motion.div>
          </main>
        </div>
      </div>

      <Dialog
        open={commandOpen}
        onOpenChange={(open) => {
          setCommandOpen(open);
          if (!open) {
            setQuery("");
          }
        }}
      >
        <DialogContent className="gap-0 p-0 sm:max-w-lg">
          <DialogHeader className="border-b border-border/70 px-4 py-3">
            <DialogTitle className="text-sm">
              {t("Quick navigation")}
            </DialogTitle>
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
              className="h-10 w-full rounded-xl border border-input bg-background px-3 text-sm outline-none transition-colors placeholder:text-muted-foreground focus:border-ring focus:ring-2 focus:ring-ring/20"
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
                    className="flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left text-sm transition-colors hover:bg-muted"
                  >
                    <span className="flex size-8 items-center justify-center rounded-lg bg-muted text-muted-foreground">
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
