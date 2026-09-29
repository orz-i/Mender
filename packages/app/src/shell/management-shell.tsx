import { motion } from "motion/react";
import { NavLink, Outlet } from "react-router";

import {
  type Locale,
  setLocale,
  t,
  useLocale,
} from "../i18n/i18n";

const navigation = [
  { to: "/", label: "Overview", end: true },
  { to: "/providers", label: "Providers" },
  { to: "/skills", label: "Skills" },
  { to: "/catalog", label: "Catalog" },
  { to: "/profiles", label: "Profiles" },
  { to: "/servers", label: "Servers" },
  { to: "/policy", label: "Policy" },
  { to: "/credentials", label: "Credentials" },
  { to: "/audit", label: "Audit" },
] as const;

export function ManagementShell() {
  const locale = useLocale();

  return (
    <div className="min-h-screen bg-background text-foreground">
      <div className="grid min-h-screen grid-cols-[220px_minmax(0,1fr)]">
        <aside className="flex min-h-screen flex-col border-r border-border bg-card/60 px-4 py-5">
          <div className="mb-7 px-2">
            <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
              Mender
            </p>
            <p className="mt-1 text-sm font-medium">{t("Tool control plane")}</p>
          </div>
          <nav className="flex-1 space-y-1">
            {navigation.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.to === "/"}
                className={({ isActive }) =>
                  [
                    "block rounded-lg px-3 py-2 text-sm transition-colors",
                    isActive
                      ? "bg-primary text-primary-foreground"
                      : "text-muted-foreground hover:bg-muted hover:text-foreground",
                  ].join(" ")
                }
              >
                {t(item.label)}
              </NavLink>
            ))}
          </nav>

          <label className="mt-6 border-t border-border px-2 pt-4 text-xs font-medium text-muted-foreground">
            <span className="mb-2 block">{t("Language")}</span>
            <select
              aria-label={t("Language")}
              value={locale}
              onChange={(event) => setLocale(event.target.value as Locale)}
              className="h-9 w-full rounded-lg border border-input bg-background px-2 text-sm text-foreground outline-none focus:ring-2 focus:ring-ring/40"
            >
              <option value="en">{t("English")}</option>
              <option value="zh-CN">{t("Simplified Chinese")}</option>
            </select>
          </label>
        </aside>

        <main className="min-w-0">
          <motion.div
            key="management-shell"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ duration: 0.15 }}
            className="mx-auto max-w-7xl px-8 py-7"
          >
            <Outlet />
          </motion.div>
        </main>
      </div>
    </div>
  );
}
