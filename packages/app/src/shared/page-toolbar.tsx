import type { ReactNode } from "react";
import { isValidElement } from "react";

import { Card } from "@mender/ui/components/card";
import { cn } from "@mender/ui/lib/utils";

import { formatNumber, t } from "../i18n/i18n";
import { EmptyState } from "./page";
import { EmptyPanelState } from "./workbench";

function hasEmptyState(node: ReactNode): boolean {
  if (!node) return false;
  if (Array.isArray(node)) {
    return node.some(hasEmptyState);
  }
  if (isValidElement(node)) {
    if (
      node.type === EmptyState ||
      node.type === EmptyPanelState ||
      (typeof node.type === "function" &&
        (node.type.name === "EmptyState" || node.type.name === "EmptyPanelState")) ||
      (typeof (node.type as { displayName?: string })?.displayName === "string" &&
        ((node.type as { displayName?: string }).displayName === "EmptyState" ||
          (node.type as { displayName?: string }).displayName === "EmptyPanelState")) ||
      (node.props as { "data-slot"?: string })?.["data-slot"] === "empty-panel-state" ||
      (node.props as { "data-slot"?: string })?.["data-slot"] === "empty-state"
    ) {
      return true;
    }
    const props = node.props as { children?: ReactNode };
    if (props && props.children) {
      return hasEmptyState(props.children);
    }
  }
  return false;
}

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
      className={["mb-5 flex min-w-0 flex-wrap items-center justify-between gap-x-4 gap-y-3", className].join(" ")}
    >
      {tabs ? <div data-slot="page-toolbar-tabs" className="flex min-w-0 max-w-full flex-wrap items-center gap-2">{tabs}</div> : null}
      {filters || actions ? (
        <div
          data-slot="page-toolbar-controls"
          className="flex min-w-0 flex-1 flex-wrap items-center justify-end gap-2"
        >
          {filters ? (
            <div data-slot="page-toolbar-filters" className="flex min-w-0 flex-1 flex-wrap items-center justify-end gap-2">
              {filters}
            </div>
          ) : null}
          {actions ? <div data-slot="page-toolbar-actions" className="flex min-w-0 shrink-0 flex-wrap items-center justify-end gap-2">{actions}</div> : null}
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
  variant,
  empty,
}: {
  title: string;
  children: ReactNode;
  className?: string;
  variant?: "card" | "plain";
  empty?: boolean;
}) {
  const isPlain =
    variant === "plain" ||
    empty === true ||
    (variant !== "card" && empty !== false && hasEmptyState(children));

  return (
    <section
      aria-label={t(title)}
      data-slot="result-panel"
      className={cn("min-w-0", isPlain && "flex flex-1 flex-col justify-center")}
    >
      <h2 className="sr-only">{t(title)}</h2>
      {isPlain ? (
        <div data-slot="result-panel-plain" className={cn("flex min-w-0 flex-1 flex-col items-center justify-center", className)}>
          {children}
        </div>
      ) : (
        <Card className={cn("gap-0 py-0", className)}>
          {children}
        </Card>
      )}
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
        "inline-flex h-9 shrink-0 items-center gap-1.5 rounded-xl px-3 text-sm font-medium whitespace-nowrap",
        "transition-[background-color,color,box-shadow] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring",
        active ? "bg-card text-foreground shadow-sm" : "text-muted-foreground hover:bg-card/65 hover:text-foreground",
      ].join(" ")}
    >
      {t(label)}
      {count !== undefined ? (
        <span className={["rounded-md px-1.5 py-0.5 text-xs tabular-nums", active ? "bg-muted text-foreground" : "bg-muted/80 text-muted-foreground"].join(" ")}>
          {formatNumber(count)}
        </span>
      ) : null}
    </button>
  );
}
