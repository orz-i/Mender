import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { ChevronRightIcon } from "lucide-react";
import { Link } from "react-router";

import { cn } from "@mender/ui/lib/utils";

import { t } from "../i18n/i18n";

/** Magpie-inspired connected resource rows. The link remains an honest navigation
 * action; destructive or privileged controls belong on detail pages, not inside it. */
export function ResourceList({
  label,
  children,
  bordered = false,
  slot,
  className,
}: {
  label: string;
  children: ReactNode;
  bordered?: boolean;
  slot?: string;
  className?: string;
}) {
  return (
    <div role="list" aria-label={t(label)} data-slot={slot ?? "resource-list"}
      className={cn(
        "min-w-0 divide-y divide-border/75",
        bordered && "overflow-hidden rounded-xl border border-border bg-card",
        className,
      )}>
      {children}
    </div>
  );
}

export function ResourceRow({
  to,
  label,
  icon: Icon,
  title,
  description,
  detail,
  status,
  className,
}: {
  to: string;
  label: string;
  icon: LucideIcon;
  title: string;
  description?: ReactNode;
  detail?: ReactNode;
  status?: ReactNode;
  className?: string;
}) {
  return (
    <div role="listitem" className="min-w-0">
      <Link to={to} aria-label={label}
        className={cn(
          "group flex min-h-[76px] min-w-0 items-center gap-3 rounded-sm px-4 py-3 text-left",
          "transition-colors hover:bg-muted/40 focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-ring sm:gap-4 sm:px-5",
          className,
        )}>
        <span data-slot="resource-icon" aria-hidden="true"
          className="flex size-9 shrink-0 items-center justify-center rounded-[10px] bg-secondary/80 text-muted-foreground">
          <Icon className="size-[19px]" strokeWidth={1.9} />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[13px] font-semibold tracking-[-0.01em] text-foreground group-hover:text-primary-ink sm:text-[14px]">
            {title}
          </span>
          {description !== undefined ? (
            <span data-slot="resource-description" className="mt-1 block min-w-0 truncate text-[12px] leading-4 text-muted-foreground">
              {description}
            </span>
          ) : null}
        </span>
        {detail !== undefined ? (
          <span data-slot="resource-detail"
            className="hidden min-w-0 max-w-[32%] shrink-0 text-right text-[12px] text-muted-foreground md:block">
            {detail}
          </span>
        ) : null}
        {status !== undefined ? (
          <span data-slot="resource-status" className="max-w-[42%] shrink-0 text-right text-[11px] sm:text-[12px]">
            {status}
          </span>
        ) : null}
        <ChevronRightIcon aria-hidden="true"
          className="size-4 shrink-0 text-muted-foreground/45 transition-transform group-hover:translate-x-0.5 group-hover:text-foreground" />
      </Link>
    </div>
  );
}
