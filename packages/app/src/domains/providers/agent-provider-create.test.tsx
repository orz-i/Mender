import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { MenderClient } from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { ProvidersPage } from "./providers-page";

beforeEach(() => { resetLocaleForTesting(); setLocale("en"); });
afterEach(() => { cleanup(); resetLocaleForTesting(); });

it("opens a scoped Agent delegation Provider with its AgentID already selected", async () => {
  const agentId="agent_0123456789abcdef0123456789abcdef";
  const plugin={
    pluginId:"io.mender.provider.agent-delegation",
    displayName:"Agent Delegation Tools",sourceKind:"builtin",enabled:true,
    version:"1.0.0",packageVersion:"1.0.0",configVersion:1,
    capabilities:["agent.delegate"],
    configSchema: {
      type:"object",additionalProperties:false,
      required:["agentId"],properties:{
        agentId:{type:"string",pattern:"^agent_[0-9a-f]{32}$"},
      },
    },
  };
  const createProvider=vi.fn().mockResolvedValue({provider:{providerId:"prov_0123456789abcdef0123456789abcdef"},sync:{}});
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
  const modal=await screen.findByRole("dialog",{name:/Add Agent Delegation Tools provider/});
  const namespace=within(modal).getByRole<HTMLInputElement>("textbox",{name:"Namespace"});
  expect(namespace.value).toBe("agent-89abcdef");
  const displayName=within(modal).getByRole<HTMLInputElement>("textbox",{name:"Display name"});
  expect(displayName.value).toBe("Agent Delegation");
  expect(within(modal).getByDisplayValue(agentId)).toBeTruthy();
  fireEvent.click(within(modal).getByRole("button",{name:"Add provider"}));
  await waitFor(()=>expect(createProvider).toHaveBeenCalledWith(expect.objectContaining({
    namespace:"agent-89abcdef",displayName:"Agent Delegation",
    pluginId:plugin.pluginId,config:{agentId},
  })));
});
