import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { BellIcon } from "lucide-react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { InsightBanner } from "./insight-banner";

afterEach(cleanup);

describe("InsightBanner", () => {
  it("renders title, description and accessible region role", () => {
    render(
      <InsightBanner
        title="Optimization recommendation"
        description="Connect a local workspace sandbox to safely isolate tool runs."
      />,
    );

    const banner = screen.getByRole("region", { name: "Optimization recommendation" });
    expect(banner).toBeTruthy();
    expect(screen.getByText("Optimization recommendation")).toBeTruthy();
    expect(
      screen.getByText("Connect a local workspace sandbox to safely isolate tool runs."),
    ).toBeTruthy();
    expect(banner.getAttribute("data-slot")).toBe("insight-banner");
  });

  it("renders custom actions and triggers dismiss callback", () => {
    const onDismiss = vi.fn();
    const onAction = vi.fn();

    render(
      <InsightBanner
        title="New release available"
        description="A new driver update is available for installed plugins."
        icon={BellIcon}
        actions={
          <button type="button" onClick={onAction}>
            Update drivers
          </button>
        }
        onDismiss={onDismiss}
      />,
    );

    expect(screen.getByRole("button", { name: "Update drivers" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Update drivers" }));
    expect(onAction).toHaveBeenCalledTimes(1);

    const dismissBtn = screen.getByRole("button", { name: "Dismiss recommendation" });
    expect(dismissBtn).toBeTruthy();
    fireEvent.click(dismissBtn);
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });
});
