import { ChevronLeftIcon, ChevronRightIcon } from "lucide-react";

import { formatNumber, t } from "../i18n/i18n";
import { IconButton } from "./icon-button";
import type { PaginationState } from "./pagination";
import { cn } from "@mender/ui/lib/utils";

export function TablePagination<T>({
  label,
  pagination,
  className,
}: {
  label: string;
  pagination: PaginationState<T>;
  className?: string;
}) {
  // A one-page inventory needs neither page navigation nor a page-size selector.
  // Keep the selector after increasing page size so users can still switch back.
  const smallestPageSize = pagination.pageSizeOptions[0] ?? pagination.pageSize;
  if (pagination.total <= smallestPageSize) {
    return null;
  }

  return (
    <nav
      aria-label={t("{{label}} pagination", { label: t(label) })}
      className={cn(
        "mt-4 flex flex-col gap-3 px-1 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between",
        className,
      )}
    >
      <p aria-live="polite">
        {t("Rows {{from}}–{{to}} of {{total}}", {
          from: formatNumber(pagination.from),
          to: formatNumber(pagination.to),
          total: formatNumber(pagination.total),
        })}
      </p>

      <div className="flex flex-wrap items-center gap-3">
        <label className="flex items-center gap-2 whitespace-nowrap">
          <span>{t("Rows per page")}</span>
          <select
            aria-label={t("{{label}} rows per page", { label: t(label) })}
            value={pagination.pageSize}
            onChange={(event) =>
              pagination.setPageSize(Number(event.target.value))
            }
            className="h-8 rounded-lg border border-input bg-background px-2 text-sm text-foreground outline-none focus:ring-2 focus:ring-ring/40"
          >
            {pagination.pageSizeOptions.map((pageSize) => (
              <option key={pageSize} value={pageSize}>
                {formatNumber(pageSize)}
              </option>
            ))}
          </select>
        </label>

        <span className="whitespace-nowrap">
          {t("Page {{page}} of {{pageCount}}", {
            page: formatNumber(pagination.page),
            pageCount: formatNumber(pagination.pageCount),
          })}
        </span>

        <div className="flex items-center gap-2">
          <IconButton
            type="button"
            variant="outline"
            label={t("{{label}} previous page", { label: t(label) })}
            disabled={pagination.page <= 1}
            onClick={() => pagination.setPage(pagination.page - 1)}
          >
            <ChevronLeftIcon />
          </IconButton>
          <IconButton
            type="button"
            variant="outline"
            label={t("{{label}} next page", { label: t(label) })}
            disabled={pagination.page >= pagination.pageCount}
            onClick={() => pagination.setPage(pagination.page + 1)}
          >
            <ChevronRightIcon />
          </IconButton>
        </div>
      </div>
    </nav>
  );
}
