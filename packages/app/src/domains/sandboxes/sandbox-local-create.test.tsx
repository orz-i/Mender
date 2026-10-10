import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { MenderClient, Sandbox } from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { CreateSandboxPage } from "./sandboxes-page";

const created: Sandbox = {
  sandboxId: "sbx_0123456789abcdef0123456789abcdef",
  name: "Local project",
  backend: "local_workspace",
  isolation: "host",
  state: "ready",
  localRoot: "/Users/dev/project",
  ownerKind: "local_user",
  ownerRef: "owner",
  activeLeases: 0,
  backendAvailable: true,
};

function renderLocal(chooseLocalDirectory?: () => Promise<string | null>) {
  const createLocalSandbox = vi.fn().mockResolvedValue(created);
  const client = {
    getRuntimeGovernance: vi.fn().mockResolvedValue({ backend: { restrictedHostAvailable: false } }),
    getSandboxBackends: vi.fn().mockResolvedValue({ dockerAvailable: false, daytonaConfigured: false }),
    listCredentials: vi.fn().mockResolvedValue({ credentials: [] }),
    createLocalSandbox,
  } as unknown as MenderClient;

  render(
    <TooltipProvider>
      <MemoryRouter initialEntries={["/sandboxes/new"]}>
        <Routes>
          <Route path="/sandboxes/new" element={
            <CreateSandboxPage client={client} {...(chooseLocalDirectory === undefined ? {} : { chooseLocalDirectory })} />
          } />
          <Route path="/sandboxes/:sandboxId" element={<div>Sandbox detail reached</div>} />
        </Routes>
      </MemoryRouter>
    </TooltipProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: /^Local workspace/ }));
  return createLocalSandbox;
}

beforeEach(() => {
  window.localStorage.clear();
  resetLocaleForTesting();
  setLocale("en");
});

afterEach(() => {
  cleanup();
  resetLocaleForTesting();
});

describe("local Sandbox directory selection", () => {
  it("fills the editable absolute path from the desktop directory picker and submits the reviewed path", async () => {
    const chooseLocalDirectory = vi.fn().mockResolvedValue("/Users/dev/project");
    const createLocalSandbox = renderLocal(chooseLocalDirectory);
    const root = screen.getByRole<HTMLInputElement>("textbox", { name: "Absolute local directory" });

    fireEvent.change(screen.getByRole("textbox", { name: "Sandbox name" }), {
      target: { value: "Local project" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Choose local directory" }));
    await waitFor(() => expect(root.value).toBe("/Users/dev/project"));
    expect(chooseLocalDirectory).toHaveBeenCalledTimes(1);

    fireEvent.change(screen.getByRole("combobox", { name: "Isolation" }), {
      target: { value: "host" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Review" }));
    expect(screen.getByText("/Users/dev/project")).toBeTruthy();
    expect(createLocalSandbox).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Create sandbox" }));
    await waitFor(() => expect(createLocalSandbox).toHaveBeenCalledWith({
      name: "Local project",
      root: "/Users/dev/project",
      isolation: "host",
    }));
    expect(await screen.findByText("Sandbox detail reached")).toBeTruthy();
  });

  it("does not overwrite a manually entered path when the directory dialog is cancelled", async () => {
    const chooseLocalDirectory = vi.fn().mockResolvedValue(null);
    renderLocal(chooseLocalDirectory);
    const root = screen.getByRole<HTMLInputElement>("textbox", { name: "Absolute local directory" });
    fireEvent.change(root, { target: { value: "/Users/dev/existing" } });

    fireEvent.click(screen.getByRole("button", { name: "Choose local directory" }));
    await waitFor(() => expect(screen.getByRole<HTMLButtonElement>("button", {
      name: "Choose local directory",
    }).disabled).toBe(false));
    expect(chooseLocalDirectory).toHaveBeenCalledOnce();
    expect(root.value).toBe("/Users/dev/existing");
  });

  it("shows picker errors and allows retry without losing the manually entered path", async () => {
    const chooseLocalDirectory = vi.fn()
      .mockRejectedValueOnce(new Error("Directory picker unavailable"))
      .mockResolvedValueOnce("/Users/dev/selected");
    renderLocal(chooseLocalDirectory);
    const root = screen.getByRole<HTMLInputElement>("textbox", { name: "Absolute local directory" });
    fireEvent.change(root, { target: { value: "/Users/dev/manual" } });

    fireEvent.click(screen.getByRole("button", { name: "Choose local directory" }));
    expect(await screen.findByText("Directory picker unavailable")).toBeTruthy();
    expect(root.value).toBe("/Users/dev/manual");

    fireEvent.click(screen.getByRole("button", { name: "Choose local directory" }));
    await waitFor(() => expect(root.value).toBe("/Users/dev/selected"));
    expect(screen.queryByText("Directory picker unavailable")).toBeNull();
  });

  it("keeps manual path entry available when the web host has no directory picker", () => {
    renderLocal();
    const root = screen.getByRole<HTMLInputElement>("textbox", { name: "Absolute local directory" });
    expect(screen.queryByRole("button", { name: "Choose local directory" })).toBeNull();
    fireEvent.change(root, { target: { value: "/Users/dev/manual" } });
    expect(root.value).toBe("/Users/dev/manual");
  });
});
