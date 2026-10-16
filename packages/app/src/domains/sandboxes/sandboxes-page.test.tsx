import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { MenderClient, Sandbox } from "@mender/sdk";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { SandboxesPage } from "./sandboxes-page";

const sandboxes: Sandbox[] = [
  {
    sandboxId: "sbx_local_01",
    name: "Primary Workspace",
    backend: "local_workspace",
    isolation: "host",
    state: "ready",
    localRoot: "D:/work/repo",
    backendAvailable: true,
    ownerKind: "local_user",
    ownerRef: "os:501",
    activeLeases: 0,
  },
  {
    sandboxId: "sbx_docker_02",
    name: "Container Environment",
    backend: "docker",
    isolation: "container",
    state: "failed",
    localRoot: "",
    backendAvailable: false,
    ownerKind: "local_user",
    ownerRef: "os:501",
    activeLeases: 0,
  },
];

function mount(items: Sandbox[] = sandboxes) {
  const listSandboxes = vi.fn().mockResolvedValue({ sandboxes: items });
  const client = { listSandboxes } as unknown as MenderClient;
  render(
    <MemoryRouter>
      <SandboxesPage client={client} />
    </MemoryRouter>,
  );
  return { listSandboxes };
}

beforeEach(() => {
  resetLocaleForTesting();
  setLocale("en");
});

afterEach(() => {
  cleanup();
  resetLocaleForTesting();
});

describe("SandboxesPage responsive table", () => {
  it("renders mobile-card-ready table layout and responsive grid classes", async () => {
    mount();
    const table = await screen.findByRole("table", { name: "Sandbox inventory" });
    expect(table).toBeTruthy();
    expect(table.className).toContain("max-md:block");

    const header = table.querySelector("thead");
    expect(header?.className).toContain("max-md:sr-only");

    const rows = table.querySelectorAll("tbody tr");
    expect(rows.length).toBe(2);
    expect(rows[0]?.className).toContain("max-md:grid");

    expect(within(table).getByText("Primary Workspace")).toBeTruthy();
    expect(within(table).getAllByText("Local workspace").length).toBeGreaterThanOrEqual(1);
    expect(within(table).getByText("Ready")).toBeTruthy();
    expect(within(table).getByText("D:/work/repo")).toBeTruthy();

    expect(within(table).getByText("Container Environment")).toBeTruthy();
    expect(within(table).getByText("Backend unavailable")).toBeTruthy();
  });

  it("filters sandboxes by query and category tab", async () => {
    mount();
    expect(await screen.findByText("Primary Workspace")).toBeTruthy();
    expect(screen.getByText("Container Environment")).toBeTruthy();

    fireEvent.change(screen.getByRole("textbox", { name: "Search sandboxes" }), {
      target: { value: "Primary" },
    });
    expect(screen.getByText("Primary Workspace")).toBeTruthy();
    expect(screen.queryByText("Container Environment")).toBeNull();

    fireEvent.change(screen.getByRole("textbox", { name: "Search sandboxes" }), {
      target: { value: "" },
    });
    expect(screen.getByText("Container Environment")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: /Ready sandboxes/ }));
    expect(screen.getByText("Primary Workspace")).toBeTruthy();
    expect(screen.queryByText("Container Environment")).toBeNull();
  });

  it("displays empty state when inventory has no items", async () => {
    mount([]);
    expect(await screen.findByText("No sandboxes")).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
  });
});
