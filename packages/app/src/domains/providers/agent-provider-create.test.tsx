import { cleanup, render, screen } from "@testing-library/react";
import type { MenderClient } from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { ProvidersPage } from "./providers-page";

beforeEach(() => { resetLocaleForTesting(); setLocale("en"); });
afterEach(() => { cleanup(); resetLocaleForTesting(); });

it("rejects legacy Agent Delegation Provider deep links without creating a second facade", async () => {
  const agentId="agent_0123456789abcdef0123456789abcdef";
  const plugin={
    pluginId:"io.mender.provider.agent-delegation",
    displayName:"Agent Delegation Tools",sourceKind:"builtin",enabled:true,
    version:"1.0.0",packageVersion:"1.0.0",configVersion:1,
    capabilities:["agent.delegate"],
    configSchema:{
      type:"object",additionalProperties:false,
      required:["agentId"],properties:{agentId:{type:"string"}},
    },
  };
  const createProvider=vi.fn();
  const client={
    listProviderPlugins:vi.fn().mockResolvedValue({plugins:[plugin],issues:[]}),
    listProviders:vi.fn().mockResolvedValue({providers:[]}),
    listCredentials:vi.fn().mockResolvedValue({credentials:[]}),
    listSandboxes:vi.fn().mockResolvedValue({sandboxes:[]}),
    createProvider,
  } as unknown as MenderClient;
  const query=new URLSearchParams({create:"1",pluginId:plugin.pluginId,agentId});
  render(<MemoryRouter initialEntries={["/providers?"+query.toString()]}>
    <TooltipProvider><ProvidersPage client={client}/></TooltipProvider>
  </MemoryRouter>);
  expect(await screen.findByText("The requested item was not found.")).toBeTruthy();
  expect(screen.queryByRole("dialog",{name:/Add Agent Delegation/})).toBeNull();
  expect(createProvider).not.toHaveBeenCalled();
});
