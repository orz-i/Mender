import { ChevronLeftIcon, ChevronRightIcon } from "lucide-react";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@mender/ui/components/select";
import { cn } from "@mender/ui/lib/utils";

import { formatNumber, t } from "../i18n/i18n";
import { IconButton } from "./icon-button";
import type { PaginationState } from "./pagination";

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
        <div className="flex items-center gap-2 whitespace-nowrap">
          <span>{t("Rows per page")}</span>
          <Select
            value={String(pagination.pageSize)}
            onValueChange={(next) => {
              if (next !== null) pagination.setPageSize(Number(next));
            }}
          >
            <SelectTrigger
              size="sm"
              aria-label={t("{{label}} rows per page", { label: t(label) })}
              className="h-8 min-w-[70px] rounded-lg bg-background text-sm text-foreground"
            >
              <SelectValue>{formatNumber(pagination.pageSize)}</SelectValue>
            </SelectTrigger>
            <SelectContent className="min-w-[70px]">
              {pagination.pageSizeOptions.map((pageSize) => (
                <SelectItem key={pageSize} value={String(pageSize)}>
                  {formatNumber(pageSize)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

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
