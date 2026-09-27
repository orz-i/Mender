import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type { MenderClient } from "@mender/sdk";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";

import { MenderApp } from "./mender-app";

afterEach(cleanup);

describe("management pages", () => {
  it("keeps Provider creation out of the primary list surface", async () => {
    const provider = {
      providerId: "prov_1",
      namespace: "demo",
      displayName: "Demo",
      description: "",
      createdAt: "2026-09-26T00:00:00Z",
      updatedAt: "2026-09-26T00:00:00Z",
      transport: "http" as const,
      toolCount: 1,
      endpoint: "https://example.test/mcp",
    };
    const listProviders = vi
      .fn()
      .mockResolvedValueOnce({ providers: [] })
      .mockResolvedValueOnce({ providers: [provider] });
    const createHTTPProvider = vi.fn().mockResolvedValue({
      provider,
      sync: { created: 1, updated: 0, removed: 0, total: 1 },
    });

    renderApp("/providers", {
      listProviders,
      createHTTPProvider,
    });

    expect(await screen.findByText("No providers")).toBeTruthy();
    expect(screen.queryByText("Remote MCP")).toBeNull();
    expect(screen.queryByText("Local MCP")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Add provider" }));
    expect(screen.getByRole("dialog")).toBeTruthy();
    expect(screen.getByText("Remote MCP")).toBeTruthy();
    expect(screen.getByText("Local MCP")).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Namespace"), {
      target: { value: "demo" },
    });
    fireEvent.change(screen.getByLabelText("Display name"), {
      target: { value: "Demo" },
    });
    fireEvent.change(screen.getByLabelText("Endpoint"), {
      target: { value: "https://example.test/mcp" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add provider" }));

    await waitFor(() => {
      expect(createHTTPProvider).toHaveBeenCalledWith({
        namespace: "demo",
        displayName: "Demo",
        endpoint: "https://example.test/mcp",
      });
    });
    expect(await screen.findByText("demo")).toBeTruthy();
  });

  it("imports common remote MCP JSON with env-backed headers", async () => {
    const provider = {
      providerId: "prov_anysearch",
      namespace: "anysearch",
      displayName: "anysearch",
      description: "",
      createdAt: "2026-09-26T00:00:00Z",
      updatedAt: "2026-09-26T00:00:00Z",
      transport: "http" as const,
      toolCount: 4,
      endpoint: "https://api.anysearch.com/mcp",
      headers: {
        Authorization: "Bearer ${ANYSEARCH_API_KEY}",
      },
    };
    const listProviders = vi
      .fn()
      .mockResolvedValueOnce({ providers: [] })
      .mockResolvedValueOnce({ providers: [provider] });
    const createHTTPProvider = vi.fn().mockResolvedValue({
      provider,
      sync: { created: 4, updated: 0, removed: 0, total: 4 },
    });

    renderApp("/providers", {
      listProviders,
      createHTTPProvider,
    });

    expect(await screen.findByText("No providers")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Add provider" }));
    fireEvent.click(screen.getByText("Advanced settings"));

    fireEvent.change(screen.getByLabelText("MCP JSON"), {
      target: {
        value: JSON.stringify({
          mcpServers: {
            anysearch: {
              type: "streamable-http",
              url: "https://api.anysearch.com/mcp",
              headers: {
                Authorization: "Bearer ${ANYSEARCH_API_KEY}",
              },
            },
          },
        }),
      },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Import configuration" }),
    );

    expect(
      screen.getByLabelText<HTMLInputElement>("Namespace").value,
    ).toBe("anysearch");
    expect(
      screen.getByLabelText<HTMLInputElement>("Endpoint").value,
    ).toBe("https://api.anysearch.com/mcp");
    expect(
      screen.getByLabelText<HTMLTextAreaElement>("HTTP header templates").value,
    ).toContain("Authorization: Bearer ${ANYSEARCH_API_KEY}");

    fireEvent.click(screen.getByRole("button", { name: "Add provider" }));

    await waitFor(() => {
      expect(createHTTPProvider).toHaveBeenCalledWith({
        namespace: "anysearch",
        displayName: "anysearch",
        endpoint: "https://api.anysearch.com/mcp",
        headers: {
          Authorization: "Bearer ${ANYSEARCH_API_KEY}",
        },
      });
    });
  });

  it("registers a stdio Provider from the same Add dialog", async () => {
    const provider = {
      providerId: "prov_2",
      namespace: "local",
      displayName: "Local Tools",
      description: "",
      createdAt: "2026-09-26T00:00:00Z",
      updatedAt: "2026-09-26T00:00:00Z",
      transport: "stdio" as const,
      toolCount: 1,
      executable: "/usr/local/bin/local-tools",
      args: ["--stdio"],
    };
    const listProviders = vi
      .fn()
      .mockResolvedValueOnce({ providers: [] })
      .mockResolvedValueOnce({ providers: [provider] });
    const createStdioProvider = vi.fn().mockResolvedValue({
      provider,
      sync: { created: 1, updated: 0, removed: 0, total: 1 },
    });

    renderApp("/providers", {
      listProviders,
      createStdioProvider,
    });

    expect(await screen.findByText("No providers")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Add provider" }));
    fireEvent.click(screen.getByRole("button", { name: "Local MCP" }));

    fireEvent.change(screen.getByLabelText("Namespace"), {
      target: { value: "local" },
    });
    fireEvent.change(screen.getByLabelText("Display name"), {
      target: { value: "Local Tools" },
    });
    fireEvent.change(screen.getByLabelText("Executable"), {
      target: { value: "/usr/local/bin/local-tools" },
    });
    fireEvent.change(screen.getByLabelText("Arguments"), {
      target: { value: "--stdio" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add provider" }));

    await waitFor(() => {
      expect(createStdioProvider).toHaveBeenCalledWith({
        namespace: "local",
        displayName: "Local Tools",
        executable: "/usr/local/bin/local-tools",
        args: ["--stdio"],
      });
    });
    expect(await screen.findByText("Local Tools")).toBeTruthy();
  });

  it("views and edits a Provider without leaving the list page", async () => {
    const original = {
      providerId: "prov_1",
      namespace: "search",
      displayName: "AnySearch",
      description: "Search provider",
      createdAt: "2026-09-26T00:00:00Z",
      updatedAt: "2026-09-26T00:00:00Z",
      transport: "http" as const,
      toolCount: 4,
      endpoint: "https://api.anysearch.com/mcp",
    };
    const updated = {
      ...original,
      displayName: "AnySearch Production",
      description: "Production search provider",
      updatedAt: "2026-09-26T01:00:00Z",
    };
    const listProviders = vi
      .fn()
      .mockResolvedValueOnce({ providers: [original] })
      .mockResolvedValueOnce({ providers: [updated] });
    const updateProvider = vi.fn().mockResolvedValue({
      provider: updated,
      sync: { created: 0, updated: 1, removed: 0, total: 1 },
    });

    renderApp("/providers", {
      listProviders,
      updateProvider,
    });

    expect(await screen.findByText("AnySearch")).toBeTruthy();
    expect(screen.queryByText("prov_1")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "View search" }));
    expect(await screen.findByText("Provider details")).toBeTruthy();
    expect(screen.getByText("prov_1")).toBeTruthy();
    expect(
      screen.getAllByText("https://api.anysearch.com/mcp").length,
    ).toBeGreaterThanOrEqual(2);

    const closeButtons = screen.getAllByRole("button", { name: "Close" });
    fireEvent.click(closeButtons[0]!);

    fireEvent.click(screen.getByRole("button", { name: "Edit search" }));
    expect(await screen.findByText("Edit provider")).toBeTruthy();
    expect(screen.getAllByText("search").length).toBeGreaterThanOrEqual(2);

    fireEvent.change(screen.getByLabelText("Display name"), {
      target: { value: "AnySearch Production" },
    });
    fireEvent.change(screen.getByLabelText("Description"), {
      target: { value: "Production search provider" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(updateProvider).toHaveBeenCalledWith("prov_1", {
        transport: "http",
        displayName: "AnySearch Production",
        description: "Production search provider",
        endpoint: "https://api.anysearch.com/mcp",
      });
    });
    expect(await screen.findByText("AnySearch Production")).toBeTruthy();
  });

  it("filters Catalog browse and search by Provider", async () => {
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

    const browseTools = vi.fn().mockResolvedValue({
      tools: [
        {
          rank: 1,
          toolId: "tool_browse",
          providerId: "prov_1",
          name: "web_search",
          title: "Web search",
          description: "Browse synchronized Tool",
        },
      ],
    });
    const searchTools = vi.fn().mockResolvedValue({
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
    });

    renderApp("/catalog", {
      listProviders: vi.fn().mockResolvedValue({
        providers: [
          {
            providerId: "prov_1",
            namespace: "demo",
            displayName: "Demo",
            description: "",
            createdAt: "2026-09-26T00:00:00Z",
            updatedAt: "2026-09-26T00:00:00Z",
            transport: "http",
            toolCount: 2,
            endpoint: "https://example.test/mcp",
          },
        ],
      }),
      browseTools,
      searchTools,
      getTool,
    });

    expect(await screen.findByText("Web search")).toBeTruthy();
    expect(getTool).not.toHaveBeenCalled();
    expect(screen.queryByLabelText("Catalog profile filter")).toBeNull();
    expect(screen.getByRole("table").classList.contains("table-fixed")).toBe(
      true,
    );
    expect(screen.getByRole("columnheader", { name: "Rank" }).classList).toContain(
      "hidden",
    );
    expect(
      screen.getByRole("columnheader", { name: "Actions" }),
    ).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Catalog provider filter"), {
      target: { value: "prov_1" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Browse" }));
    await waitFor(() => {
      expect(browseTools).toHaveBeenLastCalledWith({
        limit: 100,
        providerId: "prov_1",
      });
    });

    fireEvent.change(
      screen.getByPlaceholderText("Search intent, e.g. create issue"),
      { target: { value: "create issue" } },
    );
    fireEvent.click(screen.getByRole("button", { name: "Search" }));

    await waitFor(() => {
      expect(searchTools).toHaveBeenCalledWith({
        query: "create issue",
        limit: 30,
        providerId: "prov_1",
      });
    });
    expect(await screen.findByText("Create issue")).toBeTruthy();
    expect(getTool).not.toHaveBeenCalled();
    expect(screen.queryByText("Input schema")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: /Create issue/ }));

    expect(await screen.findByText("Create issue")).toBeTruthy();
    expect(getTool).toHaveBeenCalledWith("tool_1");
    expect(within(screen.getByRole("dialog")).getByText("Description")).toBeTruthy();
    expect(screen.queryByText("Input schema")).toBeNull();
    expect(screen.queryByLabelText("Invocation arguments")).toBeNull();

    fireEvent.click(
      screen.getByRole("button", { name: "Read full description" }),
    );
    expect(await screen.findByText("Tool description")).toBeTruthy();
    expect(screen.queryByText("Input schema")).toBeNull();
    expect(screen.queryByLabelText("Invocation arguments")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    fireEvent.click(screen.getByRole("button", { name: "View schemas" }));
    expect(await screen.findByText("Input schema")).toBeTruthy();
    expect(screen.queryByLabelText("Invocation arguments")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    fireEvent.click(screen.getByRole("button", { name: "Invoke tool" }));
    expect(await screen.findByLabelText("Invocation arguments")).toBeTruthy();
    expect(screen.queryByText("Input schema")).toBeNull();
  });

  it("manages Profile membership through a Tool picker without ToolID input", async () => {
    const profile = {
      profileId: "prof_1",
      slug: "default",
      displayName: "Default",
      mode: "hybrid" as const,
      tools: [
        {
          toolId: "tool_1",
          direct: true,
          alias: "search_prod",
          titleOverride: "",
          descriptionOverride: "",
          policyConstraint: "inherit" as const,
          concurrencyConstraint: "serial" as const,
        },
      ],
    };
    const replaceProfileTools = vi.fn().mockImplementation(
      (_slug: string, body: { tools: Array<Record<string, unknown>> }) =>
        Promise.resolve({
          ...profile,
          tools: body.tools.map((tool) => ({
            direct: false,
            alias: "",
            titleOverride: "",
            descriptionOverride: "",
            policyConstraint: "inherit" as const,
            concurrencyConstraint: "inherit" as const,
            ...tool,
          })),
        }),
    );

    renderApp("/profiles", {
      listProfiles: vi.fn().mockResolvedValue({ profiles: [profile] }),
      listProviders: vi.fn().mockResolvedValue({
        providers: [
          {
            providerId: "prov_1",
            namespace: "search",
            displayName: "AnySearch",
            description: "",
            createdAt: "2026-09-26T00:00:00Z",
            updatedAt: "2026-09-26T00:00:00Z",
            transport: "http",
            toolCount: 2,
            endpoint: "https://api.anysearch.com/mcp",
          },
        ],
      }),
      browseTools: vi.fn().mockResolvedValue({
        tools: [
          {
            rank: 1,
            toolId: "tool_1",
            providerId: "prov_1",
            name: "search",
            title: "Search",
            description: "Search the web",
          },
          {
            rank: 2,
            toolId: "tool_2",
            providerId: "prov_1",
            name: "extract",
            title: "Extract",
            description: "Extract content",
          },
        ],
      }),
      replaceProfileTools,
    });

    expect((await screen.findAllByText("Default")).length).toBeGreaterThan(0);
    expect(screen.queryByText("tool_1")).toBeNull();
    expect(screen.queryByPlaceholderText("tool_...")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Manage default" }));
    expect(await screen.findByText("Search")).toBeTruthy();
    expect(screen.queryByText("tool_1")).toBeNull();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);

    fireEvent.click(screen.getByRole("button", { name: "Choose tools" }));
    expect(await screen.findByText("Extract")).toBeTruthy();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(screen.getByText("Extract content").classList.contains("line-clamp-2")).toBe(
      true,
    );
    expect(
      screen.getByRole<HTMLInputElement>("checkbox", { name: "Select Extract" })
        .disabled,
    ).toBe(false);

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(await screen.findByRole("button", { name: "Choose tools" })).toBeTruthy();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);

    fireEvent.click(screen.getByRole("button", { name: "Choose tools" }));
    expect(await screen.findByText("Extract")).toBeTruthy();
    fireEvent.click(screen.getByRole("checkbox", { name: "Select Extract" }));
    fireEvent.click(screen.getByRole("button", { name: "Save selection" }));

    await waitFor(() => {
      expect(replaceProfileTools).toHaveBeenCalledWith("default", {
        tools: [
          {
            toolId: "tool_1",
            direct: true,
            alias: "search_prod",
            titleOverride: "",
            descriptionOverride: "",
            policyConstraint: "inherit",
            concurrencyConstraint: "serial",
          },
          { toolId: "tool_2", direct: false },
        ],
      });
    });
    expect(await screen.findByText("Extract")).toBeTruthy();
    expect(screen.getByText("Alias: search_prod")).toBeTruthy();
    expect(screen.getByText("Serial")).toBeTruthy();
    expect(screen.queryByPlaceholderText("tool_...")).toBeNull();

    fireEvent.click(
      screen.getByRole("button", { name: "Presentation for Search" }),
    );
    expect(await screen.findByText("Tool presentation")).toBeTruthy();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(screen.queryByRole("button", { name: "Choose tools" })).toBeNull();

    fireEvent.change(screen.getByLabelText("Tool binding alias"), {
      target: { value: "search_release" },
    });
    fireEvent.change(screen.getByLabelText("Tool binding title override"), {
      target: { value: "Release Search" },
    });
    const longDescription =
      "Search release documentation with a deliberately long Profile-specific description that remains truncated in the selected Tool row.";
    fireEvent.change(screen.getByLabelText("Tool binding description override"), {
      target: { value: longDescription },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save presentation" }));

    expect(await screen.findByText("Release Search")).toBeTruthy();
    expect(screen.getByText("Alias: search_release")).toBeTruthy();
    expect(screen.getByText(/Search release documentation/).classList).toContain(
      "truncate",
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Governance for Release Search" }),
    );
    expect(await screen.findByText("Tool governance")).toBeTruthy();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    fireEvent.change(screen.getByLabelText("Binding policy constraint"), {
      target: { value: "deny" },
    });
    fireEvent.change(screen.getByLabelText("Binding concurrency constraint"), {
      target: { value: "serial" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save governance" }));

    expect(await screen.findByText("Denied")).toBeTruthy();
    expect(screen.getByText("Serial")).toBeTruthy();
    expect(screen.queryByText("tool_1")).toBeNull();
  });

  it("starts, restarts, and stops a managed MCP server", async () => {
    const profile = {
      profileId: "prof_1",
      slug: "default",
      displayName: "Default",
      mode: "hybrid" as const,
      tools: [{ toolId: "tool_1", direct: true }],
    };
    const started = {
      serverId: "srv_1",
      profile: "default",
      transport: "streamable-http" as const,
      address: "127.0.0.1:8765",
      endpoint: "http://127.0.0.1:8765/mcp",
      status: "running" as const,
      startedAt: "2026-09-26T09:00:00Z",
    };
    const restarted = {
      ...started,
      serverId: "srv_2",
      endpoint: "http://127.0.0.1:8766/mcp",
      address: "127.0.0.1:8766",
      startedAt: "2026-09-26T09:01:00Z",
    };

    const startMCPServer = vi.fn().mockResolvedValue(started);
    const restartMCPServer = vi.fn().mockResolvedValue(restarted);
    const stopMCPServer = vi.fn().mockResolvedValue(undefined);

    renderApp("/servers", {
      listProfiles: vi.fn().mockResolvedValue({ profiles: [profile] }),
      listMCPServers: vi.fn().mockResolvedValue({ servers: [] }),
      startMCPServer,
      restartMCPServer,
      stopMCPServer,
    });

    expect(await screen.findByText("No running MCP servers")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Start server" }));
    expect(await screen.findByText("Start MCP server")).toBeTruthy();
    expect(
      screen.getByLabelText<HTMLSelectElement>("Server profile").value,
    ).toBe("default");

    fireEvent.click(screen.getByRole("button", { name: "Start server" }));

    await waitFor(() => {
      expect(startMCPServer).toHaveBeenCalledWith({
        profile: "default",
        address: "127.0.0.1:8765",
      });
    });
    expect(await screen.findByText(started.endpoint)).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Restart" }));
    await waitFor(() => {
      expect(restartMCPServer).toHaveBeenCalledWith("srv_1");
    });
    expect(await screen.findByText(restarted.endpoint)).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Stop" }));
    await waitFor(() => {
      expect(stopMCPServer).toHaveBeenCalledWith("srv_2");
    });
    expect(await screen.findByText("No running MCP servers")).toBeTruthy();
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
            errorSummary: "",
            origin: "management_http",
            scopeKind: "catalog",
            scopeReference: "",
            outcomeKind: "complete",
            rejectionCode: "",
            failureSource: "",
            failureCode: "",
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

  it("shows a bounded Audit failure summary in a detail dialog", async () => {
    renderApp("/audit", {
      listAudit: vi.fn().mockResolvedValue({
        records: [
          {
            invocationId: "inv_failed",
            toolId: "tool_1",
            providerId: "prov_1",
            caller: "desktop",
            startedAt: "2026-09-26T00:00:00Z",
            finishedAt: "2026-09-26T00:00:01Z",
            policyKind: "allow",
            policyReason: "local.default",
            approval: "not_required",
            status: "failed",
            resultKind: "",
            errorClass: "execution",
            errorSummary: "provider rejected request: missing required query",
            origin: "desktop",
            scopeKind: "profile",
            scopeReference: "prof_1",
            outcomeKind: "failed",
            rejectionCode: "",
            failureSource: "provider",
            failureCode: "provider_error",
          },
        ],
      }),
    });

    expect(await screen.findByText("failed")).toBeTruthy();
    fireEvent.click(
      screen.getByRole("button", { name: "View audit inv_failed" }),
    );

    expect(await screen.findByText("Invocation details")).toBeTruthy();
    expect(
      screen.getByText("provider rejected request: missing required query"),
    ).toBeTruthy();
    const dialog = within(screen.getByRole("dialog"));
    expect(dialog.getByText("execution")).toBeTruthy();
    expect(dialog.getAllByText("desktop").length).toBeGreaterThanOrEqual(2);
    expect(dialog.getByText("profile")).toBeTruthy();
    expect(dialog.getByText("provider")).toBeTruthy();
    expect(dialog.getByText("provider_error")).toBeTruthy();
    expect(screen.queryByText(/TOP-SECRET/)).toBeNull();
  });
});

function renderApp(path: string, client: Partial<MenderClient>) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <MenderApp client={client as MenderClient} />
    </MemoryRouter>,
  );
}
