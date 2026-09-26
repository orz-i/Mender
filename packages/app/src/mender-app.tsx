import type { MenderClient } from "@mender/sdk";
import { Route, Routes } from "react-router";

import { CatalogPage } from "./domains/catalog/catalog-page";
import { OverviewPage } from "./domains/overview/overview-page";
import { ProfilesPage } from "./domains/profiles/profiles-page";
import { ProvidersPage } from "./domains/providers/providers-page";
import { ManagementShell } from "./shell/management-shell";

export function MenderApp({ client }: { client: MenderClient }) {
  return (
    <Routes>
      <Route element={<ManagementShell />}>
        <Route index element={<OverviewPage client={client} />} />
        <Route path="providers" element={<ProvidersPage client={client} />} />
        <Route path="catalog" element={<CatalogPage client={client} />} />
        <Route path="profiles" element={<ProfilesPage client={client} />} />
        <Route path="*" element={<OverviewPage client={client} />} />
      </Route>
    </Routes>
  );
}
