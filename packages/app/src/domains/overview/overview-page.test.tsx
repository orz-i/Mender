import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { MenderClient } from "@mender/sdk";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { OverviewPage } from "./overview-page";

beforeEach(() => {
  resetLocaleForTesting();
  setLocale("en");
});

afterEach(() => {
  cleanup();
  resetLocaleForTesting();
});

describe("OverviewPage load states", () => {
  it("shows a retriable error instead of fake zero metrics or empty activity", async () => {
    const getSystemStatus = vi.fn()
      .mockRejectedValueOnce(new Error("Runtime request failed"))
      .mockResolvedValue({ ready: true });
    const client = {
      getSystemStatus,
      listProviders: vi.fn().mockResolvedValue({ providers: [] }),
      listProfiles: vi.fn().mockResolvedValue({ profiles: [] }),
      listSkills: vi.fn().mockResolvedValue({ installations: [] }),
      listAudit: vi.fn().mockResolvedValue({ records: [] }),
      browseTools: vi.fn().mockResolvedValue({ tools: [] }),
    } as unknown as MenderClient;

    render(<MemoryRouter><OverviewPage client={client} /></MemoryRouter>);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Runtime request failed");
    expect(screen.queryByText("No invocation records yet.")).toBeNull();
    expect(document.querySelector('[data-slot="metric-strip"]')).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
    await screen.findByText("Get started with tool providers");
    await waitFor(() => expect(getSystemStatus).toHaveBeenCalledTimes(2));
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.queryByText("No invocation records yet.")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Providers / Profiles" }));
    expect(document.querySelector('[data-slot="metric-strip"]')).not.toBeNull();
  });
});
