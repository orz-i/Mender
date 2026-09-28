import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { usePagination } from "./pagination";
import { TablePagination } from "./table-pagination";

afterEach(cleanup);

function PaginationHarness({ items }: { items: string[] }) {
  const pagination = usePagination(items, {
    initialPageSize: 2,
    pageSizeOptions: [2, 3],
  });

  return (
    <>
      <ul>
        {pagination.items.map((item) => (
          <li key={item}>{item}</li>
        ))}
      </ul>
      <TablePagination label="Test rows" pagination={pagination} />
    </>
  );
}

describe("table pagination", () => {
  it("navigates, reports the visible range, and resets after a page-size change", () => {
    render(<PaginationHarness items={["one", "two", "three", "four", "five"]} />);

    expect(screen.getByText("Rows 1–2 of 5")).toBeTruthy();
    expect(screen.getByText("Page 1 of 3")).toBeTruthy();
    expect(screen.getByText("one")).toBeTruthy();
    expect(screen.queryByText("three")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Test rows next page" }));

    expect(screen.getByText("Rows 3–4 of 5")).toBeTruthy();
    expect(screen.getByText("Page 2 of 3")).toBeTruthy();
    expect(screen.getByText("three")).toBeTruthy();
    expect(screen.queryByText("one")).toBeNull();

    fireEvent.change(screen.getByLabelText("Test rows rows per page"), {
      target: { value: "3" },
    });

    expect(screen.getByText("Rows 1–3 of 5")).toBeTruthy();
    expect(screen.getByText("Page 1 of 2")).toBeTruthy();
    expect(screen.getByText("one")).toBeTruthy();
  });

  it("clamps the current page when the collection shrinks", () => {
    const { rerender } = render(
      <PaginationHarness items={["one", "two", "three", "four", "five"]} />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Test rows next page" }));
    fireEvent.click(screen.getByRole("button", { name: "Test rows next page" }));
    expect(screen.getByText("Page 3 of 3")).toBeTruthy();
    expect(screen.getByText("five")).toBeTruthy();

    rerender(<PaginationHarness items={["one", "two", "three"]} />);

    expect(screen.getByText("Rows 3–3 of 3")).toBeTruthy();
    expect(screen.getByText("Page 2 of 2")).toBeTruthy();
    expect(screen.getByText("three")).toBeTruthy();
  });

  it("does not render controls for an empty collection", () => {
    render(<PaginationHarness items={[]} />);

    expect(screen.queryByRole("navigation", { name: "Test rows pagination" })).toBeNull();
  });
});
