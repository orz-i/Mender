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
    <div data-slot="detail-workspace" className={cn("flex min-w-0 flex-col gap-4", className)}>
      {sections.length > 0 ? (
        <nav aria-label={t(label)} data-slot="detail-sections"
          className="min-w-0 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          <div role="group" aria-label={t(label)}
            className="inline-flex min-w-0 items-center gap-0.5 rounded-xl border border-border/70 bg-secondary/70 p-1">
            {sections.map((section) => (
              <button key={section.value} type="button" aria-label={t(section.label)}
                aria-pressed={active === section.value} onClick={() => onChange(section.value)}
                className={cn(
                  "inline-flex h-9 shrink-0 items-center gap-1.5 rounded-lg px-3 text-[12px] font-medium whitespace-nowrap transition-colors",
                  "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring",
                  active === section.value
                    ? "bg-card text-foreground shadow-[0_1px_3px_rgb(0_0_0/0.11)]"
                    : "text-muted-foreground hover:bg-card/60 hover:text-foreground",
                )}>
                <span>{t(section.label)}</span>
                {section.count !== undefined ? (
                  <span aria-hidden="true" className={cn(
                    "rounded-md px-1.5 text-[11px] tabular-nums",
                    active === section.value ? "bg-secondary text-foreground" : "bg-muted text-muted-foreground",
                  )}>
                    {formatNumber(section.count)}
                  </span>
                ) : null}
              </button>
            ))}
          </div>
        </nav>
      ) : null}
      <div data-slot="detail-workspace-content" className="min-w-0 flex-1">
        {children}
      </div>
    </div>
  );
}
