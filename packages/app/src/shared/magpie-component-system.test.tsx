import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { BoxesIcon } from "lucide-react";
import { useState } from "react";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it } from "vitest";

import { DataPanel, SearchField } from "./workbench";
import { ResourceList, ResourceRow } from "./resource-list";
import { StatusInline } from "./status-badge";
import { EmptyPanelState } from "./workbench";
import { EmptyState } from "./page";
import { DetailWorkspace } from "./detail-workspace";

afterEach(cleanup);

function SearchHarness() {
  const [query, setQuery] = useState("");
  return <SearchField value={query} onChange={setQuery} label="Search profiles" placeholder="Search profiles…" />;
}

describe("Magpie-compatible component primitives", () => {
  it("keeps panel titles text-first with a compact header and accessible actions", () => {
    render(
      <DataPanel title="Profiles" description="Selected Tools" actions={<button type="button">Edit</button>}>
        <p>Two configured profiles</p>
      </DataPanel>,
    );
    const panel = document.querySelector('[data-kind="data-panel"]')!;
    expect(panel.className).toContain("rounded-xl");
    expect(panel.className).not.toContain("rounded-[15px]");
    const heading = within(panel as HTMLElement).getByText("Profiles");
    expect(heading).toBeTruthy();
    expect(panel.querySelector('[data-slot="card-header"] svg')).toBeNull();
    expect(within(panel as HTMLElement).getByText("Selected Tools")).toBeTruthy();
    expect(within(panel as HTMLElement).getByRole("button", { name: "Edit" })).toBeTruthy();
  });

  it("clears a nonempty search by a button or Escape without changing unrelated UI state", () => {
    render(<SearchHarness />);
    const search = screen.getByRole<HTMLInputElement>("textbox", { name: "Search profiles" });
    expect(screen.queryByRole("button", { name: "Clear search" })).toBeNull();
    fireEvent.change(search, { target: { value: "work" } });
    expect(search.value).toBe("work");
    fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
    expect(search.value).toBe("");
    fireEvent.change(search, { target: { value: "development" } });
    fireEvent.keyDown(search, { key: "Escape" });
    expect(search.value).toBe("");
    expect(screen.queryByRole("button", { name: "Clear search" })).toBeNull();
  });

  it("keeps one accessible navigation target per resource and quiet inline status", () => {
    render(<MemoryRouter><ResourceList label="Sandbox inventory" slot="sandbox-collection">
      <ResourceRow icon={BoxesIcon} title="Local sandbox" label="Local sandbox"
        to="/sandboxes/sbx_a" description="Local workspace · Restricted host"
        detail="Workspace files" status={<StatusInline status="ready">Ready</StatusInline>} />
      <ResourceRow icon={BoxesIcon} title="Remote host" label="Remote host"
        to="/sandboxes/sbx_b" description="SSH Remote · Host"
        status={<StatusInline status="unknown">Unknown</StatusInline>} />
    </ResourceList></MemoryRouter>);

    const list = screen.getByRole("list", { name: "Sandbox inventory" });
    expect(within(list).getAllByRole("listitem")).toHaveLength(2);
    expect(within(list).getByRole("link", { name: "Local sandbox" }).getAttribute("href")).toBe("/sandboxes/sbx_a");
    expect(list.querySelectorAll('[data-slot="resource-icon"]')).toHaveLength(2);
    expect(within(list).getByText("Unknown").closest('[data-slot="status-inline"]')?.querySelector("span")?.className)
      .toContain("bg-sunlight");
    expect(within(list).getByText("Ready")).toBeTruthy();
  });

  it("retains a horizontal, stateful object tab strip and never squeezes content into a secondary sidebar", () => {
    function DetailHarness() {
      const [topic, setTopic] = useState<"overview" | "tools">("overview");
      return <DetailWorkspace label="Agent details" sections={[
        { value: "overview", label: "Overview" },
        { value: "tools", label: "Tools", count: 8 },
      ]} active={topic} onChange={setTopic}>
        <p>Topic: {topic}</p>
      </DetailWorkspace>;
    }
    render(<DetailHarness />);
    const workspace = document.querySelector('[data-slot="detail-workspace"]')!;
    const nav = screen.getByRole("navigation", { name: "Agent details" });
    expect(workspace.className).toContain("flex");
    expect(workspace.className).not.toContain("grid");
    expect(nav.className).toContain("overflow-x-auto");
    const toolTab = within(nav).getByRole("button", { name: "Tools" });
    expect(toolTab.textContent).toContain("8");
    fireEvent.click(toolTab);
    expect(toolTab.getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByText("Topic: tools")).toBeTruthy();
  });

  it("keeps empty guidance adjacent to the page rather than another tall dashed card", () => {
    render(<EmptyPanelState><EmptyState title="No sandboxes" description="Create a local sandbox first." /></EmptyPanelState>);
    expect(document.querySelector('[data-slot="empty-panel-state"]')?.className).toContain("min-h-[154px]");
    expect(document.querySelector('[data-slot="empty-state"]')?.className).not.toContain("border-dashed");
    expect(screen.getByText("Create a local sandbox first.")).toBeTruthy();
  });
});
