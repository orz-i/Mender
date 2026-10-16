import type { ReactNode } from "react";

import { cn } from "@mender/ui/lib/utils";

import { t } from "../i18n/i18n";

export function PageHeader({
  actions,
}: {
  actions?: ReactNode;
}) {
  if (!actions) {
    return null;
  }

  return (
    <div className="mb-5 flex min-w-0 justify-end">
      <div className="min-w-0 max-w-full">{actions}</div>
    </div>
  );
}

export function ErrorNotice({ message }: { message: string }) {
  return (
    <div className="mb-5 rounded-2xl border border-destructive/20 bg-destructive/10 px-5 py-4 text-sm text-destructive">
      {message}
    </div>
  );
}

export function EmptyState({
  title,
  description,
  action,
  className = "",
}: {
  title: string;
  description: string;
  action?: ReactNode;
  className?: string;
}) {
  return (
    <div
      data-slot="empty-state"
      className={cn(
        "w-full max-w-xl rounded-[22px] border border-dashed border-border/80 bg-card/60 px-6 py-12 text-center backdrop-blur-sm sm:px-10 sm:py-14",
        className,
      )}
    >
      <p className="text-[15px] font-medium tracking-tight text-foreground">{t(title)}</p>
      <p className="mx-auto mt-1.5 max-w-md text-sm leading-6 text-muted-foreground">
        {t(description)}
      </p>
      {action ? <div className="mt-5 flex justify-center">{action}</div> : null}
    </div>
  );
}
EmptyState.displayName = "EmptyState";

export { InsightBanner, type InsightBannerProps } from "./insight-banner";

