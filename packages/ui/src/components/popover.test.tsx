import { render, screen, fireEvent } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Popover, PopoverContent, PopoverTrigger } from "./popover";

describe("Popover component", () => {
  it("renders trigger and displays content when opened", () => {
    render(
      <Popover>
        <PopoverTrigger render={<button type="button">Open Popover</button>} />
        <PopoverContent>
          <p>Popover content body</p>
        </PopoverContent>
      </Popover>
    );

    const trigger = screen.getByRole("button", { name: "Open Popover" });
    expect(trigger).toBeTruthy();
    expect(screen.queryByText("Popover content body")).toBeNull();

    fireEvent.click(trigger);
    expect(screen.getByText("Popover content body")).toBeTruthy();
    const content = document.querySelector('[data-slot="popover-content"]');
    expect(content?.classList.contains("z-50")).toBe(true);
  });
});
