import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ProfileCLIInstructions } from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { ProfileCLIDeliveryPage } from "./profile-cli-delivery-page";

const db = "/Users/mini/Library/Application Support/Mender/mender.db";
const base = ["mender", "--db", db];
const recipe: ProfileCLIInstructions = {
  profileSlug: "delivery", databasePath: db, platform: "darwin",
  search: [...base, "profile", "tool", "search", "delivery", "<query>"],
  describe: [...base, "profile", "tool", "describe", "delivery", "<tool-id>"],
  call: [...base, "profile", "tool", "call", "delivery", "<tool-id>", "--args", "{}"],
  mcpStdio: [...base, "serve", "mcp", "--profile", "delivery", "--stdio"],
};

function show(input: {
  getLocalProfileCLI?: (slug: string) => Promise<ProfileCLIInstructions>;
  copyText?: (value: string) => Promise<void>;
} = {}) {
  render(<MemoryRouter initialEntries={["/profiles/delivery/deliveries/cli"]}>
    <TooltipProvider><Routes>
      <Route path="/profiles/:slug/deliveries/cli" element={<ProfileCLIDeliveryPage
        {...input} />} />
      <Route path="/profiles/:slug" element={<p>Profile detail</p>} />
    </Routes></TooltipProvider>
  </MemoryRouter>);
}

beforeEach(() => { resetLocaleForTesting(); setLocale("en"); });
afterEach(() => { cleanup(); resetLocaleForTesting(); });

it("only renders local CLI recipes with this Profile and exact Desktop database", async () => {
  const getLocalProfileCLI = vi.fn().mockResolvedValue(recipe);
  const copyText = vi.fn().mockResolvedValue(undefined);
  show({ getLocalProfileCLI, copyText });
  expect(await screen.findByText(db)).toBeTruthy();
  expect(getLocalProfileCLI).toHaveBeenCalledWith("delivery");
  expect(screen.getAllByRole("button", { name: "Copy" })).toHaveLength(4);
  const examples = screen.getAllByText(/mender --db/);
  expect(examples).toHaveLength(4);
  for (const [index, item] of examples.entries()) {
    if (index < 3) {
      expect(item.textContent).toContain("profile tool");
      expect(item.textContent).toContain("delivery");
      expect(item.textContent).not.toContain("--profile");
    } else {
      expect(item.textContent).toContain("--profile delivery");
    }
    expect(item.textContent).toContain("'" + db + "'");
  }
  fireEvent.click(screen.getAllByRole("button", { name: "Copy" })[2]!);
  await waitFor(() => expect(copyText).toHaveBeenCalledWith(expect.stringContaining("profile tool call delivery")));
  expect(await screen.findByRole("button", { name: "Copied" })).toBeTruthy();
});

it("shows remote management as read-only and does not guess a local database path", async () => {
  show();
  expect(await screen.findByText(/available only on the Mender Desktop host/)).toBeTruthy();
  expect(screen.queryByText(db)).toBeNull();
  expect(screen.queryByRole("button", { name: "Copy" })).toBeNull();
});

it("preserves a recoverable loading error with no copy controls", async () => {
  const getLocalProfileCLI = vi.fn()
    .mockRejectedValueOnce(new Error("Local host data unavailable"))
    .mockResolvedValueOnce(recipe);
  show({ getLocalProfileCLI });
  expect((await screen.findByRole("alert")).textContent).toContain("Local host data unavailable");
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  expect(await screen.findByText(db)).toBeTruthy();
  expect(getLocalProfileCLI).toHaveBeenCalledTimes(2);
});

it("returns to the owning Profile rather than opening a global CLI inventory", async () => {
  show();
  fireEvent.click(screen.getByRole("button", { name: "Back to profile" }));
  expect(await screen.findByText("Profile detail")).toBeTruthy();
});
