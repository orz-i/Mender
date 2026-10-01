import { describe, expect, it } from "vitest";

import {
  humanizePluginId,
  marketplaceCursorPage,
  providerPluginsToMarketplace,
} from "./marketplace";

describe("providerPluginsToMarketplace", () => {
  it("keeps built-ins and disabled external installations in one catalog", () => {
    const plugins = [
      {
        pluginId: "io.mender.provider.mcp",
        version: "1.0.0",
        packageVersion: "1.0.0",
        displayName: "MCP",
        description: "Model Context Protocol provider",
        configVersion: 1,
        sourceKind: "builtin" as const,
        capabilities: ["http", "stdio"],
        enabled: true,
      },
    ];
    const installations = [
      {
        installationId: "pinst_0123456789abcdef0123456789abcdef",
        pluginId: "dev.example.echo",
        packageVersion: "2.0.0",
        sourceKind: "local" as const,
        digest: "sha256:" + "a".repeat(64),
        status: "disabled" as const,
        installedAt: "2026-09-30T00:00:00Z",
        displayName: "Example Echo",
        description: "Echo Provider plugin",
        configVersion: 2,
        protocolVersion: 1,
        capabilities: ["echo"],
      },
    ];

    const result = providerPluginsToMarketplace(
      plugins,
      installations,
      [],
    );

    expect(result).toHaveLength(2);
    expect(result[0]).toMatchObject({
      displayName: "MCP",
      source: "builtin",
      status: "enabled",
      enabled: true,
      createAction: {
        label: "Add provider",
        href: "/providers?create=1&pluginId=io.mender.provider.mcp",
      },
    });
    expect(result[1]).toMatchObject({
      displayName: "Example Echo",
      description: "Echo Provider plugin",
      source: "local",
      status: "disabled",
      enabled: false,
      capabilities: ["echo"],
      actionRef: installations[0]?.installationId,
    });
    expect(result[1]?.technicalDetails).toContainEqual({
      label: "Plugin ID",
      value: "dev.example.echo",
      mono: true,
    });
    expect(result[1]?.technicalDetails).toContainEqual({
      label: "Installation ID",
      value: installations[0]?.installationId,
      mono: true,
    });
    expect(result[1]?.createAction).toBeUndefined();
  });

  it("creates exact Provider handoffs for enabled external installations", () => {
    const installationId = "pinst_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
    const result = providerPluginsToMarketplace(
      [
        {
          pluginId: "dev.example.echo",
          installationId,
          version: "1.0.0",
          packageVersion: "1.2.3",
          displayName: "Example Echo",
          description: "Echo Provider plugin",
          configVersion: 1,
          sourceKind: "external",
          capabilities: ["echo"],
          enabled: true,
        },
      ],
      [
        {
          installationId,
          pluginId: "dev.example.echo",
          packageVersion: "1.2.3",
          sourceKind: "local",
          digest: "sha256:" + "a".repeat(64),
          status: "enabled",
          installedAt: "2026-09-30T00:00:00Z",
        },
      ],
      [],
    );

    expect(result).toHaveLength(1);
    expect(result[0]?.createAction).toEqual({
      label: "Add provider",
      href:
        "/providers?create=1&pluginId=dev.example.echo&installationId=" +
        installationId,
    });
  });

  it("pages the sorted catalog from stable plugin-key cursors", () => {
    const plugins = Array.from({ length: 7 }, (_, index) => ({
      pluginId: `dev.example.plugin-${String(index + 1).padStart(2, "0")}`,
      version: "1.0.0",
      packageVersion: "1.0.0",
      displayName: `Plugin ${String(index + 1).padStart(2, "0")}`,
      description: "Test plugin",
      configVersion: 1,
      sourceKind: "builtin" as const,
      capabilities: [],
      enabled: true,
    }));
    const marketplace = providerPluginsToMarketplace(plugins, [], []);

    const first = marketplaceCursorPage(marketplace, null, 3);
    expect(first.items.map((plugin) => plugin.displayName)).toEqual([
      "Plugin 01",
      "Plugin 02",
      "Plugin 03",
    ]);
    expect(first.nextCursor).toBe(first.items.at(-1)?.key);
    expect(first).toMatchObject({ from: 1, to: 3, total: 7 });

    const second = marketplaceCursorPage(
      marketplace,
      first.nextCursor,
      3,
    );
    expect(second.items.map((plugin) => plugin.displayName)).toEqual([
      "Plugin 04",
      "Plugin 05",
      "Plugin 06",
    ]);
    expect(second).toMatchObject({ from: 4, to: 6, total: 7 });
  });

  it("projects installation issues as degraded state", () => {
    const installationId =
      "pinst_fedcba9876543210fedcba9876543210";
    const result = providerPluginsToMarketplace(
      [],
      [
        {
          installationId,
          pluginId: "dev.example.echo",
          packageVersion: "1.0.0",
          sourceKind: "local",
          digest: "sha256:" + "b".repeat(64),
          status: "disabled",
          installedAt: "2026-09-30T00:00:00Z",
        },
      ],
      [
        {
          installationId,
          code: "descriptor_invalid",
          message: "descriptor rejected",
        },
      ],
    );

    expect(result[0]).toMatchObject({
      displayName: "Echo",
      status: "degraded",
      enabled: false,
      actions: {
        enable: false,
        disable: false,
        remove: true,
      },
      issue: {
        code: "descriptor_invalid",
      },
    });
  });

  it("humanizes unknown plugin IDs without exposing their full identifier", () => {
    expect(humanizePluginId("dev.example.audit-exporter")).toBe(
      "Audit Exporter",
    );
  });
});
