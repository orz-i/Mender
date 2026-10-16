import { MenderClientInvariantError, type PolicyRuleInput } from "@mender/sdk";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock(
  "../bindings/github.com/orz-i/Mender/backend/internal/desktop/binding",
  () => ({
    Service: {
      BrowseTools: vi.fn(),
      CallTool: vi.fn(),
      CreateEnvironmentCredential: vi.fn(),
      CreateNativeCredential: vi.fn(),
      CreateProvider: vi.fn(),
      CreatePolicyRule: vi.fn(),
      DeleteCredential: vi.fn(),
      DeletePolicyRule: vi.fn(),
      DeleteSkillInstallation: vi.fn(),
      DeploySkill: vi.fn(),
      DisableSkillInstallation: vi.fn(),
      EnableSkillInstallation: vi.fn(),
      GetCredential: vi.fn(),
      GetPolicy: vi.fn(),
      GetRuntimeGovernance: vi.fn(),
      GetSandboxProfileGrants: vi.fn(),
      PutSandboxProfileGrants: vi.fn(),
      RevokeSandboxProfileGrants: vi.fn(),
      GetSandboxProfileGrantEvents: vi.fn(),
      GetSkillInstallation: vi.fn(),
      GetPolicyStatus: vi.fn(),
      GetSystemStatus: vi.fn(),
      InstallProviderPlugin: vi.fn(),
      InstallLocalPlugin: vi.fn(),
      ListPluginCatalogue: vi.fn(),
      ListDockerImages: vi.fn(),
      EnablePluginInstallation:vi.fn(),
      DisablePluginInstallation:vi.fn(),
      DeletePluginInstallation:vi.fn(),
      CreateExternalSandbox:vi.fn(),
      CreateSSHRemoteSandbox:vi.fn(),
      InspectSSHHostKey:vi.fn(),
      ReconcileSandbox:vi.fn(),
      CreateExternalTunnel:vi.fn(),
      InstallSkill: vi.fn(),
      ListAudit: vi.fn(),
      ListCredentials: vi.fn(),
      ListMCPServers: vi.fn(),
      ListProviderPlugins: vi.fn(),
      ListProviders: vi.fn(),
      ListSkillDeployments: vi.fn(),
      ListSkills: vi.fn(),
      RebindProvider: vi.fn(),
      ReplaceProfileTools: vi.fn(),
      RestartMCPServer: vi.fn(),
      RotateNativeCredential: vi.fn(),
      SearchTools: vi.fn(),
      SetPolicyDefault: vi.fn(),
      SetPolicyRuleEnabled: vi.fn(),
      StartMCPServer: vi.fn(),
      StopMCPServer: vi.fn(),
      TrustSkillInstallation: vi.fn(),
      UndeploySkill: vi.fn(),
      UntrustSkillInstallation: vi.fn(),
      UpdateConfiguredProvider: vi.fn(),
      UpdatePolicyRule: vi.fn(),
      UpdateRuntimeBudget: vi.fn(),
      UpdateRuntimeIsolation: vi.fn(),
    },
  }),
);

import { Service } from "../bindings/github.com/orz-i/Mender/backend/internal/desktop/binding";

import {
  createDesktopMenderClient,
  deployDesktopSkill,
  installDesktopProviderPlugin,
  installDesktopPlugin,
  installDesktopSkill,
} from "./desktop-client";

describe("desktop Mender client", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("forwards SSH key import-only passphrases through Wails without persisting them in returned metadata", async () => {
    const stored = {
      reference: "credential://ssh/rocky",
      namespace: "ssh",
      name: "rocky",
      displayName: "Rocky",
      source: "native",
      environmentVariable: "",
      createdAt: "2026-10-12T00:00:00Z",
      updatedAt: "2026-10-12T00:00:00Z",
    };
    vi.mocked(Service.CreateNativeCredential).mockResolvedValue(stored);
    vi.mocked(Service.RotateNativeCredential).mockResolvedValue(stored);
    const client = createDesktopMenderClient();

    const created = await client.createNativeCredential({
      namespace: "ssh", name: "rocky", displayName: "Rocky",
      secret: "-----BEGIN RSA PRIVATE KEY-----", sshKeyPassphrase: "import-passphrase",
    });
    expect(Service.CreateNativeCredential).toHaveBeenCalledWith(
      "ssh", "rocky", "Rocky", "-----BEGIN RSA PRIVATE KEY-----", "import-passphrase",
    );
    expect("sshKeyPassphrase" in created).toBe(false);

    const rotated = await client.rotateCredential("ssh", "rocky", {
      secret: "-----BEGIN OPENSSH PRIVATE KEY-----", sshKeyPassphrase: "rotation-passphrase",
    });
    expect(Service.RotateNativeCredential).toHaveBeenCalledWith(
      "ssh", "rocky", "-----BEGIN OPENSSH PRIVATE KEY-----", "rotation-passphrase",
    );
    expect("sshKeyPassphrase" in rotated).toBe(false);

    await client.createNativeCredential({
      namespace: "ssh", name: "rocky", displayName: "Rocky", secret: "fixture",
    });
    expect(Service.CreateNativeCredential).toHaveBeenLastCalledWith(
      "ssh", "rocky", "Rocky", "fixture", "",
    );
  });

  it("normalizes Wails status models into the shared client contract", async () => {
    vi.mocked(Service.GetSystemStatus).mockResolvedValue({
      ready: true,
      policy: {
        mode: "configured",
        editable: true,
        approval: "fail_closed_when_required",
        defaultDecision: "allow",
        ruleCount: 0,
        enabledRuleCount: 0,
      },
      credentials: {
        resolver: "inventory",
        persistentStore: true,
        editable: true,
      },
      mcp: {
        stdio: true,
        streamableHttp: true,
        exposureModes: ["direct", "dynamic", "hybrid"],
      },
    });

    const client = createDesktopMenderClient();

    await expect(client.getSystemStatus()).resolves.toEqual({
      ready: true,
      policy: {
        mode: "configured",
        editable: true,
        approval: "fail_closed_when_required",
        defaultDecision: "allow",
        ruleCount: 0,
        enabledRuleCount: 0,
      },
      credentials: {
        resolver: "inventory",
        persistentStore: true,
        editable: true,
      },
      mcp: {
        stdio: true,
        streamableHttp: true,
        exposureModes: ["direct", "dynamic", "hybrid"],
      },
    });
  });

  it("maps Runtime governance and forwards updates", async () => {
    const status = {
      budget: {
        maxInvocations: 32,
        maxPrincipalInvocations: 8,
        maxProviderInvocations: 8,
        maxToolInvocations: 4,
        maxProviderActivations: 4,
        maxExecutionTimeoutMs: 120000,
        maxArgumentBytes: 1048576,
        maxResultBytes: 1048576,
      },
      isolation: {
        workspaceShell: "restricted_host" as const,
        externalProvider: "host" as const,
        tunnelClient: "restricted_host" as const,
      },
      backend: { name: "seatbelt", restrictedHostAvailable: true },
      usage: { invocations: 2, activations: 1 },
    };
    vi.mocked(Service.GetRuntimeGovernance).mockResolvedValue(status);
    vi.mocked(Service.UpdateRuntimeBudget).mockResolvedValue(status);
    vi.mocked(Service.UpdateRuntimeIsolation).mockResolvedValue(status);

    const client = createDesktopMenderClient();
    await expect(client.getRuntimeGovernance()).resolves.toEqual(status);
    await client.updateRuntimeBudget(status.budget);
    await client.updateRuntimeIsolation(status.isolation);

    expect(Service.UpdateRuntimeBudget).toHaveBeenCalledWith(status.budget);
    expect(Service.UpdateRuntimeIsolation).toHaveBeenCalledWith(status.isolation);
  });

  it("uses one typed Wails catalogue for all extension kinds",async()=>{
    vi.mocked(Service.ListPluginCatalogue).mockResolvedValue({plugins:[
      {key:"io.mender.tunnel.openai@builtin",pluginId:"io.mender.tunnel.openai",extensionId:"main",
        kind:"tunnel.transport",marketplaceAction:"create.tunnel",displayName:"OpenAI Secure MCP Tunnel",description:"Internal",
        packageVersion:"1.0.0",sourceKind:"builtin",status:"enabled",enabled:true,
        capabilities:["managed-loopback-mcp"],usageCount:0},
      {key:"dev.example.sandbox@pinst_0123456789abcdef0123456789abcdef",
        pluginId:"dev.example.sandbox",extensionId:"main",kind:"sandbox.backend",marketplaceAction:"create.sandbox",
        displayName:"Example Sandbox",description:"Fixture",packageVersion:"1.0.0",
        sourceKind:"local",status:"enabled",enabled:true,
        installationId:"pinst_0123456789abcdef0123456789abcdef",
        capabilities:["file.read"],usageCount:2},
    ]});
    const client=createDesktopMenderClient();
    const result=await client.listPluginCatalogue();
    expect(result.plugins.map(p=>p.kind)).toEqual(["tunnel.transport","sandbox.backend"]);
    expect(result.plugins[1]).toMatchObject({usageCount:2,installationId:"pinst_0123456789abcdef0123456789abcdef"});
    expect(Service.ListPluginCatalogue).toHaveBeenCalledTimes(1);
  });

  it("accepts the Phase 33 Agent Backends alongside all previous Wails marketplace extension kinds",async()=>{
    const builtin=(pluginId:string,kind:string,overrides:Record<string,unknown>={})=>({
      key:pluginId+"@builtin",pluginId,extensionId:"main",kind,
      marketplaceAction:kind==="provider.tools"?"create.provider":kind==="sandbox.backend"?"create.sandbox":
        kind==="tunnel.transport"?"create.tunnel":"create.agent",
      displayName:pluginId,description:"Host-reviewed built-in extension",
      packageVersion:"1.0.0",sourceKind:"builtin",status:"enabled",
      enabled:true,capabilities:[],usageCount:0,configVersion:1,protocolVersion:1,
      ...overrides,
    });
    const codex=builtin("io.mender.agent.codex","agent.backend",{
      status:"unavailable",enabled:false,issue:"codex-acp adapter not installed",
      capabilities:["acp.v1","tasks","local_workspace"],
    });
    const grok=builtin("io.mender.agent.grok","agent.backend",{
      capabilities:["acp.v1","tasks","local_workspace"],
    });
    const entries=[
      builtin("io.mender.provider.mcp","provider.tools"),
      builtin("io.mender.sandbox.local-workspace","sandbox.backend"),
      builtin("io.mender.tunnel.openai","tunnel.transport"),
      codex,grok,
    ];
    vi.mocked(Service.ListPluginCatalogue).mockResolvedValue({plugins:entries});
    const result=await createDesktopMenderClient().listPluginCatalogue();
    expect(result.plugins).toHaveLength(entries.length);
    expect(result.plugins.map(plugin=>plugin.kind)).toEqual(entries.map(entry=>entry.kind));
    expect(result.plugins.find(p=>p.pluginId==="io.mender.agent.codex")).toMatchObject({
      kind:"agent.backend",sourceKind:"builtin",status:"unavailable",enabled:false,
      issue:"codex-acp adapter not installed",
    });
    expect(result.plugins.find(p=>p.pluginId==="io.mender.agent.grok")).toMatchObject({
      kind:"agent.backend",sourceKind:"builtin",status:"enabled",enabled:true,
      capabilities:["acp.v1","tasks","local_workspace"],
    });
    expect(Service.ListPluginCatalogue).toHaveBeenCalledOnce();
  });

  it("rejects genuinely unrecognized Wails extension kinds, rather than treating them as Agent plugins",async()=>{
    vi.mocked(Service.ListPluginCatalogue).mockResolvedValue({plugins:[{
      key:"io.mender.agent.unknown@builtin",pluginId:"io.mender.agent.unknown",
      extensionId:"main",kind:"agent.unknown",marketplaceAction:"none",displayName:"Unknown Agent",
      description:"Unexpected new kind",packageVersion:"1.0.0",
      sourceKind:"builtin",status:"enabled",enabled:true,
      capabilities:[],usageCount:0,
    }]});
    await expect(createDesktopMenderClient().listPluginCatalogue())
      .rejects.toBeInstanceOf(MenderClientInvariantError);
  });

  it("rejects roadmap-only entries and missing host-reviewed actions from Wails",async()=>{
    const fixture={
      key:"io.mender.sandbox.e2b@builtin",pluginId:"io.mender.sandbox.e2b",
      extensionId:"main",kind:"sandbox.backend",marketplaceAction:"none",
      displayName:"E2B",description:"Not an installed backend",
      packageVersion:"",sourceKind:"planned",status:"planned",
      enabled:false,capabilities:[],usageCount:0,
    };
    vi.mocked(Service.ListPluginCatalogue).mockResolvedValue({plugins:[fixture]});
    await expect(createDesktopMenderClient().listPluginCatalogue())
      .rejects.toBeInstanceOf(MenderClientInvariantError);
    vi.mocked(Service.ListPluginCatalogue).mockResolvedValue({
      plugins:[{...fixture,sourceKind:"builtin",status:"unavailable",marketplaceAction:"forged.create"}],
    });
    await expect(createDesktopMenderClient().listPluginCatalogue())
      .rejects.toBeInstanceOf(MenderClientInvariantError);
  });

  it("reads host-verified immutable cached Docker images from the Wails binding",async()=>{
    const image={
      imageDigest:"docker.io/library/alpine@sha256:"+"a".repeat(64),
      displayName:"alpine:3.22",
    };
    vi.mocked(Service.ListDockerImages).mockResolvedValue({images:[image]});
    await expect(createDesktopMenderClient().listDockerImages()).resolves.toEqual({images:[image]});
    expect(Service.ListDockerImages).toHaveBeenCalledWith();
  });

  it("accepts the unavailable Docker built-in from the real typed desktop catalogue", async()=>{
    const docker={
      key:"io.mender.sandbox.docker@builtin",
      pluginId:"io.mender.sandbox.docker",extensionId:"main",
      kind:"sandbox.backend",marketplaceAction:"create.sandbox",displayName:"Docker Engine",
      description:"Trusted opt-in Docker Engine backend",
      packageVersion:"1.0.0",sourceKind:"builtin",
      status:"unavailable",enabled:false,
      issue:"Docker Engine is not configured",
      capabilities:["file.read","file.search","file.edit","process.shell"],
      usageCount:0,configVersion:1,protocolVersion:1,
    };
    vi.mocked(Service.ListPluginCatalogue).mockResolvedValue({plugins:[docker]});
    await expect(createDesktopMenderClient().listPluginCatalogue()).resolves.toEqual({
      plugins:[docker],
    });
    expect(Service.ListPluginCatalogue).toHaveBeenCalledOnce();
  });

  it("rejects truly unknown plugin status rather than masking a contract change", async()=>{
    vi.mocked(Service.ListPluginCatalogue).mockResolvedValue({plugins:[{
      key:"io.mender.sandbox.docker@builtin",
      pluginId:"io.mender.sandbox.docker",extensionId:"main",
      kind:"sandbox.backend",marketplaceAction:"create.sandbox",displayName:"Docker Engine",description:"Docker",
      packageVersion:"1.0.0",sourceKind:"builtin",
      status:"unsupported_future_status",enabled:false,
      capabilities:[],usageCount:0,
    }]});
    await expect(createDesktopMenderClient().listPluginCatalogue())
      .rejects.toBeInstanceOf(MenderClientInvariantError);
  });

  it("installs any typed package through a trusted Wails-local folder selection",async()=>{
    const installed={
      installationId:"pinst_0123456789abcdef0123456789abcdef",
      pluginId:"dev.example.tunnel",packageVersion:"1.0.0",sourceKind:"local",
      digest:"sha256:"+"a".repeat(64),status:"enabled",
      installedAt:"2026-10-11T00:00:00Z",
    };
    vi.mocked(Service.InstallLocalPlugin).mockResolvedValue(installed);
    await expect(installDesktopPlugin("/tmp/selected-plugin")).resolves.toMatchObject(installed);
    expect(Service.InstallLocalPlugin).toHaveBeenCalledWith("/tmp/selected-plugin");
  });

  it("previews the server host key through a local Wails call without submitting SSH credentials", async () => {
    const identity = {
      status: "untrusted", algorithm: "ssh-ed25519",
      fingerprint: "SHA256:test-server-fingerprint",
      hostPublicKey: "ssh-ed25519 " + "A".repeat(68),
    };
    vi.mocked(Service.InspectSSHHostKey).mockResolvedValue(identity);
    const client = createDesktopMenderClient();
    await expect(client.inspectSSHHostKey("example.org", 22)).resolves.toEqual(identity);
    expect(Service.InspectSSHHostKey).toHaveBeenCalledExactlyOnceWith("example.org", 22);
    expect(Service.CreateSSHRemoteSandbox).not.toHaveBeenCalled();

    vi.mocked(Service.InspectSSHHostKey).mockResolvedValue({...identity, status: "invalid-state"});
    await expect(client.inspectSSHHostKey("example.org", 22)).rejects.toBeInstanceOf(MenderClientInvariantError);
  });

  it("uses the native SSH Remote binding without sending private key material",async()=>{
    const request={
      name:"Remote Dev",host:"example.org",port:22,user:"agent",root:"/home/agent/work",
      hostPublicKey:"ssh-ed25519 "+"A".repeat(70),
      credentialRef:"credential://ssh/build",
    };
    const sandbox={
      sandboxId:"sbx_0123456789abcdef0123456789abcdef",name:"Remote Dev",
      backend:"ssh_remote",isolation:"remote",state:"ready",localRoot:"",
      ownerKind:"local_user",ownerRef:"test",activeLeases:0,backendAvailable:true,
    };
    vi.mocked(Service.CreateSSHRemoteSandbox).mockResolvedValue(sandbox);
    await expect(createDesktopMenderClient().createSSHRemoteSandbox(request)).resolves.toMatchObject(sandbox);
    expect(Service.CreateSSHRemoteSandbox).toHaveBeenCalledExactlyOnceWith(request);
    expect(JSON.stringify(vi.mocked(Service.CreateSSHRemoteSandbox).mock.calls)).not.toContain("PRIVATE KEY");
  });

  it("passes the exact external Sandbox and Tunnel bindings to Wails",async()=>{
    const sandbox={
      sandboxId:"sbx_0123456789abcdef0123456789abcdef",name:"Fixture",
      backend:"external",isolation:"remote",state:"ready",localRoot:"",
      ownerKind:"local_user",ownerRef:"test",activeLeases:0,backendAvailable:true,
      pluginId:"dev.example.sandbox",extensionId:"main",
      installationId:"pinst_0123456789abcdef0123456789abcdef",
    };
    vi.mocked(Service.CreateExternalSandbox).mockResolvedValue(sandbox);
    const tunnel={
      tunnelId:"tun_0123456789abcdef0123456789abcdef",displayName:"Fixture Tunnel",
      provider:"external",externalTunnelId:"fixture",
      runtimeCredentialRef:"credential://example/key",
      pluginId:"dev.example.tunnel",extensionId:"main",
      installationId:"pinst_0123456789abcdef0123456789abcdef",
    };
    vi.mocked(Service.CreateExternalTunnel).mockResolvedValue(tunnel);
    const client=createDesktopMenderClient();
    const config={message:"hello"};
    await expect(client.createExternalSandbox({
      name:"Fixture",installationId:sandbox.installationId,config,
    })).resolves.toMatchObject({backend:"external",installationId:sandbox.installationId});
    expect(Service.CreateExternalSandbox).toHaveBeenCalledWith({
      name:"Fixture",installationId:sandbox.installationId,configJson:JSON.stringify(config),
    });
    await expect(client.createExternalTunnel({
      displayName:"Fixture Tunnel",installationId:tunnel.installationId,
      externalTunnelId:"fixture",runtimeCredentialRef:"credential://example/key",
    })).resolves.toMatchObject({provider:"external",pluginId:"dev.example.tunnel"});
    expect(Service.CreateExternalTunnel).toHaveBeenCalledWith(
      "Fixture Tunnel",tunnel.installationId,"fixture","credential://example/key",
    );
  });

  it("installs Provider plugins only through the desktop-local Wails capability", async () => {
    vi.mocked(Service.InstallProviderPlugin).mockResolvedValue({
      installationId: "pinst_0123456789abcdef0123456789abcdef",
      pluginId: "dev.example.echo",
      packageVersion: "1.0.0",
      sourceKind: "local",
      digest: "sha256:" + "a".repeat(64),
      status: "enabled",
      installedAt: "2026-09-28T00:00:00Z",
      displayName: "Example Echo",
      description: "Echo Provider plugin",
      configVersion: 2,
      protocolVersion: 1,
      capabilities: ["echo"],
    });

    await expect(
      installDesktopProviderPlugin("/tmp/provider-plugin"),
    ).resolves.toEqual({
      installationId: "pinst_0123456789abcdef0123456789abcdef",
      pluginId: "dev.example.echo",
      packageVersion: "1.0.0",
      sourceKind: "local",
      digest: "sha256:" + "a".repeat(64),
      status: "enabled",
      installedAt: "2026-09-28T00:00:00Z",
      displayName: "Example Echo",
      description: "Echo Provider plugin",
      configVersion: 2,
      protocolVersion: 1,
      capabilities: ["echo"],
    });
    expect(Service.InstallProviderPlugin).toHaveBeenCalledWith(
      "/tmp/provider-plugin",
    );
  });

  it("adapts Skill inventory and host-local install/deploy capabilities to Wails", async () => {
    const installationId = "sinst_0123456789abcdef0123456789abcdef";
    const deploymentId = "sdep_0123456789abcdef0123456789abcdef";
    const installation = {
      skillId: "skill_0123456789abcdef0123456789abcdef",
      installationId,
      name: "example-skill",
      description: "Example Skill",
      sourceKind: "local-directory",
      sourceReference: "/tmp/example-skill",
      digest: "sha256:" + "a".repeat(64),
      versionHint: "1.0.0",
      trusted: true,
      enabled: true,
      builtIn: false,
      installedAt: "2026-09-28T00:00:00Z",
    };

    vi.mocked(Service.InstallSkill).mockResolvedValue(installation);
    vi.mocked(Service.DeploySkill).mockResolvedValue({
      deploymentId,
      installationId,
      target: "agents-project",
      root: "/workspace",
      destination: "/workspace/.agents/skills/example-skill",
      digest: installation.digest,
      deployedAt: "2026-09-28T01:00:00Z",
      drift: "current",
      currentDigest: installation.digest,
    });
    vi.mocked(Service.ListSkills).mockResolvedValue({
      installations: [installation],
    });
    vi.mocked(Service.GetSkillInstallation).mockResolvedValue({
      installation,
      metadata: {
        name: "example-skill",
        description: "Example Skill",
        license: "Apache-2.0",
        compatibility: "Requires git",
        metadata: { version: "1.0.0" },
        allowedTools: "Bash(git:*)",
      },
      resources: [
        {
          path: "SKILL.md",
          kind: "manifest",
          size: 123,
          sha256: "sha256:" + "b".repeat(64),
          executable: false,
        },
      ],
      totalBytes: 123,
    });
    vi.mocked(Service.ListSkillDeployments).mockResolvedValue({
      deployments: [
        {
          deploymentId,
          installationId,
          target: "agents-project",
          root: "/workspace",
          destination: "/workspace/.agents/skills/example-skill",
          digest: installation.digest,
          deployedAt: "2026-09-28T01:00:00Z",
          drift: "current",
          currentDigest: installation.digest,
        },
      ],
    });

    await expect(
      installDesktopSkill("/tmp/example-skill"),
    ).resolves.toMatchObject({
      installationId,
      sourceKind: "local-directory",
      trusted: true,
    });
    expect(Service.InstallSkill).toHaveBeenCalledWith("/tmp/example-skill");

    await expect(
      deployDesktopSkill(
        installationId,
        "agents-project",
        "/workspace",
      ),
    ).resolves.toMatchObject({
      deploymentId,
      target: "agents-project",
      drift: "current",
    });
    expect(Service.DeploySkill).toHaveBeenCalledWith(
      installationId,
      "agents-project",
      "/workspace",
    );

    const client = createDesktopMenderClient();
    await expect(client.listSkills()).resolves.toMatchObject({
      installations: [{ installationId, name: "example-skill" }],
    });
    await expect(
      client.getSkillInstallation(installationId),
    ).resolves.toMatchObject({
      metadata: {
        license: "Apache-2.0",
        allowedTools: "Bash(git:*)",
      },
      resources: [{ kind: "manifest", executable: false }],
    });
    await expect(client.listSkillDeployments()).resolves.toMatchObject({
      deployments: [{ deploymentId, drift: "current" }],
    });
  });

  it("adapts generic Provider plugin discovery and configuration to Wails", async () => {
    vi.mocked(Service.ListProviderPlugins).mockResolvedValue({
      plugins: [
        {
          pluginId: "io.mender.provider.mcp",
          version: "1.0.0",
          packageVersion: "1.0.0",
          displayName: "MCP",
          description: "Model Context Protocol provider",
          configVersion: 1,
          sourceKind: "builtin",
          capabilities: null,
          enabled: true,
        },
      ],
      issues: [
        {
          installationId: "pinst_00000000000000000000000000000091",
          code: "descriptor_backfill_failed",
          message: "database is read-only",
        },
      ],
    });
    vi.mocked(Service.CreateProvider).mockResolvedValue({
      provider: {
        providerId: "prov_00000000000000000000000000000001",
        namespace: "upstream",
        displayName: "Upstream",
        description: "",
        createdAt: "2026-09-27T00:00:00Z",
        updatedAt: "2026-09-27T00:00:00Z",
        pluginId: "io.mender.provider.mcp",
        pluginVersion: "1.0.0",
        pluginDisplayName: "MCP",
        configVersion: 1,
        connectionSummary: "Remote MCP · example.test",
        config: {
          transport: {
            kind: "streamable-http",
            endpoint: "https://example.test/mcp",
          },
        },
        transport: "http",
        toolCount: 1,
        endpoint: "https://example.test/mcp",
      },
      sync: { created: 1, updated: 0, removed: 0, total: 1 },
    });

    const client = createDesktopMenderClient();
    await expect(client.listProviderPlugins()).resolves.toEqual({
      plugins: [
        {
          pluginId: "io.mender.provider.mcp",
          version: "1.0.0",
          packageVersion: "1.0.0",
          displayName: "MCP",
          description: "Model Context Protocol provider",
          configVersion: 1,
          sourceKind: "builtin",
          capabilities: [],
          enabled: true,
        },
      ],
      issues: [
        {
          installationId: "pinst_00000000000000000000000000000091",
          code: "descriptor_backfill_failed",
          message: "database is read-only",
        },
      ],
    });

    const config = {
      transport: {
        kind: "streamable-http",
        endpoint: "https://example.test/mcp",
      },
    };
    await expect(
      client.createProvider({
        namespace: "upstream",
        displayName: "Upstream",
        pluginId: "io.mender.provider.mcp",
        config,
      }),
    ).resolves.toMatchObject({
      provider: {
        pluginId: "io.mender.provider.mcp",
        connectionSummary: "Remote MCP · example.test",
        config,
        transport: "http",
      },
    });
    expect(Service.CreateProvider).toHaveBeenCalledWith(
      "upstream",
      "Upstream",
      "",
      "io.mender.provider.mcp",
      "",
      config,
    );
  });

  it("rebinds an external Provider through the Wails host", async () => {
    vi.mocked(Service.RebindProvider).mockResolvedValue({
      provider: {
        providerId: "prov_external",
        namespace: "external",
        displayName: "External",
        description: "",
        createdAt: "2026-09-28T00:00:00Z",
        updatedAt: "2026-09-28T00:00:00Z",
        pluginId: "dev.example.echo",
        installationId: "pinst_22222222222222222222222222222222",
        configVersion: 1,
        connectionSummary: "Example Echo",
        config: { prefix: "next: " },
        transport: "unconfigured",
        toolCount: 1,
      },
      sync: { created: 0, updated: 1, removed: 0, total: 1 },
    });

    const client = createDesktopMenderClient();
    await expect(
      client.rebindProvider("prov_external", {
        installationId: "pinst_22222222222222222222222222222222",
        config: { prefix: "next: " },
      }),
    ).resolves.toMatchObject({
      provider: {
        providerId: "prov_external",
        installationId: "pinst_22222222222222222222222222222222",
        connectionSummary: "Example Echo",
      },
      sync: { updated: 1, total: 1 },
    });
    expect(Service.RebindProvider).toHaveBeenCalledWith(
      "prov_external",
      "pinst_22222222222222222222222222222222",
      { prefix: "next: " },
    );
  });

  it("preserves Host-verified Tool service ownership across the native Wails boundary", async () => {
    const common = {
      namespace: "tools",
      displayName: "Internal facade",
      description: "",
      createdAt: "2026-10-16T00:00:00Z",
      updatedAt: "2026-10-16T00:00:00Z",
      transport: "unconfigured",
      toolCount: 4,
    };
    vi.mocked(Service.ListProviders).mockResolvedValue({
      providers: [
        {
          ...common, providerId: "prov_agent",
          toolSourceKind: "agent",
          toolSourceId: "agent_grok",
          toolSourceName: "Grok - executor",
          pluginId: "io.mender.provider.agent-delegation",
        },
        {
          ...common, providerId: "prov_sandbox",
          toolSourceKind: "sandbox",
          toolSourceId: "sbx_rocky",
          toolSourceName: "Rocky",
          pluginId: "io.mender.provider.workspace",
        },
        {
          ...common, providerId: "prov_anysearch",
          toolSourceKind: "provider",
          toolSourceId: "prov_anysearch",
          toolSourceName: "AnySearch",
          displayName: "AnySearch",
          pluginId: "io.mender.provider.mcp",
        },
        {
          ...common, providerId: "prov_legacy",
          toolSourceKind: "unclaimed",
          pluginId: "io.mender.provider.workspace",
        },
      ],
    });

    const { providers } = await createDesktopMenderClient().listProviders();
    expect(providers.map(({ toolSourceKind, toolSourceId, toolSourceName }) => ({
      toolSourceKind, toolSourceId, toolSourceName,
    }))).toEqual([
      { toolSourceKind: "agent", toolSourceId: "agent_grok", toolSourceName: "Grok - executor" },
      { toolSourceKind: "sandbox", toolSourceId: "sbx_rocky", toolSourceName: "Rocky" },
      { toolSourceKind: "provider", toolSourceId: "prov_anysearch", toolSourceName: "AnySearch" },
      { toolSourceKind: "unclaimed", toolSourceId: undefined, toolSourceName: undefined },
    ]);
    expect(providers[0]?.displayName).toBe("Internal facade");
    expect(providers[2]?.displayName).toBe("AnySearch");
  });

  it("fails closed when Wails returns unsupported or incomplete Tool service ownership", async () => {
    const base = {
      providerId: "prov_unknown",
      namespace: "unknown",
      displayName: "Unknown",
      description: "",
      createdAt: "2026-10-16T00:00:00Z",
      updatedAt: "2026-10-16T00:00:00Z",
      transport: "unconfigured",
      toolCount: 0,
    };
    const client = createDesktopMenderClient();
    vi.mocked(Service.ListProviders).mockResolvedValue({
      providers: [{ ...base, toolSourceKind: "plugin-marketplace" }],
    });
    await expect(client.listProviders()).rejects.toThrow("Unsupported Tool source kind");

    vi.mocked(Service.ListProviders).mockResolvedValue({
      providers: [{ ...base, toolSourceKind: "agent", toolSourceId: "agent_grok" }],
    });
    await expect(client.listProviders()).rejects.toThrow(
      "Resource-owned Tool source is missing its verified owner",
    );
  });

  it("normalizes nullable binding arrays without leaking Wails models", async () => {
    vi.mocked(Service.ListProviders).mockResolvedValue({
      providers: [
        {
          providerId: "prov_00000000000000000000000000000001",
          namespace: "upstream",
          displayName: "Upstream",
          description: "",
          createdAt: "2026-09-26T00:00:00Z",
          updatedAt: "2026-09-26T00:00:00Z",
          transport: "stdio",
          toolCount: 1,
          executable: "example",
          args: null,
        },
      ],
    });

    const client = createDesktopMenderClient();

    await expect(client.listProviders()).resolves.toEqual({
      providers: [
        {
          providerId: "prov_00000000000000000000000000000001",
          namespace: "upstream",
          displayName: "Upstream",
          description: "",
          createdAt: "2026-09-26T00:00:00Z",
          updatedAt: "2026-09-26T00:00:00Z",
          transport: "stdio",
          toolCount: 1,
          executable: "example",
        },
      ],
    });
  });

  it("sends typed MCP config through the single Wails Provider update method", async () => {
    vi.mocked(Service.UpdateConfiguredProvider).mockResolvedValue({
      provider: {
        providerId: "prov_00000000000000000000000000000001",
        namespace: "upstream",
        displayName: "Upstream edited",
        description: "Edited",
        createdAt: "2026-09-26T00:00:00Z",
        updatedAt: "2026-09-26T01:00:00Z",
        transport: "http",
        toolCount: 1,
        endpoint: "https://example.test/mcp",
      },
      sync: { created: 0, updated: 1, removed: 0, total: 1 },
    });
    const client = createDesktopMenderClient();
    const config = {transport: {kind: "streamable-http", endpoint: "https://example.test/mcp",
      headers: {"X-Mode": "read"},
    }};
    const result = await client.updateProvider(
      "prov_00000000000000000000000000000001",
      {displayName: "Upstream edited", description: "Edited", config},
    );
    expect(Service.UpdateConfiguredProvider).toHaveBeenCalledWith(
      "prov_00000000000000000000000000000001",
      "Upstream edited",
      "Edited",
      config,
    );
    expect(result.provider.displayName).toBe("Upstream edited");
    expect(result.provider.toolCount).toBe(1);
  });

  it("browses compact synchronized Tools through Wails", async () => {
    vi.mocked(Service.BrowseTools).mockResolvedValue({
      tools: [
        {
          rank: 1,
          toolId: "tool_1",
          providerId: "prov_1",
          name: "search",
          title: "",
          description: "Search",
        },
      ],
    });

    const client = createDesktopMenderClient();

    await expect(client.browseTools()).resolves.toEqual({
      tools: [
        {
          rank: 1,
          toolId: "tool_1",
          providerId: "prov_1",
          name: "search",
          title: "",
          description: "Search",
        },
      ],
    });
    expect(Service.BrowseTools).toHaveBeenCalledWith(100, "", "");
  });

  it("forwards Provider-scoped Tool search through Wails", async () => {
    vi.mocked(Service.SearchTools).mockResolvedValue({
      tools: [
        {
          rank: 1,
          toolId: "tool_1",
          providerId: "prov_1",
          name: "search",
          title: "",
          description: "Search",
        },
      ],
    });

    const client = createDesktopMenderClient();
    await expect(
      client.searchTools({
        query: "search",
        limit: 30,
        providerId: "prov_1",
      }),
    ).resolves.toMatchObject({
      tools: [{ toolId: "tool_1", providerId: "prov_1" }],
    });

    expect(Service.SearchTools).toHaveBeenCalledWith(
      "search",
      30,
      "prov_1",
      "",
      "",
    );
  });


  it("forwards revision-scoped tool cursors and nextCursor through Wails",async()=>{
    vi.mocked(Service.BrowseTools).mockResolvedValue({
      tools:[{rank:51,toolId:"tool_51",providerId:"prov_1",name:"search",
        title:"Search",description:"Search the web"}],
      nextCursor:"browse-next",
    });
    vi.mocked(Service.SearchTools).mockResolvedValue({
      tools:[{rank:31,toolId:"tool_31",providerId:"prov_1",name:"search",
        title:"Search",description:"Search the web"}],
      nextCursor:"search-next",
    });
    const client=createDesktopMenderClient();
    await expect(client.browseTools({limit:50,providerId:"prov_1",cursor:"opaque-browse"}))
      .resolves.toMatchObject({nextCursor:"browse-next",tools:[{toolId:"tool_51",rank:51}]});
    expect(Service.BrowseTools).toHaveBeenCalledWith(50,"prov_1","opaque-browse");
    await expect(client.searchTools({query:"search",limit:30,providerId:"prov_1",
      profile:"default",cursor:"opaque-search"}))
      .resolves.toMatchObject({nextCursor:"search-next",tools:[{toolId:"tool_31",rank:31}]});
    expect(Service.SearchTools).toHaveBeenCalledWith("search",30,"prov_1","default","opaque-search");
  });

  it("maps Credential inventory without exposing secret values", async () => {
    vi.mocked(Service.ListCredentials).mockResolvedValue({
      credentials: [
        {
          reference: "credential://demo/work",
          namespace: "demo",
          name: "work",
          displayName: "Demo work",
          source: "environment",
          environmentVariable: "DEMO_WORK_TOKEN",
          createdAt: "2026-09-27T00:00:00Z",
          updatedAt: "2026-09-27T00:00:00Z",
        },
      ],
    });
    vi.mocked(Service.CreateEnvironmentCredential).mockResolvedValue({
      reference: "credential://demo/work",
      namespace: "demo",
      name: "work",
      displayName: "Demo work",
      source: "environment",
      environmentVariable: "DEMO_WORK_TOKEN",
      createdAt: "2026-09-27T00:00:00Z",
      updatedAt: "2026-09-27T00:00:00Z",
    });

    const client = createDesktopMenderClient();

    await expect(client.listCredentials()).resolves.toEqual({
      credentials: [
        {
          reference: "credential://demo/work",
          namespace: "demo",
          name: "work",
          displayName: "Demo work",
          source: "environment",
          environmentVariable: "DEMO_WORK_TOKEN",
          createdAt: "2026-09-27T00:00:00Z",
          updatedAt: "2026-09-27T00:00:00Z",
        },
      ],
    });
    await expect(
      client.createEnvironmentCredential({
        namespace: "demo",
        name: "work",
        displayName: "Demo work",
        environmentVariable: "DEMO_WORK_TOKEN",
      }),
    ).resolves.toMatchObject({
      reference: "credential://demo/work",
      source: "environment",
    });
    await client.deleteCredential("demo", "work");

    expect(Service.CreateEnvironmentCredential).toHaveBeenCalledWith(
      "demo",
      "work",
      "Demo work",
      "DEMO_WORK_TOKEN",
    );
    expect(Service.DeleteCredential).toHaveBeenCalledWith("demo", "work");
  });

  it("maps persistent Policy CRUD through Wails", async () => {
    const rule = {
      ruleId: "rule_00000000000000000000000000000001",
      displayName: "Guard MCP",
      enabled: true,
      decision: "deny",
      match: {
        actions: ["tool.invoke"],
        origins: ["mcp"],
        principalKinds: ["service"],
        principalRefs: [],
        scopeKinds: ["profile"],
        profileRefs: ["prof_00000000000000000000000000000001"],
        providerIds: ["prov_00000000000000000000000000000001"],
        toolIds: ["tool_00000000000000000000000000000001"],
        factKnowledge: "known",
        effectsAny: ["filesystem.write"],
        resourcesAny: [
          { kind: "path", operator: "prefix", value: "/workspace/release" },
        ],
      },
      createdAt: "2026-09-27T00:00:00Z",
      updatedAt: "2026-09-27T00:00:00Z",
    };
    vi.mocked(Service.GetPolicy).mockResolvedValue({
      defaultDecision: "allow",
      rules: [rule],
    });
    vi.mocked(Service.SetPolicyDefault).mockResolvedValue({
      defaultDecision: "deny",
      rules: [rule],
    });
    vi.mocked(Service.CreatePolicyRule).mockResolvedValue(rule);
    vi.mocked(Service.UpdatePolicyRule).mockResolvedValue({
      ...rule,
      displayName: "Updated guard",
    });
    vi.mocked(Service.SetPolicyRuleEnabled).mockResolvedValue({
      ...rule,
      enabled: false,
    });

    const client = createDesktopMenderClient();
    const input: PolicyRuleInput = {
      displayName: "Guard MCP",
      enabled: true,
      decision: "deny" as const,
      match: {
        actions: ["tool.invoke"],
        origins: ["mcp"],
        principalKinds: ["service"],
        principalRefs: [],
        scopeKinds: ["profile"],
        profileRefs: ["prof_00000000000000000000000000000001"],
        providerIds: ["prov_00000000000000000000000000000001"],
        toolIds: ["tool_00000000000000000000000000000001"],
        factKnowledge: "known",
        effectsAny: ["filesystem.write"],
        resourcesAny: [
          {
            kind: "path",
            operator: "prefix",
            value: "/workspace/release",
          },
        ],
      },
    };

    await expect(client.getPolicy()).resolves.toMatchObject({
      defaultDecision: "allow",
      rules: [{ decision: "deny", match: { origins: ["mcp"] } }],
    });
    await expect(
      client.setPolicyDefault({ decision: "deny" }),
    ).resolves.toMatchObject({ defaultDecision: "deny" });
    await expect(client.createPolicyRule(input)).resolves.toMatchObject({
      ruleId: rule.ruleId,
      decision: "deny",
    });
    await expect(
      client.updatePolicyRule(rule.ruleId, {
        ...input,
        displayName: "Updated guard",
      }),
    ).resolves.toMatchObject({ displayName: "Updated guard" });
    await expect(
      client.setPolicyRuleEnabled(rule.ruleId, false),
    ).resolves.toMatchObject({ enabled: false });
    await client.deletePolicyRule(rule.ruleId);

    expect(Service.SetPolicyDefault).toHaveBeenCalledWith("deny");
    expect(Service.CreatePolicyRule).toHaveBeenCalledWith(
      expect.objectContaining({
        displayName: "Guard MCP",
        decision: "deny",
      }),
    );
    expect(Service.UpdatePolicyRule).toHaveBeenCalledWith(
      rule.ruleId,
      expect.objectContaining({ displayName: "Updated guard" }),
    );
    expect(Service.SetPolicyRuleEnabled).toHaveBeenCalledWith(
      rule.ruleId,
      false,
    );
    expect(Service.DeletePolicyRule).toHaveBeenCalledWith(rule.ruleId);
  });

  it("replaces Profile Tool membership through Wails", async () => {
    vi.mocked(Service.ReplaceProfileTools).mockResolvedValue({
      profileId: "prof_1",
      slug: "default",
      displayName: "Default",
      mode: "hybrid",
      tools: [
        {
          toolId: "tool_1",
          direct: true,
          alias: "profile_tool",
          titleOverride: "Profile Tool",
          descriptionOverride: "Profile-specific presentation",
          policyConstraint: "deny",
          concurrencyConstraint: "serial",
          credentialRefs: ["credential://github/work"],
        },
        {
          toolId: "tool_2",
          direct: false,
          alias: "",
          titleOverride: "",
          descriptionOverride: "",
          policyConstraint: "inherit",
          concurrencyConstraint: "inherit",
          credentialRefs: [],
        },
      ],
    });

    const client = createDesktopMenderClient();
    const result = await client.replaceProfileTools("default", {
      tools: [
        {
          toolId: "tool_1",
          direct: true,
          alias: "profile_tool",
          titleOverride: "Profile Tool",
          descriptionOverride: "Profile-specific presentation",
          policyConstraint: "deny",
          concurrencyConstraint: "serial",
          credentialRefs: ["credential://github/work"],
        },
        { toolId: "tool_2", direct: false },
      ],
    });

    expect(Service.ReplaceProfileTools).toHaveBeenCalledWith("default", [
      {
        toolId: "tool_1",
        direct: true,
        alias: "profile_tool",
        titleOverride: "Profile Tool",
        descriptionOverride: "Profile-specific presentation",
        policyConstraint: "deny",
        concurrencyConstraint: "serial",
        credentialRefs: ["credential://github/work"],
      },
      {
        toolId: "tool_2",
        direct: false,
        alias: "",
        titleOverride: "",
        descriptionOverride: "",
        policyConstraint: "inherit",
        concurrencyConstraint: "inherit",
        credentialRefs: [],
      },
    ]);
    expect(result.tools).toHaveLength(2);
    expect(result.tools[0]).toMatchObject({
      alias: "profile_tool",
      policyConstraint: "deny",
      concurrencyConstraint: "serial",
    });
  });

  it("adapts managed MCP server lifecycle to Wails", async () => {
    const instance = {
      serverId: "srv_1",
      profile: "default",
      transport: "streamable-http",
      address: "127.0.0.1:8765",
      endpoint: "http://127.0.0.1:8765/mcp",
      status: "running",
      startedAt: "2026-09-26T09:00:00Z",
    };
    vi.mocked(Service.ListMCPServers).mockResolvedValue({
      servers: [instance],
    });
    vi.mocked(Service.StartMCPServer).mockResolvedValue(instance);
    vi.mocked(Service.RestartMCPServer).mockResolvedValue({
      ...instance,
      serverId: "srv_2",
    });
    vi.mocked(Service.StopMCPServer).mockResolvedValue(undefined);

    const client = createDesktopMenderClient();

    await expect(client.listMCPServers()).resolves.toEqual({
      servers: [instance],
    });
    await expect(
      client.startMCPServer({
        profile: "default",
        address: "127.0.0.1:8765",
      }),
    ).resolves.toEqual(instance);
    expect(Service.StartMCPServer).toHaveBeenCalledWith(
      "default",
      "127.0.0.1:8765",
    );

    await expect(client.restartMCPServer("srv_1")).resolves.toMatchObject({
      serverId: "srv_2",
    });
    await client.stopMCPServer("srv_2");
    expect(Service.StopMCPServer).toHaveBeenCalledWith("srv_2");
  });

  it("maps governed invocation outcomes and Audit metadata through Wails", async () => {
    vi.mocked(Service.CallTool).mockResolvedValue({
      invocationId: "inv_1",
      kind: "failed",
      message: "tool result does not match the declared output schema",
      source: "result_guard",
      code: "output_schema_mismatch",
    });
    vi.mocked(Service.ListAudit).mockResolvedValue({
      records: [
        {
          invocationId: "inv_1",
          toolId: "tool_1",
          providerId: "prov_1",
          principalKind: "local_user",
          principalRef: "os:501",
          startedAt: "2026-09-26T00:00:00Z",
          finishedAt: "2026-09-26T00:00:01Z",
          policyKind: "allow",
          policyReason: "local.default",
          approval: "not_required",
          status: "failed",
          resultKind: "",
          errorClass: "result_guard",
          errorSummary: "",
          origin: "desktop",
          scopeKind: "catalog",
          scopeReference: "",
          outcomeKind: "failed",
          rejectionCode: "",
          failureSource: "result_guard",
          failureCode: "output_schema_mismatch",
        },
      ],
    });

    const client = createDesktopMenderClient();

    await expect(client.callTool("tool_1", {})).resolves.toEqual({
      invocationId: "inv_1",
      kind: "failed",
      message: "tool result does not match the declared output schema",
      source: "result_guard",
      code: "output_schema_mismatch",
    });
    await expect(client.listAudit(10)).resolves.toMatchObject({
      records: [
        {
          origin: "desktop",
          scopeKind: "catalog",
          outcomeKind: "failed",
          failureSource: "result_guard",
          failureCode: "output_schema_mismatch",
        },
      ],
    });
  });

  it("adapts owner-only Sandbox Profile grant methods", async () => {
    const sandboxId = "sbx_0123456789abcdef0123456789abcdef";
    const profileId = "prof_0123456789abcdef0123456789abcdef";
    const grant = { sandboxId, grants: [{ profileId, capabilities: ["file.read"] }] };
    vi.mocked(Service.GetSandboxProfileGrants).mockResolvedValue(grant);
    vi.mocked(Service.PutSandboxProfileGrants).mockResolvedValue(grant);
    vi.mocked(Service.RevokeSandboxProfileGrants).mockResolvedValue({sandboxId,grants:[]});
    vi.mocked(Service.GetSandboxProfileGrantEvents).mockResolvedValue({sandboxId,events:[]});
    const client = createDesktopMenderClient();
    await expect(client.getSandboxProfileGrants(sandboxId)).resolves.toEqual(grant);
    await expect(client.putSandboxProfileGrants(sandboxId,profileId,{capabilities:["file.read"]})).resolves.toEqual(grant);
    expect(Service.PutSandboxProfileGrants).toHaveBeenCalledWith(sandboxId,profileId,["file.read"]);
    await expect(client.revokeSandboxProfileGrants(sandboxId,profileId)).resolves.toEqual({sandboxId,grants:[]});
    await expect(client.getSandboxProfileGrantEvents(sandboxId)).resolves.toEqual({sandboxId,events:[]});
    vi.mocked(Service.GetSandboxProfileGrants).mockResolvedValue({sandboxId,grants:[{profileId,capabilities:["invalid"]}]});
    await expect(client.getSandboxProfileGrants(sandboxId)).rejects.toBeInstanceOf(MenderClientInvariantError);
  });

  it("rejects unexpected binding enum values at the host boundary", async () => {
    vi.mocked(Service.ListProviders).mockResolvedValue({
      providers: [
        {
          providerId: "prov_00000000000000000000000000000001",
          namespace: "upstream",
          displayName: "Upstream",
          description: "",
          createdAt: "2026-09-26T00:00:00Z",
          updatedAt: "2026-09-26T00:00:00Z",
          transport: "unknown",
          toolCount: 0,
        },
      ],
    });

    const client = createDesktopMenderClient();
    const request = client.listProviders();

    await expect(request).rejects.toBeInstanceOf(MenderClientInvariantError);
    await expect(request).rejects.toThrow(
      "Unsupported desktop provider transport",
    );
  });
});
