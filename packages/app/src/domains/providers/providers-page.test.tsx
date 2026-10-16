import { cleanup, render, screen, within } from "@testing-library/react";
import type { MenderClient, Provider, ProviderPlugin } from "@mender/sdk";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { ProvidersPage } from "./providers-page";

const sampleProvider: Provider = {
  providerId: "prov_echo_01",
  namespace: "echo_service",
  displayName: "Echo Service",
  description: "Echo service description",
  pluginId: "mcp",
  pluginDisplayName: "Model Context Protocol",
  configVersion: 1,
  transport: "http" as const,
  endpoint: "https://api.echo.com/mcp",
  toolCount: 5,
  createdAt: "2026-10-01T00:00:00Z",
  updatedAt: "2026-10-01T00:00:00Z",
};

const samplePlugin: ProviderPlugin = {
  pluginId: "mcp",
  displayName: "Model Context Protocol",
  description: "Model Context Protocol",
  version: "1.0.0",
  packageVersion: "1.0.0",
  configVersion: 1,
  enabled: true,
  sourceKind: "builtin",
  configSchema: {},
  capabilities: ["mcp.tools"],
};

function mount(providers: Provider[] = [sampleProvider], plugins: ProviderPlugin[] = [samplePlugin]) {
  const client = {
    listProviders: vi.fn().mockResolvedValue({ providers }),
    listProviderPlugins: vi.fn().mockResolvedValue({ plugins, issues: [] }),
    listCredentials: vi.fn().mockResolvedValue({ credentials: [] }),
    listSandboxes: vi.fn().mockResolvedValue({ sandboxes: [] }),
  } as unknown as MenderClient;

  render(
    <MemoryRouter>
      <ProvidersPage client={client} />
    </MemoryRouter>,
  );
  return { client };
}

beforeEach(() => {
  resetLocaleForTesting();
  setLocale("en");
});

afterEach(() => {
  cleanup();
  resetLocaleForTesting();
});

describe("Tool services responsive cards", () => {
  it("shows one named service card with a Tool drilldown action", async () => {
    mount();
    const link=await screen.findByRole("link",{name:"Echo Service"});
    expect(link.getAttribute("href")).toBe("/tools/prov_echo_01");
    const card=link.closest<HTMLElement>('[data-slot="card"]');
    expect(card).toBeTruthy();
    expect(within(card!).getByText("5 tools")).toBeTruthy();
    expect(within(card!).queryByText("Model Context Protocol")).toBeNull();
    expect(screen.getByRole("button",{name:"View tools"})).toBeTruthy();
    expect(screen.getByRole("button",{name:"View Echo Service"})).toBeTruthy();
    expect(screen.queryByRole("button",{name:"Show internal providers"})).toBeNull();
  });
});
