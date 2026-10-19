import type { MenderClient } from "@mender/sdk";
import { useParams } from "react-router";

import { CatalogPage } from "../catalog/catalog-page";
import { ProvidersPage } from "../providers/providers-page";

/** The Tools workspace has two levels: added services, then that service's Tools. */
export function ToolsPage({ client }: { client: MenderClient }) {
  return <ProvidersPage client={client} />;
}

export function ToolServicePage({ client }: { client: MenderClient }) {
  const { providerId } = useParams();
  if (!providerId) return null;
  return <CatalogPage key={providerId} client={client} providerId={providerId} />;
}
