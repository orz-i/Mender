import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ManagementClient, ToolDetail, ToolSummary } from "@mender/sdk";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../../i18n/i18n";
import { ToolPlaygroundPage } from "./tool-playground-page";

beforeEach(() => {
  resetLocaleForTesting();
  setLocale("en");
});

afterEach(() => {
  cleanup();
  resetLocaleForTesting();
});

const mockTools: ToolSummary[] = [
  {
    toolId: "tool_test_01",
    name: "fetch_weather",
    title: "Fetch Weather",
    description: "Get current weather for location",
    sourceId: "provider:weather_svc",
    sourceKind: "provider",
    rank: 1,
  },
  {
    toolId: "tool_test_02",
    name: "create_ticket",
    title: "Create Ticket",
    description: "Create an issue ticket",
    sourceId: "provider:jira_svc",
    sourceKind: "provider",
    rank: 2,
  },
];

const mockToolDetail: ToolDetail = {
  toolId: "tool_test_01",
  name: "fetch_weather",
  title: "Fetch Weather",
  description: "Get current weather for location",
  sourceId: "provider:weather_svc",
  sourceKind: "provider",
  sourceKey: "weather.get",
  tags: ["utility", "weather"],
  inputSchema: {
    type: "object",
    properties: {
      location: { type: "string", default: "Shanghai" },
      days: { type: "number", default: 3 },
    },
    required: ["location"],
  },
};

function createMockClient() {
  const callToolMock = vi.fn().mockResolvedValue({
    invocationId: "inv_123456",
    kind: "complete",
    content: { temp: 24, condition: "Sunny" },
  });

  const client = {
    listProviders: vi.fn().mockResolvedValue({
      providers: [
        {
          providerId: "weather_svc",
          displayName: "Weather Service",
          namespace: "weather",
        },
      ],
    }),
    browseTools: vi.fn().mockResolvedValue({ tools: mockTools }),
    getTool: vi.fn().mockResolvedValue(mockToolDetail),
    callTool: callToolMock,
  } as unknown as ManagementClient;

  return { client, callTool: callToolMock };
}

describe("ToolPlaygroundPage studio workbench", () => {
  it("renders 3-pane playground layout with tool selector, argument editor, and output", async () => {
    const { client } = createMockClient();
    render(
      <MemoryRouter>
        <ToolPlaygroundPage client={client} />
      </MemoryRouter>,
    );

    expect(screen.getByRole("heading", { name: "Tool Playground" })).toBeTruthy();
    expect(await screen.findByText("Fetch Weather")).toBeTruthy();
    expect(await screen.findByRole("textbox", { name: "Arguments (JSON)" })).toBeTruthy();
    expect(screen.getByText("Execution Output")).toBeTruthy();
    expect(screen.getByRole("button", { name: /Run Tool/ })).toBeTruthy();
  });

  it("invokes tool with parsed arguments and displays execution result", async () => {
    const { client, callTool } = createMockClient();
    render(
      <MemoryRouter>
        <ToolPlaygroundPage client={client} />
      </MemoryRouter>,
    );

    const runBtn = await screen.findByRole("button", { name: /Run Tool/ });
    fireEvent.click(runBtn);

    await waitFor(() => {
      expect(callTool).toHaveBeenCalledWith("tool_test_01", {
        location: "Shanghai",
        days: 3,
      });
    });

    expect(await screen.findByText(/Sunny/)).toBeTruthy();
    expect(screen.getByText("Recent Runs")).toBeTruthy();
  });

  it("validates invalid JSON and blocks execution", async () => {
    const { client } = createMockClient();
    render(
      <MemoryRouter>
        <ToolPlaygroundPage client={client} />
      </MemoryRouter>,
    );

    const editor = await screen.findByRole("textbox", { name: "Arguments (JSON)" });
    fireEvent.change(editor, { target: { value: "{ invalid-json" } });

    expect(screen.getByText("Invalid JSON")).toBeTruthy();
    const runBtn = screen.getByRole("button", { name: /Run Tool/ });
    expect(runBtn.hasAttribute("disabled")).toBe(true);
  });
});
