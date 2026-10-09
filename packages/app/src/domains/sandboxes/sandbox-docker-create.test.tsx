import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { MenderClient, Sandbox } from "@mender/sdk";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CreateSandboxPage } from "./sandboxes-page";

afterEach(cleanup);

const cachedImage = `docker.io/library/alpine@sha256:${"a".repeat(64)}`;
const created: Sandbox = {
  sandboxId: "sbx_0123456789abcdef0123456789abcdef",
  name: "Safe Container",
  backend: "docker",
  isolation: "container",
  state: "ready",
  localRoot: "",
  ownerKind: "local_user",
  ownerRef: "owner",
  activeLeases: 0,
  backendAvailable: true,
};

function showCreate(client: Partial<MenderClient>) {
  render(
    <MemoryRouter initialEntries={["/sandboxes/new"]}>
      <Routes>
        <Route path="/sandboxes/new" element={<CreateSandboxPage client={client as MenderClient} />} />
        <Route path="/sandboxes/:sandboxId" element={<div>Sandbox detail route reached</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

function runtimeGovernanceStub() {
  return { backend: { restrictedHostAvailable: false } };
}

describe("Docker Sandbox creation UX", () => {
  it("never offers Docker creation when the host Engine is not verified", async () => {
    const createDockerSandbox = vi.fn();
    showCreate({
      getRuntimeGovernance: vi.fn().mockResolvedValue(runtimeGovernanceStub()),
      getSandboxBackends: vi.fn().mockResolvedValue({
        dockerAvailable: false,
        dockerStatus: "Docker Engine socket has not been configured",
      }),
      createDockerSandbox,
    });
    expect(await screen.findByText(/Docker Engine socket has not been configured/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Docker/ })).toBeNull();
    expect(createDockerSandbox).not.toHaveBeenCalled();
  });

  it("requires a reviewed pinned image and creates through the typed Docker API", async () => {
    const createDockerSandbox = vi.fn().mockResolvedValue(created);
    showCreate({
      getRuntimeGovernance: vi.fn().mockResolvedValue(runtimeGovernanceStub()),
      getSandboxBackends: vi.fn().mockResolvedValue({ dockerAvailable: true, dockerStatus: "ready" }),
      createDockerSandbox,
    });

    fireEvent.click(await screen.findByRole("button", { name: /Docker/ }));
    const nameField = screen.getByRole<HTMLInputElement>("textbox", { name: "Sandbox name" });
    const digestField = screen.getByRole<HTMLInputElement>("textbox", { name: "Immutable image digest" });
    fireEvent.change(nameField, { target: { value: "Safe Container" } });
    fireEvent.change(digestField, { target: { value: "alpine:latest" } });
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Review" }).disabled).toBe(true);
    fireEvent.change(digestField, { target: { value: cachedImage } });
    fireEvent.click(screen.getByRole("button", { name: "Review" }));
    expect(screen.getByText(/Workspace files are ephemeral/i)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Create sandbox" }));
    await waitFor(() => expect(createDockerSandbox).toHaveBeenCalledWith({
      name: "Safe Container",
      imageDigest: cachedImage,
      userId: 10001,
      groupId: 10001,
      cpuMilli: 1000,
      memoryMiB: 512,
      workspaceMiB: 64,
      pids: 64,
    }));
    expect(await screen.findByText("Sandbox detail route reached")).toBeTruthy();
  });

  it("takes an uncertain persisted result to its detail instead of retriggering Docker create", async () => {
    const createDockerSandbox = vi.fn().mockResolvedValue({ ...created, state: "unknown" });
    showCreate({
      getRuntimeGovernance: vi.fn().mockResolvedValue(runtimeGovernanceStub()),
      getSandboxBackends: vi.fn().mockResolvedValue({ dockerAvailable: true, dockerStatus: "ready" }),
      createDockerSandbox,
    });
    fireEvent.click(await screen.findByRole("button", { name: /Docker/ }));
    fireEvent.change(screen.getByRole("textbox", { name: "Sandbox name" }), { target: { value: "Uncertain Container" } });
    fireEvent.change(screen.getByRole("textbox", { name: "Immutable image digest" }), { target: { value: cachedImage } });
    fireEvent.click(screen.getByRole("button", { name: "Review" }));
    fireEvent.click(screen.getByRole("button", { name: "Create sandbox" }));
    expect(await screen.findByText("Sandbox detail route reached")).toBeTruthy();
    expect(createDockerSandbox).toHaveBeenCalledTimes(1);
  });
});
