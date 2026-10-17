import {cleanup,fireEvent,render,screen,waitFor} from "@testing-library/react";
import type {AgentApproval,AgentTask,MenderClient} from "@mender/sdk";
import {afterEach,beforeEach,describe,expect,it,vi} from "vitest";
import {resetLocaleForTesting,setLocale} from "../../i18n/i18n";
import {AgentTaskApprovals} from "./agent-approvals";

const task:AgentTask={
 taskId:"atask_00000000000000000000000000000123",
 agentId:"agent_00000000000000000000000000000456",
 status:"running",createdAt:"2026-10-17T09:00:00Z",
 updatedAt:"2026-10-17T09:00:00Z",cancelRequested:false,
};
const approval:AgentApproval={
 approvalId:"aapr_00000000000000000000000000000789",
 taskId:task.taskId,agentId:task.agentId,
 kind:"read",target:"fixture.txt",canAllow:true,status:"pending",
 createdAt:"2026-10-17T09:00:00Z",expiresAt:"2099-01-01T00:00:00Z",
};
beforeEach(()=>{resetLocaleForTesting();setLocale("en");});
afterEach(()=>{cleanup();resetLocaleForTesting();});

describe("ACP task approval owner view",()=>{
 it("shows a focused single request without revealing raw commands or a permanent allow",async()=>{
  const decide=vi.fn().mockResolvedValue({...approval,status:"allowed_once"});
  const client={
   listAgentApprovals:vi.fn().mockResolvedValue({approvals:[approval]}),
   getAgentTask:vi.fn().mockResolvedValue(task),
   decideAgentApproval:decide,
  } as unknown as MenderClient;
  render(<AgentTaskApprovals client={client} task={task}/>);
  expect(await screen.findByText(/Awaiting owner approval/)).toBeTruthy();
  expect(screen.getByText("Read")).toBeTruthy();
  expect(screen.getByText("fixture.txt")).toBeTruthy();
  expect(screen.getByText(/cooperative OpenCode permission request/)).toBeTruthy();
  expect(screen.getByRole("button",{name:"Allow once"})).toBeTruthy();
  expect(screen.getByRole("button",{name:"Reject"})).toBeTruthy();
  expect(screen.queryByText(/always/i)).toBeNull();
  fireEvent.click(screen.getByRole("button",{name:"Allow once"}));
  await waitFor(()=>expect(decide).toHaveBeenCalledWith(approval.approvalId,"allow_once"));
 });
 it("can explicitly reject without granting any Agent capability",async()=>{
  const decide=vi.fn().mockResolvedValue({...approval,status:"rejected"});
  const client={
   listAgentApprovals:vi.fn().mockResolvedValue({approvals:[approval]}),
   getAgentTask:vi.fn().mockResolvedValue(task),
   decideAgentApproval:decide,
  } as unknown as MenderClient;
  render(<AgentTaskApprovals client={client} task={task}/>);
  fireEvent.click(await screen.findByRole("button",{name:"Reject"}));
  await waitFor(()=>expect(decide).toHaveBeenCalledWith(approval.approvalId,"reject"));
 });
 it("does not show approval controls for resolved or missing permission requests",async()=>{
  const list=vi.fn().mockResolvedValue({approvals:[{...approval,status:"expired"}]});
  const decide=vi.fn();
  const client={
   listAgentApprovals:list,
   getAgentTask:vi.fn().mockResolvedValue({...task,status:"completed"}),
   decideAgentApproval:decide,
  } as unknown as MenderClient;
  render(<AgentTaskApprovals client={client} task={{...task,status:"completed"}}/>);
  await waitFor(()=>expect(list).toHaveBeenCalledWith(task.taskId));
  expect(screen.queryByRole("button",{name:"Allow once"})).toBeNull();
  expect(decide).not.toHaveBeenCalled();
 });
 it("hides owner Allow once for unreviewable native edits and Shell requests",async()=>{
  const blocked={...approval,kind:"execute" as const,canAllow:false,target:undefined};
  const decide=vi.fn().mockResolvedValue({...blocked,status:"rejected"});
  const client={
    listAgentApprovals:vi.fn().mockResolvedValue({approvals:[blocked]}),
    getAgentTask:vi.fn().mockResolvedValue(task),
    decideAgentApproval:decide,
  } as unknown as MenderClient;
  render(<AgentTaskApprovals client={client} task={task}/>);
  expect(await screen.findByText(/Awaiting owner approval/)).toBeTruthy();
  expect(screen.getByText(/lacks a reviewable in-workspace/)).toBeTruthy();
  expect(screen.queryByRole("button",{name:"Allow once"})).toBeNull();
  fireEvent.click(screen.getByRole("button",{name:"Reject"}));
  await waitFor(()=>expect(decide).toHaveBeenCalledWith(approval.approvalId,"reject"));
 });

});
