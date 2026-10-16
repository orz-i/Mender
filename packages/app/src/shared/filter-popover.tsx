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
              "h-9 gap-1.5 rounded-xl border-input bg-card px-3 text-xs font-medium text-foreground hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring",
              activeCount > 0 && "border-primary/40 bg-accent text-accent-foreground font-semibold",
              className,
            )}
          />
        }
      >
        <span>{labelText}</span>
        {activeCount > 0 ? (
          <span className="flex size-4 items-center justify-center rounded-full bg-primary text-[10px] font-bold text-primary-foreground">
            {activeCount}
          </span>
        ) : null}
        <ListFilterIcon className="size-3.5 text-muted-foreground" aria-hidden="true" />
      </PopoverTrigger>
      <PopoverContent
        align="end"
        side="bottom"
        sideOffset={6}
        className="w-72 rounded-2xl border border-border/80 bg-popover p-4 text-popover-foreground shadow-xl"
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
    <label className={cn("grid gap-1.5 text-xs font-medium text-muted-foreground", className)}>
      <span className="text-foreground">{t(label)}</span>
      <select
        aria-label={t(label)}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        className="h-8.5 w-full rounded-xl border border-input bg-card px-2.5 text-xs text-foreground outline-none transition-colors focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/40"
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
