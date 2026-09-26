import type { MenderClient } from "@mender/sdk";
import { Route, Routes } from "react-router";

import { OverviewPage } from "./domains/overview/overview-page";
import { ManagementShell } from "./shell/management-shell";

export function MenderApp({ client }: { client: MenderClient }) {
  return (
    <Routes>
      <Route element={<ManagementShell />}>
        <Route index element={<OverviewPage client={client} />} />
        <Route path="*" element={<OverviewPage client={client} />} />
      </Route>
    </Routes>
  );
}
