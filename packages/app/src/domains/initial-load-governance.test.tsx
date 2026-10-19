import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactElement } from "react";
import type { MenderClient } from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../i18n/i18n";
import { AuditPage } from "./audit/audit-page";
import { CredentialsPage } from "./credentials/credentials-page";
import { PolicyPage } from "./policy/policy-page";
import { RuntimePage } from "./runtime/runtime-page";
import { SandboxesPage } from "./sandboxes/sandboxes-page";
import { ServersPage } from "./servers/servers-page";
import { TunnelsPage } from "./tunnels/tunnels-page";

function mount(view: ReactElement) {
  render(<MemoryRouter><TooltipProvider>{view}</TooltipProvider></MemoryRouter>);
}

async function retryInitialFailure(expectedMessage: string, absentText: string) {
  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain(expectedMessage);
  expect(screen.queryByText(absentText)).toBeNull();
  fireEvent.click(within(alert).getByRole("button", { name: "Refresh" }));
}

beforeEach(() => {
  resetLocaleForTesting();
  setLocale("en");
});
afterEach(() => {
  cleanup();
  resetLocaleForTesting();
});

describe("Initial failure is not mistaken for an empty inventory", () => {
  it("retries Sandboxes without showing false zero counts", async () => {
    const listSandboxes = vi.fn()
      .mockRejectedValueOnce(new Error("Sandbox inventory unavailable"))
      .mockResolvedValue({ sandboxes: [] });
    mount(<SandboxesPage client={{ listSandboxes } as unknown as MenderClient} />);
    await retryInitialFailure("Sandbox inventory unavailable", "No sandboxes");
    await screen.findByText("No sandboxes");
    expect(listSandboxes).toHaveBeenCalledTimes(2);
  });

  it("retries Tunnels before claiming no tunnels are installed", async () => {
    const listTunnels = vi.fn()
      .mockRejectedValueOnce(new Error("Tunnel inventory unavailable"))
      .mockResolvedValue({ tunnels: [] });
    mount(<TunnelsPage client={{
      listTunnels,
      listCredentials: vi.fn().mockResolvedValue({ credentials: [] }),
      listPluginCatalogue: vi.fn().mockResolvedValue({ plugins: [] }),
    } as unknown as MenderClient} />);
    await retryInitialFailure("Tunnel inventory unavailable", "No Tunnels");
    await screen.findByText("No Tunnels");
    expect(listTunnels).toHaveBeenCalledTimes(2);
  });

  it("retries MCP Servers without falsely claiming no server is running", async () => {
    const listMCPServers = vi.fn()
      .mockRejectedValueOnce(new Error("Server inventory unavailable"))
      .mockResolvedValue({ servers: [] });
    mount(<ServersPage client={{
      listProfiles: vi.fn().mockResolvedValue({ profiles: [] }),
      listMCPServers,
    } as unknown as MenderClient} />);
    await retryInitialFailure("Server inventory unavailable", "No running MCP servers");
    await screen.findByText("No running MCP servers");
    expect(listMCPServers).toHaveBeenCalledTimes(2);
  });

  it("retries Credentials without a false empty credential inventory", async () => {
    const listCredentials = vi.fn()
      .mockRejectedValueOnce(new Error("Credential inventory unavailable"))
      .mockResolvedValue({ credentials: [] });
    mount(<CredentialsPage client={{ listCredentials } as unknown as MenderClient} />);
    await retryInitialFailure("Credential inventory unavailable", "No Credentials");
    await screen.findByText("No Credentials");
    expect(listCredentials).toHaveBeenCalledTimes(2);
  });

  it("does not default Policy to Allow when the server has not answered", async () => {
    const getPolicy = vi.fn()
      .mockRejectedValueOnce(new Error("Policy request unavailable"))
      .mockResolvedValue({ defaultDecision: "deny", rules: [] });
    mount(<PolicyPage client={{
      getPolicy,
      listProfiles: vi.fn().mockResolvedValue({ profiles: [] }),
      listProviders: vi.fn().mockResolvedValue({ providers: [] }),
      browseTools: vi.fn().mockResolvedValue({ tools: [] }),
    } as unknown as MenderClient} />);
    await retryInitialFailure("Policy request unavailable", "No Policy Rules");
    await screen.findByText("No Policy Rules");
    expect(getPolicy).toHaveBeenCalledTimes(2);
    expect(screen.getByText("deny")).toBeTruthy();
  });

  it("keeps failed Audit history different from a genuinely empty audit", async () => {
    const listAudit = vi.fn()
      .mockRejectedValueOnce(new Error("Audit history unavailable"))
      .mockResolvedValue({ records: [] });
    mount(<AuditPage client={{
      listAudit,
      listProviderActivationAudit: vi.fn().mockResolvedValue({ records: [] }),
    } as unknown as MenderClient} />);
    await retryInitialFailure("Audit history unavailable", "No audit records");
    await screen.findByText("No audit records");
    expect(listAudit).toHaveBeenCalledTimes(2);
  });

  it("does not label Runtime isolation unavailable before any status response", async () => {
    const getRuntimeGovernance = vi.fn()
      .mockRejectedValueOnce(new Error("Runtime status unavailable"))
      .mockResolvedValue({
        budget: {
          maxInvocations: 32, maxPrincipalInvocations: 8, maxProviderInvocations: 8,
          maxToolInvocations: 4, maxProviderActivations: 4,
          maxExecutionTimeoutMs: 120000, maxArgumentBytes: 1048576, maxResultBytes: 1048576,
        },
        isolation: {
          workspaceShell: "restricted_host", externalProvider: "restricted_host",
          tunnelClient: "restricted_host",
        },
        backend: { name: "test-host", restrictedHostAvailable: true },
        usage: { invocations: 0, activations: 0 },
      });
    mount(<RuntimePage client={{ getRuntimeGovernance } as unknown as MenderClient} />);
    await retryInitialFailure("Runtime status unavailable", "Restricted host");
    await waitFor(() => expect(getRuntimeGovernance).toHaveBeenCalledTimes(2));
    expect(await screen.findByText("test-host")).toBeTruthy();
  });
});
