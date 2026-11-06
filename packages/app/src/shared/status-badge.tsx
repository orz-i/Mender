import type { ReactNode } from "react";

import { Badge } from "@mender/ui/components/badge";
import { cn } from "@mender/ui/lib/utils";

export type VisualStatus = "positive" | "in-progress" | "attention" | "negative" | "neutral";

/**
 * Pure presentation classification for statuses already supplied by Mender.
 * Never infer a Task outcome: unknown stays explicitly uncertain, not failed.
 */
function classifyVisualStatus(value: string): VisualStatus {
  switch (value.trim().toLowerCase().replaceAll("_", "-")) {
    case "completed":
    case "succeeded":
    case "success":
    case "healthy":
    case "ready":
    case "connected":
    case "admitted":
    case "enabled":
      return "positive";
    case "running":
    case "queued":
    case "starting":
    case "connecting":
    case "processing":
    case "in-progress":
      return "in-progress";
    case "unknown":
    case "degraded":
    case "pending":
    case "needs-attention":
    case "unavailable":
    case "denied":
      return "attention";
    case "failed":
    case "error":
      return "negative";
    default:
      return "neutral";
  }
}

export function StatusBadge({
  status,
  children,
  className,
}: {
  status: string;
  children?: ReactNode;
  className?: string;
}) {
  const visual = classifyVisualStatus(status);
  const variant =
    visual === "positive" ? "positive" :
    visual === "in-progress" ? "information" :
    visual === "attention" ? "caution" :
    visual === "negative" ? "destructive" : "outline";

  return (
    <Badge variant={variant} className={cn("max-w-full", className)} title={status}>
      <span className="truncate">{children ?? status}</span>
    </Badge>
  );
}


/** Quiet Magpie-style status for dense resource rows. Not a Task outcome
 * inference: the literal label remains supplied by the caller. */
export function StatusInline({
  status,
  children,
  className,
}: {
  status: string;
  children: ReactNode;
  className?: string;
}) {
  const visual = classifyVisualStatus(status);
  const color = visual === "positive" ? "bg-teal" :
    visual === "in-progress" ? "bg-primary" :
    visual === "attention" ? "bg-sunlight" :
    visual === "negative" ? "bg-destructive" : "bg-muted-foreground/65";
  return (
    <span data-slot="status-inline" className={cn(
      "inline-flex max-w-full items-center justify-end gap-1.5 text-[12px] leading-4 text-muted-foreground",
      className,
    )}>
      <span className={cn("size-[7px] shrink-0 rounded-full", color)} aria-hidden="true" />
      <span className="min-w-0 truncate" title={status}>{children}</span>
    </span>
  );
}
