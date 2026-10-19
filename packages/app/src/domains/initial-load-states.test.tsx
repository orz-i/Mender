import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { MenderClient } from "@mender/sdk";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../i18n/i18n";
import { AgentsPage } from "./agents/agents-page";
import { CatalogPage } from "./catalog/catalog-page";
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
  it("retries an Agent inventory instead of displaying empty counts after a failed load", async () => {
    const listAgents = vi.fn()
      .mockRejectedValueOnce(new Error("Agent registry offline"))
      .mockResolvedValue({ agents: [] });
    const client = { listAgents } as unknown as MenderClient;

    render(<MemoryRouter><AgentsPage client={client} /></MemoryRouter>);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Agent registry offline");
    expect(screen.queryByText("No agents")).toBeNull();
    expect(screen.queryByRole("button", { name: /All agents/ })).toBeNull();
    retryError();
    expect(await screen.findByText("No agents")).toBeTruthy();
    await waitFor(() => expect(listAgents).toHaveBeenCalledTimes(2));
  });

  it("retries the Tool detail catalogue without claiming there are no synchronized Tools", async () => {
    const browseTools = vi.fn()
      .mockRejectedValueOnce(new Error("Tool browse failed"))
      .mockResolvedValue({ tools: [], nextCursor: null });
    const client = {
      listProviders: vi.fn().mockResolvedValue({ providers: [] }),
      browseTools,
    } as unknown as MenderClient;

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
          providerId: "prov_example",
          name: "search_web",
          title: "Search Web",
          description: "Searches the web",
          rank: 1,
          tags: ["search"],
        }],
        nextCursor: null,
      }),
    } as unknown as MenderClient;

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
    } as unknown as MenderClient;

    render(<MemoryRouter><SkillsPage client={client} /></MemoryRouter>);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Skills offline");
    expect(screen.queryByText("No matching Skills")).toBeNull();
    retryError();
    expect(await screen.findByText("No matching Skills")).toBeTruthy();
    await waitFor(() => expect(listSkills).toHaveBeenCalledTimes(2));
  });

  it("retries the whole Profile view, not only the Profile list, after initial dependency failure", async () => {
    const listCredentials = vi.fn()
      .mockRejectedValueOnce(new Error("Credentials offline"))
      .mockResolvedValue({ credentials: [] });
    const client = {
      listProfiles: vi.fn().mockResolvedValue({ profiles: [] }),
      listProviders: vi.fn().mockResolvedValue({ providers: [] }),
      listCredentials,
      listTunnels: vi.fn().mockResolvedValue({ tunnels: [] }),
      browseTools: vi.fn().mockResolvedValue({ tools: [] }),
    } as unknown as MenderClient;

    render(<MemoryRouter><ProfilesPage client={client} /></MemoryRouter>);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Credentials offline");
    expect(screen.queryByText("No profiles")).toBeNull();
    retryError();
    expect(await screen.findByText("No profiles")).toBeTruthy();
    await waitFor(() => expect(listCredentials).toHaveBeenCalledTimes(2));
  });
});
