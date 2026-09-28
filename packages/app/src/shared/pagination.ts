import { useMemo, useState } from "react";

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50] as const;

export interface PaginationState<T> {
  items: T[];
  page: number;
  pageSize: number;
  pageCount: number;
  total: number;
  from: number;
  to: number;
  pageSizeOptions: number[];
  setPage: (page: number) => void;
  setPageSize: (pageSize: number) => void;
  reset: () => void;
}

export function usePagination<T>(
  items: readonly T[],
  {
    initialPageSize = 10,
    pageSizeOptions = DEFAULT_PAGE_SIZE_OPTIONS,
  }: {
    initialPageSize?: number;
    pageSizeOptions?: readonly number[];
  } = {},
): PaginationState<T> {
  const normalizedPageSizeOptions = useMemo(
    () =>
      Array.from(new Set([...pageSizeOptions, initialPageSize]))
        .filter((value) => Number.isInteger(value) && value > 0)
        .sort((left, right) => left - right),
    [initialPageSize, pageSizeOptions],
  );

  const safeInitialPageSize =
    normalizedPageSizeOptions.includes(initialPageSize)
      ? initialPageSize
      : normalizedPageSizeOptions[0] ?? 10;

  const [page, setPageState] = useState(1);
  const [pageSize, setPageSizeState] = useState(safeInitialPageSize);

  const total = items.length;
  const pageCount = Math.max(1, Math.ceil(total / pageSize));
  const currentPage = Math.min(page, pageCount);
  const start = (currentPage - 1) * pageSize;
  const paginatedItems = items.slice(start, start + pageSize);
  const from = total === 0 ? 0 : start + 1;
  const to = total === 0 ? 0 : Math.min(start + pageSize, total);

  function setPage(nextPage: number) {
    if (!Number.isFinite(nextPage)) {
      return;
    }
    setPageState(Math.min(Math.max(1, Math.trunc(nextPage)), pageCount));
  }

  function setPageSize(nextPageSize: number) {
    if (!normalizedPageSizeOptions.includes(nextPageSize)) {
      return;
    }
    setPageSizeState(nextPageSize);
    setPageState(1);
  }

  return {
    items: paginatedItems,
    page: currentPage,
    pageSize,
    pageCount,
    total,
    from,
    to,
    pageSizeOptions: normalizedPageSizeOptions,
    setPage,
    setPageSize,
    reset: () => setPageState(1),
  };
}
