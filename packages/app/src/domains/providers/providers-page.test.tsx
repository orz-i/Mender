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
      <ProvidersPage client={client} onViewChange={() => {}} />
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

describe("ProvidersPage responsive table", () => {
  it("renders mobile-card-ready layout and responsive grid classes", async () => {
    mount();
    const table = await screen.findByRole("table", { name: "Provider inventory" });
    expect(table).toBeTruthy();
    expect(table.className).toContain("max-md:block");

    const header = table.querySelector("thead");
    expect(header?.className).toContain("max-md:sr-only");

    const rows = table.querySelectorAll("tbody tr");
    expect(rows.length).toBe(1);
    expect(rows[0]?.className).toContain("max-md:grid");

    expect(within(table).getByText("Echo Service")).toBeTruthy();
    expect(within(table).getByText("Model Context Protocol")).toBeTruthy();
    expect(within(table).getByText("5 synced Tools")).toBeTruthy();

    const viewButton = screen.getByRole("button", { name: "View Echo Service" });
    expect(viewButton).toBeTruthy();
  });
});
