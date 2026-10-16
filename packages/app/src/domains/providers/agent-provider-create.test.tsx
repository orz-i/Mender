import { cleanup, render, screen } from "@testing-library/react";
import type { MenderClient } from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { ProvidersPage } from "./providers-page";

beforeEach(() => { resetLocaleForTesting(); setLocale("en"); });
afterEach(() => { cleanup(); resetLocaleForTesting(); });

it.each([
  "io.mender.provider.agent-delegation",
  "io.mender.provider.workspace",
  "io.mender.provider.sandbox-jobs",
])("rejects a generic Provider deep link to managed facade %s", async (pluginId) => {
  const plugin={
    pluginId,displayName:"Internal Resource Tools",sourceKind:"builtin",enabled:true,
    version:"1.0.0",packageVersion:"1.0.0",configVersion:1,
    capabilities:["internal.tool"],configSchema:{type:"object"},
  };
  const createProvider=vi.fn();
  const client={
    listProviderPlugins:vi.fn().mockResolvedValue({plugins:[plugin],issues:[]}),
    listProviders:vi.fn().mockResolvedValue({providers:[]}),
    listCredentials:vi.fn().mockResolvedValue({credentials:[]}),
    createProvider,
  } as unknown as MenderClient;
  const query=new URLSearchParams({create:"1",pluginId,agentId:"agent_0123456789abcdef0123456789abcdef"});
  render(<MemoryRouter initialEntries={["/providers?"+query.toString()]}>
    <TooltipProvider><ProvidersPage client={client}/></TooltipProvider>
  </MemoryRouter>);
  expect(await screen.findByText("The requested item was not found.")).toBeTruthy();
  expect(screen.queryByRole("dialog",{name:/Add Internal Resource Tools/})).toBeNull();
  expect(createProvider).not.toHaveBeenCalled();
});
