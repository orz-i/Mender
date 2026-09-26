import { motion } from "motion/react";
import { NavLink, Outlet } from "react-router";

const navigation = [
  { to: "/", label: "Overview", end: true },
  { to: "/providers", label: "Providers" },
  { to: "/catalog", label: "Catalog" },
  { to: "/profiles", label: "Profiles" },
  { to: "/servers", label: "Servers" },
  { to: "/policy", label: "Policy" },
  { to: "/credentials", label: "Credentials" },
  { to: "/audit", label: "Audit" },
] as const;

export function ManagementShell() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <div className="grid min-h-screen grid-cols-[220px_minmax(0,1fr)]">
        <aside className="border-r border-border bg-card/60 px-4 py-5">
          <div className="mb-7 px-2">
            <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
              Mender
            </p>
            <p className="mt-1 text-sm font-medium">Tool control plane</p>
          </div>
          <nav className="space-y-1">
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
                {item.label}
              </NavLink>
            ))}
          </nav>
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
