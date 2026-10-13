import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { MenderClient, Sandbox, SSHHostIdentity } from "@mender/sdk";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CreateSandboxPage } from "./sandboxes-page";

afterEach(cleanup);

// Synthetic public host key; protocol tests generate real test-only keys.
const hostKey = "ssh-ed25519 " + "A".repeat(68);
const hostIdentity: SSHHostIdentity = {
  status: "untrusted",
  algorithm: "ssh-ed25519",
  fingerprint: "SHA256:fixture-host-fingerprint",
  hostPublicKey: hostKey,
};
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
      dockerAvailable: false, dockerStatus: "not detected",
      daytonaConfigured: false, daytonaStatus: "disabled",
    }),
    listCredentials: vi.fn().mockResolvedValue({
      credentials: [credential, {
        ...credential, reference: "credential://other/api", namespace: "other", source: "environment",
      }],
    }),
    inspectSSHHostKey: vi.fn().mockResolvedValue(hostIdentity),
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

function fillSSHFields(user = "agent") {
  fireEvent.change(screen.getByRole("textbox", { name: "Sandbox name" }), { target: { value: "Remote Dev" } });
  fireEvent.change(screen.getByRole("textbox", { name: "SSH host" }), { target: { value: "203.0.113.25" } });
  fireEvent.change(screen.getByRole("spinbutton", { name: "SSH port" }), { target: { value: "2222" } });
  fireEvent.change(screen.getByRole("textbox", { name: "SSH login user" }), { target: { value: user } });
  fireEvent.change(screen.getByRole("textbox", { name: "Remote workspace root" }), {
    target: { value: "/data/workspace" },
  });
}
async function readyForCreation(user = "root") {
  await screen.findByRole("textbox", { name: "Sandbox name" });
  fillSSHFields(user);
  await screen.findByRole("option", { name: /SSH build key/ });
  fireEvent.change(screen.getByRole("combobox", { name: "SSH private key credential" }), {
    target: { value: credential.reference },
  });
  fireEvent.click(screen.getByRole("button", { name: "Review" }));
  expect(screen.getByText("Remote account (not isolated)")).toBeTruthy();
}

describe("SSH Remote Sandbox with automatic host identity", () => {
  it("removes required host public key field and reports other missing fields", async () => {
    const client = renderSSH({}, true);
    expect(await screen.findByRole("textbox", { name: "Sandbox name" })).toBeTruthy();
    expect(screen.queryByRole("textbox", { name: "Pinned SSH host public key" })).toBeNull();
    expect(screen.getByText(/Mender checks your known_hosts automatically/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Review" }));
    expect(screen.getByText("Enter a sandbox name (up to 160 bytes).")).toBeTruthy();
    expect(screen.getByText("Choose an existing SSH private key credential.")).toBeTruthy();
    expect(client.inspectSSHHostKey).not.toHaveBeenCalled();
  });

  it("first connection shows server fingerprint, cancellation does not create, confirmation pins exact key", async () => {
    const client = renderSSH({}, true);
    await readyForCreation("root");
    expect(screen.getByText(/Root SSH login has system-wide privileges/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Create sandbox" }));
    expect(await screen.findByRole("dialog", { name: "Confirm SSH server identity" })).toBeTruthy();
    expect(screen.getByText("SHA256:fixture-host-fingerprint")).toBeTruthy();
    expect(screen.getByText(/A login key from your cloud console is not the server host key/)).toBeTruthy();
    expect(client.inspectSSHHostKey).toHaveBeenCalledWith("203.0.113.25", 2222);
    expect(client.createSSHRemoteSandbox).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog", { name: "Confirm SSH server identity" })).toBeNull();
    expect(client.createSSHRemoteSandbox).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Create sandbox" }));
    await screen.findByRole("dialog", { name: "Confirm SSH server identity" });
    fireEvent.click(screen.getByRole("button", { name: "I verified the fingerprint — create sandbox" }));
    await waitFor(() => expect(client.createSSHRemoteSandbox).toHaveBeenCalledWith({
      name: "Remote Dev", host: "203.0.113.25", port: 2222, user: "root",
      root: "/data/workspace", hostPublicKey: hostKey, credentialRef: credential.reference,
    }));
    expect(await screen.findByText("SSH Sandbox details")).toBeTruthy();
  });

  it("reuses an existing known_hosts pin automatically without another dialog", async () => {
    const client = renderSSH({
      inspectSSHHostKey: vi.fn().mockResolvedValue({...hostIdentity, status:"trusted"}),
    }, true);
    await readyForCreation();
    fireEvent.click(screen.getByRole("button", { name: "Create sandbox" }));
    await waitFor(() => expect(client.createSSHRemoteSandbox).toHaveBeenCalledExactlyOnceWith({
      name: "Remote Dev", host: "203.0.113.25", port: 2222, user: "root",
      root: "/data/workspace", hostPublicKey: hostKey, credentialRef: credential.reference,
    }));
    expect(screen.queryByRole("dialog", { name: "Confirm SSH server identity" })).toBeNull();
    expect(await screen.findByText("SSH Sandbox details")).toBeTruthy();
  });

  it("blocks changed trusted server keys, without allowing a one-click override", async () => {
    const client = renderSSH({
      inspectSSHHostKey: vi.fn().mockResolvedValue({...hostIdentity, status:"changed"}),
    }, true);
    await readyForCreation();
    fireEvent.click(screen.getByRole("button", { name: "Create sandbox" }));
    expect(await screen.findByText(/SSH server host key differs from previously trusted records/)).toBeTruthy();
    expect(screen.queryByRole("dialog", { name: "Confirm SSH server identity" })).toBeNull();
    expect(client.createSSHRemoteSandbox).not.toHaveBeenCalled();
  });

  it("never leaks untrusted SSH backend details if provision fails after trusted host verification", async () => {
    const client = renderSSH({
      inspectSSHHostKey: vi.fn().mockResolvedValue({...hostIdentity, status:"trusted"}),
      createSSHRemoteSandbox: vi.fn().mockRejectedValue(
        new Error("SSH remote connection failed: ssh_remote_failure:host_key_mismatch; secret-backend-details"),
      ),
    }, true);
    await readyForCreation();
    fireEvent.click(screen.getByRole("button", { name: "Create sandbox" }));
    expect(await screen.findByText(/SSH host identity verification failed/)).toBeTruthy();
    expect(screen.queryByText(/secret-backend-details/)).toBeNull();
    expect(client.createSSHRemoteSandbox).toHaveBeenCalled();
  });

  it("does not probe or create without a selected SSH credential", async () => {
    const client = renderSSH({ listCredentials: vi.fn().mockResolvedValue({ credentials: [] }) });
    fireEvent.click(await screen.findByRole("button", { name: /SSH Remote/ }));
    expect(await screen.findByText(/No SSH key credentials/)).toBeTruthy();
    fillSSHFields();
    fireEvent.click(screen.getByRole("button", { name: "Review" }));
    expect(screen.getByText("Choose an existing SSH private key credential.")).toBeTruthy();
    expect(client.inspectSSHHostKey).not.toHaveBeenCalled();
    expect(client.createSSHRemoteSandbox).not.toHaveBeenCalled();
  });

  it("returns to the selected plugin from the form", async () => {
    renderSSH({}, true);
    fireEvent.click(await screen.findByRole("button", { name: "Back" }));
    expect(await screen.findByText("SSH plugin details")).toBeTruthy();
  });
});
