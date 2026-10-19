import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { MenderClient } from "@mender/sdk";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { PluginsPage } from "./plugins-page";

beforeEach(() => {
  resetLocaleForTesting();
  setLocale("en");
});

afterEach(() => {
  cleanup();
  resetLocaleForTesting();
});

describe("PluginsPage initial load", () => {
  it("does not confuse request errors with an empty marketplace and offers retry", async () => {
    const listPluginCatalogue = vi.fn()
      .mockRejectedValueOnce(new Error("Catalogue unavailable"))
      .mockResolvedValue({ plugins: [] });
    const client = {
      listPluginCatalogue,
      listIntegrationOfferings: vi.fn().mockResolvedValue({ integrations: [] }),
    } as unknown as MenderClient;

    render(<MemoryRouter><PluginsPage client={client} /></MemoryRouter>);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Catalogue unavailable");
    expect(screen.queryByText("No matching plugins")).toBeNull();

    fireEvent.click(within(alert).getByRole("button", { name: "Refresh" }));
    await screen.findByText("No matching plugins");
    expect(screen.queryByRole("alert")).toBeNull();
    expect(listPluginCatalogue).toHaveBeenCalledTimes(2);
  });
});
