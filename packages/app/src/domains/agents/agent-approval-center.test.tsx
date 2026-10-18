import {cleanup,fireEvent,render,screen,waitFor,within} from "@testing-library/react";
import type {Agent,AgentApproval,AgentApprovalPolicy,MenderClient,PendingAgentApproval} from "@mender/sdk";
import {afterEach,beforeEach,describe,expect,it,vi} from "vitest";
import {resetLocaleForTesting,setLocale} from "../../i18n/i18n";
import {AgentApprovalCenter} from "./agent-approval-center";

const agent:Agent={
  agentId:"agent_0123456789abcdef0123456789abcdef",name:"Review Agent",
  backend:"opencode",pluginId:"io.mender.agent.opencode",extensionId:"main",
  workspaceRoot:"/private/tmp/mender-agent-review",isolation:"host",backendAvailable:true,
  createdAt:"2026-10-17T00:00:00Z",
};
const approval:AgentApproval={
  approvalId:"aapr_0123456789abcdef0123456789abcdef",
  taskId:"atask_0123456789abcdef0123456789abcdef",agentId:agent.agentId,
  kind:"read",target:"safe.txt",canAllow:true,status:"pending",
  createdAt:"2026-10-17T00:00:00Z",expiresAt:"2099-01-01T00:00:00Z",
};
const request:PendingAgentApproval={approval,agentName:agent.name,workspaceKey:""};
const defaultPolicy:AgentApprovalPolicy={agentId:agent.agentId,workspaceKey:"",mode:"manual"};
function makeClient(overrides:Record<string,unknown>={}):MenderClient {
 return {
   supportsLocalAgentApprovals:true,
   listPendingAgentApprovals:vi.fn().mockResolvedValue({approvals:[request]}),
   listAgents:vi.fn().mockResolvedValue({agents:[agent]}),
   getAgent:vi.fn().mockResolvedValue(agent),
   listAgentWorkspaces:vi.fn().mockResolvedValue({workspaces:[]}),
   listAgentApprovalPolicies:vi.fn().mockResolvedValue({policies:[]}),
   getAgentApprovalPolicy:vi.fn().mockResolvedValue(defaultPolicy),
   setAgentApprovalPolicy:vi.fn().mockResolvedValue(defaultPolicy),
   releasePendingAgentApprovals:vi.fn().mockResolvedValue({released:1}),
   decideAgentApproval:vi.fn().mockResolvedValue({...approval,status:"allowed_once"}),
   ...overrides,
 } as unknown as MenderClient;
}
beforeEach(()=>{resetLocaleForTesting();setLocale("en");});
afterEach(()=>{cleanup();resetLocaleForTesting();});

describe("owner Agent approval center",()=>{
 it("shows cross-Agent pending requests and resolves one from the inbox, without task navigation",async()=>{
  const decision=vi.fn().mockResolvedValue({...approval,status:"allowed_once"});
  const client=makeClient({decideAgentApproval:decision});
  render(<AgentApprovalCenter client={client}/>);
  expect(await screen.findByText("Review Agent")).toBeTruthy();
  expect(screen.getByText("safe.txt")).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"Allow once"}));
  await waitFor(()=>expect(decision).toHaveBeenCalledWith(approval.approvalId,"allow_once"));
 });
 it("keeps an unreviewable action reject-only and can open the scoped authorization dialog",async()=>{
  const blocked:PendingAgentApproval={...request,approval:{...approval,kind:"execute",canAllow:false,target:""}};
  const client=makeClient({listPendingAgentApprovals:vi.fn().mockResolvedValue({approvals:[blocked]})});
  render(<AgentApprovalCenter client={client}/>);
  expect(await screen.findByText("Review Agent")).toBeTruthy();
  expect(screen.queryByRole("button",{name:"Allow once"})).toBeNull();
  expect(screen.getByRole("button",{name:"Reject"})).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"Set authorization mode"}));
  const dialog=await screen.findByRole("dialog");
  expect(within(dialog).getByText("Ask every time")).toBeTruthy();
  expect(within(dialog).getByText("Allow workspace reads")).toBeTruthy();
  expect(within(dialog).getByText("Trust native Agent actions (24 hours)")).toBeTruthy();
  expect(within(dialog).queryByText(/trusted can edit outside/i)).toBeNull();
 });
 it("sets read-only unattended mode and releases already pending matching requests",async()=>{
  const set=vi.fn().mockResolvedValue({...defaultPolicy,mode:"workspace_read"});
  const release=vi.fn().mockResolvedValue({released:1});
  const client=makeClient({setAgentApprovalPolicy:set,releasePendingAgentApprovals:release});
  render(<AgentApprovalCenter client={client}/>);
  expect(await screen.findByText("Review Agent")).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"Set authorization mode"}));
  const dialog=await screen.findByRole("dialog");
  fireEvent.click(within(dialog).getByRole("radio",{name:"Allow workspace reads"}));
  fireEvent.click(within(dialog).getByRole("button",{name:"Save"}));
  await waitFor(()=>expect(set).toHaveBeenCalledWith(agent.agentId,"","workspace_read"));
  await waitFor(()=>expect(release).toHaveBeenCalledWith(agent.agentId,""));
 });
 it("keeps additional workspace authorization separate from the default root",async()=>{
  const set=vi.fn().mockResolvedValue({...defaultPolicy,workspaceKey:"secondary",mode:"workspace_read"});
  const client=makeClient({
   listAgentWorkspaces:vi.fn().mockResolvedValue({workspaces:[{
    key:"secondary",workspaceRoot:"/private/tmp/mender-agent-review/secondary",profileIds:[],
   }]}),
   listAgentApprovalPolicies:vi.fn().mockResolvedValue({
    policies:[{...defaultPolicy,mode:"workspace_read"}],
   }),
   setAgentApprovalPolicy:set,
  });
  render(<AgentApprovalCenter client={client}/>);
  expect(await screen.findByText("Review Agent")).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"Set authorization mode"}));
  const dialog=await screen.findByRole("dialog");
  expect(within(dialog).getByRole<HTMLInputElement>("radio",{name:"Allow workspace reads"}).checked).toBe(true);
  fireEvent.change(within(dialog).getByRole("combobox",{name:"Workspace"}),{
   target:{value:"secondary"},
  });
  expect(within(dialog).getByText("/private/tmp/mender-agent-review/secondary")).toBeTruthy();
  expect(within(dialog).getByRole<HTMLInputElement>("radio",{name:"Ask every time"}).checked).toBe(true);
  fireEvent.click(within(dialog).getByRole("radio",{name:"Allow workspace reads"}));
  fireEvent.click(within(dialog).getByRole("button",{name:"Save"}));
  await waitFor(()=>expect(set).toHaveBeenCalledWith(agent.agentId,"secondary","workspace_read"));
 });
 it("requires a separate risk acknowledgement for 24-hour native Host trust",async()=>{
  const set=vi.fn().mockResolvedValue({...defaultPolicy,mode:"trusted",expiresAt:"2099-01-01T00:00:00Z"});
  const client=makeClient({setAgentApprovalPolicy:set});
  render(<AgentApprovalCenter client={client}/>);
  expect(await screen.findByText("Review Agent")).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"Set authorization mode"}));
  const dialog=await screen.findByRole("dialog");
  fireEvent.click(within(dialog).getByRole("radio",{name:"Trust native Agent actions (24 hours)"}));
  expect(within(dialog).getByText(/NOT sandboxed by Mender/)).toBeTruthy();
  const save=within(dialog).getByRole<HTMLButtonElement>("button",{name:"Save"});
  expect(save.disabled).toBe(true);
  fireEvent.click(within(dialog).getByRole("checkbox",{name:/I understand the Host-mode risk/}));
  expect(save.disabled).toBe(false);
  fireEvent.click(save);
  await waitFor(()=>expect(set).toHaveBeenCalledWith(agent.agentId,"","trusted"));
 });
 it("offers Antigravity's 24-hour YOLO instead of claiming ACP approvals",async()=>{
  const agy:Agent={...agent,name:"Antigravity Local",backend:"antigravity",pluginId:"io.mender.agent.antigravity"};
  const set=vi.fn().mockResolvedValue({...defaultPolicy,mode:"trusted"});
  const client=makeClient({
   listPendingAgentApprovals:vi.fn().mockResolvedValue({approvals:[]}),
   listAgents:vi.fn().mockResolvedValue({agents:[agy]}),
   getAgent:vi.fn().mockResolvedValue(agy),setAgentApprovalPolicy:set,
  });
  render(<AgentApprovalCenter client={client}/>);
  fireEvent.click(screen.getByRole("button",{name:"Unattended authorization"}));
  expect(await screen.findByText("Antigravity Local")).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"Configure authorization"}));
  const dialog=await screen.findByRole("dialog");
  expect(within(dialog).getByText(/trusted mode uses the CLI YOLO flag/)).toBeTruthy();
  expect(within(dialog).queryByRole("radio",{name:"Allow workspace reads"})).toBeNull();
  expect(within(dialog).getByRole("radio",{name:"Follow Antigravity native permissions (default)"})).toBeTruthy();
  fireEvent.click(within(dialog).getByRole("radio",{name:"YOLO: auto-approve all Antigravity tools (24 hours)"}));
  expect(within(dialog).getByText(/--dangerously-skip-permissions/)).toBeTruthy();
  const save=within(dialog).getByRole<HTMLButtonElement>("button",{name:"Save"});
  expect(save.disabled).toBe(true);
  fireEvent.click(within(dialog).getByRole("checkbox",{name:/I understand the Host-mode risk/}));
  fireEvent.click(save);
  await waitFor(()=>expect(set).toHaveBeenCalledWith(agy.agentId,"","trusted"));
 });
 it("restricts the inbox to the native owner",()=>{
  render(<AgentApprovalCenter client={makeClient({supportsLocalAgentApprovals:false})}/>);
  expect(screen.getByText("Local approval access required")).toBeTruthy();
  expect(screen.queryByText("Pending Agent approvals")).toBeNull();
 });
});