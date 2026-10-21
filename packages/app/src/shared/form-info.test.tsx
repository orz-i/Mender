import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../i18n/i18n";
import { FormHeading, FormInfo } from "./form-info";

beforeEach(() => {
  resetLocaleForTesting();
  setLocale("en");
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  resetLocaleForTesting();
});

it("keeps explanatory content out of the layout until the info button is activated", async () => {
  render(<FormHeading label="Profile" help="Select the tool Profile shared by this server." />);
  const help = screen.getByRole("button", { name: "Help for Profile" });
  expect(help.getAttribute("type")).toBe("button");
  expect(screen.queryByText("Select the tool Profile shared by this server.")).toBeNull();

  fireEvent.click(help);
  expect(await screen.findByText("Select the tool Profile shared by this server.")).toBeTruthy();

  fireEvent.keyDown(help, { key: "Escape" });
  await waitFor(() => expect(help.getAttribute("aria-expanded")).toBe("false"));
});

it("a help icon can be used as a standalone non-submitting action", async () => {
  const submit = vi.fn((event: React.FormEvent<HTMLFormElement>) => event.preventDefault());
  render(
    <form onSubmit={submit}>
      <label htmlFor="value">Value</label>
      <FormInfo label="Value" side="bottom">Not a required instruction.</FormInfo>
      <input id="value" />
      <button type="submit">Save</button>
    </form>,
  );
  fireEvent.click(screen.getByRole("button", { name: "Help for Value" }));
  expect(await screen.findByText("Not a required instruction.")).toBeTruthy();
  expect(submit).not.toHaveBeenCalled();
});
