import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { MenderClient, Sandbox } from "@mender/sdk";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CreateSandboxPage } from "./sandboxes-page";

afterEach(cleanup);

// Deliberately synthetic host key data; backend cryptographic tests use generated keys.
const hostKey = "ssh-ed25519 " + "A".repeat(68);
const credential = {
  reference: "credential://ssh/build",
  namespace: "ssh",
  name: "build",
  displayName: "SSH build key",
  source: "native" as const,
  environmentVariable: "",
  createdAt: "2026-10-11T00:00:00Z",
  updatedAt: "2026-10-11T00:00:00Z",
};
const sandbox: Sandbox = {
  sandboxId: "sbx_00000000000000000000000000000123",
  name: "Remote Dev",
  backend: "ssh_remote",
  isolation: "remote",
  state: "ready",
  localRoot: "",
  ownerKind: "local_user",
  ownerRef: "os:fixture",
  activeLeases: 0,
  backendAvailable: true,
};

function renderSSH(overrides: Partial<MenderClient> = {}, builtin = false) {
  const client: Partial<MenderClient> = {
    getRuntimeGovernance: vi.fn().mockResolvedValue({ backend: { restrictedHostAvailable: false } }),
    getSandboxBackends: vi.fn().mockResolvedValue({
      dockerAvailable: false,
      dockerStatus: "not detected",
      daytonaConfigured: false,
      daytonaStatus: "disabled",
    }),
    listCredentials: vi.fn().mockResolvedValue({
      credentials: [credential, { ...credential, reference: "credential://other/api", namespace: "other", source: "environment" }],
    }),
    createSSHRemoteSandbox: vi.fn().mockResolvedValue(sandbox),
    ...overrides,
  };
  const location = builtin ? "/sandboxes/new?pluginId=io.mender.sandbox.ssh-remote" : "/sandboxes/new";
  render(
    <MemoryRouter initialEntries={[location]}>
      <Routes>
        <Route path="/sandboxes/new" element={<CreateSandboxPage client={client as MenderClient} />} />
        <Route path="/sandboxes/:sandboxId" element={<div>SSH Sandbox details</div>} />
        <Route path="/plugins/:key" element={<div>SSH plugin details</div>} />
      </Routes>
    </MemoryRouter>,
  );
  return client;
}

function fillSSHFields(user = "agent", key = hostKey) {
  fireEvent.change(screen.getByRole("textbox", { name: "Sandbox name" }), { target: { value: "Remote Dev" } });
  fireEvent.change(screen.getByRole("textbox", { name: "SSH host" }), { target: { value: "203.0.113.25" } });
  fireEvent.change(screen.getByRole("spinbutton", { name: "SSH port" }), { target: { value: "2222" } });
  fireEvent.change(screen.getByRole("textbox", { name: "SSH login user" }), { target: { value: user } });
  fireEvent.change(screen.getByRole("textbox", { name: "Remote workspace root" }), {
    target: { value: "/data/workspace" },
  });
  fireEvent.change(screen.getByRole("textbox", { name: "Pinned SSH host public key" }), {
    target: { value: key },
  });
}

describe("SSH Remote Sandbox creation", () => {
  it("always permits attempting Review and explains missing SSH fields without provisioning", async () => {
    const client = renderSSH({}, true);
    expect(await screen.findByRole("textbox", { name: "Sandbox name" })).toBeTruthy();
    expect(screen.queryByLabelText("Cached Docker image")).toBeNull();
    expect(screen.queryByLabelText("Mender Credential Reference")).toBeNull();
    expect(screen.getByText(/SSH Remote is not container or VM isolation/)).toBeTruthy();

    const review = screen.getByRole<HTMLButtonElement>("button", { name: "Review" });
    expect(review.disabled).toBe(false);
    fireEvent.click(review);
    expect(screen.getByText("Enter a sandbox name (up to 160 bytes).")).toBeTruthy();
    expect(screen.getByText("Enter a valid SSH hostname or IP address.")).toBeTruthy();
    expect(screen.getByText("Choose an existing SSH private key credential.")).toBeTruthy();
    expect(screen.getByRole("status").textContent).toContain("Check these SSH fields before review");
    expect(screen.queryByText("Review Sandbox authority")).toBeNull();
    expect(client.createSSHRemoteSandbox).not.toHaveBeenCalled();
  });

  it("warns on root login, but only blocks malformed host keys", async () => {
    const client = renderSSH({}, true);
    await screen.findByRole("textbox", { name: "Sandbox name" });
    fillSSHFields("root", "AAAAB3NzaC1yc2EAAAADAQABAAAB");
    const user = screen.getByRole<HTMLInputElement>("textbox", { name: "SSH login user" });
    const key = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Pinned SSH host public key" });

    expect(user.getAttribute("aria-invalid")).toBe("false");
    expect(key.getAttribute("aria-invalid")).toBe("true");
    expect(screen.getByText(/Root SSH login has system-wide privileges/)).toBeTruthy();
    expect(screen.getByText(/Paste the full SSH host public key/)).toBeTruthy();

    await screen.findByRole("option", { name: /SSH build key/ });
    fireEvent.change(screen.getByRole("combobox", { name: "SSH private key credential" }), {
      target: { value: credential.reference },
    });
    const review = screen.getByRole<HTMLButtonElement>("button", { name: "Review" });
    expect(review.disabled).toBe(false);
    fireEvent.click(review);
    expect(screen.getByRole("status").textContent).not.toContain("SSH login user");
    expect(screen.getByRole("status").textContent).toContain("Pinned SSH host public key");
    expect(screen.queryByText("Review Sandbox authority")).toBeNull();
    expect(client.createSSHRemoteSandbox).not.toHaveBeenCalled();

    fireEvent.change(key, { target: { value: hostKey + "\n" } });
    expect(key.getAttribute("aria-invalid")).toBe("false");
    fireEvent.click(review);
    expect(screen.getByText("Remote account (not isolated)")).toBeTruthy();
    expect(screen.getByText(/Root SSH login has system-wide privileges/)).toBeTruthy();
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Create sandbox" }).disabled).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "Create sandbox" }));
    await waitFor(() => expect(client.createSSHRemoteSandbox).toHaveBeenCalledWith({
      name: "Remote Dev",
      host: "203.0.113.25",
      port: 2222,
      user: "root",
      root: "/data/workspace",
      hostPublicKey: hostKey, // pasted terminal newline removed
      credentialRef: credential.reference,
    }));
    expect(await screen.findByText("SSH Sandbox details")).toBeTruthy();
  });

  it("rejects known_hosts hostname-prefix lines with clear guidance", async () => {
    renderSSH({}, true);
    await screen.findByRole("textbox", { name: "Sandbox name" });
    fillSSHFields("agent", "203.0.113.25 " + hostKey);
    expect(screen.getByText(/Remove the hostname from the known_hosts line/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Review" }));
    expect(screen.queryByText("Review Sandbox authority")).toBeNull();
  });

  it("does not provision a sandbox without a selected native SSH credential", async () => {
    const client = renderSSH({
      listCredentials: vi.fn().mockResolvedValue({ credentials: [] }),
    });
    fireEvent.click(await screen.findByRole("button", { name: /SSH Remote/ }));
    expect(await screen.findByText(/No SSH key credentials/)).toBeTruthy();
    fillSSHFields();
    fireEvent.click(screen.getByRole("button", { name: "Review" }));
    expect(screen.getByText("Choose an existing SSH private key credential.")).toBeTruthy();
    expect(screen.queryByText("Review Sandbox authority")).toBeNull();
    expect(client.createSSHRemoteSandbox).not.toHaveBeenCalled();
  });

  it("returns to the selected plugin instead of showing unrelated configuration", async () => {
    renderSSH({}, true);
    fireEvent.click(await screen.findByRole("button", { name: "Back" }));
    expect(await screen.findByText("SSH plugin details")).toBeTruthy();
  });
});
