import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { SearchIcon, XIcon } from "lucide-react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@mender/ui/components/card";
import { Input } from "@mender/ui/components/input";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@mender/ui/components/select";

import { formatNumber, t } from "../i18n/i18n";
import { cn } from "@mender/ui/lib/utils";

export function MetricStrip({
  children,
  className = "",
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div data-slot="metric-strip" className={cn("@container/metrics mb-5 min-w-0", className)}>
      <div className="grid grid-cols-1 gap-px overflow-hidden rounded-xl border border-border bg-border/75 @min-[490px]/metrics:grid-cols-2 @min-[820px]/metrics:grid-cols-4">
        {children}
      </div>
    </div>
  );
}

export function MetricTile({
  icon: Icon,
  value,
  label,
  description,
  action,
}: {
  icon: LucideIcon;
  value: ReactNode;
  label: string;
  description?: string;
  action?: ReactNode;
}) {
  return (
    <div data-slot="metric-tile" className="flex min-h-[78px] min-w-0 flex-col justify-center bg-card px-4 py-3">
      <div className="min-w-0 w-full">
        <p className="flex items-center gap-1.5 truncate text-[11px] font-medium text-muted-foreground">
          <Icon className="size-3.5 shrink-0" aria-hidden="true" />
          {t(label)}
        </p>
        <div className="mt-1.5 flex min-w-0 items-center justify-between gap-2">
          <p className="min-w-0 truncate text-[22px] leading-none font-semibold tracking-[-0.035em] text-foreground tabular-nums">
            {typeof value === "number" ? formatNumber(value) : value}
          </p>
          {action ? <div className="shrink-0">{action}</div> : null}
        </div>
        {description ? (
          <p className="mt-1 truncate text-[11px] leading-4 text-muted-foreground">
            {t(description)}
          </p>
        ) : null}
      </div>
    </div>
  );
}

export function DataPanel({
  title,
  description,
  actions,
  toolbar,
  icon,
  children,
  className = "",
}: {
  title: string;
  description?: ReactNode;
  actions?: ReactNode;
  toolbar?: ReactNode;
  icon?: LucideIcon;
  children: ReactNode;
  className?: string;
}) {
  const Icon = icon;
  return (
    <Card data-kind="data-panel" className={cn("gap-0 overflow-hidden rounded-xl py-0 shadow-none", className)}>
      <CardHeader className="min-h-[54px] px-5 py-3.5">
        <div className="flex min-w-0 flex-wrap items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-2.5">
            {Icon ? <Icon className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" /> : null}
            <div className="min-w-0">
              <CardTitle className="text-[14px] font-semibold tracking-[-0.015em]">
                {t(title)}
              </CardTitle>
              {description ? (
                <CardDescription className="mt-0.5 text-xs leading-4">
                  {typeof description === "string" ? t(description) : description}
                </CardDescription>
              ) : null}
            </div>
          </div>
          {actions ? <div className="min-w-0 shrink-0">{actions}</div> : null}
        </div>
      </CardHeader>
      {toolbar ? (
        <div className="border-b border-border/75 px-5 py-2.5">
          {toolbar}
        </div>
      ) : null}
      <CardContent className="p-0">{children}</CardContent>
    </Card>
  );
}

export function WorkbenchToolbar({ children }: { children: ReactNode }) {
  return (
    <div className="flex flex-col gap-2 md:flex-row md:flex-wrap md:items-center">
      {children}
    </div>
  );
}

export function SearchField({
  value,
  onChange,
  onKeyDown,
  placeholder,
  label,
  className = "",
}: {
  value: string;
  onChange: (value: string) => void;
  onKeyDown?: (event: React.KeyboardEvent<HTMLInputElement>) => void;
  placeholder: string;
  label: string;
  className?: string;
}) {
  return (
    <div className={["relative min-w-0 flex-1", className].join(" ")}>
      <SearchIcon
        aria-hidden="true"
        className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground"
      />
      <Input
        aria-label={t(label)}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Escape" && value) {
            event.preventDefault();
            onChange("");
            return;
          }
          onKeyDown?.(event);
        }}
        placeholder={t(placeholder)}
        className={cn("h-9 rounded-lg bg-card pr-9 pl-9 text-[13px] shadow-none", value && "pr-9")}
      />
      {value ? (
        <button type="button" aria-label={t("Clear search")}
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => onChange("")}
          className="absolute top-1/2 right-1.5 flex size-7 -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:outline-2 focus-visible:outline-ring">
          <XIcon className="size-3.5" aria-hidden="true" />
        </button>
      ) : null}
    </div>
  );
}

export function FilterSelect({
  value,
  onValueChange,
  label,
  options,
  className = "",
}: {
  value: string;
  onValueChange: (value: string) => void;
  label: string;
  options: Array<{ value: string; label: string }>;
  className?: string;
}) {
  const selected = options.find((option) => option.value === value)?.label ?? "";

  return (
    <Select value={value} onValueChange={(next) => onValueChange(String(next))}>
      <SelectTrigger
        aria-label={t(label)}
        className={["h-9 min-w-36 bg-card", className].join(" ")}
      >
        <SelectValue>{t(selected)}</SelectValue>
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {t(option.label)}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  );
}

export function EmptyPanelState({
  children,
  className = "",
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div
      data-slot="empty-panel-state"
      className={cn(
        "flex min-h-[154px] w-full flex-col items-center justify-center px-4 py-6 text-center sm:px-6",
        className,
      )}
    >
      {children}
    </div>
  );
}
EmptyPanelState.displayName = "EmptyPanelState";
