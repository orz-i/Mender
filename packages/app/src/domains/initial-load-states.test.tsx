import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ManagementClient } from "@mender/sdk";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../i18n/i18n";
import { AgentsPage } from "./agents/agents-page";
import { CatalogPage } from "./tools/catalog/catalog-page";
import { ProfilesPage } from "./profiles/profiles-page";
import { SkillsPage } from "./skills/skills-page";

beforeEach(() => {
  resetLocaleForTesting();
  setLocale("en");
});

afterEach(() => {
  cleanup();
  resetLocaleForTesting();
});

function retryError() {
  const alert = screen.getByRole("alert");
  fireEvent.click(within(alert).getByRole("button", { name: "Refresh" }));
}

describe("Initial request failures remain separate from real empty inventories", () => {
  it("retries failed Agent resource discovery without confusing a preset with a configured instance", async () => {
    const listAgents = vi.fn()
      .mockRejectedValueOnce(new Error("Agent registry offline"))
      .mockResolvedValue({ agents: [] });
    const client = {
      listAgents,
      listPluginCatalogue: vi.fn().mockResolvedValue({ plugins: [{
        pluginId: "io.mender.agent.opencode", key: "io.mender.agent.opencode@builtin",
        sourceKind: "builtin", kind: "agent.backend", status: "enabled", enabled: true,
        marketplaceAction: "create.agent", displayName: "OpenCode",
      }] }),
    } as unknown as ManagementClient;

    render(<MemoryRouter><AgentsPage client={client} /></MemoryRouter>);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Agent registry offline");
    expect(screen.queryByText("No agents")).toBeNull();
    expect(screen.getByRole("list", { name: "Available Agent backends" })).toBeTruthy();
    expect(screen.getAllByText("Status unverified")).toHaveLength(4);
    retryError();
    await waitFor(() => expect(listAgents).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
    expect(screen.getByRole("link", { name: "Configure OpenCode" })).toBeTruthy();
    expect(screen.getByText("Ready to configure")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /All agents/ })).toBeNull();
  });

  it("retries the Tool detail catalogue without claiming there are no synchronized Tools", async () => {
    const browseTools = vi.fn()
      .mockRejectedValueOnce(new Error("Tool browse failed"))
      .mockResolvedValue({ tools: [], nextCursor: null });
    const client = {
      listProviders: vi.fn().mockResolvedValue({ providers: [] }),
      browseTools,
    } as unknown as ManagementClient;

    render(<MemoryRouter><CatalogPage client={client} providerId="prov_example" /></MemoryRouter>);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Tool browse failed");
    expect(screen.queryByText("No synchronized tools")).toBeNull();
    retryError();
    expect(await screen.findByText("No synchronized tools")).toBeTruthy();
    await waitFor(() => expect(browseTools).toHaveBeenCalledTimes(2));
  });

  it("offers the existing Profile workflow as the next step after discovering service tools", async () => {
    const client = {
      listProviders: vi.fn().mockResolvedValue({ providers: [] }),
      browseTools: vi.fn().mockResolvedValue({
        tools: [{
          toolId: "tool_search",
          providerId: "prov_example", sourceId:"provider:"+("prov_example"),sourceKind:"provider",
          name: "search_web",
          title: "Search Web",
          description: "Searches the web",
          rank: 1,
          tags: ["search"],
        }],
        nextCursor: null,
      }),
    } as unknown as ManagementClient;

    render(<MemoryRouter><CatalogPage client={client} providerId="prov_example" /></MemoryRouter>);

    expect(await screen.findByText("Search Web")).toBeTruthy();
    const nextStep = screen.getByRole("link", { name: "Open Profiles" });
    expect(nextStep.getAttribute("href")).toBe("/profiles");
  });

  it("keeps a failed Skill request distinct from an empty Skills library", async () => {
    const listSkills = vi.fn()
      .mockRejectedValueOnce(new Error("Skills offline"))
      .mockResolvedValue({ installations: [] });
    const client = {
      listSkills,
      listSkillDeployments: vi.fn().mockResolvedValue({ deployments: [] }),
    } as unknown as ManagementClient;

    render(<MemoryRouter><SkillsPage client={client} /></MemoryRouter>);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Skills offline");
    expect(screen.queryByText("No matching Skills")).toBeNull();
    retryError();
    expect(await screen.findByText("No matching Skills")).toBeTruthy();
    await waitFor(() => expect(listSkills).toHaveBeenCalledTimes(2));
  });

  it("retries Profile list metadata after an initial dependency failure", async () => {
    const listProfiles = vi.fn()
      .mockRejectedValueOnce(new Error("Profiles offline"))
      .mockResolvedValue({ profiles: [] });
    const listTunnels = vi.fn().mockResolvedValue({tunnels:[]});
    const client = {
      listProfiles,
      listProviders: vi.fn().mockResolvedValue({ providers: [] }),
      listTunnels,
      browseTools: vi.fn().mockResolvedValue({ tools: [] }),
    } as unknown as ManagementClient;

    render(<MemoryRouter><ProfilesPage client={client} /></MemoryRouter>);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Profiles offline");
    expect(screen.queryByText("No profiles")).toBeNull();
    retryError();
    expect(await screen.findByText("No profiles")).toBeTruthy();
    await waitFor(() => expect(listProfiles).toHaveBeenCalledTimes(2));
    expect(listTunnels).not.toHaveBeenCalled();
  });
});
