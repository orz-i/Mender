import { InfoIcon } from "lucide-react";
import type { ReactNode } from "react";

import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@mender/ui/components/popover";

import { t } from "../i18n/i18n";

/** Optional, field-scoped help — never use for risks or requirements that
 * must be seen before submitting. Base UI Popover supports hover, press/touch,
 * keyboard focus and Escape, unlike a hover-only Tooltip. */
export function FormInfo({
  label,
  children,
  side = "bottom",
}: {
  label: string;
  children: ReactNode;
  side?: "top" | "bottom" | "left" | "right";
}) {
  return (
    <Popover>
      <PopoverTrigger
        openOnHover
        delay={150}
        closeDelay={120}
        render={
          <button
            type="button"
            aria-label={t("Help for {{field}}", { field: label })}
            className="inline-flex size-7 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-ring"
          />
        }
      >
        <InfoIcon aria-hidden="true" className="size-4" />
      </PopoverTrigger>
      <PopoverContent
        side={side}
        align="start"
        className="w-64 max-w-[calc(100vw-2rem)] rounded-xl px-3 py-2.5 text-xs leading-5 shadow-md"
      >
        {children}
      </PopoverContent>
    </Popover>
  );
}

/** The help button must stay OUTSIDE the associated HTML label. */
export function FormHeading({
  label,
  help,
  side,
}: {
  label: string;
  help?: ReactNode;
  side?: "top" | "bottom" | "left" | "right";
}) {
  return (
    <div className="flex min-h-7 items-center gap-1">
      <span className="text-sm font-medium text-foreground">{label}</span>
      {help ? <FormInfo label={label} {...(side ? { side } : {})}>{help}</FormInfo> : null}
    </div>
  );
}
