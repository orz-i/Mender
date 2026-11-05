import type { ManagementClient } from "@mender/sdk";
import { useParams } from "react-router";

import { CatalogPage } from "./catalog-page";

export function ToolServicePage({ client }: { client: ManagementClient }) {
  const { providerId } = useParams();
  if (!providerId) return null;
  return <CatalogPage key={providerId} client={client} providerId={providerId} />;
}
