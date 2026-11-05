import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ManagementClient, Sandbox } from "@mender/sdk";
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
  const client = { listSandboxes } as unknown as ManagementClient;
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

describe("SandboxesPage resource rows", () => {
  it("renders a readable full-width inventory with identity, status and isolation", async () => {
    mount();
    const collection = await screen.findByRole("list", { name: "Sandbox inventory" });
    expect(collection.getAttribute("data-slot")).toBe("sandbox-collection");
    expect(within(collection).getAllByRole("listitem")).toHaveLength(2);
    expect(within(collection).getByRole("link", { name: "Primary Workspace" })).toBeTruthy();
    expect(within(collection).getAllByText("Local workspace").length).toBeGreaterThanOrEqual(1);
    expect(within(collection).getByText("Ready")).toBeTruthy();
    expect(within(collection).getByText("D:/work/repo")).toBeTruthy();
    expect(within(collection).getByText("Host (not isolated)")).toBeTruthy();
    expect(within(collection).getByRole("link", { name: "Container Environment" })).toBeTruthy();
    expect(within(collection).getByText("Backend unavailable")).toBeTruthy();
    expect(within(collection).queryByRole("table")).toBeNull();
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
    expect(screen.queryByRole("list", { name: "Sandbox inventory" })).toBeNull();
  });
});
