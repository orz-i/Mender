import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
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
      includeAlerts: { type: "boolean", default: false },
      unit: { type: "string", enum: ["celsius", "fahrenheit"], default: "celsius" },
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
        {
          providerId: "jira_svc",
          displayName: "Jira Service",
          namespace: "jira",
        },
      ],
    }),
    browseTools: vi.fn().mockResolvedValue({ tools: mockTools }),
    getTool: vi.fn().mockResolvedValue(mockToolDetail),
    callTool: callToolMock,
  } as unknown as ManagementClient;

  return { client, callTool: callToolMock };
}

async function selectOption(combobox: HTMLElement, optionName: string) {
  fireEvent.click(combobox);
  const option = await screen.findByRole("option", { name: optionName });
  fireEvent.pointerDown(option, { pointerId: 1, pointerType: "mouse", button: 0, isPrimary: true });
  fireEvent.pointerUp(option, { pointerId: 1, pointerType: "mouse", button: 0, isPrimary: true });
  fireEvent.click(option);
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
        includeAlerts: false,
        unit: "celsius",
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
    expect(screen.getByRole("button", { name: "Form" }).hasAttribute("disabled")).toBe(true);
  });

  it("filters tools with the Select provider control and syncs Form and JSON bidirectionally", async () => {
    const { client, callTool } = createMockClient();
    const { container } = render(
      <MemoryRouter>
        <ToolPlaygroundPage client={client} />
      </MemoryRouter>,
    );

    expect(await screen.findByText("Create Ticket")).toBeTruthy();
    const playgroundRoot = container.querySelector('[data-slot="tool-playground"]');
    expect(playgroundRoot?.className).toContain("lg:h-[calc(100dvh-148px)]");

    await selectOption(screen.getByRole("combobox", { name: "Filter by provider" }), "Weather Service");
    await waitFor(() => expect(screen.queryByText("Create Ticket")).toBeNull());
    expect(within(screen.getByRole("listbox", { name: "Available tools" })).getByText("Fetch Weather")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Form" }));
    const locationInput = screen.getByLabelText<HTMLInputElement>(/location/);
    const daysInput = screen.getByLabelText<HTMLInputElement>(/days/);
    const alertsCheckbox = screen.getByLabelText<HTMLInputElement>(/includeAlerts/);
    expect(locationInput.value).toBe("Shanghai");
    expect(daysInput.value).toBe("3");
    expect(alertsCheckbox.checked).toBe(false);

    fireEvent.change(locationInput, { target: { value: "Beijing" } });
    fireEvent.change(daysInput, { target: { value: "5" } });
    fireEvent.click(alertsCheckbox);
    await selectOption(screen.getByRole("combobox", { name: "unit" }), "fahrenheit");

    fireEvent.click(screen.getByRole("button", { name: "JSON" }));
    const jsonEditor = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Arguments (JSON)" });
    expect(JSON.parse(jsonEditor.value)).toEqual({
      location: "Beijing",
      days: 5,
      includeAlerts: true,
      unit: "fahrenheit",
    });

    fireEvent.change(jsonEditor, {
      target: {
        value: JSON.stringify({
          location: "Tokyo",
          days: 2,
          includeAlerts: false,
          unit: "celsius",
        }),
      },
    });
    fireEvent.click(screen.getByRole("button", { name: "Form" }));
    expect(screen.getByLabelText<HTMLInputElement>(/location/).value).toBe("Tokyo");
    expect(screen.getByLabelText<HTMLInputElement>(/days/).value).toBe("2");

    fireEvent.click(screen.getByRole("button", { name: /Run Tool/ }));
    await waitFor(() => {
      expect(callTool).toHaveBeenCalledWith("tool_test_01", {
        location: "Tokyo",
        days: 2,
        includeAlerts: false,
        unit: "celsius",
      });
    });
  });
});
