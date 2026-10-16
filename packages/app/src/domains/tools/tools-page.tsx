import type { MenderClient } from "@mender/sdk";
import { useSearchParams } from "react-router";

import { CatalogPage } from "../catalog/catalog-page";
import { ProvidersPage } from "../providers/providers-page";
import type { ToolsView } from "./tools-view-switch";

/** Product navigation only: underlying Catalog/Provider identities remain distinct. */
export function ToolsPage({ client }: { client: MenderClient }) {
  const [params, setParams] = useSearchParams();
  const view: ToolsView = params.get("view") === "sources" ? "sources" : "catalog";
  const changeView = (next: ToolsView) => {
    setParams(new URLSearchParams({ view: next }));
  };
  return view === "sources"
    ? <ProvidersPage client={client} onViewChange={changeView} />
    : <CatalogPage client={client} onViewChange={changeView} />;
}
