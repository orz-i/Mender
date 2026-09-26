import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { MenderClient } from "@mender/sdk";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";

import { MenderApp } from "./mender-app";

afterEach(cleanup);

describe("management pages", () => {
  it("registers an HTTP Provider through the SDK boundary", async () => {
    const listProviders = vi
      .fn()
      .mockResolvedValueOnce({ providers: [] })
      .mockResolvedValueOnce({
        providers: [
          {
            providerId: "prov_1",
            namespace: "demo",
            displayName: "Demo",
            description: "",
            createdAt: "2026-09-26T00:00:00Z",
            updatedAt: "2026-09-26T00:00:00Z",
            transport: "http",
            endpoint: "http://example.test/mcp",
          },
        ],
      });
    const createHTTPProvider = vi.fn().mockResolvedValue({
      provider: {
        providerId: "prov_1",
        namespace: "demo",
        displayName: "Demo",
        description: "",
        createdAt: "2026-09-26T00:00:00Z",
        updatedAt: "2026-09-26T00:00:00Z",
        transport: "http",
        endpoint: "http://example.test/mcp",
      },
      sync: { created: 1, updated: 0, removed: 0, total: 1 },
    });

    renderApp("/providers", {
      listProviders,
      createHTTPProvider,
    });

    expect(await screen.findByText("No providers")).toBeTruthy();

    fireEvent.change(screen.getAllByPlaceholderText("namespace")[0]!, {
      target: { value: "demo" },
    });
    fireEvent.change(screen.getAllByPlaceholderText("Display name")[0]!, {
      target: { value: "Demo" },
    });
    fireEvent.change(screen.getByPlaceholderText("https://example.com/mcp"), {
      target: { value: "http://example.test/mcp" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Register HTTP provider" }),
    );

    await waitFor(() => {
      expect(createHTTPProvider).toHaveBeenCalledWith({
        namespace: "demo",
        displayName: "Demo",
        endpoint: "http://example.test/mcp",
      });
    });
    expect(await screen.findByText("demo")).toBeTruthy();
  });

  it("keeps Catalog schemas deferred until a search result is selected", async () => {
    const getTool = vi.fn().mockResolvedValue({
      toolId: "tool_1",
      providerId: "prov_1",
      sourceKey: "issues.create",
      name: "create_issue",
      title: "Create issue",
      description: "Create a repository issue",
      inputSchema: { type: "object" },
      outputSchema: { type: "object" },
      tags: ["mcp"],
    });

    renderApp("/catalog", {
      listProfiles: vi.fn().mockResolvedValue({ profiles: [] }),
      searchTools: vi.fn().mockResolvedValue({
        tools: [
          {
            rank: 1,
            toolId: "tool_1",
            providerId: "prov_1",
            name: "create_issue",
            title: "Create issue",
            description: "Create a repository issue",
          },
        ],
      }),
      getTool,
    });

    fireEvent.change(
      screen.getByPlaceholderText("Search intent, e.g. create issue"),
      { target: { value: "create issue" } },
    );
    fireEvent.click(screen.getByRole("button", { name: "Search" }));

    expect(await screen.findByText("Create issue")).toBeTruthy();
    expect(getTool).not.toHaveBeenCalled();
    expect(screen.queryByText("Input schema")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: /Create issue/ }));

    expect(await screen.findByText("Input schema")).toBeTruthy();
    expect(getTool).toHaveBeenCalledWith("tool_1");
  });

  it("renders persisted Profile membership management", async () => {
    renderApp("/profiles", {
      listProfiles: vi.fn().mockResolvedValue({
        profiles: [
          {
            profileId: "prof_1",
            slug: "default",
            displayName: "Default",
            mode: "hybrid",
            tools: [{ toolId: "tool_1", direct: true }],
          },
        ],
      }),
    });

    expect((await screen.findAllByText("Default")).length).toBeGreaterThan(0);
    expect(screen.getByText("Direct export")).toBeTruthy();
    expect(screen.getByPlaceholderText("tool_...")).toBeTruthy();
  });

  it("shows Policy and Credential capabilities as read-only", async () => {
    const policyClient = renderApp("/policy", {
      getPolicyStatus: vi.fn().mockResolvedValue({
        mode: "local_allow_all",
        editable: false,
        approval: "fail_closed_when_required",
      }),
    });
    expect(await screen.findByText("local_allow_all")).toBeTruthy();
    expect(screen.getByText("Read only")).toBeTruthy();
    policyClient.unmount();

    renderApp("/credentials", {
      getCredentialStatus: vi.fn().mockResolvedValue({
        resolver: "unavailable",
        persistentStore: false,
        editable: false,
      }),
    });
    expect(await screen.findByText("unavailable")).toBeTruthy();
    expect(screen.getByText("Read only")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /add credential/i })).toBeNull();
  });

  it("renders only minimized Audit metadata", async () => {
    renderApp("/audit", {
      listAudit: vi.fn().mockResolvedValue({
        records: [
          {
            invocationId: "inv_1",
            toolId: "tool_1",
            providerId: "prov_1",
            caller: "web",
            startedAt: "2026-09-26T00:00:00Z",
            finishedAt: "2026-09-26T00:00:01Z",
            policyKind: "allow",
            policyReason: "local.default",
            approval: "not_required",
            status: "completed",
            resultKind: "complete",
            errorClass: "",
          },
        ],
      }),
    });

    expect(await screen.findByText("completed")).toBeTruthy();
    expect(screen.getAllByText("tool_1").length).toBeGreaterThan(0);
    expect(screen.queryByRole("columnheader", { name: "Arguments" })).toBeNull();
    expect(screen.queryByRole("columnheader", { name: "Result" })).toBeNull();
    expect(screen.queryByRole("columnheader", { name: "Secret" })).toBeNull();
  });
});

function renderApp(path: string, client: Partial<MenderClient>) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <MenderApp client={client as MenderClient} />
    </MemoryRouter>,
  );
}
