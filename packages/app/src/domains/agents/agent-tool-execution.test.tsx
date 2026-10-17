import {cleanup,fireEvent,render,screen,waitFor,within} from "@testing-library/react";
import type {Agent,AgentToolPlacement,MenderClient,Sandbox,SandboxToolSuite} from "@mender/sdk";
import {afterEach,beforeEach,describe,expect,it,vi} from "vitest";
import {resetLocaleForTesting,setLocale} from "../../i18n/i18n";
import {AgentToolExecutionSettings} from "./agent-tool-execution";

const root="/Users/mini/Coding/Gaoge-Agent-Runtime";
const agent:Agent={
  agentId:"agent_0123456789abcdef0123456789abcdef",name:"Reusable Grok",
  backend:"grok",pluginId:"io.mender.agent.grok",extensionId:"main",
  workspaceRoot:root,isolation:"restricted_host",
  backendAvailable:true,createdAt:"2026-10-17T00:00:00Z",
};
const sandboxes:Sandbox[]=[
 {sandboxId:"sbx_0123456789abcdef0123456789abcdef",name:"Approved local root",
  backend:"local_workspace",isolation:"restricted_host",state:"ready",localRoot:root,
  ownerKind:"local_user",ownerRef:"os:501",activeLeases:0,backendAvailable:true},
 {sandboxId:"sbx_1123456789abcdef0123456789abcdef",name:"Other location",
  backend:"local_workspace",isolation:"restricted_host",state:"ready",localRoot:"/tmp/elsewhere",
  ownerKind:"local_user",ownerRef:"os:501",activeLeases:0,backendAvailable:true},
 {sandboxId:"sbx_2123456789abcdef0123456789abcdef",name:"Host sandbox",
  backend:"local_workspace",isolation:"host",state:"ready",localRoot:root,
  ownerKind:"local_user",ownerRef:"os:501",activeLeases:0,backendAvailable:true},
];
const toolRead="tool_0123456789abcdef0123456789abcdef";
const toolEdit="tool_1123456789abcdef0123456789abcdef";
const toolShell="tool_2123456789abcdef0123456789abcdef";
const suite:SandboxToolSuite={
 suiteId:"sandbox.workspace",sandboxId:sandboxes[0]!.sandboxId,status:"ready",available:true,
 tools:[
  {toolId:toolRead,sourceKey:"workspace.read",name:"read",title:"Read"},
  {toolId:toolEdit,sourceKey:"workspace.edit",name:"edit",title:"Edit"},
  {toolId:toolShell,sourceKey:"workspace.shell",name:"shell",title:"Shell"},
 ],
};
const base={
 listAgentWorkspaces:vi.fn().mockResolvedValue({workspaces:[]}),
 listAgentToolPlacements:vi.fn().mockResolvedValue({placements:[]}),
 listSandboxes:vi.fn().mockResolvedValue({sandboxes}),
 getSandboxTools:vi.fn().mockResolvedValue(suite),
 configureAgentToolPlacement:vi.fn().mockResolvedValue(undefined),
 disableAgentToolPlacement:vi.fn().mockResolvedValue(undefined),
};
beforeEach(()=>{resetLocaleForTesting();setLocale("en");});
afterEach(()=>{cleanup();resetLocaleForTesting();});

describe("optional Agent governed tool execution",()=>{
 it("lists only matching ready restricted Sandbox and eligible existing Workspace Tools",async()=>{
  const listSandboxes=vi.fn().mockResolvedValue({sandboxes});
  const configure=vi.fn().mockResolvedValue({workspaceKey:"",sandboxId:sandboxes[0]!.sandboxId,toolIds:[toolRead]});
  const ensure=vi.fn();
  const client={...base,listSandboxes,configureAgentToolPlacement:configure,ensureSandboxTools:ensure} as unknown as MenderClient;
  render(<AgentToolExecutionSettings client={client} agent={agent}/>);
  expect(await screen.findByText(root)).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"Configure"}));
  const dialog=screen.getByRole("dialog");
  expect(await within(dialog).findByRole("option",{name:"Approved local root"})).toBeTruthy();
  expect(within(dialog).queryByRole("option",{name:"Other location"})).toBeNull();
  expect(within(dialog).queryByRole("option",{name:"Host sandbox"})).toBeNull();
  fireEvent.change(within(dialog).getByRole("combobox",{name:"Execution environment"}),{
   target:{value:sandboxes[0]!.sandboxId},
  });
  expect(await within(dialog).findByRole("checkbox",{name:"Read"})).toBeTruthy();
  expect(within(dialog).getByRole("checkbox",{name:"Edit"})).toBeTruthy();
  expect(within(dialog).queryByRole("checkbox",{name:"Shell"})).toBeNull();
  fireEvent.click(within(dialog).getByRole("checkbox",{name:"Read"}));
  fireEvent.click(within(dialog).getByRole("button",{name:"Save"}));
  await waitFor(()=>expect(configure).toHaveBeenCalledWith(agent.agentId,"",sandboxes[0]!.sandboxId,[toolRead]));
  expect(ensure).not.toHaveBeenCalled();
 });
 it("blocks configuration of Host Agent without mutating its existing workspace",async()=>{
  const list=vi.fn();const save=vi.fn();
  const client={...base,listSandboxes:list,configureAgentToolPlacement:save} as unknown as MenderClient;
  render(<AgentToolExecutionSettings client={client} agent={{...agent,isolation:"host"}}/>);
  expect(await screen.findByText(root)).toBeTruthy();
  expect(screen.getByRole("alert").textContent).toContain("cannot enable governed execution");
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Configure"}).disabled).toBe(true);
  expect(list).not.toHaveBeenCalled();
  expect(save).not.toHaveBeenCalled();
 });
 it("shows a safe empty candidate state without provisioning Sandboxes or tools",async()=>{
  const ensure=vi.fn();const save=vi.fn();
  const client={...base,listSandboxes:vi.fn().mockResolvedValue({sandboxes:sandboxes.slice(1)}),
   ensureSandboxTools:ensure,configureAgentToolPlacement:save} as unknown as MenderClient;
  render(<AgentToolExecutionSettings client={client} agent={agent}/>);
  expect(await screen.findByText(root)).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"Configure"}));
  const dialog=screen.getByRole("dialog");
  expect(await within(dialog).findByText(/No matching ready restricted Sandbox/)).toBeTruthy();
  expect(within(dialog).getByRole<HTMLButtonElement>("button",{name:"Save"}).disabled).toBe(true);
  expect(within(dialog).queryByRole("option",{name:"Host sandbox"})).toBeNull();
  expect(ensure).not.toHaveBeenCalled();
  expect(save).not.toHaveBeenCalled();
 });
 it("keeps owner editor open and reports configuration errors without clearing selection",async()=>{
  const configure=vi.fn().mockRejectedValue(new Error("Placement is no longer eligible"));
  const client={...base,configureAgentToolPlacement:configure} as unknown as MenderClient;
  render(<AgentToolExecutionSettings client={client} agent={agent}/>);
  expect(await screen.findByText(root)).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"Configure"}));
  const dialog=screen.getByRole("dialog");
  expect(await within(dialog).findByRole("option",{name:"Approved local root"})).toBeTruthy();
  fireEvent.change(within(dialog).getByRole("combobox",{name:"Execution environment"}),{
   target:{value:sandboxes[0]!.sandboxId},
  });
  fireEvent.click(await within(dialog).findByRole("checkbox",{name:"Read"}));
  fireEvent.click(within(dialog).getByRole("button",{name:"Save"}));
  expect(await within(dialog).findByRole("alert")).toBeTruthy();
  expect(within(dialog).getByRole("alert").textContent).toContain("Placement is no longer eligible");
  expect(within(dialog).getByRole<HTMLInputElement>("checkbox",{name:"Read"}).checked).toBe(true);
  expect(configure).toHaveBeenCalledOnce();
 });
 it("gives a recoverable Chinese error for stale unstructured Wails save rejection",async()=>{
  setLocale("zh-CN");
  const configure=vi.fn().mockRejectedValue("agent backend unavailable");
  const client={...base,configureAgentToolPlacement:configure} as unknown as MenderClient;
  render(<AgentToolExecutionSettings client={client} agent={agent}/>);
  expect(await screen.findByText(root)).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"配置"}));
  const dialog=screen.getByRole("dialog");
  expect(await within(dialog).findByRole("option",{name:"Approved local root"})).toBeTruthy();
  fireEvent.change(within(dialog).getByRole("combobox",{name:"执行环境"}),{
   target:{value:sandboxes[0]!.sandboxId},
  });
  fireEvent.click(await within(dialog).findByRole("checkbox",{name:"读取"}));
  fireEvent.click(within(dialog).getByRole("button",{name:"保存"}));
  expect(await within(dialog).findByText(/无法保存受治理工具配置/)).toBeTruthy();
  expect(within(dialog).getByRole<HTMLInputElement>("checkbox",{name:"读取"}).checked).toBe(true);
  expect(configure).toHaveBeenCalledOnce();
 });
 it("gives a recoverable Chinese error for an uncertain Wails disable result",async()=>{
  setLocale("zh-CN");
  const placement:AgentToolPlacement={
   agentId:agent.agentId,workspaceKey:"",workspaceRoot:root,
   sandboxId:sandboxes[0]!.sandboxId,toolIds:[toolRead],updatedAt:"2026-10-17T00:00:00Z",
  };
  const disable=vi.fn().mockRejectedValue(new Error("agent backend unavailable"));
  const client={...base,listAgentToolPlacements:vi.fn().mockResolvedValue({placements:[placement]}),
   disableAgentToolPlacement:disable} as unknown as MenderClient;
  render(<AgentToolExecutionSettings client={client} agent={agent}/>);
  expect(await screen.findByText("已配置")).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"编辑配置"}));
  const dialog=screen.getByRole("dialog");
  fireEvent.click(within(dialog).getByRole("button",{name:"关闭受治理工具"}));
  expect(await within(dialog).findByText(/无法确认受治理工具是否已关闭/)).toBeTruthy();
  expect(disable).toHaveBeenCalledWith(agent.agentId,"");
 });
 it("explicitly disables an existing placement instead of deleting a workspace",async()=>{
  const placement:AgentToolPlacement={
   agentId:agent.agentId,workspaceKey:"",workspaceRoot:root,
   sandboxId:sandboxes[0]!.sandboxId,toolIds:[toolRead],
   updatedAt:"2026-10-17T00:00:00Z",
  };
  const disable=vi.fn().mockResolvedValue(undefined);
  const unbind=vi.fn();
  const client={...base,listAgentToolPlacements:vi.fn().mockResolvedValue({placements:[placement]}),
   disableAgentToolPlacement:disable,unbindAgentWorkspace:unbind} as unknown as MenderClient;
  render(<AgentToolExecutionSettings client={client} agent={agent}/>);
  expect(await screen.findByText("Configured")).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"Edit configuration"}));
  fireEvent.click(within(screen.getByRole("dialog")).getByRole("button",{name:"Disable governed tools"}));
  await waitFor(()=>expect(disable).toHaveBeenCalledWith(agent.agentId,""));
  expect(unbind).not.toHaveBeenCalled();
 });
});
