import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { SparklesIcon, XIcon } from "lucide-react";

import { cn } from "@mender/ui/lib/utils";

import { t } from "../i18n/i18n";

export type InsightBannerProps = {
  title: string;
  description?: ReactNode;
  icon?: LucideIcon;
  actions?: ReactNode;
  onDismiss?: () => void;
  className?: string;
};

/**
 * Compact, dismissible guidance uses the shared Mender accent tokens.
 * Guidance is supplemental, never a duplicate of the current page content.
 */
export function InsightBanner({
  title,
  description,
  icon: Icon = SparklesIcon,
  actions,
  onDismiss,
  className = "",
}: InsightBannerProps) {
  return (
    <div
      role="region"
      aria-label={typeof title === "string" ? t(title) : t("System recommendation")}
      data-slot="insight-banner"
      className={cn(
        "mb-5 flex min-w-0 flex-col gap-4 rounded-[22px] border border-primary/30 bg-accent/75 p-4 text-foreground sm:flex-row sm:items-center sm:justify-between",
        className,
      )}
    >
      <div className="flex min-w-0 items-start gap-3.5 sm:items-center">
        <span
          className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-primary/25 text-primary-ink"
          aria-hidden="true"
        >
          <Icon className="size-5" />
        </span>
        <div className="min-w-0">
          <p className="text-sm font-semibold tracking-[-0.015em] text-foreground">
            {typeof title === "string" ? t(title) : title}
          </p>
          {description ? (
            <p className="mt-0.5 text-sm leading-5 text-muted-foreground">
              {typeof description === "string" ? t(description) : description}
            </p>
          ) : null}
        </div>
      </div>

      {actions || onDismiss ? (
        <div className="flex shrink-0 flex-wrap items-center gap-2 sm:self-center">
          {actions}
          {onDismiss ? (
            <button
              type="button"
              onClick={onDismiss}
              aria-label={t("Dismiss recommendation")}
              className="inline-flex size-9 shrink-0 items-center justify-center rounded-xl text-primary-ink transition-colors hover:bg-primary/20 focus-visible:outline-2 focus-visible:outline-ring"
            >
              <XIcon className="size-4" aria-hidden="true" />
            </button>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
