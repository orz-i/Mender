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
 * Lightweight, warm guidance banner modeled after the Elera reference design.
 * Features a soft green tint surface (#eaf7ec), clear typographic hierarchy,
 * and high-contrast action pills.
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
        "mb-5 flex min-w-0 flex-col gap-4 rounded-[22px] border border-[#c6eac3] bg-[#eaf7ec] p-4 text-foreground shadow-xs sm:flex-row sm:items-center sm:justify-between dark:border-[#2a452e] dark:bg-[#1a281c]",
        className,
      )}
    >
      <div className="flex min-w-0 items-start gap-3.5 sm:items-center">
        <span
          className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-[#d4f2d2] text-[#1b5e24] dark:bg-[#253f2a] dark:text-[#88e27c]"
          aria-hidden="true"
        >
          <Icon className="size-5" />
        </span>
        <div className="min-w-0">
          <p className="text-sm font-semibold tracking-[-0.015em] text-foreground">
            {typeof title === "string" ? t(title) : title}
          </p>
          {description ? (
            <p className="mt-0.5 text-xs leading-5 text-[#2c5b33] dark:text-[#a0cfa5]">
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
              className="inline-flex size-8 shrink-0 items-center justify-center rounded-lg text-[#2c5b33] transition-colors hover:bg-[#d8f0d6] hover:text-[#1b5e24] focus-visible:outline-2 focus-visible:outline-ring dark:text-[#a0cfa5] dark:hover:bg-[#243b29] dark:hover:text-foreground"
            >
              <XIcon className="size-4" aria-hidden="true" />
            </button>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
