import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type {
  ExportProfilePluginInput, MenderClient, Profile, ProfileAgentPluginExport,
} from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { ProfileAgentPluginPage } from "./profile-agent-plugin-page";

const skillID = "sinst_00000000000000000000000000000001";
const profile: Profile = {
  profileId: "prof_00000000000000000000000000000001",
  slug: "delivery", displayName: "Delivery", revision: 4, mode: "hybrid",
  tools: [{
    toolId: "tool_00000000000000000000000000000001",
    direct: true, alias: "", titleOverride: "", descriptionOverride: "",
    policyConstraint: "inherit", concurrencyConstraint: "inherit", credentialRefs: [],
  }],
  skills: [skillID],
};
const output: ProfileAgentPluginExport = {
  profileSlug: profile.slug, profileRevision: 4,
  destination: "/tmp/plugins/delivery", name: "delivery", hasMCP: false,
};

function show(input: {
  chosen?: Profile;
  exporter?: (slug: string, input: ExportProfilePluginInput) => Promise<ProfileAgentPluginExport | null>;
} = {}) {
  const getProfile = vi.fn().mockResolvedValue(input.chosen ?? profile);
  const client = { getProfile } as unknown as MenderClient;
  render(<MemoryRouter initialEntries={["/profiles/delivery/deliveries/plugin"]}>
    <TooltipProvider><Routes>
      <Route path="/profiles/:slug/deliveries/plugin" element={<ProfileAgentPluginPage
        client={client}
        {...(input.exporter === undefined ? {} : { exportProfilePlugin: input.exporter })} />} />
      <Route path="/profiles/:slug" element={<p>Profile detail</p>} />
    </Routes></TooltipProvider>
  </MemoryRouter>);
  return { getProfile };
}

beforeEach(() => { resetLocaleForTesting(); setLocale("en"); });
afterEach(() => { cleanup(); resetLocaleForTesting(); });

it("exports exact selected Profile Skills with an explicit native folder chooser", async () => {
  const exporter = vi.fn().mockResolvedValue(output);
  const { getProfile } = show({ exporter });
  expect(await screen.findByText("Delivery")).toBeTruthy();
  expect(getProfile).toHaveBeenCalledWith("delivery");
  expect(screen.getByRole("checkbox", { name: "Include remote MCP connection" })).toHaveProperty("checked", false);
  fireEvent.click(screen.getByRole("button", { name: "Choose folder and export" }));
  await waitFor(() => expect(exporter).toHaveBeenCalledWith("delivery", {
    name: "delivery", version: "1.0.0", description: "", remoteMCPURL: "",
  }));
  expect(await screen.findByText(output.destination)).toBeTruthy();
  expect(screen.getByText(/revision 4/)).toBeTruthy();
});

it("requires explicit valid HTTPS before adding remote MCP and rejects invalid inputs", async () => {
  const exporter = vi.fn().mockResolvedValue({ ...output, hasMCP: true });
  show({ exporter });
  await screen.findByText("Delivery");
  fireEvent.click(screen.getByRole("checkbox", { name: "Include remote MCP connection" }));
  const submit = screen.getByRole("button", { name: "Choose folder and export" });
  expect(submit).toHaveProperty("disabled", true);
  fireEvent.change(screen.getByLabelText("Remote MCP URL"), { target: { value: "http://127.0.0.1:8765/mcp" } });
  expect(submit).toHaveProperty("disabled", true);
  fireEvent.change(screen.getByLabelText("Remote MCP URL"), { target: { value: "https://mcp.example.com/mcp?secret=bad" } });
  expect(submit).toHaveProperty("disabled", true);
  fireEvent.change(screen.getByLabelText("Remote MCP URL"), { target: { value: "https://mcp.example.com/mcp" } });
  expect(submit).toHaveProperty("disabled", false);
  fireEvent.change(screen.getByLabelText("Plugin package name"), { target: { value: "../unsafe" } });
  expect(submit).toHaveProperty("disabled", true);
  fireEvent.change(screen.getByLabelText("Plugin package name"), { target: { value: "safe.bundle" } });
  fireEvent.click(submit);
  await waitFor(() => expect(exporter).toHaveBeenCalledWith("delivery", {
    name: "safe.bundle", version: "1.0.0", description: "", remoteMCPURL: "https://mcp.example.com/mcp",
  }));
});

it("supports a Tools-only Profile with an explicit MCP dependency", async () => {
  const exporter = vi.fn().mockResolvedValue({ ...output, hasMCP: true });
  show({ chosen: { ...profile, skills: [] }, exporter });
  await screen.findByRole("button", { name: "Choose folder and export" });
  expect(screen.getByRole("button", { name: "Choose folder and export" })).toHaveProperty("disabled", true);
  fireEvent.click(screen.getByRole("checkbox", { name: "Include remote MCP connection" }));
  fireEvent.change(screen.getByLabelText("Remote MCP URL"), { target: { value: "https://example.org/mcp" } });
  fireEvent.click(screen.getByRole("button", { name: "Choose folder and export" }));
  await waitFor(() => expect(exporter).toHaveBeenCalledWith("delivery", expect.objectContaining({
    remoteMCPURL: "https://example.org/mcp",
  })));
});

it("does not claim success on a cancelled native folder chooser and preserves errors for retry", async () => {
  const exporter = vi.fn().mockResolvedValueOnce(null)
    .mockRejectedValueOnce(new Error("Output directory already exists"));
  show({ exporter });
  await screen.findByRole("button", { name: "Choose folder and export" });
  fireEvent.click(screen.getByRole("button", { name: "Choose folder and export" }));
  await waitFor(() => expect(exporter).toHaveBeenCalledTimes(1));
  expect(screen.queryByText("Plugin package exported")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Choose folder and export" }));
  expect((await screen.findByRole("alert")).textContent).toContain("already exists");
  expect(screen.queryByText("Plugin package exported")).toBeNull();
  expect(screen.getByLabelText("Plugin package name")).toHaveProperty("value", "delivery");
});

it("keeps remote management read-only; no host path selection or package writes", async () => {
  show();
  expect(await screen.findByText(/Portable package export requires Mender Desktop/)).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Choose folder and export" })).toBeNull();
});

it("never activates remote MCP for a Profile without Tool membership", async () => {
  const exporter = vi.fn();
  show({ chosen: { ...profile, tools: [] }, exporter });
  await screen.findByRole("checkbox", { name: "Include remote MCP connection" });
  expect(screen.getByRole("checkbox", { name: "Include remote MCP connection" })).toHaveProperty("disabled", true);
  expect(exporter).not.toHaveBeenCalled();
});
