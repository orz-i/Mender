import type { ReactNode } from "react";
import {
  BookOpenIcon,
  KeyRoundIcon,
  Layers3Icon,
  LayoutDashboardIcon,
  PackageSearchIcon,
  PlugIcon,
  ScrollTextIcon,
  ServerCogIcon,
  ShieldCheckIcon,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";

import { t } from "../i18n/i18n";

const pageIcons: Record<string, LucideIcon> = {
  Overview: LayoutDashboardIcon,
  Providers: PlugIcon,
  Skills: BookOpenIcon,
  Catalog: PackageSearchIcon,
  Profiles: Layers3Icon,
  Servers: ServerCogIcon,
  Policy: ShieldCheckIcon,
  Credentials: KeyRoundIcon,
  Audit: ScrollTextIcon,
};

export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description: string;
  actions?: ReactNode;
}) {
  const Icon = pageIcons[title];

  return (
    <header className="mb-4 flex flex-col items-stretch gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="flex min-w-0 items-center gap-2.5">
        {Icon ? (
          <span className="flex size-9 shrink-0 items-center justify-center rounded-lg border border-border/70 bg-muted/55 text-muted-foreground">
            <Icon className="size-4" aria-hidden="true" />
          </span>
        ) : null}
        <div className="min-w-0">
          <h1 className="text-xl font-semibold tracking-[-0.02em] text-foreground">
            {t(title)}
          </h1>
          <p className="mt-0.5 max-w-3xl text-[13px] leading-[1.15rem] text-muted-foreground">
            {t(description)}
          </p>
        </div>
      </div>
      {actions ? (
        <div className="shrink-0 self-start sm:self-auto">{actions}</div>
      ) : null}
    </header>
  );
}

export function ErrorNotice({ message }: { message: string }) {
  return (
    <div className="mb-5 rounded-xl border border-destructive/20 bg-destructive/6 px-4 py-3 text-sm text-destructive">
      {message}
    </div>
  );
}

export function EmptyState({
  title,
  description,
}: {
  title: string;
  description: string;
}) {
  return (
    <div className="rounded-2xl border border-dashed border-border/90 bg-muted/15 px-6 py-12 text-center">
      <p className="font-medium tracking-tight">{t(title)}</p>
      <p className="mx-auto mt-1 max-w-lg text-sm leading-5 text-muted-foreground">
        {t(description)}
      </p>
    </div>
  );
}
