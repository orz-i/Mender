import type { MenderClient, ProviderPluginInstallation } from "@mender/sdk";
import { Route, Routes } from "react-router";

import { AuditPage } from "./domains/audit/audit-page";
import { CatalogPage } from "./domains/catalog/catalog-page";
import { CredentialsPage } from "./domains/credentials/credentials-page";
import { OverviewPage } from "./domains/overview/overview-page";
import { PolicyPage } from "./domains/policy/policy-page";
import { ProfilesPage } from "./domains/profiles/profiles-page";
import { ProvidersPage } from "./domains/providers/providers-page";
import { ServersPage } from "./domains/servers/servers-page";
import { useLocale } from "./i18n/i18n";
import { ManagementShell } from "./shell/management-shell";

export function MenderApp({
  client,
  copyText,
  installProviderPlugin,
}: {
  client: MenderClient;
  copyText?: (value: string) => Promise<void>;
  installProviderPlugin?: () => Promise<ProviderPluginInstallation | null>;
}) {
  useLocale();

  return (
    <Routes>
      <Route element={<ManagementShell />}>
        <Route index element={<OverviewPage client={client} />} />
        <Route
          path="providers"
          element={
            <ProvidersPage
              client={client}
              {...(installProviderPlugin === undefined
                ? {}
                : { installProviderPlugin })}
            />
          }
        />
        <Route path="catalog" element={<CatalogPage client={client} />} />
        <Route path="profiles" element={<ProfilesPage client={client} />} />
        <Route
          path="servers"
          element={
            <ServersPage
              client={client}
              {...(copyText === undefined ? {} : { copyText })}
            />
          }
        />
        <Route path="policy" element={<PolicyPage client={client} />} />
        <Route
          path="credentials"
          element={<CredentialsPage client={client} />}
        />
        <Route path="audit" element={<AuditPage client={client} />} />
        <Route path="*" element={<OverviewPage client={client} />} />
      </Route>
    </Routes>
  );
}
