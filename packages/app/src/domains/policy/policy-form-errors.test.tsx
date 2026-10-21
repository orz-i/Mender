import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { MenderClient } from "@mender/sdk";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { PolicyPage } from "./policy-page";

beforeEach(() => { resetLocaleForTesting(); setLocale("en"); });
afterEach(() => { cleanup(); resetLocaleForTesting(); });

function renderPolicy(overrides: Partial<MenderClient> = {}) {
  const client = {
    getPolicy: vi.fn().mockResolvedValue({ defaultDecision: "allow", rules: [] }),
    listProfiles: vi.fn().mockResolvedValue({ profiles: [] }),
    listProviders: vi.fn().mockResolvedValue({ providers: [] }),
    browseTools: vi.fn().mockResolvedValue({ tools: [] }),
    ...overrides,
  } as unknown as MenderClient;
  render(<MemoryRouter><PolicyPage client={client} /></MemoryRouter>);
  return client;
}

describe("Policy modal mutation errors", () => {
  it("shows a rejected default update in its open dialog, preserves the chosen decision, and permits retry", async () => {
    const setPolicyDefault = vi.fn()
      .mockRejectedValueOnce(new Error("Default decision save failed"))
      .mockResolvedValue({ defaultDecision: "deny", rules: [] });
    renderPolicy({ setPolicyDefault });
    expect(await screen.findByText("No Policy Rules")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    const dialog = screen.getByRole("dialog", { name: "Edit default decision" });
    fireEvent.change(within(dialog).getByLabelText("Default Policy decision"), { target: { value: "deny" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Save default" }));

    expect(await within(dialog).findByRole("alert")).toHaveProperty("textContent", "Default decision save failed");
    expect(within(dialog).getByLabelText<HTMLSelectElement>("Default Policy decision").value).toBe("deny");
    expect(screen.getAllByRole("alert")).toHaveLength(1);
    fireEvent.click(within(dialog).getByRole("button", { name: "Save default" }));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Edit default decision" })).toBeNull());
    expect(setPolicyDefault).toHaveBeenCalledTimes(2);
  });

  it("keeps rule draft and retry within the rule editor when creation fails", async () => {
    const createPolicyRule = vi.fn()
      .mockRejectedValueOnce(new Error("Policy rule could not be saved"))
      .mockResolvedValue({ ruleId: "rule_test" });
    renderPolicy({ createPolicyRule });
    expect(await screen.findByText("No Policy Rules")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Add rule" }));
    const dialog = screen.getByRole("dialog", { name: "Add Policy rule" });
    fireEvent.change(within(dialog).getByLabelText("Policy Rule display name"), {
      target: { value: "Only allow verified requests" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "Next" }));
    fireEvent.click(within(dialog).getByRole("button", { name: "Next" }));
    fireEvent.click(within(dialog).getByRole("button", { name: "Create rule" }));

    const errorAlert = await within(dialog).findByRole("alert");
    expect(errorAlert).toHaveProperty("textContent", "Policy rule could not be saved");
    expect(errorAlert.closest('[data-slot="dialog-header"]')).toBeTruthy();
    expect(screen.getAllByRole("alert")).toHaveLength(1);

    fireEvent.click(within(dialog).getByRole("button", { name: "Back" }));
    expect(within(dialog).queryByRole("alert")).toBeNull();
    fireEvent.click(within(dialog).getByRole("button", { name: "Back" }));
    expect(within(dialog).getByLabelText<HTMLInputElement>("Policy Rule display name").value)
      .toBe("Only allow verified requests");
    fireEvent.click(within(dialog).getByRole("button", { name: "Next" }));
    fireEvent.click(within(dialog).getByRole("button", { name: "Next" }));
    fireEvent.click(within(dialog).getByRole("button", { name: "Create rule" }));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Add Policy rule" })).toBeNull());
    expect(createPolicyRule).toHaveBeenCalledTimes(2);
  });

  it("does not carry a dismissed rule error into the next edit", async () => {
    const createPolicyRule = vi.fn().mockRejectedValue(new Error("Try again"));
    renderPolicy({ createPolicyRule });
    expect(await screen.findByText("No Policy Rules")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Add rule" }));
    const dialog = screen.getByRole("dialog", { name: "Add Policy rule" });
    fireEvent.change(within(dialog).getByLabelText("Policy Rule display name"), {
      target: { value: "Temporary" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "Next" }));
    fireEvent.click(within(dialog).getByRole("button", { name: "Next" }));
    fireEvent.click(within(dialog).getByRole("button", { name: "Create rule" }));
    await within(dialog).findByRole("alert");
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    fireEvent.click(screen.getByRole("button", { name: "Add rule" }));
    expect(within(screen.getByRole("dialog", { name: "Add Policy rule" })).queryByRole("alert")).toBeNull();
  });
});
