import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type { MenderClient, PolicyRuleInput } from "@mender/sdk";
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
      pluginId: "io.mender.provider.mcp",
      pluginDisplayName: "MCP",
      configVersion: 1,
      transport: "http" as const,
      toolCount: 1,
      endpoint: "https://example.test/mcp",
    };
    const credential = {
      reference: "credential://demo/work",
      namespace: "demo",
      name: "work",
      displayName: "Demo work",
      source: "environment" as const,
      environmentVariable: "DEMO_TOKEN",
      createdAt: "2026-09-27T00:00:00Z",
      updatedAt: "2026-09-27T00:00:00Z",
    };
    const listProviders = vi
      .fn()
      .mockResolvedValueOnce({ providers: [] })
      .mockResolvedValueOnce({ providers: [provider] });
    const createProvider = vi.fn().mockResolvedValue({
      provider,
      sync: { created: 1, updated: 0, removed: 0, total: 1 },
    });

    renderApp("/providers", {
      listProviders,
      listCredentials: vi.fn().mockResolvedValue({ credentials: [credential] }),
      createProvider,
    });

    expect(await screen.findByText("No providers")).toBeTruthy();
    expect(screen.queryByText("Remote endpoint")).toBeNull();
    expect(screen.queryByText("Local command")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Add provider" }));
    expect(screen.getByRole("dialog")).toBeTruthy();
    expect(screen.getByText("Remote endpoint")).toBeTruthy();
    expect(screen.getByText("Local command")).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Namespace"), {
      target: { value: "demo" },
    });
    fireEvent.change(screen.getByLabelText("Display name"), {
      target: { value: "Demo" },
    });
    fireEvent.change(screen.getByLabelText("Endpoint"), {
      target: { value: "https://example.test/mcp" },
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Configure authentication" }),
    );
    expect(await screen.findByText("Provider authentication")).toBeTruthy();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    fireEvent.click(
      screen.getByRole("button", { name: "Add credential header" }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Use authentication" }),
    );

    expect(await screen.findByText("Add provider")).toBeTruthy();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(screen.getByLabelText<HTMLInputElement>("Namespace").value).toBe(
      "demo",
    );
    expect(screen.getByLabelText<HTMLInputElement>("Endpoint").value).toBe(
      "https://example.test/mcp",
    );

    fireEvent.click(screen.getByRole("button", { name: "Add provider" }));

    await waitFor(() => {
      expect(createProvider).toHaveBeenCalledWith({
        namespace: "demo",
        displayName: "Demo",
        pluginId: "io.mender.provider.mcp",
        config: {
          transport: {
            kind: "streamable-http",
            endpoint: "https://example.test/mcp",
            credentialHeaders: [
              {
                name: "Authorization",
                scheme: "Bearer",
                credentialRef: "credential://demo/work",
              },
            ],
          },
        },
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
      pluginId: "io.mender.provider.mcp",
      pluginDisplayName: "MCP",
      configVersion: 1,
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
    const createProvider = vi.fn().mockResolvedValue({
      provider,
      sync: { created: 4, updated: 0, removed: 0, total: 4 },
    });

    renderApp("/providers", {
      listProviders,
      createProvider,
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
      expect(createProvider).toHaveBeenCalledWith({
        namespace: "anysearch",
        displayName: "anysearch",
        pluginId: "io.mender.provider.mcp",
        config: {
          transport: {
            kind: "streamable-http",
            endpoint: "https://api.anysearch.com/mcp",
            headers: {
              Authorization: "Bearer ${ANYSEARCH_API_KEY}",
            },
          },
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
      pluginId: "io.mender.provider.mcp",
      pluginDisplayName: "MCP",
      configVersion: 1,
      transport: "stdio" as const,
      toolCount: 1,
      executable: "/usr/local/bin/local-tools",
      args: ["--stdio"],
    };
    const listProviders = vi
      .fn()
      .mockResolvedValueOnce({ providers: [] })
      .mockResolvedValueOnce({ providers: [provider] });
    const createProvider = vi.fn().mockResolvedValue({
      provider,
      sync: { created: 1, updated: 0, removed: 0, total: 1 },
    });

    renderApp("/providers", {
      listProviders,
      createProvider,
    });

    expect(await screen.findByText("No providers")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Add provider" }));
    fireEvent.click(screen.getByRole("button", { name: "Local command" }));

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
      expect(createProvider).toHaveBeenCalledWith({
        namespace: "local",
        displayName: "Local Tools",
        pluginId: "io.mender.provider.mcp",
        config: {
          transport: {
            kind: "stdio",
            executable: "/usr/local/bin/local-tools",
            args: ["--stdio"],
          },
        },
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
      pluginId: "io.mender.provider.mcp",
      pluginDisplayName: "MCP",
      configVersion: 1,
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
        displayName: "AnySearch Production",
        description: "Production search provider",
        config: {
          transport: {
            kind: "streamable-http",
            endpoint: "https://api.anysearch.com/mcp",
          },
        },
      });
    });
    expect(await screen.findByText("AnySearch Production")).toBeTruthy();
  });

  it("configures Provider authentication from Credential inventory", async () => {
    const credential = {
      reference: "credential://github/work",
      namespace: "github",
      name: "work",
      displayName: "GitHub work",
      source: "environment" as const,
      environmentVariable: "GITHUB_TOKEN",
      createdAt: "2026-09-27T00:00:00Z",
      updatedAt: "2026-09-27T00:00:00Z",
    };
    const provider = {
      providerId: "prov_1",
      namespace: "search",
      displayName: "Search",
      description: "",
      createdAt: "2026-09-26T00:00:00Z",
      updatedAt: "2026-09-26T00:00:00Z",
      pluginId: "io.mender.provider.mcp",
      pluginDisplayName: "MCP",
      configVersion: 1,
      transport: "http" as const,
      toolCount: 1,
      endpoint: "https://example.test/mcp",
    };
    const updated = {
      ...provider,
      credentialHeaders: [
        {
          name: "Authorization",
          scheme: "Bearer",
          credentialRef: credential.reference,
        },
      ],
    };
    const listProviders = vi
      .fn()
      .mockResolvedValueOnce({ providers: [provider] })
      .mockResolvedValue({ providers: [updated] });
    const updateProvider = vi.fn().mockResolvedValue({
      provider: updated,
      sync: { created: 0, updated: 1, removed: 0, total: 1 },
    });

    renderApp("/providers", {
      listProviders,
      listCredentials: vi.fn().mockResolvedValue({ credentials: [credential] }),
      updateProvider,
    });

    expect(await screen.findByText("Search")).toBeTruthy();
    fireEvent.click(
      screen.getByRole("button", { name: "Authentication search" }),
    );

    expect(await screen.findByText("Provider authentication")).toBeTruthy();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(screen.queryByLabelText("Credential reference")).toBeNull();

    fireEvent.click(
      screen.getByRole("button", { name: "Add credential header" }),
    );
    expect(
      screen.getByLabelText<HTMLSelectElement>(
        "Credential header credential 1",
      ).value,
    ).toBe(credential.reference);

    fireEvent.click(
      screen.getByRole("button", { name: "Save authentication" }),
    );

    await waitFor(() => {
      expect(updateProvider).toHaveBeenCalledWith("prov_1", {
        displayName: "Search",
        config: {
          transport: {
            kind: "streamable-http",
            endpoint: "https://example.test/mcp",
            credentialHeaders: [
              {
                name: "Authorization",
                scheme: "Bearer",
                credentialRef: "credential://github/work",
              },
            ],
          },
        },
      });
    });
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
          credentialRefs: [],
        },
      ],
    };
    const credential = {
      reference: "credential://github/work",
      namespace: "github",
      name: "work",
      displayName: "GitHub work",
      source: "environment" as const,
      environmentVariable: "GITHUB_TOKEN",
      createdAt: "2026-09-27T00:00:00Z",
      updatedAt: "2026-09-27T00:00:00Z",
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
            credentialRefs: [],
            ...tool,
          })),
        }),
    );

    renderApp("/profiles", {
      listProfiles: vi.fn().mockResolvedValue({ profiles: [profile] }),
      listCredentials: vi.fn().mockResolvedValue({ credentials: [credential] }),
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
            credentialRefs: [],
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

    fireEvent.click(
      screen.getByRole("button", { name: "Credentials for Release Search" }),
    );
    expect(await screen.findByText("Tool credentials")).toBeTruthy();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(screen.queryByLabelText("Credential reference")).toBeNull();
    fireEvent.click(
      screen.getByRole("checkbox", { name: /GitHub work/ }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Save credentials" }));

    await waitFor(() => {
      const lastCall = replaceProfileTools.mock.lastCall as unknown as [
        string,
        {
          tools: Array<{
            toolId: string;
            credentialRefs?: string[];
          }>;
        },
      ];
      expect(lastCall[0]).toBe("default");
      expect(
        lastCall[1].tools.find((tool) => tool.toolId === "tool_1")
          ?.credentialRefs,
      ).toEqual(["credential://github/work"]);
    });
    expect(await screen.findByText("1 credential")).toBeTruthy();
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

  it("manages Policy rules and Credential inventory without raw identities or secret echo", async () => {
    const policyProfile = {
      profileId: "prof_00000000000000000000000000000001",
      slug: "default",
      displayName: "Default",
      mode: "hybrid" as const,
      tools: [],
    };
    const policyProvider = {
      providerId: "prov_00000000000000000000000000000001",
      namespace: "demo",
      displayName: "Demo Provider",
      description: "",
      createdAt: "2026-09-27T00:00:00Z",
      updatedAt: "2026-09-27T00:00:00Z",
      transport: "http" as const,
      toolCount: 1,
      endpoint: "https://example.test/mcp",
    };
    const policyTool = {
      rank: 1,
      toolId: "tool_00000000000000000000000000000001",
      providerId: policyProvider.providerId,
      name: "search",
      title: "Search",
      description: "Search documentation",
    };
    const createdRule = {
      ruleId: "rule_00000000000000000000000000000001",
      displayName: "Approve MCP search",
      enabled: true,
      decision: "require_approval" as const,
      match: {
        origins: ["mcp"] as Array<"mcp">,
        scopeKinds: ["profile"] as Array<"profile">,
        profileRefs: [policyProfile.profileId],
        providerIds: [policyProvider.providerId],
        toolIds: [policyTool.toolId],
        factKnowledge: "unknown" as const,
        effectsAny: [],
        resourcesAny: [],
      },
      createdAt: "2026-09-27T01:00:00Z",
      updatedAt: "2026-09-27T01:00:00Z",
    };
    const getPolicy = vi
      .fn()
      .mockResolvedValueOnce({ defaultDecision: "allow", rules: [] })
      .mockResolvedValueOnce({ defaultDecision: "deny", rules: [] })
      .mockResolvedValue({ defaultDecision: "deny", rules: [createdRule] });
    const getPolicyStatus = vi
      .fn()
      .mockResolvedValueOnce({
        mode: "configured",
        editable: true,
        approval: "fail_closed_when_required",
        defaultDecision: "allow",
        ruleCount: 0,
        enabledRuleCount: 0,
      })
      .mockResolvedValueOnce({
        mode: "configured",
        editable: true,
        approval: "fail_closed_when_required",
        defaultDecision: "deny",
        ruleCount: 0,
        enabledRuleCount: 0,
      })
      .mockResolvedValue({
        mode: "configured",
        editable: true,
        approval: "fail_closed_when_required",
        defaultDecision: "deny",
        ruleCount: 1,
        enabledRuleCount: 1,
      });
    const setPolicyDefault = vi.fn().mockResolvedValue({
      defaultDecision: "deny",
      rules: [],
    });
    const createPolicyRule = vi.fn().mockResolvedValue(createdRule);

    const policyClient = renderApp("/policy", {
      getPolicy,
      getPolicyStatus,
      setPolicyDefault,
      createPolicyRule,
      listProfiles: vi.fn().mockResolvedValue({ profiles: [policyProfile] }),
      listProviders: vi.fn().mockResolvedValue({ providers: [policyProvider] }),
      browseTools: vi.fn().mockResolvedValue({ tools: [policyTool] }),
    });

    expect(await screen.findByText("No Policy Rules")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    expect(await screen.findByText("Edit default decision")).toBeTruthy();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    fireEvent.change(screen.getByLabelText("Default Policy decision"), {
      target: { value: "deny" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save default" }));
    await waitFor(() => {
      expect(setPolicyDefault).toHaveBeenCalledWith({ decision: "deny" });
    });

    fireEvent.click(screen.getByRole("button", { name: "Add rule" }));
    expect(await screen.findByText("Add Policy rule")).toBeTruthy();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(screen.queryByLabelText("Rule ID")).toBeNull();
    fireEvent.change(screen.getByLabelText("Policy Rule display name"), {
      target: { value: "Approve MCP search" },
    });
    fireEvent.change(screen.getByLabelText("Policy Rule decision"), {
      target: { value: "require_approval" },
    });
    expect(screen.getByText(/Approval adapter is fail-closed/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Next" }));

    expect(screen.getByText(/Scope and targets/)).toBeTruthy();
    fireEvent.click(screen.getByText("MCP").closest("label")!);
    fireEvent.click(screen.getByText("Profile").closest("label")!);
    fireEvent.click(screen.getByText("Default").closest("label")!);
    fireEvent.click(screen.getByText("Demo Provider").closest("label")!);
    fireEvent.click(screen.getByText("Search").closest("label")!);
    fireEvent.click(screen.getByRole("button", { name: "Next" }));

    expect(screen.getByText("Governance Facts")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Policy fact knowledge"), {
      target: { value: "unknown" },
    });
    expect(screen.queryByLabelText("Policy effects")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Create rule" }));

    await waitFor(() => {
      expect(createPolicyRule).toHaveBeenCalledTimes(1);
    });
    const createInput = createPolicyRule.mock.calls[0]?.[0] as PolicyRuleInput;
    expect(createInput).toMatchObject({
      displayName: "Approve MCP search",
      decision: "require_approval",
      match: {
        origins: ["mcp"],
        scopeKinds: ["profile"],
        profileRefs: [policyProfile.profileId],
        providerIds: [policyProvider.providerId],
        toolIds: [policyTool.toolId],
        factKnowledge: "unknown",
      },
    });
    expect("ruleId" in createInput).toBe(false);
    expect(await screen.findByText("Approve MCP search")).toBeTruthy();
    policyClient.unmount();

    const credential = {
      reference: "credential://github/work",
      namespace: "github",
      name: "work",
      displayName: "GitHub work",
      source: "native" as const,
      environmentVariable: "",
      createdAt: "2026-09-27T00:00:00Z",
      updatedAt: "2026-09-27T00:00:00Z",
    };
    const listCredentials = vi
      .fn()
      .mockResolvedValueOnce({ credentials: [] })
      .mockResolvedValue({ credentials: [credential] });
    const createNativeCredential = vi.fn().mockResolvedValue(credential);
    const rotateCredential = vi.fn().mockResolvedValue({
      ...credential,
      updatedAt: "2026-09-27T01:00:00Z",
    });

    renderApp("/credentials", {
      listCredentials,
      createNativeCredential,
      rotateCredential,
    });

    expect(await screen.findByText("No Credentials")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Add credential" }));
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(screen.queryByLabelText("Credential reference")).toBeNull();

    fireEvent.change(screen.getByLabelText("Credential namespace"), {
      target: { value: "github" },
    });
    fireEvent.change(screen.getByLabelText("Credential name"), {
      target: { value: "work" },
    });
    fireEvent.change(screen.getByLabelText("Credential display name"), {
      target: { value: "GitHub work" },
    });
    fireEvent.change(screen.getByLabelText("Credential secret"), {
      target: { value: "TOP-SECRET" },
    });
    expect(screen.getByText("credential://github/work")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Create credential" }));
    await waitFor(() => {
      expect(createNativeCredential).toHaveBeenCalledWith({
        namespace: "github",
        name: "work",
        displayName: "GitHub work",
        secret: "TOP-SECRET",
      });
    });

    expect(await screen.findByText("GitHub work")).toBeTruthy();
    expect(screen.getByText("credential://github/work")).toBeTruthy();
    expect(screen.queryByText("TOP-SECRET")).toBeNull();
    expect(screen.queryByDisplayValue("TOP-SECRET")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Rotate" }));
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    fireEvent.change(screen.getByLabelText("New Credential secret"), {
      target: { value: "NEW-SECRET" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Rotate secret" }));
    await waitFor(() => {
      expect(rotateCredential).toHaveBeenCalledWith("github", "work", {
        secret: "NEW-SECRET",
      });
    });
    expect(screen.queryByText("NEW-SECRET")).toBeNull();
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
  const resolvedClient = {
    listProviderPlugins: vi.fn().mockResolvedValue({
      plugins: [
        {
          pluginId: "io.mender.provider.mcp",
          version: "1.0.0",
          displayName: "MCP",
          description: "Model Context Protocol provider",
          configVersion: 1,
        },
      ],
    }),
    listCredentials: vi.fn().mockResolvedValue({ credentials: [] }),
    ...client,
  } as MenderClient;
  return render(
    <MemoryRouter initialEntries={[path]}>
      <MenderApp client={resolvedClient} />
    </MemoryRouter>,
  );
}
