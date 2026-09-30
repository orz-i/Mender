import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { SearchIcon } from "lucide-react";

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
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">{children}</div>
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
    <Card className="gap-0 py-0 shadow-none">
      <CardContent className="flex min-h-24 items-center gap-3 px-4 py-4">
        <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-primary/8 text-primary">
          <Icon aria-hidden="true" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <p className="text-xl font-semibold tracking-[-0.03em] text-foreground">
                {typeof value === "number" ? formatNumber(value) : value}
              </p>
              <p className="text-xs font-medium text-muted-foreground">
                {t(label)}
              </p>
            </div>
            {action ? <div className="shrink-0">{action}</div> : null}
          </div>
          {description ? (
            <p className="mt-0.5 truncate text-[11px] text-muted-foreground/80">
              {t(description)}
            </p>
          ) : null}
        </div>
      </CardContent>
    </Card>
  );
}

export function DataPanel({
  title,
  description,
  actions,
  toolbar,
  children,
  className = "",
}: {
  title: string;
  description?: ReactNode;
  actions?: ReactNode;
  toolbar?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <Card className={["gap-0 py-0 shadow-none", className].join(" ")}>
      <CardHeader className="min-h-16 border-b border-border/65 px-4 py-3.5">
        <div className="flex items-center justify-between gap-4">
          <div className="min-w-0">
            <CardTitle className="text-sm font-semibold tracking-tight">
              {t(title)}
            </CardTitle>
            {description ? (
              <CardDescription className="mt-0.5 text-xs">
                {typeof description === "string" ? t(description) : description}
              </CardDescription>
            ) : null}
          </div>
          {actions ? <div className="shrink-0">{actions}</div> : null}
        </div>
      </CardHeader>
      {toolbar ? (
        <div className="border-b border-border/65 bg-muted/10 px-4 py-3">
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
        className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
      />
      <Input
        aria-label={t(label)}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={t(placeholder)}
        className="h-9 pl-9"
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
        className={["h-9 min-w-36 bg-background", className].join(" ")}
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
}: {
  children: ReactNode;
}) {
  return <div className="px-4 py-5">{children}</div>;
}
