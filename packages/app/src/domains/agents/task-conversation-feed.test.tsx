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
});
