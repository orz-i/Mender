import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { Layers3Icon, SearchIcon } from "lucide-react";

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

export function MetricStrip({ children }: { children: ReactNode }) {
  return (
    <div className="grid overflow-hidden rounded-[22px] border border-border/40 bg-card shadow-[var(--card-elevation)] sm:grid-cols-2 xl:grid-cols-4">
      {children}
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
    <div className="flex min-h-[108px] min-w-0 items-center gap-3 border-b border-border/70 px-5 py-4 last:border-b-0 sm:odd:border-r sm:[&:nth-child(-n+2)]:border-b xl:border-r xl:border-b-0 xl:last:border-r-0 xl:[&:nth-child(-n+2)]:border-b-0">
      <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-foreground text-background dark:bg-muted dark:text-foreground">
        <Icon className="size-[19px]" aria-hidden="true" />
      </span>
      <div className="min-w-0 flex-1">
        <p className="truncate text-xs font-medium text-muted-foreground">{t(label)}</p>
        <div className="mt-1 flex min-w-0 items-center justify-between gap-2">
          <p className="min-w-0 truncate text-[26px] leading-none font-semibold tracking-[-0.05em] text-foreground tabular-nums">
            {typeof value === "number" ? formatNumber(value) : value}
          </p>
          {action ? <div className="shrink-0">{action}</div> : null}
        </div>
        {description ? (
          <p className="mt-1.5 truncate text-[11px] leading-4 text-muted-foreground">
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
  icon: Icon = Layers3Icon,
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
  return (
    <Card className={["gap-0 py-0", className].join(" ")}>
      <CardHeader className="min-h-[78px] px-5 pt-4 pb-3">
        <div className="flex min-w-0 flex-wrap items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-3">
            <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-foreground text-background dark:bg-muted dark:text-foreground">
              <Icon className="size-[19px]" aria-hidden="true" />
            </span>
            <div className="min-w-0">
              <CardTitle className="text-[15px] font-semibold tracking-[-0.015em]">
                {t(title)}
              </CardTitle>
              {description ? (
                <CardDescription className="mt-0.5 text-xs leading-5">
                  {typeof description === "string" ? t(description) : description}
                </CardDescription>
              ) : null}
            </div>
          </div>
          {actions ? <div className="min-w-0 shrink-0">{actions}</div> : null}
        </div>
      </CardHeader>
      {toolbar ? (
        <div className="border-b border-border/65 px-5 pt-1 pb-4">
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
  placeholder,
  label,
  className = "",
}: {
  value: string;
  onChange: (value: string) => void;
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
        placeholder={t(placeholder)}
        className="h-9 bg-background pl-9"
      />
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

export function EmptyPanelState({ children }: { children: ReactNode }) {
  return <div className="px-5 py-8">{children}</div>;
}
