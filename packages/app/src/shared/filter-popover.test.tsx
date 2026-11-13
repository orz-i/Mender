import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { FilterPopover, FilterSelectField } from "./filter-popover";

afterEach(cleanup);

function FilterHarness({ onReset }: { onReset?: () => void }) {
  const [source, setSource] = useState("all");
  const [status, setStatus] = useState("all");

  const activeCount = (source !== "all" ? 1 : 0) + (status !== "all" ? 1 : 0);

  return (
    <FilterPopover
      activeCount={activeCount}
      onReset={() => {
        setSource("all");
        setStatus("all");
        onReset?.();
      }}
    >
      <FilterSelectField
        label="Skill source filter"
        value={source}
        onChange={setSource}
        options={[
          { value: "all", label: "All Sources" },
          { value: "builtin", label: "Built-in" },
          { value: "local", label: "Local" },
        ]}
      />
      <FilterSelectField
        label="Skill status filter"
        value={status}
        onChange={setStatus}
        options={[
          { value: "all", label: "All Status" },
          { value: "enabled", label: "Enabled" },
          { value: "disabled", label: "Disabled" },
        ]}
      />
    </FilterPopover>
  );
}

describe("FilterPopover", () => {
  it("renders trigger and expands popover with filters and active count", async () => {
    const handleReset = vi.fn();
    render(<FilterHarness onReset={handleReset} />);

    const trigger = screen.getByRole("button", { name: "Filters" });
    expect(trigger).toBeTruthy();
    expect(screen.queryByRole("combobox", { name: "Skill source filter" })).toBeNull();

    // Click trigger to open popover
    fireEvent.click(trigger);
    expect(screen.getByRole("combobox", { name: "Skill source filter" })).toBeTruthy();
    expect(screen.getByRole("combobox", { name: "Skill status filter" })).toBeTruthy();

    // Change a filter
    fireEvent.click(screen.getByRole("combobox", { name: "Skill source filter" }));
    const builtinOption = await screen.findByRole("option", { name: "Built-in" });
    fireEvent.pointerDown(builtinOption, { pointerId: 1, pointerType: "mouse", button: 0, isPrimary: true });
    fireEvent.pointerUp(builtinOption, { pointerId: 1, pointerType: "mouse", button: 0, isPrimary: true });
    fireEvent.click(builtinOption);

    // Check that reset button appears
    const reset = await screen.findByRole("button", { name: "Reset" });
    expect(reset).toBeTruthy();
    fireEvent.click(reset);

    expect(handleReset).toHaveBeenCalled();
  });
});
