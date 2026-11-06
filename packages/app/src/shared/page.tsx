import type { ReactNode } from "react";
import { CircleAlertIcon } from "lucide-react";

import { Button } from "@mender/ui/components/button";
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
    <div className="mb-3 flex min-w-0 justify-end">
      <div className="min-w-0 max-w-full">{actions}</div>
    </div>
  );
}

export function ErrorNotice({ message, onRetry }: { message: string; onRetry?: (() => void) | undefined }) {
  return (
    <div role="alert" className="mb-3 flex flex-wrap items-center gap-3 rounded-xl border border-destructive/20 bg-card px-4 py-3 text-[13px] text-foreground">
      <CircleAlertIcon className="size-4 shrink-0 text-destructive" aria-hidden="true" />
      <p className="min-w-0 flex-1 break-words leading-5">{message}</p>
      {onRetry ? (
        <Button type="button" size="sm" variant="outline" onClick={onRetry}>
          {t("Refresh")}
        </Button>
      ) : null}
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
        "w-full max-w-xl rounded-xl border border-border/80 bg-card px-5 py-7 text-center sm:px-8 sm:py-8",
        className,
      )}
    >
      <p className="text-[14px] font-semibold tracking-tight text-foreground">{t(title)}</p>
      <p className="mx-auto mt-1.5 max-w-md text-[13px] leading-5 text-muted-foreground">
        {t(description)}
      </p>
      {action ? <div className="mt-5 flex justify-center">{action}</div> : null}
    </div>
  );
}
EmptyState.displayName = "EmptyState";

export { InsightBanner, type InsightBannerProps } from "./insight-banner";

