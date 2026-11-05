import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { DetailWorkspace } from "./detail-workspace";

afterEach(cleanup);

function Harness() {
  const [active, setActive] = useState<"overview" | "tools" | "access">("overview");
  return <DetailWorkspace label="Agent details" active={active} onChange={setActive} sections={[
    { value: "overview", label: "Overview" },
    { value: "tools", label: "Tools", count: 7 },
    { value: "access", label: "Access", count: 0 },
  ]}><p>Topic: {active}</p></DetailWorkspace>;
}

describe("DetailWorkspace", () => {
  it("organizes an object's topics into one accessible local navigation", () => {
    render(<Harness />);
    const localNavigation = screen.getByRole("navigation", { name: "Agent details" });
    const group = within(localNavigation).getByRole("group", { name: "Agent details" });
    expect(localNavigation.getAttribute("data-slot")).toBe("detail-sections");
    expect(within(group).getByRole("button", { name: "Overview" }).getAttribute("aria-pressed")).toBe("true");
    // The count is visible, but it must not change the control's exact accessible name.
    const tools = within(group).getByRole("button", { name: "Tools" });
    expect(tools.textContent).toContain("7");
    expect(tools.getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(tools);
    expect(screen.getByText("Topic: tools")).toBeTruthy();
    expect(tools.getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(within(group).getByRole("button", { name: "Access" }));
    expect(screen.getByText("Topic: access")).toBeTruthy();
  });
});
