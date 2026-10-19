import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Credential, MenderClient } from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { CredentialsPage } from "./credentials-page";

const connected: Credential = {
  reference: "credential://oauth/work",
  namespace: "oauth",
  name: "work",
  displayName: "OAuth Work",
  source: "oauth",
  environmentVariable: "",
  oauth: {
    resource: "https://mcp.example.test/mcp",
    issuer: "https://id.example.test",
    clientId: "mender-public",
    tokenEndpoint: "https://id.example.test/token",
    scopes: ["read"],
    status: "connected",
  },
  createdAt: "2026-10-19T00:00:00Z",
  updatedAt: "2026-10-19T00:00:00Z",
};

beforeEach(() => {
  window.localStorage.clear();
  resetLocaleForTesting();
  setLocale("en");
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  resetLocaleForTesting();
  window.localStorage.clear();
});

describe("OAuth Credential inventory", () => {
  it("displays account and status without treating OAuth as a native secret", async () => {
    const client = {
      listCredentials: vi.fn().mockResolvedValue({ credentials: [connected] }),
    } as unknown as MenderClient;
    render(<TooltipProvider><CredentialsPage client={client} /></TooltipProvider>);
    expect(await screen.findByText("OAuth Work")).toBeTruthy();
    expect(screen.getByText("OAuth · Connected")).toBeTruthy();
    expect(screen.getByText("https://mcp.example.test/mcp")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Rotate" })).toBeNull();
    expect(screen.queryByText("Bearer")).toBeNull();
  });

  it("disconnects a connected OAuth Credential but preserves its inventory entry", async () => {
    const disconnected: Credential = {
      ...connected,
      oauth: { ...connected.oauth!, status: "authorization_required" },
    };
    const listCredentials = vi.fn()
      .mockResolvedValueOnce({ credentials: [connected] })
      .mockResolvedValue({ credentials: [disconnected] });
    const disconnectOAuthCredential = vi.fn().mockResolvedValue(disconnected);
    const client = { listCredentials, disconnectOAuthCredential } as unknown as MenderClient;
    vi.spyOn(window, "confirm").mockReturnValue(true);
    render(<TooltipProvider><CredentialsPage client={client} /></TooltipProvider>);
    expect(await screen.findByText("OAuth Work")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Disconnect OAuth" }));
    await waitFor(() => expect(disconnectOAuthCredential).toHaveBeenCalledWith("oauth", "work"));
    expect(await screen.findByText("OAuth · Authorization required")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Disconnect OAuth" })).toBeNull();
  });
});
