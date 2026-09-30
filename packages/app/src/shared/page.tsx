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

import { Badge } from "@mender/ui/components/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@mender/ui/components/card";

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
    <header className="mb-6 flex flex-col items-stretch gap-4 sm:flex-row sm:items-center sm:justify-between">
      <div className="flex min-w-0 items-center gap-4">
        {Icon ? (
          <span className="flex size-14 shrink-0 items-center justify-center rounded-2xl bg-primary text-primary-foreground shadow-sm shadow-primary/20">
            <Icon className="size-6" aria-hidden="true" />
          </span>
        ) : null}
        <div className="min-w-0">
          <h1 className="text-[1.75rem] font-semibold tracking-[-0.03em] text-foreground">
            {t(title)}
          </h1>
          <p className="mt-1 max-w-3xl text-sm leading-5 text-muted-foreground">
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

export function StatusCard({
  title,
  value,
  description,
  status = "neutral",
  icon,
}: {
  title: string;
  value: string;
  description: string;
  status?: "neutral" | "good" | "warning";
  icon?: ReactNode;
}) {
  return (
    <Card className="gap-3 py-4">
      <CardHeader className="gap-2 pb-0">
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-2.5">
            {icon ? (
              <span className="flex size-9 items-center justify-center rounded-xl bg-primary/8 text-primary [&_svg]:size-[17px]">
                {icon}
              </span>
            ) : null}
            <div>
              <CardDescription className="font-medium">
                {t(title)}
              </CardDescription>
              <CardTitle className="mt-0.5 text-2xl font-semibold tracking-[-0.03em]">
                {value}
              </CardTitle>
            </div>
          </div>
          <Badge
            variant={status === "warning" ? "secondary" : "outline"}
            className="rounded-full px-2 py-0.5 text-[10px]"
          >
            {t(status)}
          </Badge>
        </div>
      </CardHeader>
      <CardContent>
        <p className="text-xs leading-5 text-muted-foreground">
          {t(description)}
        </p>
      </CardContent>
    </Card>
  );
}
