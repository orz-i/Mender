import {cleanup,fireEvent,render,screen,waitFor} from "@testing-library/react";
import type {AgentTask,MenderClient} from "@mender/sdk";
import {afterEach,beforeEach,describe,expect,it,vi} from "vitest";
import {resetLocaleForTesting,setLocale} from "../../i18n/i18n";
import {AgentTaskLifecycleActions} from "./agent-task-lifecycle-actions";

const unknown:AgentTask={
  taskId:"atask_"+"1".repeat(32),agentId:"agent_"+"2".repeat(32),
  status:"unknown",diagnostic:"Agent outcome unknown after timeout",
  createdAt:"2026-10-18T10:00:00Z",updatedAt:"2026-10-18T10:10:00Z",
  cancelRequested:false,resumeAvailability:{eligible:false,reason:"unknown_outcome"},
};
const completed:AgentTask={
  ...unknown,status:"completed",diagnostic:"",
  result:"previous work",resumeAvailability:{eligible:true,reason:"candidate"},
};
beforeEach(()=>{resetLocaleForTesting();setLocale("en");});
afterEach(()=>{cleanup();resetLocaleForTesting();});

describe("Agent task owner lifecycle controls",()=>{
  it("never provides Resume for an unknown original, even after externally handled acknowledgement",async()=>{
    const updated={...unknown,disposition:{handling:"handled_elsewhere" as const,recordedAt:"2026-10-18T13:00:00Z"}};
    const mark=vi.fn().mockResolvedValue(updated);
    const resume=vi.fn();
    const onUpdated=vi.fn();
    const client={reviewUnknownAgentTask:mark,resumeAgentTask:resume} as unknown as MenderClient;
    const {rerender}=render(<AgentTaskLifecycleActions client={client} task={unknown}
      onTaskUpdated={onUpdated} onTaskCreated={vi.fn()}/>);
    expect(screen.getByText("Unknown task: do not repeat execution")).toBeTruthy();
    expect(screen.queryByRole("button",{name:"Compose new instruction"})).toBeNull();
    expect(screen.queryByRole("textbox",{name:"New instruction"})).toBeNull();
    expect(mark).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button",{name:"Mark handled elsewhere"}));
    expect(mark).not.toHaveBeenCalled();
    expect(screen.getByText(/This records only your review/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button",{name:"Confirm handling review"}));
    await waitFor(()=>expect(mark).toHaveBeenCalledExactlyOnceWith(unknown.taskId,"handled_elsewhere"));
    expect(onUpdated).toHaveBeenCalledWith(updated);
    rerender(<AgentTaskLifecycleActions client={client} task={updated}
      onTaskUpdated={onUpdated} onTaskCreated={vi.fn()}/>);
    expect(screen.getByText(/Owner reported this work was handled elsewhere/)).toBeTruthy();
    expect(screen.getByRole("button",{name:"Reopen handling review"})).toBeTruthy();
    expect(resume).not.toHaveBeenCalled();
  });

  it("does not let the desktop owner hijack the original Profile caller session",()=>{
    const client={resumeAgentTask:vi.fn()} as unknown as MenderClient;
    render(<AgentTaskLifecycleActions client={client}
      task={{...completed,resumeAvailability:{eligible:false,reason:"original_profile_required"}}}
      onTaskUpdated={vi.fn()} onTaskCreated={vi.fn()}/>);
    expect(screen.getByText(/belongs to its original Profile/)).toBeTruthy();
    expect(screen.queryByRole("button",{name:"Compose new instruction"})).toBeNull();
  });

  it("requires a fresh instruction and explicit review before new completed-session execution",async()=>{
    const resume=vi.fn().mockResolvedValue({...completed,taskId:"atask_"+"3".repeat(32),
      continuedFromTaskId:completed.taskId,status:"queued"});
    const created=vi.fn();
    render(<AgentTaskLifecycleActions client={{resumeAgentTask:resume} as unknown as MenderClient}
      task={completed} onTaskUpdated={vi.fn()} onTaskCreated={created}/>);
    expect(screen.queryByRole("textbox",{name:"New instruction"})).toBeNull();
    expect(resume).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button",{name:"Compose new instruction"}));
    const submit=screen.getByRole<HTMLButtonElement>("button",{name:"Submit new continuation task"});
    expect(submit.disabled).toBe(true);
    fireEvent.change(screen.getByRole("textbox",{name:"New instruction"}),{target:{value:"Check ONLY remaining new work"}});
    expect(submit.disabled).toBe(true);
    fireEvent.click(screen.getByRole("checkbox"));
    expect(submit.disabled).toBe(false);
    fireEvent.click(submit);
    await waitFor(()=>expect(resume).toHaveBeenCalledExactlyOnceWith(completed.taskId,"Check ONLY remaining new work"));
    expect(created).toHaveBeenCalledTimes(1);
  });
});
