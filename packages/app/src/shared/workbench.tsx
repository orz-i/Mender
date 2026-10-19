import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import {
  ActivityIcon,
  BlocksIcon,
  BookOpenIcon,
  BoxIcon,
  CableIcon,
  CpuIcon,
  GaugeIcon,
  KeyRoundIcon,
  Layers3Icon,
  PackageSearchIcon,
  PlugIcon,
  ScrollTextIcon,
  SearchIcon,
  ServerCogIcon,
  ShieldCheckIcon,
  WrenchIcon,
} from "lucide-react";

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

const panelIcons: Record<string, LucideIcon> = {
  "Recent invocations": ActivityIcon,
  "Plugin catalog": BlocksIcon,
  "Plugin details": BlocksIcon,
  "Capabilities": WrenchIcon,
  "Technical details": CpuIcon,
  "Provider inventory": PlugIcon,
  "Skill Library": BookOpenIcon,
  "Tools": WrenchIcon,
  "Profile inventory": Layers3Icon,
  "Tunnel inventory": CableIcon,
  "Running MCP servers": ServerCogIcon,
  "Runtime budget": GaugeIcon,
  "Process isolation": ShieldCheckIcon,
  "Sandbox inventory": BoxIcon,
  "Sandbox overview": BoxIcon,
  "Create sandbox": BoxIcon,
  "Sandbox activity": ActivityIcon,
  "Security boundary": ShieldCheckIcon,
  "Policy rules": ShieldCheckIcon,
  "Credential inventory": KeyRoundIcon,
  "Invocation history": ScrollTextIcon,
  "Provider activation history": PlugIcon,
  "Providers": PlugIcon,
  "Profiles": Layers3Icon,
  "Skills": BookOpenIcon,
  "Catalog": PackageSearchIcon,
};

export function MetricStrip({
  children,
  className = "",
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div data-slot="metric-strip" className={cn("@container/metrics mb-5 min-w-0", className)}>
      <div className="grid grid-cols-1 gap-px overflow-hidden rounded-[22px] border border-border/50 bg-border/70 shadow-[var(--card-elevation)] @min-[490px]/metrics:grid-cols-2 @min-[820px]/metrics:grid-cols-4">
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
    <div data-slot="metric-tile" className="flex min-h-[108px] min-w-0 items-center gap-3 bg-card px-4 py-4 @min-[820px]/metrics:px-4">
      <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-foreground text-background dark:bg-muted dark:text-foreground">
        <Icon className="size-[18px]" aria-hidden="true" />
      </span>
      <div className="min-w-0 flex-1">
        <p className="truncate text-xs font-medium text-muted-foreground">{t(label)}</p>
        <div className="mt-1 flex min-w-0 items-center justify-between gap-2">
          <p className="min-w-0 truncate text-[24px] leading-none font-semibold tracking-[-0.045em] text-foreground tabular-nums">
            {typeof value === "number" ? formatNumber(value) : value}
          </p>
          {action ? <div className="shrink-0">{action}</div> : null}
        </div>
        {description ? (
          <p className="mt-1.5 truncate text-xs leading-4 text-muted-foreground">
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
  const Icon = icon ?? panelIcons[title] ?? Layers3Icon;
  return (
    <Card className={["gap-0 py-0", className].join(" ")}>
      <CardHeader className="min-h-[74px] px-5 pt-4 pb-3">
        <div className="flex min-w-0 flex-wrap items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-3">
            <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-foreground text-background dark:bg-muted dark:text-foreground">
              <Icon className="size-[18px]" aria-hidden="true" />
            </span>
            <div className="min-w-0">
              <CardTitle className="text-base font-semibold tracking-[-0.015em]">
                {t(title)}
              </CardTitle>
              {description ? (
                <CardDescription className="mt-0.5 text-[13px] leading-5">
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
        onKeyDown={onKeyDown}
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
        "flex min-h-[46vh] w-full flex-1 flex-col items-center justify-center py-10 px-4 text-center sm:px-6",
        className,
      )}
    >
      {children}
    </div>
  );
}
EmptyPanelState.displayName = "EmptyPanelState";
