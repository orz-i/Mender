import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { StatusBadge } from "./status-badge";

afterEach(cleanup);

describe("status badge semantics", () => {
  it("visually distinguishes uncertain, completed, active and failed status without inferring outcomes", () => {
    render(<>
      <StatusBadge status="unknown" />
      <StatusBadge status="completed" />
      <StatusBadge status="running" />
      <StatusBadge status="failed" />
      <StatusBadge status="cancelled" />
    </>);
    expect(screen.getByText("unknown").closest('[data-slot="badge"]')?.className).toContain("bg-sunlight/20");
    expect(screen.getByText("completed").closest('[data-slot="badge"]')?.className).toContain("bg-accent");
    expect(screen.getByText("running").closest('[data-slot="badge"]')?.className).toContain("bg-teal/10");
    expect(screen.getByText("failed").closest('[data-slot="badge"]')?.className).toContain("bg-destructive/10");
    expect(screen.getByText("cancelled").closest('[data-slot="badge"]')?.className).toContain("bg-muted/30");
  });

  it("preserves the underlying status text or a localized label", () => {
    render(<><StatusBadge status="unknown">未知</StatusBadge><StatusBadge status="completed" /></>);
    expect(screen.getByText("未知").closest('[data-slot="badge"]')?.getAttribute("title")).toBe("unknown");
    expect(screen.getByText("completed")).toBeTruthy();
  });
});
