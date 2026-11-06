import type { ReactNode } from "react";
import { ListFilterIcon } from "lucide-react";

import { Button } from "@mender/ui/components/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@mender/ui/components/popover";
import { cn } from "@mender/ui/lib/utils";

import { t } from "../i18n/i18n";

export function FilterPopover({
  children,
  activeCount = 0,
  onReset,
  title = "Filter & sort",
  triggerLabel = "Filters",
  className = "",
}: {
  children: ReactNode;
  activeCount?: number;
  onReset?: () => void;
  title?: string;
  triggerLabel?: string;
  className?: string;
}) {
  const labelText = triggerLabel === "Filters" ? t("Filters") : t(triggerLabel);
  const titleText = title === "Filter & sort" ? t("Filter & sort") : t(title);

  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button
            type="button"
            variant="outline"
            size="sm"
            aria-label={labelText}
            className={cn(
              "h-8 gap-1.5 rounded-lg border-input bg-card px-2.5 text-[12px] font-medium text-foreground hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring",
              activeCount > 0 && "border-border bg-card font-semibold",
              className,
            )}
          />
        }
      >
        <span>{labelText}</span>
        {activeCount > 0 ? (
          <span className="flex min-w-5 items-center justify-center rounded-md bg-secondary px-1 text-[11px] font-semibold text-foreground">
            {activeCount}
          </span>
        ) : null}
        <ListFilterIcon className="size-3.5 text-muted-foreground" aria-hidden="true" />
      </PopoverTrigger>
      <PopoverContent
        align="end"
        side="bottom"
        sideOffset={6}
        className="w-72 rounded-xl border border-border bg-popover p-3.5 text-popover-foreground shadow-lg"
      >
        <div className="flex items-center justify-between border-b border-border/70 pb-2.5">
          <div className="flex items-center gap-2">
            <ListFilterIcon className="size-4 text-muted-foreground" aria-hidden="true" />
            <span className="text-sm font-semibold">{titleText}</span>
          </div>
          {activeCount > 0 && onReset ? (
            <button
              type="button"
              onClick={onReset}
              className="text-xs font-medium text-primary-ink hover:underline focus-visible:outline-2 focus-visible:outline-ring dark:text-primary"
            >
              {t("Reset")}
            </button>
          ) : null}
        </div>
        <div className="grid gap-3 pt-3.5">
          {children}
        </div>
      </PopoverContent>
    </Popover>
  );
}

export function FilterSelectField({
  label,
  value,
  onChange,
  options,
  className = "",
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: Array<{ value: string; label: string }>;
  className?: string;
}) {
  return (
    <label className={cn("grid gap-1.5 text-sm font-medium text-muted-foreground", className)}>
      <span className="text-foreground">{t(label)}</span>
      <select
        aria-label={t(label)}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        className="h-9 w-full rounded-lg border border-input bg-card px-2.5 text-sm text-foreground outline-none transition-colors focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/40"
      >
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {t(option.label)}
          </option>
        ))}
      </select>
    </label>
  );
}
