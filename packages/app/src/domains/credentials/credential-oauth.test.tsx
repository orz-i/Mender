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

describe("Interactive MCP OAuth authorization", () => {
  it("starts authorization without a raw secret and offers a browser link", async () => {
    const startOAuthCredential = vi.fn().mockResolvedValue({
      flowId: "mfo_test", authorizationUrl: "https://accounts.example.test/authorize?state=public-fixture",
    });
    const getOAuthFlow = vi.fn().mockResolvedValue({
      flowId: "mfo_test", status: "pending", createdAt: "2026-10-19T00:00:00Z", expiresAt: "2026-10-19T00:10:00Z",
    });
    const cancelOAuthFlow = vi.fn().mockResolvedValue(undefined);
    const listCredentials = vi.fn().mockResolvedValue({ credentials: [] });
    const client = { listCredentials, startOAuthCredential, getOAuthFlow, cancelOAuthFlow } as unknown as MenderClient;
    render(<TooltipProvider><CredentialsPage client={client} /></TooltipProvider>);
    fireEvent.click(screen.getByRole("button", { name: "Add credential" }));
    fireEvent.change(screen.getByRole("combobox", { name: "Credential source" }), { target: { value: "oauth" } });
    expect(screen.queryByRole("textbox", { name: "Credential secret" })).toBeNull();
    fireEvent.change(screen.getByRole("textbox", { name: "Credential namespace" }), { target: { value: "OAuth" } });
    fireEvent.change(screen.getByRole("textbox", { name: "Credential name" }), { target: { value: "Work" } });
    fireEvent.change(screen.getByRole("textbox", { name: "Credential display name" }), { target: { value: "Work OAuth" } });
    fireEvent.change(screen.getByRole("textbox", { name: "OAuth MCP endpoint" }), { target: { value: "https://mcp.example.test/mcp" } });
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Connect OAuth" }).disabled).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "Connect OAuth" }));
    await waitFor(() => expect(startOAuthCredential).toHaveBeenCalledWith({
      namespace: "oauth", name: "work", displayName: "Work OAuth", mcpEndpoint: "https://mcp.example.test/mcp",
    }));
    const link = await screen.findByRole<HTMLAnchorElement>("link", { name: "Open authorization page" });
    expect(link.href).toContain("https://accounts.example.test/authorize");
    await waitFor(() => expect(getOAuthFlow).toHaveBeenCalledWith("mfo_test"));
    fireEvent.click(screen.getByRole("button", { name: "Cancel authorization" }));
    await waitFor(() => expect(cancelOAuthFlow).toHaveBeenCalledWith("mfo_test"));
  });

  it("does not enable OAuth connect for an insecure remote endpoint", () => {
    const client = { listCredentials: vi.fn().mockResolvedValue({ credentials: [] }) } as unknown as MenderClient;
    render(<TooltipProvider><CredentialsPage client={client} /></TooltipProvider>);
    fireEvent.click(screen.getByRole("button", { name: "Add credential" }));
    fireEvent.change(screen.getByRole("combobox", { name: "Credential source" }), { target: { value: "oauth" } });
    fireEvent.change(screen.getByRole("textbox", { name: "Credential namespace" }), { target: { value: "oauth" } });
    fireEvent.change(screen.getByRole("textbox", { name: "Credential name" }), { target: { value: "work" } });
    fireEvent.change(screen.getByRole("textbox", { name: "Credential display name" }), { target: { value: "OAuth Work" } });
    fireEvent.change(screen.getByRole("textbox", { name: "OAuth MCP endpoint" }), { target: { value: "http://insecure.example.test/mcp" } });
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Connect OAuth" }).disabled).toBe(true);
  });
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
    fireEvent.click(screen.getByRole("button", { name: "Manage OAuth" }));
    fireEvent.click(screen.getByRole("button", { name: "Disconnect OAuth" }));
    await waitFor(() => expect(disconnectOAuthCredential).toHaveBeenCalledWith("oauth", "work"));
    expect(await screen.findByText("OAuth · Authorization required")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Disconnect OAuth" })).toBeNull();
  });
});
