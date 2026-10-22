import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { MenderClient, Profile, Provider, ToolSummary } from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { ProfilesPage } from "./profiles-page";
import { ProfileDetailPage } from "./profile-detail-page";

const boundTool = {
  toolId: "tool_search",
  direct: true,
  alias: "search_prod",
  titleOverride: "",
  descriptionOverride: "",
  policyConstraint: "inherit" as const,
  concurrencyConstraint: "serial" as const,
  credentialRefs: [],
};
const profile: Profile = {
  profileId: "prof_0123456789abcdef0123456789abcdef",
  slug: "my-profile",
  displayName: "My Profile",
  mode: "hybrid",
  tools: [boundTool],
};
const provider: Provider = {
  providerId: "prov_source", namespace: "anysearch", displayName: "AnySearch",
  description: "", toolSourceKind: "provider", toolSourceSelectable: true,
  createdAt: "2026-10-12T00:00:00Z", updatedAt: "2026-10-12T00:00:00Z",
  transport: "http", toolCount: 1,
};
const tool: ToolSummary = {
  rank: 1, toolId: "tool_search", providerId: "prov_source",
  name: "search", title: "Search", description: "Search the web",
};
function show(initial: string, overrides: Partial<MenderClient> = {}) {
  const client = {
    listProfiles: vi.fn().mockResolvedValue({ profiles: [profile] }),
    listProviders: vi.fn().mockResolvedValue({ providers: [provider] }),
    listTunnels: vi.fn().mockResolvedValue({ tunnels: [] }),
    browseTools: vi.fn().mockResolvedValue({ tools: [tool] }),
    getProfile: vi.fn().mockResolvedValue(profile),
    getTool: vi.fn().mockResolvedValue({ ...tool, inputSchema: {}, tags: [] }),
    replaceProfileTools: vi.fn(),
    setProfileTunnel: vi.fn(),
    ...overrides,
  } as unknown as MenderClient;
  render(
    <MemoryRouter initialEntries={[initial]}>
      <TooltipProvider>
        <Routes>
          <Route path="/profiles" element={<ProfilesPage client={client} />} />
          <Route path="/profiles/:slug" element={<ProfileDetailPage client={client} />} />
          <Route path="/profiles/:slug/edit" element={<div>Profile editing route</div>} />
          <Route path="/profiles/new" element={<div>Profile creation route</div>} />
        </Routes>
      </TooltipProvider>
    </MemoryRouter>,
  );
  return client;
}
beforeEach(() => { resetLocaleForTesting(); setLocale("en"); });
afterEach(() => { cleanup(); resetLocaleForTesting(); });

it("opens a read-only detail from the inventory and drills down by source", async () => {
  const replaceProfileTools = vi.fn();
  show("/profiles", { replaceProfileTools: replaceProfileTools as MenderClient["replaceProfileTools"] });
  fireEvent.click(await screen.findByRole("button", { name: "My Profile" }));
  await screen.findByRole("button", { name: "Edit profile" });
  expect(screen.queryByText("search_prod")).toBeNull(); // No tool list is exposed in overview.
  expect(screen.getByText("hybrid")).toBeTruthy();
  expect(screen.getByText("Local only")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Tools" }));
  expect(screen.getByRole("button", { name: /AnySearch/ })).toBeTruthy();
  expect(screen.queryByText("Search the web")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: /AnySearch/ }));
  expect(screen.getByText("Search the web")).toBeTruthy();
  expect(screen.getByText(/Alias: search_prod/)).toBeTruthy();
  expect(screen.getByText("Serial")).toBeTruthy();
  expect(replaceProfileTools).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Edit profile" }));
  expect(screen.getByText("Profile editing route")).toBeTruthy();
});

it("links inventory creation to the independent new route", async () => {
  show("/profiles");
  fireEvent.click(await screen.findByRole("button", { name: "Create profile" }));
  expect(screen.getByText("Profile creation route")).toBeTruthy();
});

it("hydrates selected ToolIDs missing from the first Catalog page without broad source permission", async () => {
  const getTool = vi.fn().mockResolvedValue({ ...tool, inputSchema: {}, tags: [] });
  show("/profiles/my-profile", {
    browseTools: vi.fn().mockResolvedValue({ tools: [] }),
    getTool,
  });
  fireEvent.click(await screen.findByRole("button", { name: "Tools" }));
  await waitFor(() => expect(getTool).toHaveBeenCalledWith("tool_search"));
  const source = await screen.findByRole("button", { name: /AnySearch/ });
  fireEvent.click(source);
  expect(screen.getByText("Search the web")).toBeTruthy();
});

it("shows a recoverable error on unknown profile and does not offer editing", async () => {
  const getProfile = vi.fn().mockRejectedValue(new Error("Profile not found"));
  show("/profiles/no-such", { getProfile });
  expect((await screen.findByRole("alert")).textContent).toContain("Profile not found");
  expect(screen.queryByRole("button", { name: "Edit profile" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await waitFor(() => expect(getProfile).toHaveBeenCalledTimes(2));
});
