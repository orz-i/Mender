import {cleanup,fireEvent,render,screen,waitFor} from "@testing-library/react";
import {MenderApiError} from "@mender/sdk";
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
 it("shows backend rejection inside the editing dialog instead of failing invisibly",async()=>{
  const bindAgentWorkspace=vi.fn().mockRejectedValue(new Error("local directory unavailable"));
  const client={
   listAgentWorkspaces:vi.fn().mockResolvedValue({workspaces:[]}),
   listProfiles:vi.fn().mockResolvedValue({profiles:[]}),bindAgentWorkspace,
  } as unknown as MenderClient;
  render(<MemoryRouter><AgentWorkspaceSettings client={client} agent={agent}/></MemoryRouter>);
  expect(await screen.findByText("No additional workspaces")).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"Add workspace"}));
  fireEvent.change(screen.getByRole("textbox",{name:"Workspace key"}),{target:{value:"GAR"}});
  fireEvent.change(screen.getByRole("textbox",{name:"Workspace directory"}),{target:{value:localRoot}});
  fireEvent.click(screen.getByRole("button",{name:"Confirm"}));
  await waitFor(()=>expect(screen.getByRole("dialog").textContent).toContain("local directory unavailable"));
  expect(screen.getByRole("dialog")).toBeTruthy();
  expect(bindAgentWorkspace).toHaveBeenCalledTimes(1);
 });

 it("explains the duplicate default directory in Chinese before an owner can submit",async()=>{
  setLocale("zh-CN");
  const bindAgentWorkspace=vi.fn();
  const client={
   listAgentWorkspaces:vi.fn().mockResolvedValue({workspaces:[]}),
   listProfiles:vi.fn().mockResolvedValue({profiles:[]}),bindAgentWorkspace,
  } as unknown as MenderClient;
  render(<MemoryRouter><AgentWorkspaceSettings client={client} agent={agent}/></MemoryRouter>);
  expect(await screen.findByText("没有附加工作区")).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"添加工作区"}));
  fireEvent.change(screen.getByRole("textbox",{name:"工作区别名"}),{target:{value:"mender"}});
  fireEvent.change(screen.getByRole("textbox",{name:"工作目录"}),{target:{value:agent.workspaceRoot+"/"}});
  expect(screen.getByRole("alert").textContent).toContain("此目录已经是智能体的默认工作区");
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"确认"}).disabled).toBe(true);
  expect(bindAgentWorkspace).not.toHaveBeenCalled();
 });
 it("separately identifies an existing alias and an existing nondefault directory",async()=>{
  const bindAgentWorkspace=vi.fn();
  const client={
   listAgentWorkspaces:vi.fn().mockResolvedValue({workspaces:[{key:"gaoge",workspaceRoot:localRoot,profileIds:[]}]}),
   listProfiles:vi.fn().mockResolvedValue({profiles:[]}),bindAgentWorkspace,
  } as unknown as MenderClient;
  render(<MemoryRouter><AgentWorkspaceSettings client={client} agent={agent}/></MemoryRouter>);
  expect(await screen.findByText(localRoot)).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"Add workspace"}));
  fireEvent.change(screen.getByRole("textbox",{name:"Workspace key"}),{target:{value:"GAOGE"}});
  fireEvent.change(screen.getByRole("textbox",{name:"Workspace directory"}),{target:{value:"/tmp/new-directory"}});
  expect(screen.getByRole("alert").textContent).toContain("workspace alias already exists");
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Confirm"}).disabled).toBe(true);
  fireEvent.change(screen.getByRole("textbox",{name:"Workspace key"}),{target:{value:"another"}});
  fireEvent.change(screen.getByRole("textbox",{name:"Workspace directory"}),{target:{value:localRoot+"/"}});
  expect(screen.getByRole("alert").textContent).toContain("already registered as workspace gaoge");
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Confirm"}).disabled).toBe(true);
  expect(bindAgentWorkspace).not.toHaveBeenCalled();
 });
 it("maps stale Wails duplicate conflicts to Chinese instead of Unexpected error",async()=>{
  setLocale("zh-CN");
  const bindAgentWorkspace=vi.fn().mockRejectedValue(new Error("agent resource conflict: agent_workspace_default_directory"));
  const client={
   listAgentWorkspaces:vi.fn().mockResolvedValue({workspaces:[]}),
   listProfiles:vi.fn().mockResolvedValue({profiles:[]}),bindAgentWorkspace,
  } as unknown as MenderClient;
  render(<MemoryRouter><AgentWorkspaceSettings client={client} agent={agent}/></MemoryRouter>);
  expect(await screen.findByText("没有附加工作区")).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"添加工作区"}));
  fireEvent.change(screen.getByRole("textbox",{name:"工作区别名"}),{target:{value:"different"}});
  fireEvent.change(screen.getByRole("textbox",{name:"工作目录"}),{target:{value:"/tmp/linked-to-default"}});
  fireEvent.click(screen.getByRole("button",{name:"确认"}));
  await waitFor(()=>expect(screen.getByRole("dialog").textContent).toContain("此目录已经是智能体的默认工作区"));
  expect(screen.getByRole("dialog").textContent).not.toContain("发生意外错误");
  expect(bindAgentWorkspace).toHaveBeenCalledTimes(1);
 });
 it("maps stale HTTP directory conflicts without mistaking unrelated backend errors for duplicates",async()=>{
  setLocale("zh-CN");
  const bindAgentWorkspace=vi.fn().mockRejectedValue(
   new MenderApiError(409,"agent_workspace_directory_exists","workspace duplicate"));
  const client={
   listAgentWorkspaces:vi.fn().mockResolvedValue({workspaces:[]}),
   listProfiles:vi.fn().mockResolvedValue({profiles:[]}),bindAgentWorkspace,
  } as unknown as MenderClient;
  render(<MemoryRouter><AgentWorkspaceSettings client={client} agent={agent}/></MemoryRouter>);
  expect(await screen.findByText("没有附加工作区")).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"添加工作区"}));
  fireEvent.change(screen.getByRole("textbox",{name:"工作区别名"}),{target:{value:"another"}});
  fireEvent.change(screen.getByRole("textbox",{name:"工作目录"}),{target:{value:"/tmp/linked-existing-directory"}});
  fireEvent.click(screen.getByRole("button",{name:"确认"}));
  await waitFor(()=>expect(screen.getByRole("dialog").textContent).toContain("此目录已登记到当前智能体"));
 });

});
