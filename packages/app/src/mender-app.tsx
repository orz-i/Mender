import type {
  MenderClient,
  ProviderPluginInstallation,
  SkillDeployment,
  SkillInstallation,
} from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { Route, Routes } from "react-router";

import { AuditPage } from "./domains/audit/audit-page";
import { CatalogPage } from "./domains/catalog/catalog-page";
import { CredentialsPage } from "./domains/credentials/credentials-page";
import { OverviewPage } from "./domains/overview/overview-page";
import {
  PluginDetailPage,
  PluginsPage,
} from "./domains/plugins/plugins-page";
import { providerPluginInstaller } from "./domains/plugins/marketplace";
import { PolicyPage } from "./domains/policy/policy-page";
import { ProfilesPage } from "./domains/profiles/profiles-page";
import { ProvidersPage } from "./domains/providers/providers-page";
import { RuntimePage } from "./domains/runtime/runtime-page";
import { ServersPage } from "./domains/servers/servers-page";
import { SkillsPage } from "./domains/skills/skills-page";
import { TunnelsPage } from "./domains/tunnels/tunnels-page";
import { useLocale } from "./i18n/i18n";
import { ManagementShell } from "./shell/management-shell";

export function MenderApp({
  client,
  copyText,
  installProviderPlugin,
  installSkill,
  deploySkill,
}: {
  client: MenderClient;
  copyText?: (value: string) => Promise<void>;
  installProviderPlugin?: () => Promise<ProviderPluginInstallation | null>;
  installSkill?: () => Promise<SkillInstallation | null>;
  deploySkill?: (
    installationId: string,
    target: SkillDeployment["target"],
  ) => Promise<SkillDeployment | null>;
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
                  installProviderPlugin === undefined
                    ? []
                    : [providerPluginInstaller(installProviderPlugin)]
                }
              />
            }
          />
          <Route
            path="plugins/:pluginKey"
            element={<PluginDetailPage client={client} />}
          />
          <Route
            path="providers"
            element={<ProvidersPage client={client} />}
          />
          <Route
            path="skills"
            element={
              <SkillsPage
                client={client}
                {...(installSkill === undefined ? {} : { installSkill })}
                {...(deploySkill === undefined ? {} : { deploySkill })}
              />
            }
          />
          <Route path="catalog" element={<CatalogPage client={client} />} />
          <Route path="profiles" element={<ProfilesPage client={client} />} />
          <Route path="tunnels" element={<TunnelsPage client={client} />} />
          <Route
            path="servers"
            element={
              <ServersPage
                client={client}
                {...(copyText === undefined ? {} : { copyText })}
              />
            }
          />
          <Route path="runtime" element={<RuntimePage client={client} />} />
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
