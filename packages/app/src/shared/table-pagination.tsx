import { Button } from "@mender/ui/components/button";

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
      aria-label={`${label} pagination`}
      className="mt-4 flex flex-col gap-3 border-t border-border pt-4 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between"
    >
      <p aria-live="polite">
        Rows {pagination.from}–{pagination.to} of {pagination.total}
      </p>

      <div className="flex flex-wrap items-center gap-3">
        <label className="flex items-center gap-2 whitespace-nowrap">
          <span>Rows per page</span>
          <select
            aria-label={`${label} rows per page`}
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
          Page {pagination.page} of {pagination.pageCount}
        </span>

        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            aria-label={`${label} previous page`}
            disabled={pagination.page <= 1}
            onClick={() => pagination.setPage(pagination.page - 1)}
          >
            Previous
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            aria-label={`${label} next page`}
            disabled={pagination.page >= pagination.pageCount}
            onClick={() => pagination.setPage(pagination.page + 1)}
          >
            Next
          </Button>
        </div>
      </div>
    </nav>
  );
}
