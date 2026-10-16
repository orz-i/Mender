import type { ReactNode } from "react";

import { Card } from "@mender/ui/components/card";

import { formatNumber, t } from "../i18n/i18n";

/** Page-level controls: tabs on the left, filters + actions to the right. */
export function PageToolbar({
  tabs,
  filters,
  actions,
  className = "",
}: {
  tabs?: ReactNode;
  filters?: ReactNode;
  actions?: ReactNode;
  className?: string;
}) {
  return (
    <div
      data-slot="page-toolbar"
      className={["mb-5 flex min-w-0 flex-col gap-3 xl:flex-row xl:flex-wrap xl:items-center xl:justify-between", className].join(" ")}
    >
      {tabs ? <div data-slot="page-toolbar-tabs" className="flex w-full min-w-0 max-w-full flex-wrap items-center gap-2 xl:w-auto xl:shrink-0">{tabs}</div> : null}
      {filters || actions ? (
        <div
          data-slot="page-toolbar-controls"
          className="flex w-full min-w-0 flex-wrap items-center justify-end gap-2 xl:ml-auto xl:w-auto xl:flex-1"
        >
          {filters ? (
            <div data-slot="page-toolbar-filters" className="flex w-full min-w-0 flex-wrap items-center justify-start gap-2 sm:flex-1 xl:w-auto xl:justify-end">
              {filters}
            </div>
          ) : null}
          {actions ? <div data-slot="page-toolbar-actions" className="flex shrink-0 flex-wrap items-center justify-end gap-2">{actions}</div> : null}
        </div>
      ) : null}
    </div>
  );
}

/** Results-only surface: descriptive heading retained for screen readers, not repeated visually. */
export function ResultPanel({
  title,
  children,
  className = "",
}: {
  title: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section aria-label={t(title)} data-slot="result-panel" className="min-w-0">
      <Card className={["gap-0 py-0", className].join(" ")}>
        <h2 className="sr-only">{t(title)}</h2>
        {children}
      </Card>
    </section>
  );
}

/** Compact segmented tabs following the provided reference design. */
export function PageTabs({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div
      role="group"
      aria-label={t(label)}
      className="inline-flex max-w-full items-center gap-1 overflow-x-auto rounded-2xl bg-secondary p-1 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
    >
      {children}
    </div>
  );
}

export function PageTab({
  label,
  count,
  active,
  onClick,
}: {
  label: string;
  count?: number;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={[
        "inline-flex h-9 shrink-0 items-center gap-1.5 rounded-xl px-3 text-xs font-medium whitespace-nowrap",
        "transition-[background-color,color,box-shadow] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring",
        active ? "bg-card text-foreground shadow-sm" : "text-muted-foreground hover:bg-card/65 hover:text-foreground",
      ].join(" ")}
    >
      {t(label)}
      {count !== undefined ? (
        <span className={["rounded-md px-1.5 py-0.5 text-[10px] tabular-nums", active ? "bg-muted text-foreground" : "bg-muted/80 text-muted-foreground"].join(" ")}>
          {formatNumber(count)}
        </span>
      ) : null}
    </button>
  );
}
