import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { MenderClient, Provider } from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { ProvidersPage } from "./providers-page";

const mcpPlugin = {
  pluginId: "io.mender.provider.mcp",
  displayName: "MCP",
  version: "1.0.0",
  packageVersion: "1.0.0",
  configVersion: 1,
  sourceKind: "builtin" as const,
  capabilities: [],
  enabled: true,
};
const provider: Provider = {
  providerId: "prov_0123456789abcdef0123456789abcdef",
  namespace: "openai-search",
  displayName: "Search Provider",
  description: "",
  createdAt: "2026-10-10T00:00:00Z",
  updatedAt: "2026-10-10T00:00:00Z",
  transport: "http",
  toolCount: 0,
  pluginId: mcpPlugin.pluginId,
};

function showPage(overrides: Partial<MenderClient> = {}) {
  const createProvider = vi.fn().mockResolvedValue({ provider, sync: { created: 0, removed: 0, updated: 0, total: 0 } });
  const listProviders = vi.fn().mockResolvedValue({ providers: [] });
  const client = {
    listProviderPlugins: vi.fn().mockResolvedValue({ plugins: [mcpPlugin], issues: [] }),
    listProviders,
    listCredentials: vi.fn().mockResolvedValue({ credentials: [] }),
    listSandboxes: vi.fn().mockResolvedValue({ sandboxes: [] }),
    createProvider,
    ...overrides,
  } as unknown as MenderClient;
  render(<MemoryRouter initialEntries={["/tools?view=sources&create=1&pluginId=io.mender.provider.mcp"]}><TooltipProvider><ProvidersPage client={client} onViewChange={() => {}} /></TooltipProvider></MemoryRouter>);
  return { createProvider, listProviders };
}

async function openMcp() {
  await screen.findByText("No providers");
  return within(await screen.findByRole("dialog", { name: "Add MCP provider" }));
}

function fillForm(namespace: string) {
  fireEvent.change(screen.getByRole("textbox", { name: "Namespace" }), { target: { value: namespace } });
  fireEvent.change(screen.getByRole("textbox", { name: "Display name" }), { target: { value: "Search Provider" } });
  fireEvent.change(screen.getByRole("textbox", { name: "Endpoint" }), { target: { value: "https://Example.test/API?q=MixedCase" } });
}

beforeEach(() => {
  window.localStorage.clear();
  resetLocaleForTesting();
  setLocale("en");
});
afterEach(() => {
  cleanup();
  resetLocaleForTesting();
  window.localStorage.clear();
});

describe("Provider namespace creation", () => {
  it("normalizes only the Mender-owned namespace, not connection fields", async () => {
    const { createProvider } = showPage();
    const dialog = await openMcp();
    fillForm(" OpenAI  Search ");
    expect(dialog.getByText("Result: openai-search")).toBeTruthy();

    fireEvent.click(dialog.getByRole("button", { name: "Add provider" }));
    await waitFor(() => expect(createProvider).toHaveBeenCalledWith({
      namespace: "openai-search",
      displayName: "Search Provider",
      pluginId: mcpPlugin.pluginId,
      config: { transport: { kind: "streamable-http", endpoint: "https://Example.test/API?q=MixedCase" } },
    }));
  });

  it("blocks malformed and overlong canonical namespaces before the API", async () => {
    const { createProvider } = showPage();
    const dialog = await openMcp();
    fillForm("Open_AI");
    expect(dialog.getByRole<HTMLInputElement>("textbox", { name: "Namespace" }).getAttribute("aria-invalid")).toBe("true");
    expect(dialog.getByText(/Use 1–63 characters/)).toBeTruthy();
    expect(dialog.getByRole<HTMLButtonElement>("button", { name: "Add provider" }).disabled).toBe(true);

    fillForm("a".repeat(64));
    expect(dialog.getByRole<HTMLButtonElement>("button", { name: "Add provider" }).disabled).toBe(true);
    fillForm("OpenAI Search");
    expect(dialog.getByText("Result: openai-search")).toBeTruthy();
    expect(dialog.getByRole<HTMLButtonElement>("button", { name: "Add provider" }).disabled).toBe(false);
    expect(createProvider).not.toHaveBeenCalled();
  });

  it("keeps an error and editable form open on a normalized namespace collision", async () => {
    const createProvider = vi.fn().mockRejectedValue(new Error("catalog: provider namespace already exists"));
    const listProviders = vi.fn()
      .mockResolvedValueOnce({ providers: [] })
      .mockResolvedValue({ providers: [provider] }); // Another window created the same canonical namespace.
    showPage({ createProvider, listProviders });
    const dialog = await openMcp();
    fillForm("OpenAI Search");
    fireEvent.click(dialog.getByRole("button", { name: "Add provider" }));

    expect((await dialog.findByRole("alert")).textContent).toContain("This Provider namespace is already in use.");
    expect(screen.getByRole("dialog", { name: "Add MCP provider" })).toBeTruthy();
    expect(dialog.getByRole<HTMLInputElement>("textbox", { name: "Namespace" }).value).toBe("OpenAI Search");
    expect(screen.getAllByRole("alert")).toHaveLength(1);
    expect(createProvider).toHaveBeenCalledTimes(1);

    fireEvent.change(dialog.getByRole("textbox", { name: "Namespace" }), { target: { value: "OpenAI Search 2" } });
    expect(dialog.queryByRole("alert")).toBeNull();
  });

  it("canonicalizes imported MCP JSON key but preserves exact URL and header value", async () => {
    const { createProvider } = showPage();
    const dialog = await openMcp();
    fireEvent.click(dialog.getByText("Advanced settings"));
    fireEvent.change(dialog.getByRole("textbox", { name: "MCP JSON" }), {
      target: { value: JSON.stringify({
        mcpServers: {
          "OpenAI Service": {
            type: "streamable-http",
            url: "https://Example.test/MCP?Mode=Case",
            headers: { "X-Auth": "Token MixedCase" },
          },
        },
      }) },
    });
    fireEvent.click(dialog.getByRole("button", { name: "Import configuration" }));
    expect(dialog.getByText("Result: openai-service")).toBeTruthy();
    expect(dialog.getByRole<HTMLInputElement>("textbox", { name: "Namespace" }).value).toBe("OpenAI Service");

    fireEvent.click(dialog.getByRole("button", { name: "Add provider" }));
    await waitFor(() => expect(createProvider).toHaveBeenCalledWith(expect.objectContaining({
      namespace: "openai-service",
      displayName: "OpenAI Service",
      config: { transport: {
        kind: "streamable-http",
        endpoint: "https://Example.test/MCP?Mode=Case",
        headers: { "X-Auth": "Token MixedCase" },
      } },
    })));
  });
});
