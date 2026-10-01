import { describe, expect, it } from "vitest";

import {
  humanizePluginId,
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
