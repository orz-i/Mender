// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { AgentTask, AgentTaskEvent } from "@mender/sdk";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { TaskConversationFeed } from "./task-conversation-feed";

const mockTask: AgentTask = {
  taskId: "atask_00000000000000000000000000000999",
  agentId: "agent_00000000000000000000000000000101",
  status: "completed",
  result: "Legacy single message result",
  createdAt: "2026-11-12T10:00:00Z",
  updatedAt: "2026-11-12T10:02:00Z",
  cancelRequested: false,
};

describe("TaskConversationFeed", () => {
  beforeEach(() => {
    resetLocaleForTesting();
    setLocale("en");
  });
  afterEach(() => {
    cleanup();
    resetLocaleForTesting();
  });

  it("falls back to legacy single message view when task has no events", async () => {
    const client = {
      getAgentTaskTranscript: vi.fn().mockResolvedValue({ events: [] }),
    };

    render(<TaskConversationFeed task={mockTask} client={client} />);

    expect(await screen.findByText("Legacy single message result")).toBeTruthy();
    expect(client.getAgentTaskTranscript).toHaveBeenCalledWith(mockTask.taskId);
  });

  it("falls back to diagnostic when result is empty and no events exist", async () => {
    const client = {
      getAgentTaskTranscript: vi.fn().mockResolvedValue({ events: [] }),
    };
    const taskWithDiag: AgentTask = {
      ...mockTask,
      status: "failed",
      result: "",
      diagnostic: "Agent process could not start",
    };

    render(<TaskConversationFeed task={taskWithDiag} client={client} />);

    expect(await screen.findByText("Agent process could not start")).toBeTruthy();
  });

  it("falls back gracefully when transcript API fails", async () => {
    const client = {
      getAgentTaskTranscript: vi.fn().mockRejectedValue(new Error("Network 404")),
    };

    render(<TaskConversationFeed task={mockTask} client={client} />);

    expect(await screen.findByText("Legacy single message result")).toBeTruthy();
  });

  it("renders full conversation feed with user prompt, thinking, tool calls, assistant markdown, and terminal badge", async () => {
    const events: AgentTaskEvent[] = [
      {
        eventId: "evt_1",
        taskId: mockTask.taskId,
        seq: 1,
        type: "user_prompt",
        payload: { prompt: "Please inspect system configuration and files" },
        createdAt: "2026-11-12T10:00:01Z",
      },
      {
        eventId: "evt_2",
        taskId: mockTask.taskId,
        seq: 2,
        type: "thinking",
        payload: { thought: "Let me check the config directory first..." },
        createdAt: "2026-11-12T10:00:02Z",
      },
      {
        eventId: "evt_3",
        taskId: mockTask.taskId,
        seq: 3,
        type: "tool_call",
        payload: {
          toolCallId: "call_read_1",
          name: "mender_read",
          args: { path: "/etc/mender.conf" },
        },
        createdAt: "2026-11-12T10:00:03Z",
      },
      {
        eventId: "evt_4",
        taskId: mockTask.taskId,
        seq: 4,
        type: "tool_result",
        payload: {
          toolCallId: "call_read_1",
          output: "port = 8080\nenabled = true",
        },
        createdAt: "2026-11-12T10:00:04Z",
      },
      {
        eventId: "evt_5",
        taskId: mockTask.taskId,
        seq: 5,
        type: "message_chunk",
        payload: { text: "### Inspection Summary\n\nConfiguration is **valid**:\n```ini\nport = 8080\n```\n- Service ready" },
        createdAt: "2026-11-12T10:00:05Z",
      },
      {
        eventId: "evt_6",
        taskId: mockTask.taskId,
        seq: 6,
        type: "terminal",
        payload: { status: "completed" },
        createdAt: "2026-11-12T10:00:06Z",
      },
    ];

    const client = {
      getAgentTaskTranscript: vi.fn().mockResolvedValue({ events }),
    };

    render(<TaskConversationFeed task={mockTask} client={client} />);

    // User prompt
    expect(await screen.findByText("Please inspect system configuration and files")).toBeTruthy();
    expect(screen.getByText("User prompt")).toBeTruthy();

    // Thinking process
    expect(screen.getByText("Thinking process")).toBeTruthy();
    expect(screen.getByText("Let me check the config directory first...")).toBeTruthy();

    // Tool call and result
    expect(screen.getByText("mender_read")).toBeTruthy();
    expect(screen.getByText("Tool call succeeded")).toBeTruthy();
    expect(screen.getByText("Tool arguments")).toBeTruthy();
    expect(screen.getAllByText(/port = 8080/).length).toBeGreaterThan(0);

    // Assistant response with markdown formatting
    expect(screen.getByText("Assistant")).toBeTruthy();
    expect(screen.getByText("Inspection Summary")).toBeTruthy();
    expect(screen.getByText("valid")).toBeTruthy();
    expect(screen.getByText("Service ready")).toBeTruthy();
    expect(screen.getByText("Copy")).toBeTruthy();

    // Terminal summary badge
    expect(screen.getByText("Task completed")).toBeTruthy();
    expect(screen.getByText("Completed")).toBeTruthy();
  });

  it("renders approval requests in the conversation feed", async () => {
    const events: AgentTaskEvent[] = [
      {
        eventId: "evt_app_1",
        taskId: mockTask.taskId,
        seq: 1,
        type: "approval",
        payload: {
          approvalId: "app_100",
          target: "/workspace/config.json",
          decision: "allow_once",
        },
        createdAt: "2026-11-12T10:00:01Z",
      },
    ];

    const client = {
      getAgentTaskTranscript: vi.fn().mockResolvedValue({ events }),
    };

    render(<TaskConversationFeed task={mockTask} client={client} />);

    expect(await screen.findByText("Approval requested")).toBeTruthy();
    expect(screen.getByText("/workspace/config.json")).toBeTruthy();
    expect(screen.getByText("allow_once")).toBeTruthy();
  });

  it("handles tool call errors properly", async () => {
    const events: AgentTaskEvent[] = [
      {
        eventId: "evt_tool_err_1",
        taskId: mockTask.taskId,
        seq: 1,
        type: "tool_call",
        payload: {
          toolCallId: "call_err_1",
          name: "mender_execute",
          args: { command: "rm -rf /" },
        },
        createdAt: "2026-11-12T10:00:01Z",
      },
      {
        eventId: "evt_tool_err_2",
        taskId: mockTask.taskId,
        seq: 2,
        type: "tool_result",
        payload: {
          toolCallId: "call_err_1",
          error: "Permission denied",
        },
        createdAt: "2026-11-12T10:00:02Z",
      },
    ];

    const client = {
      getAgentTaskTranscript: vi.fn().mockResolvedValue({ events }),
    };

    render(<TaskConversationFeed task={{ ...mockTask, status: "failed" }} client={client} />);

    expect(await screen.findByText("mender_execute")).toBeTruthy();
    expect(screen.getByText("Tool call failed")).toBeTruthy();
    expect(screen.getByText("Permission denied")).toBeTruthy();
    expect(screen.getByText("Task failed")).toBeTruthy();
  });

  it("subscribes to live events when task is running and smoothly merges thinking and message chunks", async () => {
    let capturedOnEvent: ((event: AgentTaskEvent) => void) | undefined;
    const unsubscribe = vi.fn();
    const subscribeMock = vi.fn().mockImplementation((_taskId, onEvent) => {
      capturedOnEvent = onEvent;
      return unsubscribe;
    });

    const initialEvents: AgentTaskEvent[] = [
      {
        eventId: "evt_prompt",
        taskId: mockTask.taskId,
        seq: 1,
        type: "user_prompt",
        payload: { prompt: "Start live streaming task" },
        createdAt: "2026-11-12T10:00:00Z",
      },
    ];

    const client = {
      getAgentTaskTranscript: vi.fn().mockResolvedValue({ events: initialEvents }),
      subscribeAgentTaskEvents: subscribeMock,
    };

    const runningTask: AgentTask = {
      ...mockTask,
      status: "running",
    };

    const { unmount } = render(<TaskConversationFeed task={runningTask} client={client} />);

    expect(await screen.findByText("Start live streaming task")).toBeTruthy();
    expect(subscribeMock).toHaveBeenCalledWith(runningTask.taskId, expect.any(Function), expect.any(Function));
    expect(capturedOnEvent).toBeDefined();

    // Stream two thinking chunks
    capturedOnEvent!({
      eventId: "evt_think_1",
      taskId: runningTask.taskId,
      seq: 2,
      type: "thinking",
      payload: { thought: "Thinking " },
      createdAt: "2026-11-12T10:00:01Z",
    });

    capturedOnEvent!({
      eventId: "evt_think_2",
      taskId: runningTask.taskId,
      seq: 3,
      type: "thinking",
      payload: { thought: "step by step..." },
      createdAt: "2026-11-12T10:00:02Z",
    });

    // Thinking process merges smoothly into one block
    expect(await screen.findByText("Thinking step by step...")).toBeTruthy();

    // Stream message chunks
    capturedOnEvent!({
      eventId: "evt_chunk_1",
      taskId: runningTask.taskId,
      seq: 4,
      type: "message_chunk",
      payload: { text: "Hello " },
      createdAt: "2026-11-12T10:00:03Z",
    });

    capturedOnEvent!({
      eventId: "evt_chunk_2",
      taskId: runningTask.taskId,
      seq: 5,
      type: "message_chunk",
      payload: { text: "world! Real-time streaming works." },
      createdAt: "2026-11-12T10:00:04Z",
    });

    // Assistant message bubble smoothly merges chunks
    expect(await screen.findByText("Hello world! Real-time streaming works.")).toBeTruthy();

    unmount();
    expect(unsubscribe).toHaveBeenCalled();
  });

  it("handles smart auto-scroll and reveals floating button when scrolled up", async () => {
    let capturedOnEvent: ((event: AgentTaskEvent) => void) | undefined;
    const initialEvents: AgentTaskEvent[] = [
      {
        eventId: "evt_prompt_0",
        taskId: mockTask.taskId,
        seq: 1,
        type: "user_prompt",
        payload: { prompt: "Hello" },
        createdAt: "2026-11-12T10:00:00Z",
      },
    ];
    const client = {
      getAgentTaskTranscript: vi.fn().mockResolvedValue({ events: initialEvents }),
      subscribeAgentTaskEvents: vi.fn().mockImplementation((_taskId, onEvent) => {
        capturedOnEvent = onEvent;
        return vi.fn();
      }),
    };

    const runningTask: AgentTask = {
      ...mockTask,
      status: "running",
    };

    const { container } = render(<TaskConversationFeed task={runningTask} client={client} />);

    // Wait for initial prompt to render
    expect(await screen.findByText("Hello")).toBeTruthy();

    const scrollContainer = container.querySelector(".overflow-y-auto") as HTMLDivElement;
    expect(scrollContainer).toBeTruthy();

    // Mock dimensions to simulate user scrolling up away from bottom
    Object.defineProperty(scrollContainer, "scrollHeight", { value: 1000, configurable: true });
    Object.defineProperty(scrollContainer, "clientHeight", { value: 300, configurable: true });
    Object.defineProperty(scrollContainer, "scrollTop", { value: 100, configurable: true, writable: true });

    fireEvent.scroll(scrollContainer);

    // Incoming new event while user is scrolled up
    capturedOnEvent!({
      eventId: "evt_new_msg",
      taskId: runningTask.taskId,
      seq: 2,
      type: "message_chunk",
      payload: { text: "Incoming background stream text" },
      createdAt: "2026-11-12T10:00:01Z",
    });

    // Floating "New messages" button should appear
    const newMsgBtn = await screen.findByRole("button", { name: "New messages" });
    expect(newMsgBtn).toBeTruthy();

    // Clicking the floating button triggers scroll to bottom and hides button
    fireEvent.click(newMsgBtn);
    expect(screen.queryByRole("button", { name: "New messages" })).toBeNull();
  });

  it("renders interactive inline approvals and handles Allow Once decision", async () => {
    const events: AgentTaskEvent[] = [
      {
        eventId: "evt_approval_1",
        taskId: mockTask.taskId,
        seq: 1,
        type: "approval",
        payload: {
          approvalId: "app_perm_1",
          target: "/workspace/config.ts",
          kind: "edit",
          status: "pending",
        },
        createdAt: "2026-11-12T10:00:01Z",
      },
    ];

    const client = {
      getAgentTaskTranscript: vi.fn().mockResolvedValue({ events }),
    };

    const approvals = {
      decideAgentApproval: vi.fn().mockResolvedValue({
        approvalId: "app_perm_1",
        status: "allowed_once",
      }),
      listAgentApprovals: vi.fn().mockResolvedValue({ approvals: [] }),
    };

    const onTaskUpdated = vi.fn();
    const onHasInlineApproval = vi.fn();

    render(
      <TaskConversationFeed
        task={{ ...mockTask, status: "running" }}
        client={client}
        approvals={approvals}
        onTaskUpdated={onTaskUpdated}
        onHasInlineApproval={onHasInlineApproval}
      />,
    );

    expect(await screen.findByText("Approval requested")).toBeTruthy();
    expect(screen.getByText("/workspace/config.ts")).toBeTruthy();
    expect(screen.getByText("Awaiting owner approval")).toBeTruthy();

    // Verify onHasInlineApproval was notified
    expect(onHasInlineApproval).toHaveBeenCalledWith(true);

    const allowBtn = screen.getByRole("button", { name: "Allow once" });
    const rejectBtn = screen.getByRole("button", { name: "Reject" });
    expect(allowBtn).toBeTruthy();
    expect(rejectBtn).toBeTruthy();

    fireEvent.click(allowBtn);

    await waitFor(() => {
      expect(approvals.decideAgentApproval).toHaveBeenCalledWith("app_perm_1", "allow_once");
    });

    // Transitions to granted state
    expect(await screen.findByText("Approval granted")).toBeTruthy();
    expect(onTaskUpdated).toHaveBeenCalled();
  });

  it("handles inline approvals Reject decision", async () => {
    const events: AgentTaskEvent[] = [
      {
        eventId: "evt_approval_2",
        taskId: mockTask.taskId,
        seq: 1,
        type: "approval",
        payload: {
          approvalId: "app_perm_2",
          target: "/bin/destructive",
          kind: "execute",
          status: "pending",
        },
        createdAt: "2026-11-12T10:00:01Z",
      },
    ];

    const client = {
      getAgentTaskTranscript: vi.fn().mockResolvedValue({ events }),
    };

    const approvals = {
      decideAgentApproval: vi.fn().mockResolvedValue({
        approvalId: "app_perm_2",
        status: "rejected",
      }),
      listAgentApprovals: vi.fn().mockResolvedValue({ approvals: [] }),
    };

    const onTaskUpdated = vi.fn();

    render(
      <TaskConversationFeed
        task={{ ...mockTask, status: "running" }}
        client={client}
        approvals={approvals}
        onTaskUpdated={onTaskUpdated}
      />,
    );

    expect(await screen.findByText("Approval requested")).toBeTruthy();
    const rejectBtn = screen.getByRole("button", { name: "Reject" });

    fireEvent.click(rejectBtn);

    await waitFor(() => {
      expect(approvals.decideAgentApproval).toHaveBeenCalledWith("app_perm_2", "reject");
    });

    // Transitions to rejected state
    expect(await screen.findByText("Approval rejected")).toBeTruthy();
    expect(onTaskUpdated).toHaveBeenCalled();
  });

  it("renders follow-up reply bar for completed task and sends on Enter", async () => {
    const client = {
      getAgentTaskTranscript: vi.fn().mockResolvedValue({
        events: [
          {
            eventId: "evt_1",
            taskId: mockTask.taskId,
            seq: 1,
            type: "user_prompt",
            payload: { prompt: "Initial prompt" },
            createdAt: "2026-11-12T10:00:00Z",
          },
        ],
      }),
      resumeAgentTask: vi.fn().mockResolvedValue({
        taskId: "atask_resumed_1",
        agentId: mockTask.agentId,
        status: "queued",
        continuedFromTaskId: mockTask.taskId,
        timeoutSeconds: 600,
        createdAt: "2026-11-12T10:10:00Z",
        updatedAt: "2026-11-12T10:10:00Z",
        cancelRequested: false,
      }),
    };

    const onTaskResumed = vi.fn();

    render(
      <TaskConversationFeed
        task={mockTask}
        client={client}
        onTaskResumed={onTaskResumed}
      />,
    );

    expect(await screen.findByText("Initial prompt")).toBeTruthy();
    expect(screen.getByText("Continue session")).toBeTruthy();

    const textarea = screen.getByPlaceholderText("Ask a follow-up or provide next instructions...");
    expect(textarea).toBeTruthy();

    fireEvent.change(textarea, { target: { value: "Follow-up question" } });

    // Press Shift+Enter - should not submit
    fireEvent.keyDown(textarea, { key: "Enter", shiftKey: true });
    expect(client.resumeAgentTask).not.toHaveBeenCalled();

    // Press Enter - submits
    fireEvent.keyDown(textarea, { key: "Enter", shiftKey: false });

    await waitFor(() => {
      expect(client.resumeAgentTask).toHaveBeenCalledWith(mockTask.taskId, "Follow-up question");
      expect(onTaskResumed).toHaveBeenCalledWith(expect.objectContaining({ taskId: "atask_resumed_1" }));
    });
  });

  it("submits follow-up via Send button and displays error on failure", async () => {
    const client = {
      getAgentTaskTranscript: vi.fn().mockResolvedValue({ events: [] }),
      resumeAgentTask: vi.fn().mockRejectedValue(new Error("Continuation failed: session expired")),
    };

    render(
      <TaskConversationFeed
        task={mockTask}
        client={client}
      />,
    );

    expect(await screen.findByText("Continue session")).toBeTruthy();
    const textarea = screen.getByPlaceholderText("Ask a follow-up or provide next instructions...");
    fireEvent.change(textarea, { target: { value: "Retry something" } });

    const sendBtn = screen.getByRole("button", { name: "Send" });
    fireEvent.click(sendBtn);

    expect(await screen.findByRole("alert")).toBeTruthy();
    expect(screen.getByText("Continuation failed: session expired")).toBeTruthy();
  });
});
