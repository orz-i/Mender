import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { PageTab, PageTabs, PageToolbar, ResultPanel } from "./page-toolbar";

afterEach(cleanup);

function ToolbarHarness() {
  const [category, setCategory] = useState<"all" | "installed">("all");

  return (
    <main>
      <PageToolbar
        tabs={
          <PageTabs label="Skills">
            <PageTab label="All Skills" count={5} active={category === "all"} onClick={() => setCategory("all")} />
            <PageTab label="Installed Skills" count={2} active={category === "installed"} onClick={() => setCategory("installed")} />
          </PageTabs>
        }
        filters={<input aria-label="Search Skills" />}
        actions={<button type="button">Install Skill</button>}
      />
      <ResultPanel title="Skill Library">
        <p>Result row</p>
        <nav aria-label="Pagination">1 / 1</nav>
      </ResultPanel>
    </main>
  );
}

describe("page-level toolbar and results surface", () => {
  it("renders all navigation/filter/actions above and outside a results-only card", () => {
    render(<ToolbarHarness />);

    const toolbar = document.querySelector('[data-slot="page-toolbar"]');
    const results = screen.getByRole("region", { name: "Skill Library" });
    expect(toolbar).not.toBeNull();
    expect(toolbar?.contains(screen.getByRole("button", { name: "Install Skill" }))).toBe(true);
    expect(toolbar?.contains(screen.getByRole("textbox", { name: "Search Skills" }))).toBe(true);
    expect(toolbar?.contains(screen.getByRole("button", { name: /All Skills/ }))).toBe(true);
    expect(results.contains(toolbar)).toBe(false);
    expect((toolbar?.compareDocumentPosition(results) ?? 0) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(within(results).getByText("Result row")).toBeTruthy();
    expect(within(results).getByRole("navigation", { name: "Pagination" })).toBeTruthy();
    expect(results.querySelector('[data-slot="card-header"]')).toBeNull();
  });

  it("preserves an accessible selected category and count when switching tabs", () => {
    render(<ToolbarHarness />);
    const all = screen.getByRole("button", { name: /All Skills/ });
    const installed = screen.getByRole("button", { name: /Installed Skills/ });
    expect(all.getAttribute("aria-pressed")).toBe("true");
    expect(installed.getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(installed);
    expect(installed.getAttribute("aria-pressed")).toBe("true");
    expect(all.getAttribute("aria-pressed")).toBe("false");
    expect(installed.textContent).toContain("2");
  });
});
