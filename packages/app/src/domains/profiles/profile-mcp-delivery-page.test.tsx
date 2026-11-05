import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MenderApiError, type MCPServerInstance, type MenderClient, type Profile, type ProfileMCPDelivery, type ConfigureProfileMCPDeliveryRequest } from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { ProfileMCPDeliveryPage } from "./profile-mcp-delivery-page";

const alpha: Profile = {
  profileId: "prof_00000000000000000000000000000001",
  slug: "alpha", displayName: "Alpha", tools: [], skills: [],revision:7,
};
const beta: Profile = {
  profileId: "prof_00000000000000000000000000000002",
  slug: "beta", displayName: "Beta", tools: [], skills: [],revision:0,
};
const alphaServer: MCPServerInstance = {
  serverId: "srv_alpha", profile: "alpha", transport: "streamable-http",
  address: "127.0.0.1:8765", endpoint: "http://127.0.0.1:8765/mcp",
  status: "running", startedAt: "2026-10-23T00:00:00Z",
};
const betaServer: MCPServerInstance = {
  serverId: "srv_beta", profile: "beta", transport: "streamable-http",
  address: "127.0.0.1:8766", endpoint: "http://127.0.0.1:8766/mcp",
  status: "running", startedAt: "2026-10-23T00:00:00Z",
};

function mount(servers: MCPServerInstance[], options:{config?:ProfileMCPDelivery|null;profile?:Profile;saveError?:Error}={}) {
  const profile=options.profile??alpha;
  const config=options.config===undefined?{profileId:profile.profileId,profileRevision:profile.revision,revision:3,mode:"hybrid" as const,directToolIds:[]}:options.config;
  const configureProfileMCPDelivery=vi.fn<(slug:string,body:ConfigureProfileMCPDeliveryRequest)=>Promise<ProfileMCPDelivery>>().mockImplementation((_slug,body)=>{
    if(options.saveError)return Promise.reject(options.saveError);
    return Promise.resolve({profileId:profile.profileId,profileRevision:profile.revision,revision:(config?.revision??-1)+1,mode:body.mode,directToolIds:body.directToolIds,...(body.tunnelId===undefined?{}:{tunnelId:body.tunnelId})});
  });
  const startMCPServer = vi.fn().mockResolvedValue({ ...alphaServer, serverId: "srv_new", endpoint: "http://127.0.0.1:8770/mcp" });
  const restartMCPServer = vi.fn().mockResolvedValue({ ...alphaServer, serverId: "srv_restarted" });
  const stopMCPServer = vi.fn().mockResolvedValue(undefined);
  const client = {
    getProfile:vi.fn().mockResolvedValue(profile),
    getProfileMCPDelivery:vi.fn().mockResolvedValue(config),
    configureProfileMCPDelivery,
    getTool:vi.fn().mockResolvedValue({name:"Echo",title:"Echo"}),
    listTunnels:vi.fn().mockResolvedValue({tunnels:[{tunnelId:"tun_00000000000000000000000000000001",displayName:"Work Tunnel"}]}),
    listProfiles: vi.fn().mockResolvedValue({ profiles: [alpha, beta] }),
    listMCPServers: vi.fn().mockResolvedValue({ servers }),
    startMCPServer, restartMCPServer, stopMCPServer,
  } as unknown as MenderClient;
  render(<MemoryRouter initialEntries={["/profiles/alpha/deliveries/mcp"]}>
    <TooltipProvider><Routes>
      <Route path="/profiles/:slug/deliveries/mcp" element={<ProfileMCPDeliveryPage client={client} copyText={vi.fn().mockResolvedValue(undefined)} />} />
      <Route path="/profiles/:slug" element={<p>Profile detail</p>} />
    </Routes></TooltipProvider>
  </MemoryRouter>);
  return { startMCPServer, restartMCPServer, stopMCPServer, configureProfileMCPDelivery, client };
}

beforeEach(() => { resetLocaleForTesting(); setLocale("en"); });
afterEach(() => { cleanup(); resetLocaleForTesting(); });

it("shows only its Profile endpoints and never offers cross-Profile selection", async () => {
  const { startMCPServer } = mount([alphaServer, betaServer]);
  expect(await screen.findByText(alphaServer.endpoint)).toBeTruthy();
  expect(screen.queryByText(betaServer.endpoint)).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Start server" }));
  const dialog = within(screen.getByRole("dialog", { name: "Start MCP server" }));
  expect(dialog.queryByLabelText("Server profile")).toBeNull();
  expect(dialog.getByText("MCP delivery belongs to Profile Alpha.")).toBeTruthy();
  fireEvent.click(dialog.getByRole("button", { name: "Start server" }));
  await waitFor(() => expect(startMCPServer).toHaveBeenCalledWith({ profile: "alpha", address: "127.0.0.1:8765" }));
  expect(await screen.findByText("http://127.0.0.1:8770/mcp")).toBeTruthy();
  expect(screen.queryByText(betaServer.endpoint)).toBeNull();
});

it("only exposes stop/restart actions for instances in that Profile", async () => {
  const { restartMCPServer, stopMCPServer } = mount([alphaServer, betaServer]);
  expect(await screen.findByText(alphaServer.endpoint)).toBeTruthy();
  expect(screen.getAllByRole("button", { name: "Copy endpoint" })).toHaveLength(1);
  expect(screen.getAllByRole("button", { name: /More server actions:/ })).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: /More server actions:/ }));
  const menu = within(await screen.findByRole("group", { name: "Server actions" }));
  expect(menu.getByRole("button", { name: "Copy config" })).toBeTruthy();
  fireEvent.click(menu.getByRole("button", { name: "Restart" }));
  await waitFor(() => expect(restartMCPServer).toHaveBeenCalledWith("srv_alpha"));
  expect(restartMCPServer).not.toHaveBeenCalledWith("srv_beta");

  fireEvent.click(screen.getByRole("button", { name: /More server actions:/ }));
  fireEvent.click(within(await screen.findByRole("group", { name: "Server actions" }))
    .getByRole("button", { name: "Stop server" }));
  const confirmation = within(await screen.findByRole("dialog", { name: "Stop MCP server?" }));
  expect(confirmation.getByText("http://127.0.0.1:8765/mcp")).toBeTruthy();
  expect(stopMCPServer).not.toHaveBeenCalled();
  fireEvent.click(confirmation.getByRole("button", { name: "Cancel" }));
  expect(stopMCPServer).not.toHaveBeenCalled();

  fireEvent.click(screen.getByRole("button", { name: /More server actions:/ }));
  fireEvent.click(within(await screen.findByRole("group", { name: "Server actions" }))
    .getByRole("button", { name: "Stop server" }));
  fireEvent.click(within(await screen.findByRole("dialog", { name: "Stop MCP server?" }))
    .getByRole("button", { name: "Stop server" }));
  await waitFor(() => expect(stopMCPServer).toHaveBeenCalledWith("srv_restarted"));
  expect(stopMCPServer).not.toHaveBeenCalledWith("srv_beta");
});

it("goes back to its Profile instead of showing a separate Servers inventory", async () => {
  mount([]);
  fireEvent.click(await screen.findByRole("button", { name: "Back to profile" }));
  expect(await screen.findByText("Profile detail")).toBeTruthy();
});

it("loads an unconfigured Profile without configuring it or exposing lifecycle actions",async()=>{
  const {configureProfileMCPDelivery,startMCPServer}=mount([],{config:null});
  expect(await screen.findByRole("form",{name:"MCP delivery settings"})).toBeTruthy();
  expect(configureProfileMCPDelivery).not.toHaveBeenCalled();
  expect(startMCPServer).not.toHaveBeenCalled();
  expect(screen.queryByRole("button",{name:"Start server"})).toBeNull();
  fireEvent.click(screen.getByRole("button",{name:"Save MCP settings"}));
  await waitFor(()=>expect(configureProfileMCPDelivery).toHaveBeenCalledWith("alpha",{
    mode:"hybrid",tunnelId:"",directToolIds:[],expectedProfileRevision:7,
  }));
  expect(Object.hasOwn(configureProfileMCPDelivery.mock.calls[0]![1],"expectedRevision")).toBe(false);
  expect(await screen.findByRole("button",{name:"Start server"})).toBeTruthy();
  expect(startMCPServer).not.toHaveBeenCalled();
});

it("saves MCP mode, member subset and Tunnel using both zero revision fences",async()=>{
  const toolId="tool_00000000000000000000000000000001";
  const profile:Profile={...alpha,revision:0,tools:[{toolId,alias:"Echo",titleOverride:"",descriptionOverride:"",policyConstraint:"deny",concurrencyConstraint:"serial",credentialRefs:[]}]};
  const config:ProfileMCPDelivery={profileId:profile.profileId,profileRevision:0,revision:0,mode:"hybrid",directToolIds:[]};
  const {configureProfileMCPDelivery,startMCPServer}=mount([],{profile,config});
  const checkbox=await screen.findByRole("checkbox",{name:"Direct export Echo"});
  fireEvent.click(checkbox);
  fireEvent.change(screen.getByLabelText("MCP delivery tunnel"),{target:{value:"tun_00000000000000000000000000000001"}});
  fireEvent.click(screen.getByRole("button",{name:"Save MCP settings"}));
  await waitFor(()=>expect(configureProfileMCPDelivery).toHaveBeenLastCalledWith("alpha",{
    mode:"hybrid",tunnelId:"tun_00000000000000000000000000000001",directToolIds:[toolId],expectedProfileRevision:0,expectedRevision:0,
  }));
  await waitFor(()=>expect(screen.getByRole("button",{name:"Save MCP settings"}).hasAttribute("disabled")).toBe(true));
  fireEvent.change(screen.getByLabelText("Exposure mode"),{target:{value:"dynamic"}});
  expect(screen.queryByRole("checkbox",{name:"Direct export Echo"})).toBeNull();
  fireEvent.click(screen.getByRole("button",{name:"Save MCP settings"}));
  await waitFor(()=>expect(configureProfileMCPDelivery).toHaveBeenLastCalledWith("alpha",{
    mode:"dynamic",tunnelId:"tun_00000000000000000000000000000001",directToolIds:[],expectedProfileRevision:0,expectedRevision:1,
  }));
  expect(profile.tools[0]?.policyConstraint).toBe("deny");
  expect(startMCPServer).not.toHaveBeenCalled();
});

it("keeps conflicts explicit without retrying or starting a server",async()=>{
  const {configureProfileMCPDelivery,startMCPServer}=mount([],{config:null,saveError:new MenderApiError(409,"revision_conflict","Profile selection changed")});
  fireEvent.click(await screen.findByRole("button",{name:"Save MCP settings"}));
  expect(await screen.findByText("Profile selection changed")).toBeTruthy();
  expect(configureProfileMCPDelivery).toHaveBeenCalledTimes(1);
  expect(startMCPServer).not.toHaveBeenCalled();
  expect(screen.getByRole("button",{name:"Reload settings"})).toBeTruthy();
});

it("refuses a mixed Profile/configuration snapshot before offering configuration or lifecycle",async()=>{
  const {configureProfileMCPDelivery,startMCPServer}=mount([],{config:{profileId:beta.profileId,profileRevision:alpha.revision,revision:0,mode:"direct",directToolIds:[]}});
  expect(await screen.findByText("Profile changed while loading MCP delivery. Reload before saving.")).toBeTruthy();
  expect(screen.queryByRole("form",{name:"MCP delivery settings"})).toBeNull();
  expect(configureProfileMCPDelivery).not.toHaveBeenCalled();
  expect(startMCPServer).not.toHaveBeenCalled();
});
