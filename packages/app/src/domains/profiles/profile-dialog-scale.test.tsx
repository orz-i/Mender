import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { MenderClient, Profile, Provider, ToolSummary } from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { afterEach, expect, it, vi } from "vitest";

import { ProfilesPage } from "./profiles-page";

afterEach(cleanup);

const tools: ToolSummary[] = Array.from({ length: 65 }, (_, i) => ({
  rank: i + 1,
  toolId: "tool_" + String(i + 1).padStart(4, "0"),
  providerId: "prov_" + (i % 3),
  name: "tool_" + (i + 1),
  title: "Work tool " + (i + 1),
  description: "Tool description " + (i + 1),
}));
const profile: Profile = {
  profileId: "prof_0123456789abcdef0123456789abcdef",
  slug: "large-profile",
  displayName: "Large Profile",
  mode: "hybrid",
  tools: tools.map((tool, index) => ({
    toolId: tool.toolId,
    direct: index % 2 === 0,
    alias: "",
    titleOverride: "",
    descriptionOverride: "",
    policyConstraint: "inherit",
    concurrencyConstraint: "inherit",
    credentialRefs: [],
  })),
};
const providers: Provider[] = Array.from({ length: 3 }, (_, i) => ({
  providerId: "prov_" + i,
  namespace: "source-" + i,
  displayName: "Tool source " + i,
  description: "",
  createdAt: "2026-10-12T00:00:00Z",
  updatedAt: "2026-10-12T00:00:00Z",
  transport: "http",
  toolCount: 25,
}));

function show() {
  const replaceProfileTools = vi.fn().mockImplementation((_slug: string, body: { tools: Profile["tools"] }) => Promise.resolve({ ...profile, tools: body.tools }));
  const client = {
    listProfiles: vi.fn().mockResolvedValue({ profiles: [profile] }),
    listProviders: vi.fn().mockResolvedValue({ providers }),
    listCredentials: vi.fn().mockResolvedValue({ credentials: [] }),
    listTunnels: vi.fn().mockResolvedValue({ tunnels: [] }),
    browseTools: vi.fn().mockResolvedValue({ tools }),
    replaceProfileTools,
  } as unknown as MenderClient;

  render(
    <TooltipProvider>
      <ProfilesPage client={client} />
    </TooltipProvider>,
  );
  return { replaceProfileTools };
}

it("keeps a 65-tool Profile within a viewport-bound dialog and paginates the selected Tools", async () => {
  show();
  fireEvent.click(await screen.findByRole("button", { name: "Manage Large Profile" }));
  const dialog = screen.getByRole("dialog", { name: "Large Profile" });
  expect(dialog.className).toContain("grid-rows-[auto_minmax(0,1fr)_auto]");
  expect(dialog.className).toContain("overflow-hidden");
  expect(dialog.className).toContain("100dvh");
  expect(within(dialog).getByRole("group", { name: "Manage Profile sections" })).toBeTruthy();
  expect(within(dialog).getAllByRole("button", { name: /Presentation for Work tool/ })).toHaveLength(10);
  expect(within(dialog).queryByText("Work tool 11")).toBeNull();
  expect(within(dialog).getByRole("navigation", { name: "Selected Tools pagination" })).toBeTruthy();

  fireEvent.click(within(dialog).getByRole("button", { name: "Selected Tools next page" }));
  expect(within(dialog).getByText("Work tool 11")).toBeTruthy();
  expect(within(dialog).queryByText("Work tool 1")).toBeNull();

  fireEvent.change(within(dialog).getByRole("textbox", { name: "Search selected tools" }), {
    target: { value: "Work tool 64" },
  });
  expect(within(dialog).getByText("Work tool 64")).toBeTruthy();
  expect(within(dialog).getAllByRole("button", { name: /Presentation for Work tool/ })).toHaveLength(1);

  fireEvent.change(within(dialog).getByRole("combobox", { name: "Exposure filter" }), {
    target: { value: "direct" },
  });
  expect(within(dialog).getByText("No matching Tools.")).toBeTruthy();
  fireEvent.change(within(dialog).getByRole("combobox", { name: "Exposure filter" }), {
    target: { value: "dynamic" },
  });
  expect(within(dialog).getByText("Work tool 64")).toBeTruthy();

  fireEvent.click(within(dialog).getByRole("button", { name: "Overview" }));
  expect(within(dialog).getByLabelText("Profile tunnel")).toBeTruthy();
  expect(within(dialog).queryByLabelText("Search selected tools")).toBeNull();
  fireEvent.click(within(dialog).getByRole("button", { name: /^Tools/ }));
  expect(within(dialog).getByLabelText("Search selected tools")).toBeTruthy();
  expect(screen.getAllByRole("dialog")).toHaveLength(1);
});

it("keeps Tool Picker selection across pages while keeping Search and Save controls anchored", async () => {
  const { replaceProfileTools } = show();
  fireEvent.click(await screen.findByRole("button", { name: "Manage Large Profile" }));
  fireEvent.click(screen.getByRole("button", { name: "Choose tools" }));
  const picker = await screen.findByRole("dialog", { name: "Choose tools" });
  expect(picker.className).toContain("grid-rows-[auto_minmax(0,1fr)_auto]");
  expect(picker.className).toContain("overflow-hidden");
  expect(within(picker).getByRole("textbox", { name: "Profile tool search" })).toBeTruthy();
  expect(within(picker).getByRole("combobox", { name: "Provider filter" })).toBeTruthy();
  expect(within(picker).getByRole("button", { name: "Save selection" })).toBeTruthy();
  const list = picker.querySelector('[data-slot="profile-tool-picker-list"]');
  expect(list?.className).toContain("overflow-y-auto");
  expect(within(picker).getAllByRole("checkbox", { name: /Select Work tool/ })).toHaveLength(10);

  fireEvent.click(within(picker).getByRole("checkbox", { name: "Select Work tool 1" }));
  fireEvent.click(within(picker).getByRole("button", { name: "Catalog Tools next page" }));
  expect(within(picker).queryByRole("checkbox", { name: "Select Work tool 1" })).toBeNull();
  fireEvent.click(within(picker).getByRole("checkbox", { name: "Select Work tool 11" }));
  fireEvent.click(within(picker).getByRole("button", { name: "Save selection" }));
  await vi.waitFor(() => expect(replaceProfileTools).toHaveBeenCalledOnce());
  const sent = (replaceProfileTools.mock.calls[0] as [string, {tools: Profile["tools"]}] | undefined)?.[1];
  expect(sent?.tools).toHaveLength(63);
  expect(sent?.tools.some((selection) => selection.toolId === "tool_0001")).toBe(false);
  expect(sent?.tools.some((selection) => selection.toolId === "tool_0011")).toBe(false);
  expect(sent?.tools.some((selection) => selection.toolId === "tool_0065")).toBe(true);
});
