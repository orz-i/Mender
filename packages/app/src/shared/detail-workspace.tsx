import type { ReactNode } from "react";

import { cn } from "@mender/ui/lib/utils";
import { formatNumber, t } from "../i18n/i18n";

export type DetailSection<Value extends string> = {
  value: Value;
  label: string;
  count?: number;
};

/**
 * One place for an object's local topics, separate from global navigation and
 * page actions. A narrow screen keeps the topics in a single scrollable row;
 * wider screens have a fixed-width local rail beside the work area.
 */
export function DetailWorkspace<Value extends string>({
  label,
  sections,
  active,
  onChange,
  children,
  className,
}: {
  label: string;
  sections: ReadonlyArray<DetailSection<Value>>;
  active: Value;
  onChange: (value: Value) => void;
  children: ReactNode;
  className?: string;
}) {
  return (
    <div
      data-slot="detail-workspace"
      className={cn("min-w-0", sections.length > 0 && "grid gap-3 lg:grid-cols-[172px_minmax(0,1fr)] lg:gap-5", className)}
    >
      {sections.length > 0 ? (
        <nav aria-label={t(label)} data-slot="detail-sections"
          className="min-w-0 self-start lg:sticky lg:top-[74px]">
          <div role="group" aria-label={t(label)}
            className="flex max-w-full items-center gap-0.5 overflow-x-auto rounded-lg border border-border bg-card p-1 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden lg:flex-col lg:items-stretch lg:gap-0.5 lg:overflow-visible lg:rounded-none lg:border-0 lg:bg-transparent lg:p-0">
            {sections.map((section) => (
              <button key={section.value} type="button" aria-label={t(section.label)} aria-pressed={active === section.value}
                onClick={() => onChange(section.value)}
                className={cn(
                  "flex h-9 shrink-0 items-center gap-2 rounded-md px-3 text-left text-[12px] font-medium whitespace-nowrap transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring lg:w-full lg:justify-between",
                  active === section.value
                    ? "bg-accent text-foreground"
                    : "text-muted-foreground hover:bg-muted hover:text-foreground",
                )}>
                <span className="truncate">{t(section.label)}</span>
                {section.count !== undefined ? (
                  <span aria-hidden="true" className={cn("ml-auto rounded-md px-1.5 text-[11px] tabular-nums", active === section.value
                    ? "bg-card/75 text-foreground dark:bg-card/55" : "bg-muted text-muted-foreground")}>
                    {formatNumber(section.count)}
                  </span>
                ) : null}
              </button>
            ))}
          </div>
        </nav>
      ) : null}
      <div data-slot="detail-workspace-content" className="min-w-0">
        {children}
      </div>
    </div>
  );
}
