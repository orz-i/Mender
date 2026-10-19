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
