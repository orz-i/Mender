import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { MenderClient, Profile, Provider, ToolSummary } from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { ProfilesPage } from "./profiles-page";

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
  toolSourceKind: "provider",
  toolSourceSelectable: true,
  createdAt: "2026-10-12T00:00:00Z",
  updatedAt: "2026-10-12T00:00:00Z",
  transport: "http",
  toolCount: 25,
}));

beforeEach(() => {
  resetLocaleForTesting();
  setLocale("en");
});
afterEach(() => {
  cleanup();
  resetLocaleForTesting();
});

function show(
  overrides: Partial<MenderClient> = {},
  options: { initialProfile?: Profile; providers?: Provider[] } = {},
) {
  const initialProfile = options.initialProfile ?? profile;
  const replaceProfileTools = vi.fn().mockImplementation((_slug: string, body: { tools: Profile["tools"] }) =>
    Promise.resolve({ ...initialProfile, tools: body.tools }));
  const browseTools = vi.fn().mockImplementation((input?: { providerId?: string; limit?: number }) => {
    const list = input?.providerId
      ? tools.filter((tool) => tool.providerId === input.providerId)
      : tools;
    return Promise.resolve({ tools: list.slice(0, input?.limit ?? 100) });
  });
  const getTool = vi.fn().mockImplementation((id: string) => {
    const tool = tools.find((item) => item.toolId === id);
    if (!tool) return Promise.reject(new Error("not found"));
    return Promise.resolve({ ...tool, sourceKey: tool.name, inputSchema: {}, tags: [] });
  });
  const client = {
    listProfiles: vi.fn().mockResolvedValue({ profiles: [initialProfile] }),
    listProviders: vi.fn().mockResolvedValue({ providers: options.providers ?? providers }),
    listCredentials: vi.fn().mockResolvedValue({ credentials: [] }),
    listTunnels: vi.fn().mockResolvedValue({ tunnels: [] }),
    browseTools,
    getTool,
    replaceProfileTools,
    ...overrides,
  } as unknown as MenderClient;
  render(<TooltipProvider><ProfilesPage client={client} /></TooltipProvider>);
  return { replaceProfileTools, browseTools, getTool };
}

async function manage() {
  fireEvent.click(await screen.findByRole("button", { name: "Manage Large Profile" }));
  return screen.getByRole("dialog", { name: "Large Profile" });
}

async function chooseSources() {
  fireEvent.click(within(await manage()).getByRole("button", { name: "Choose tool sources" }));
  return screen.findByRole("dialog", { name: "Choose tool sources" });
}

async function openSource(name: string) {
  const picker = screen.getByRole("dialog", { name: "Choose tool sources" });
  fireEvent.click(within(picker).getByRole("button", { name: "Open source " + name }));
  // New level is always scoped to exactly one chosen source.
  await within(picker).findByRole("textbox", { name: "Profile tool search" });
  return picker;
}

it("labels Catalog tool counts without implying their Agent backend is executable", async () => {
  show();
  const picker = await chooseSources();
  expect(within(picker).getAllByText("25 tools")).toHaveLength(3);
  expect(within(picker).queryByText("25 available Tools")).toBeNull();
});

it("shows selected sources before Tools and preserves per-source management pagination", async () => {
  show();
  const dialog = await manage();
  expect(dialog.className).toContain("grid-rows-[auto_minmax(0,1fr)_auto]");
  expect(dialog.className).toContain("overflow-hidden");
  expect(dialog.className).toContain("100dvh");
  expect(within(dialog).getByRole("group", { name: "Manage Profile sections" })).toBeTruthy();
  expect(within(dialog).getByRole("textbox", { name: "Search selected sources" })).toBeTruthy();
  expect(within(dialog).getAllByRole("button", { name: /Manage source Tool source/ })).toHaveLength(3);
  expect(within(dialog).queryByRole("button", { name: /Presentation for Work tool/ })).toBeNull();

  fireEvent.click(within(dialog).getByRole("button", { name: "Manage source Tool source 0" }));
  expect(within(dialog).getAllByRole("button", { name: /Presentation for Work tool/ })).toHaveLength(10);
  expect(within(dialog).queryByText("Work tool 31")).toBeNull();
  fireEvent.click(within(dialog).getByRole("button", { name: "Selected Tools next page" }));
  expect(within(dialog).getByText("Work tool 31")).toBeTruthy();

  fireEvent.change(within(dialog).getByRole("textbox", { name: "Search selected tools" }), {
    target: { value: "Work tool 64" },
  });
  expect(within(dialog).getByText("Work tool 64")).toBeTruthy();
  fireEvent.change(within(dialog).getByRole("combobox", { name: "Exposure filter" }), {
    target: { value: "direct" },
  });
  expect(within(dialog).getByText("No matching Tools.")).toBeTruthy();
  fireEvent.change(within(dialog).getByRole("combobox", { name: "Exposure filter" }), {
    target: { value: "dynamic" },
  });
  expect(within(dialog).getByText("Work tool 64")).toBeTruthy();

  fireEvent.click(within(dialog).getByRole("button", { name: "Back to sources" }));
  expect(within(dialog).getByRole("textbox", { name: "Search selected sources" })).toBeTruthy();
  fireEvent.click(within(dialog).getByRole("button", { name: "Overview" }));
  expect(within(dialog).getByLabelText("Profile tunnel")).toBeTruthy();
  fireEvent.click(within(dialog).getByRole("button", { name: /^Tools/ }));
  expect(within(dialog).getByRole("textbox", { name: "Search selected sources" })).toBeTruthy();
  expect(screen.getAllByRole("dialog")).toHaveLength(1);
});

it("switches sources while preserving pending ToolID selections and all unrelated bindings", async () => {
  const customised: Profile = {
    ...profile,
    tools: profile.tools.map((binding, i) => i === 64
      ? { ...binding, alias: "keep_alias", policyConstraint: "deny",
          concurrencyConstraint: "serial", credentialRefs: ["cred_kept"], titleOverride: "Keep override" }
      : binding),
  };
  const { replaceProfileTools } = show({}, { initialProfile: customised });
  let picker = await chooseSources();
  expect(picker.className).toContain("grid-rows-[auto_minmax(0,1fr)_auto]");
  expect(picker.className).toContain("overflow-hidden");
  expect(within(picker).getByRole("textbox", { name: "Search sources" })).toBeTruthy();
  expect(within(picker).queryByRole("checkbox", { name: /Select Work tool/ })).toBeNull();

  picker = await openSource("Tool source 0");
  const toolOne = await within(picker).findByRole("checkbox", { name: "Select Work tool 1" });
  expect((toolOne as HTMLInputElement).checked).toBe(true);
  fireEvent.click(toolOne);
  fireEvent.click(within(picker).getByRole("button", { name: "Back to sources" }));

  picker = await openSource("Tool source 1");
  const toolTwo = await within(picker).findByRole("checkbox", { name: "Select Work tool 2" });
  fireEvent.click(toolTwo);
  fireEvent.click(within(picker).getByRole("button", { name: "Back to sources" }));

  picker = await openSource("Tool source 0");
  expect((await within(picker).findByRole<HTMLInputElement>("checkbox", { name: "Select Work tool 1" })).checked).toBe(false);
  fireEvent.click(within(picker).getByRole("button", { name: "Save selection" }));
  await vi.waitFor(() => expect(replaceProfileTools).toHaveBeenCalledOnce());
  const sent = (replaceProfileTools.mock.calls[0] as [string, { tools: Profile["tools"] }])?.[1];
  expect(sent.tools).toHaveLength(63);
  expect(sent.tools.some((binding) => binding.toolId === "tool_0001" || binding.toolId === "tool_0002")).toBe(false);
  expect(sent.tools.find((binding) => binding.toolId === "tool_0065")).toEqual(customised.tools[64]);
  expect(screen.getAllByRole("dialog")).toHaveLength(1);
});

it("paginates 105 Tools inside one source and never drops other selected sources", async () => {
  const more: ToolSummary[] = Array.from({ length: 105 }, (_, i) => ({
    rank: i + 1, toolId: "tool_" + String(i + 1).padStart(4, "0"),
    providerId: "prov_0", name: "tool_" + (i + 1),
    title: "Work tool " + (i + 1), description: "Search service tool " + (i + 1),
  }));
  const browse = vi.fn().mockImplementation((args?: { providerId?: string; cursor?: string }) => {
    if (!args?.providerId) return Promise.resolve({ tools: more.slice(0, 100) });
    if (args.cursor) return Promise.resolve({ tools: more.slice(100) });
    return Promise.resolve({ tools: more.slice(0, 100), nextCursor: "source-page-two" });
  });
  const { replaceProfileTools } = show({ browseTools: browse as MenderClient["browseTools"] });
  await chooseSources();
  const picker = await openSource("Tool source 0");
  expect(await within(picker).findByRole("button", { name: "Load more tools" })).toBeTruthy();
  fireEvent.click(within(picker).getByRole("button", { name: "Load more tools" }));
  await vi.waitFor(() => expect(browse).toHaveBeenCalledWith({
    providerId: "prov_0", limit: 100, cursor: "source-page-two",
  }));
  await vi.waitFor(() => expect(within(picker).queryByRole("button", { name: "Load more tools" })).toBeNull());
  fireEvent.change(within(picker).getByRole("combobox", { name: "Catalog Tools rows per page" }), {
    target: { value: "50" },
  });
  fireEvent.click(within(picker).getByRole("button", { name: "Catalog Tools next page" }));
  fireEvent.click(within(picker).getByRole("button", { name: "Catalog Tools next page" }));
  fireEvent.click(await within(picker).findByRole("checkbox", { name: "Select Work tool 105" }));
  fireEvent.click(within(picker).getByRole("button", { name: "Save selection" }));
  await vi.waitFor(() => expect(replaceProfileTools).toHaveBeenCalledOnce());
  const sent = (replaceProfileTools.mock.calls[0] as [string, { tools: Profile["tools"] }])?.[1];
  expect(sent.tools).toHaveLength(66);
  expect(sent.tools.some((binding) => binding.toolId === "tool_0002")).toBe(true);
  expect(sent.tools.some((binding) => binding.toolId === "tool_0105")).toBe(true);
});

it("searches Tools only inside a chosen ProviderID instead of mixing same-named sources", async () => {
  const searchTools = vi.fn().mockResolvedValue({ tools: [] });
  show({ searchTools: searchTools as MenderClient["searchTools"] });
  await chooseSources();
  const picker = await openSource("Tool source 2");
  fireEvent.change(within(picker).getByRole("textbox", { name: "Profile tool search" }), {
    target: { value: "search" },
  });
  fireEvent.click(within(picker).getByRole("button", { name: "Apply" }));
  await vi.waitFor(() => expect(searchTools).toHaveBeenCalledWith({
    query: "search", limit: 100, providerId: "prov_2",
  }));
});

it("selects only loaded ToolIDs; never implicitly grants later pages or future tools", async () => {
  const more: ToolSummary[] = Array.from({ length: 105 }, (_, i) => ({
    rank: i + 1, toolId: "tool_" + String(i + 1).padStart(4, "0"),
    providerId: "prov_0", name: "tool_" + (i + 1), title: "Work tool " + (i + 1), description: "",
  }));
  const browse = vi.fn().mockImplementation((args?: { providerId?: string }) => Promise.resolve({
    tools: more.slice(0, 100), nextCursor: args?.providerId ? "not-loaded" : undefined,
  }));
  const { replaceProfileTools } = show({ browseTools: browse as MenderClient["browseTools"] });
  await chooseSources();
  const picker = await openSource("Tool source 0");
  await within(picker).findByRole("button", { name: "Load more tools" });
  fireEvent.click(within(picker).getByRole("button", { name: "Select loaded tools" }));
  fireEvent.click(within(picker).getByRole("button", { name: "Save selection" }));
  await vi.waitFor(() => expect(replaceProfileTools).toHaveBeenCalledOnce());
  const sent = (replaceProfileTools.mock.calls[0] as [string, { tools: Profile["tools"] }])?.[1];
  expect(sent.tools).toHaveLength(100);
  expect(sent.tools.some((binding) => binding.toolId === "tool_0105")).toBe(false);
  expect(sent.tools.some((binding) => binding.toolId === "tool_0002")).toBe(true);
});

it("excludes unavailable/unclaimed sources from Add but preserves their existing bindings in Manage", async () => {
  const candidates: Provider[] = [
    providers[0]!,
    { ...providers[1]!, toolSourceSelectable: false, toolSourceKind: "sandbox" },
    { ...providers[2]!, toolSourceSelectable: false, toolSourceKind: "unclaimed" },
  ];
  show({}, { providers: candidates });
  const manageDialog = await manage();
  expect(within(manageDialog).getByRole("button", { name: "Manage source Tool source 1 · Workspace" })).toBeTruthy();
  expect(within(manageDialog).getByRole("button", { name: "Manage source Tool source 2" })).toBeTruthy();
  fireEvent.click(within(manageDialog).getByRole("button", { name: "Choose tool sources" }));
  const picker = await screen.findByRole("dialog", { name: "Choose tool sources" });
  expect(within(picker).getAllByRole("button", { name: /Open source/ })).toHaveLength(1);
  expect(within(picker).queryByRole("button", { name: "Open source Tool source 1" })).toBeNull();
  expect(within(picker).queryByRole("button", { name: "Open source Tool source 2" })).toBeNull();
});

it("hydrates a bound Tool beyond the first Catalog page before grouping it by exact ProviderID", async () => {
  const extra: ToolSummary = { rank: 105, toolId: "tool_0105", providerId: "prov_1",
    name: "tool_105", title: "Work tool 105", description: "Detail for far-page Tool" };
  const chosen: Profile = { ...profile, tools: [...profile.tools, { ...profile.tools[0]!, toolId: extra.toolId }] };
  const getTool = vi.fn().mockResolvedValue({ ...extra, sourceKey: "detail", inputSchema: {}, tags: [] });
  const { replaceProfileTools } = show({ getTool: getTool as MenderClient["getTool"] }, { initialProfile: chosen });
  const dialog = await manage();
  await vi.waitFor(() => expect(getTool).toHaveBeenCalledWith("tool_0105"));
  fireEvent.click(within(dialog).getByRole("button", { name: "Manage source Tool source 1" }));
  fireEvent.change(within(dialog).getByRole("textbox", { name: "Search selected tools" }), {
    target: { value: "105" },
  });
  expect(await within(dialog).findByText("Work tool 105")).toBeTruthy();
  fireEvent.click(within(dialog).getByRole("button", { name: "Back to sources" }));
  fireEvent.click(within(dialog).getByRole("button", { name: "Choose tool sources" }));
  const picker = await screen.findByRole("dialog", { name: "Choose tool sources" });
  expect(within(picker).getByRole<HTMLButtonElement>("button", { name: "Save selection" }).disabled).toBe(true);
  await openSource("Tool source 0");
  fireEvent.click(await within(picker).findByRole("checkbox", { name: "Select Work tool 1" }));
  fireEvent.click(within(picker).getByRole("button", { name: "Save selection" }));
  await vi.waitFor(() => expect(replaceProfileTools).toHaveBeenCalledOnce());
  const sent = (replaceProfileTools.mock.calls[0] as [string, { tools: Profile["tools"] }])?.[1];
  expect(sent.tools.some((binding) => binding.toolId === extra.toolId)).toBe(true);
});

it("distinguishes two identically named Agent instances by their unique configured source", async () => {
  const duplicate: Provider[] = [
    { ...providers[0]!, namespace: "agent-same-a", displayName: "Same Agent", toolSourceName: "Same Agent", toolSourceKind: "agent", toolSourceId: "agent_a" },
    { ...providers[1]!, namespace: "agent-same-b", displayName: "Same Agent", toolSourceName: "Same Agent", toolSourceKind: "agent", toolSourceId: "agent_b" },
    providers[2]!,
  ];
  const { browseTools } = show({}, { providers: duplicate });
  const dialog = await manage();
  expect(within(dialog).getByRole("button", { name: "Manage source Same Agent · agent-same-a" })).toBeTruthy();
  expect(within(dialog).getByRole("button", { name: "Manage source Same Agent · agent-same-b" })).toBeTruthy();
  fireEvent.click(within(dialog).getByRole("button", { name: "Choose tool sources" }));
  const picker = await screen.findByRole("dialog", { name: "Choose tool sources" });
  fireEvent.click(within(picker).getByRole("button", { name: "Open source Same Agent · agent-same-b" }));
  await within(picker).findByRole("checkbox", { name: "Select Work tool 2" });
  expect(browseTools).toHaveBeenCalledWith({ providerId: "prov_1", limit: 100 });
});

it("cancels during source loading without persisting or leaking a stale request into the next dialog", async () => {
  let finish!: (value: { tools: ToolSummary[] }) => void;
  const delayed = new Promise<{ tools: ToolSummary[] }>((resolve) => { finish = resolve; });
  const browse = vi.fn().mockImplementation(async (args?: { providerId?: string }) =>
    args?.providerId ? delayed : { tools });
  const { replaceProfileTools } = show({ browseTools: browse as MenderClient["browseTools"] });
  await chooseSources();
  const picker = screen.getByRole("dialog", { name: "Choose tool sources" });
  fireEvent.click(within(picker).getByRole("button", { name: "Open source Tool source 0" }));
  fireEvent.click(within(picker).getByRole("button", { name: "Cancel" }));
  const manageDialog = await screen.findByRole("dialog", { name: "Large Profile" });
  expect(within(manageDialog).getByRole("button", { name: "Choose tool sources" })).toBeTruthy();
  finish({ tools: tools.filter((tool) => tool.providerId === "prov_0") });
  await vi.waitFor(() => expect(replaceProfileTools).not.toHaveBeenCalled());
  fireEvent.click(within(manageDialog).getByRole("button", { name: "Choose tool sources" }));
  const reopened = await screen.findByRole("dialog", { name: "Choose tool sources" });
  expect(within(reopened).getByRole("textbox", { name: "Search sources" })).toBeTruthy();
  expect(within(reopened).queryByRole("checkbox", { name: /Select Work tool/ })).toBeNull();
});

it("retains an unreachable historical ToolID in a management-only Unknown source while editing other sources", async () => {
  const historicalId = "tool_missing_historical";
  const historicalBinding = {
    ...profile.tools[0]!, toolId: historicalId, alias: "legacy_alias", policyConstraint: "deny" as const,
    concurrencyConstraint: "serial" as const, credentialRefs: ["cred_old"],
  };
  const selected: Profile = { ...profile, tools: [...profile.tools, historicalBinding] };
  const getTool = vi.fn().mockRejectedValue(new Error("tool was retired"));
  const { replaceProfileTools } = show({ getTool: getTool as MenderClient["getTool"] }, { initialProfile: selected });
  const dialog = await manage();
  expect(within(dialog).getByRole("button", { name: "Manage source Unknown source" })).toBeTruthy();
  await vi.waitFor(() => expect(getTool).toHaveBeenCalledWith(historicalId));
  fireEvent.click(within(dialog).getByRole("button", { name: "Manage source Unknown source" }));
  expect(within(dialog).getByRole("button", { name: "Remove Unavailable Tool" })).toBeTruthy();
  fireEvent.click(within(dialog).getByRole("button", { name: "Back to sources" }));
  fireEvent.click(within(dialog).getByRole("button", { name: "Choose tool sources" }));
  const picker = await screen.findByRole("dialog", { name: "Choose tool sources" });
  fireEvent.click(within(picker).getByRole("button", { name: "Open source Tool source 0" }));
  fireEvent.click(await within(picker).findByRole("checkbox", { name: "Select Work tool 1" }));
  fireEvent.click(within(picker).getByRole("button", { name: "Save selection" }));
  await vi.waitFor(() => expect(replaceProfileTools).toHaveBeenCalledOnce());
  const sent = (replaceProfileTools.mock.calls[0] as [string, { tools: Profile["tools"] }])?.[1];
  expect(sent.tools).toHaveLength(selected.tools.length - 1);
  expect(sent.tools.find((binding) => binding.toolId === historicalId)).toEqual(historicalBinding);
});
