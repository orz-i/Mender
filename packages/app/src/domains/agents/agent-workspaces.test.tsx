import {cleanup,fireEvent,render,screen,waitFor} from "@testing-library/react";
import type {Agent,MenderClient} from "@mender/sdk";
import {MemoryRouter} from "react-router";
import {afterEach,beforeEach,describe,expect,it,vi} from "vitest";
import {resetLocaleForTesting,setLocale} from "../../i18n/i18n";
import {AgentWorkspaceSettings} from "./agent-workspaces";

const agent:Agent={
 agentId:"agent_00000000000000000000000000000011",
 name:"OpenCode",backend:"opencode",pluginId:"io.mender.agent.opencode",extensionId:"main",
 workspaceRoot:"/tmp/mender-default",isolation:"host",backendAvailable:true,
 createdAt:"2026-10-16T10:00:00Z",
};
const profileId="prof_00000000000000000000000000000011";
const localRoot="/Users/mini/Coding/Gaoge-Agent-Runtime";
beforeEach(()=>{resetLocaleForTesting();setLocale("en");});
afterEach(()=>{cleanup();resetLocaleForTesting();});

describe("Agent owner-approved local workspace editor",()=>{
 it("normalizes GAR alias and chooses a path without listing or creating Sandboxes",async()=>{
  const registered:{key:string;workspaceRoot:string;profileIds:string[]}[]=[];
  const bindAgentWorkspace=vi.fn().mockImplementation((_id:string,key:string,root:string)=>{
   registered.push({key,workspaceRoot:root,profileIds:[]});
   return Promise.resolve();
  });
  const chooseLocalDirectory=vi.fn().mockResolvedValue(localRoot);
  const listSandboxes=vi.fn();
  const client={
   listAgentWorkspaces:vi.fn().mockImplementation(()=>Promise.resolve({workspaces:[...registered]})),
   listProfiles:vi.fn().mockResolvedValue({profiles:[]}),
   listSandboxes, bindAgentWorkspace,
  } as unknown as MenderClient;
  render(<MemoryRouter><AgentWorkspaceSettings client={client} agent={agent}
    chooseLocalDirectory={chooseLocalDirectory}/></MemoryRouter>);
  expect(await screen.findByText("No additional workspaces")).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"Add workspace"}));
  expect(screen.queryByText("Local Sandbox")).toBeNull();
  fireEvent.change(screen.getByRole("textbox",{name:"Workspace key"}),{target:{value:"GAR"}});
  expect(screen.getByRole<HTMLInputElement>("textbox",{name:"Workspace key"}).value).toBe("gar");
  fireEvent.click(screen.getByRole("button",{name:"Choose local directory"}));
  await waitFor(()=>expect(screen.getByRole<HTMLInputElement>("textbox",{name:"Workspace directory"}).value).toBe(localRoot));
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Confirm"}).disabled).toBe(false);
  fireEvent.click(screen.getByRole("button",{name:"Confirm"}));
  await waitFor(()=>expect(bindAgentWorkspace).toHaveBeenCalledWith(agent.agentId,"gar",localRoot));
  expect(await screen.findByText("gar")).toBeTruthy();
  expect(screen.getByText(localRoot)).toBeTruthy();
  expect(listSandboxes).not.toHaveBeenCalled();
 });
 it("explains invalid alias and relative cwd instead of silently disabling Confirm",()=>{
  const bindAgentWorkspace=vi.fn();
  const client={
   listAgentWorkspaces:vi.fn().mockResolvedValue({workspaces:[]}),
   listProfiles:vi.fn().mockResolvedValue({profiles:[]}),bindAgentWorkspace,
  } as unknown as MenderClient;
  render(<MemoryRouter><AgentWorkspaceSettings client={client} agent={agent}/></MemoryRouter>);
  fireEvent.click(screen.getByRole("button",{name:"Add workspace"}));
  fireEvent.change(screen.getByRole("textbox",{name:"Workspace key"}),{target:{value:"GAR/../"}});
  expect(screen.getByRole("alert").textContent).toMatch(/valid workspace alias/);
  fireEvent.change(screen.getByRole("textbox",{name:"Workspace directory"}),{target:{value:"relative/subdir"}});
  expect(screen.getAllByRole("alert").some(n=>n.textContent?.includes("absolute local directory"))).toBe(true);
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Confirm"}).disabled).toBe(true);
  fireEvent.change(screen.getByRole("textbox",{name:"Workspace key"}),{target:{value:"GAR"}});
  fireEvent.change(screen.getByRole("textbox",{name:"Workspace directory"}),{target:{value:localRoot}});
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Confirm"}).disabled).toBe(false);
  expect(bindAgentWorkspace).not.toHaveBeenCalled();
 });
 it("does not grant the Profile until owner explicitly approves an added workspace",async()=>{
  const grantAgentWorkspace=vi.fn().mockResolvedValue(undefined);
  const client={
   listAgentWorkspaces:vi.fn().mockResolvedValue({workspaces:[{key:"gar",workspaceRoot:localRoot,profileIds:[]}]}),
   listProfiles:vi.fn().mockResolvedValue({profiles:[{profileId,displayName:"Default"}]}),
   grantAgentWorkspace,
  } as unknown as MenderClient;
  render(<MemoryRouter><AgentWorkspaceSettings client={client} agent={agent}/></MemoryRouter>);
  expect(await screen.findByText(localRoot)).toBeTruthy();
  expect(screen.getByText("No Profile workspace access")).toBeTruthy();
  expect(grantAgentWorkspace).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button",{name:"Grant Profile"}));
  expect(screen.getByRole("combobox",{name:"Profile"})).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"Confirm"}));
  await waitFor(()=>expect(grantAgentWorkspace).toHaveBeenCalledWith(agent.agentId,"gar",profileId));
 });
});
