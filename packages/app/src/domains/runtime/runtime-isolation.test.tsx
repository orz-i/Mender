import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { MenderClient, RuntimeGovernanceStatus } from "@mender/sdk";
import { afterEach, expect, it, vi } from "vitest";

import { RuntimePage } from "./runtime-page";

afterEach(cleanup);

const defaultBudget = {
  maxInvocations: 32,
  maxPrincipalInvocations: 8,
  maxProviderInvocations: 8,
  maxToolInvocations: 4,
  maxProviderActivations: 4,
  maxExecutionTimeoutMs: 120000,
  maxArgumentBytes: 1048576,
  maxResultBytes: 1048576,
};

function showRuntime(externalProvider: "host" | "restricted_host", available: boolean) {
  const status: RuntimeGovernanceStatus = {
    budget: defaultBudget,
    isolation: {
      workspaceShell: "restricted_host",
      externalProvider,
      tunnelClient: "restricted_host",
    },
    backend: { name: "test-host", restrictedHostAvailable: available },
    usage: { invocations: 0, activations: 0 },
  };
  const updateRuntimeIsolation = vi.fn<MenderClient["updateRuntimeIsolation"]>()
    .mockImplementation((isolation) => Promise.resolve({ ...status, isolation }));
  const client = {
    getRuntimeGovernance: () => Promise.resolve(status),
    updateRuntimeIsolation,
  } as unknown as MenderClient;
  render(<RuntimePage client={client} />);
  return updateRuntimeIsolation;
}

it("requires an explicit trust acknowledgement before elevating native Provider plugins to Host", async () => {
  const updateIsolation = showRuntime("restricted_host", true);
  fireEvent.click(await screen.findByRole("button", { name: "Process isolation" }));
  fireEvent.click(screen.getByRole("button", { name: "Edit" }));
  const externalSelect = screen.getByRole("combobox", { name: "External Providers" });
  fireEvent.click(externalSelect);
  const hostOption = await screen.findByRole("option", { name: "Host" });
  fireEvent.pointerDown(hostOption, { pointerId: 1, pointerType: "mouse", button: 0, isPrimary: true });
  fireEvent.pointerUp(hostOption, { pointerId: 1, pointerType: "mouse", button: 0, isPrimary: true });
  fireEvent.click(hostOption);

  const disclosure = screen.getByLabelText("I trust the installed external native plugins and accept unrestricted Host process access.");
  const save = screen.getByRole("button", { name: "Save isolation" });
  expect(save.hasAttribute("disabled")).toBe(true);
  expect(updateIsolation).not.toHaveBeenCalled();
  fireEvent.click(disclosure);
  expect(save.hasAttribute("disabled")).toBe(false);
  fireEvent.click(save);
  await waitFor(() => expect(updateIsolation).toHaveBeenCalledWith({
    workspaceShell: "restricted_host", externalProvider: "host", tunnelClient: "restricted_host",
  }));
});

it("discloses high host authority on read-only isolation view without forcing a no-op save", async () => {
  const updateIsolation = showRuntime("host", true);
  fireEvent.click(await screen.findByRole("button", { name: "Process isolation" }));
  expect(screen.getByRole("status").textContent).toContain("External native Providers use the Runtime user's full host process authority");
  fireEvent.click(screen.getByRole("button", { name: "Edit" }));
  expect(screen.queryByLabelText("I trust the installed external native plugins and accept unrestricted Host process access.")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Save isolation" }));
  await waitFor(() => expect(updateIsolation).toHaveBeenCalledTimes(1));
});
