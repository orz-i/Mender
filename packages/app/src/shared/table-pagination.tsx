import { ChevronLeftIcon, ChevronRightIcon } from "lucide-react";

import { t } from "../i18n/i18n";
import { IconButton } from "./icon-button";
import type { PaginationState } from "./pagination";

export function TablePagination<T>({
  label,
  pagination,
}: {
  label: string;
  pagination: PaginationState<T>;
}) {
  if (pagination.total === 0) {
    return null;
  }

  return (
    <nav
      aria-label={t("{{label}} pagination", { label: t(label) })}
      className="mt-4 flex flex-col gap-3 border-t border-border pt-4 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between"
    >
      <p aria-live="polite">
        {t("Rows {{from}}–{{to}} of {{total}}", {
          from: pagination.from,
          to: pagination.to,
          total: pagination.total,
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
                {pageSize}
              </option>
            ))}
          </select>
        </label>

        <span className="whitespace-nowrap">
          {t("Page {{page}} of {{pageCount}}", {
            page: pagination.page,
            pageCount: pagination.pageCount,
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
