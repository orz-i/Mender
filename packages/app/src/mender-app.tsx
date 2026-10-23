import type {
  MenderClient,
  ExportProfilePluginInput,
  ProfileAgentPluginExport,
  ProviderPluginInstallation,
  SkillDeployment,
  SkillInstallation,
} from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { Navigate, Route, Routes } from "react-router";

import { AgentsPage } from "./domains/agents/agents-page";
import { AgentCreatePage } from "./domains/agents/agent-create-page";
import { AgentDetailPage } from "./domains/agents/agent-detail-page";
import { AgentApprovalCenter } from "./domains/agents/agent-approval-center";
import { AuditPage } from "./domains/audit/audit-page";
import { CredentialsPage } from "./domains/credentials/credentials-page";
import { OverviewPage } from "./domains/overview/overview-page";
import {
  PluginDetailPage,
  PluginsPage,
} from "./domains/plugins/plugins-page";
import { pluginInstaller } from "./domains/plugins/marketplace";
import { PolicyPage } from "./domains/policy/policy-page";
import { ProfilesPage } from "./domains/profiles/profiles-page";
import { ProfileCreatePage } from "./domains/profiles/profile-create-page";
import { ProfileDetailPage } from "./domains/profiles/profile-detail-page";
import { ProfileEditPage } from "./domains/profiles/profile-edit-page";
import { ProfileSkillsPage } from "./domains/profiles/profile-skills-page";
import { ProfileMCPDeliveryPage } from "./domains/profiles/profile-mcp-delivery-page";
import { ProfileSkillDeliveryPage } from "./domains/profiles/profile-skill-delivery-page";
import { ProfileCLIDeliveryPage } from "./domains/profiles/profile-cli-delivery-page";
import { ProfileAgentPluginPage } from "./domains/profiles/profile-agent-plugin-page";
import { ToolsPage, ToolServicePage } from "./domains/tools/tools-page";
import { ToolWorkspacePage } from "./domains/tools/tool-workspace-page";
import { RuntimePage } from "./domains/runtime/runtime-page";
import { SandboxesPage, CreateSandboxPage, SandboxDetailPage } from "./domains/sandboxes/sandboxes-page";
import { SandboxActivityPage } from "./domains/sandboxes/sandbox-activity-page";
import { ExternalSandboxCreatePage } from "./domains/sandboxes/sandbox-external-create-page";
import { SkillsPage } from "./domains/skills/skills-page";
import { TunnelsPage } from "./domains/tunnels/tunnels-page";
import { useLocale } from "./i18n/i18n";
import { ManagementShell } from "./shell/management-shell";

export function MenderApp({
  client,
  copyText,
  chooseLocalDirectory,
  installPlugin,
  installSkill,
  deployProfileSkills,
  exportProfilePlugin,
}: {
  client: MenderClient;
  copyText?: (value: string) => Promise<void>;
  chooseLocalDirectory?: () => Promise<string | null>;
  installPlugin?: () => Promise<ProviderPluginInstallation | null>;
  installSkill?: () => Promise<SkillInstallation | null>;
  deployProfileSkills?: (slug: string, target: SkillDeployment["target"]) => Promise<SkillDeployment[] | null>;
  exportProfilePlugin?: (slug: string, input: ExportProfilePluginInput) => Promise<ProfileAgentPluginExport | null>;
}) {
  useLocale();

  return (
    <TooltipProvider>
      <Routes>
        <Route element={<ManagementShell client={client} />}>
          <Route index element={<OverviewPage client={client} />} />
          <Route
            path="plugins"
            element={
              <PluginsPage
                client={client}
                installers={
                  installPlugin === undefined
                    ? []
                    : [pluginInstaller(installPlugin)]
                }
              />
            }
          />
          <Route
            path="plugins/:pluginKey"
            element={<PluginDetailPage client={client} />}
          />
          <Route path="tools" element={<ToolsPage client={client} />} />
          <Route path="tools/:providerId" element={<ToolServicePage client={client} />} />
          <Route path="tools/:providerId/workspaces" element={<ToolWorkspacePage client={client} />} />
          <Route
            path="skills"
            element={
              <SkillsPage
                client={client}
                {...(installSkill === undefined ? {} : { installSkill })}
              />
            }
          />
          <Route path="profiles" element={<ProfilesPage client={client} />} />
          <Route path="profiles/new" element={<ProfileCreatePage client={client} />} />
          <Route path="profiles/:slug" element={<ProfileDetailPage client={client} />} />
          <Route path="profiles/:slug/edit" element={<ProfileEditPage client={client} />} />
          <Route path="profiles/:slug/skills" element={<ProfileSkillsPage client={client} />} />
          <Route path="profiles/:slug/deliveries/mcp" element={<ProfileMCPDeliveryPage client={client}
            {...(copyText === undefined ? {} : { copyText })} />} />
          <Route path="profiles/:slug/deliveries/skills" element={<ProfileSkillDeliveryPage client={client}
            {...(deployProfileSkills === undefined ? {} : { deployProfileSkills })} />} />
          <Route path="profiles/:slug/deliveries/cli" element={<ProfileCLIDeliveryPage client={client}
            {...(copyText === undefined ? {} : { copyText })} />} />
          <Route path="profiles/:slug/deliveries/plugin" element={<ProfileAgentPluginPage client={client}
            {...(exportProfilePlugin === undefined ? {} : { exportProfilePlugin })} />} />
          <Route path="tunnels" element={<TunnelsPage client={client} />} />
          <Route path="servers" element={<Navigate to="/profiles" replace />} />
          <Route path="runtime" element={<RuntimePage client={client} />} />
          <Route path="agents" element={<AgentsPage client={client} />} />
          <Route path="approvals" element={<AgentApprovalCenter client={client} />} />
          <Route path="agents/new" element={<AgentCreatePage client={client}
            {...(chooseLocalDirectory===undefined?{}:{chooseLocalDirectory})} />} />
          <Route path="agents/:agentId" element={<AgentDetailPage client={client}
            {...(chooseLocalDirectory===undefined?{}:{chooseLocalDirectory})}/>} />
          <Route path="sandboxes" element={<SandboxesPage client={client} />} />
          <Route
            path="sandboxes/new"
            element={
              <CreateSandboxPage
                client={client}
                {...(chooseLocalDirectory === undefined ? {} : { chooseLocalDirectory })}
              />
            }
          />
          <Route path="sandboxes/new/external" element={<ExternalSandboxCreatePage client={client} />} />
          <Route path="sandboxes/:sandboxId" element={<SandboxDetailPage client={client} />} />
          <Route path="sandboxes/:sandboxId/activity" element={<SandboxActivityPage client={client} />} />
          <Route path="policy" element={<PolicyPage client={client} />} />
          <Route
            path="credentials"
            element={<CredentialsPage client={client} />}
          />
          <Route path="audit" element={<AuditPage client={client} />} />
          <Route path="*" element={<OverviewPage client={client} />} />
        </Route>
      </Routes>
    </TooltipProvider>
  );
}
