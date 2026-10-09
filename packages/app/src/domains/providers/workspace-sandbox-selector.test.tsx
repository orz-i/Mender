import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { Sandbox } from "@mender/sdk";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";

import { GenericProviderConfigForm } from "./generic-provider-config-form";

afterEach(cleanup);

const availableSandbox: Sandbox = {
  sandboxId: "sbx_0123456789abcdef0123456789abcdef",
  name: "Local project",
  backend: "local_workspace",
  isolation: "restricted_host",
  state: "ready",
  localRoot: "/workspace/project",
  ownerKind: "local_user",
  ownerRef: "test-owner",
  activeLeases: 0,
};

const unavailableSandbox: Sandbox = {
  ...availableSandbox,
  sandboxId: "sbx_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  name: "Offline project",
  state: "stopped",
};

const remoteSandbox: Sandbox = {
  ...availableSandbox,
  sandboxId: "sbx_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  name: "Remote project",
  backend: "daytona",
  isolation: "remote",
};

describe("Workspace Sandbox selector", () => {
  it("selects only ready local Sandbox resources and never requests raw IDs", () => {
    const onChange = vi.fn();
    render(
      <MemoryRouter>
        <GenericProviderConfigForm
          pluginId="io.mender.provider.workspace"
          schema={{
            type: "object",
            required: ["sandboxId"],
            additionalProperties: false,
            properties: { sandboxId: { type: "string" } },
          }}
          uiSchema={undefined}
          value={{}}
          credentials={[]}
          sandboxes={[availableSandbox, unavailableSandbox, remoteSandbox]}
          onChange={onChange}
        />
      </MemoryRouter>,
    );
    const selector = screen.getByRole<HTMLSelectElement>("combobox");
    const choices = screen.getAllByRole<HTMLOptionElement>("option");
    expect(choices).toHaveLength(2);
    expect(choices.some((option) => option.value === availableSandbox.sandboxId)).toBe(true);
    expect(choices.some((option) => option.value === unavailableSandbox.sandboxId)).toBe(false);
    expect(choices.some((option) => option.value === remoteSandbox.sandboxId)).toBe(false);
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(screen.getByRole<HTMLAnchorElement>("link").getAttribute("href")).toBe("/sandboxes/new");
    fireEvent.change(selector, { target: { value: availableSandbox.sandboxId } });
    expect(onChange).toHaveBeenCalledWith({ sandboxId: availableSandbox.sandboxId });
  });
});
