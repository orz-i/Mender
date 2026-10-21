import {cleanup,fireEvent,render,screen,waitFor,within} from "@testing-library/react";
import type {MenderClient,RuntimeBudget,RuntimeGovernanceStatus} from "@mender/sdk";
import {afterEach,beforeEach,describe,expect,it,vi} from "vitest";

import {resetLocaleForTesting,setLocale} from "../../i18n/i18n";
import {RuntimePage} from "./runtime-page";

const initialBudget:RuntimeBudget={
 maxInvocations:32,maxPrincipalInvocations:8,maxProviderInvocations:8,
 maxToolInvocations:4,maxProviderActivations:4,maxExecutionTimeoutMs:120000,
 maxArgumentBytes:1048576,maxResultBytes:1048576,
};
const status:RuntimeGovernanceStatus={
 budget:initialBudget,
 isolation:{workspaceShell:"host",externalProvider:"host",tunnelClient:"host"},
 backend:{name:"test-host",restrictedHostAvailable:true},
 usage:{invocations:1,activations:0},
};
function makeClient(overrides:Record<string,unknown>={}):MenderClient {
 return {
  getRuntimeGovernance:vi.fn().mockResolvedValue(status),
  updateRuntimeBudget:vi.fn().mockImplementation((budget:RuntimeBudget)=>Promise.resolve({...status,budget})),
  updateRuntimeIsolation:vi.fn().mockImplementation((isolation:RuntimeGovernanceStatus["isolation"])=>Promise.resolve({...status,isolation})),
  ...overrides,
 } as unknown as MenderClient;
}
async function openBudget() {
 fireEvent.click(await screen.findByRole("button",{name:"Runtime budget"}));
 fireEvent.click(screen.getByRole("button",{name:"Edit"}));
 return screen.getByRole("dialog",{name:"Runtime budget"});
}
beforeEach(()=>{resetLocaleForTesting();setLocale("en");});
afterEach(()=>{cleanup();resetLocaleForTesting();});

describe("Runtime budget dialog form",()=>{
 it("keeps the footer pinned and explicitly shows valid limits without eight permanent explanations",async()=>{
  render(<RuntimePage client={makeClient()}/>);
  const dialog=await openBudget();
  expect(dialog.className).toContain("grid-rows-[auto_minmax(0,1fr)_auto]");
  expect(dialog.querySelector('[data-slot="dialog-body"]')).not.toBeNull();
  expect(within(dialog).getByText("Allowed range: 1 to 86400000")).toBeTruthy();
  expect(within(dialog).getByRole("button",{name:"Help for Global invocations"})).toBeTruthy();
  const label=within(dialog).getByText("Global invocations");
  expect(label.tagName).toBe("LABEL");
  expect(label.querySelector("button")).toBeNull();
 });
 it("does not submit an empty, fractional or out-of-range limit and keeps exactly what was typed",async()=>{
  const update=vi.fn();
  render(<RuntimePage client={makeClient({updateRuntimeBudget:update})}/>);
  const dialog=await openBudget();
  const input=within(dialog).getByRole<HTMLInputElement>("spinbutton",{name:"Global invocations"});
  const save=within(dialog).getByRole<HTMLButtonElement>("button",{name:"Save budget"});
  fireEvent.change(input,{target:{value:""}});
  expect(input.value).toBe("");
  expect(input.getAttribute("aria-invalid")).toBe("true");
  expect(within(dialog).getAllByRole("alert").some(element=>element.textContent?.includes("Enter a whole number from 1 to 4096"))).toBe(true);
  expect(save.disabled).toBe(true);
  fireEvent.change(input,{target:{value:"2.5"}});
  expect(input.value).toBe("2.5");
  expect(save.disabled).toBe(true);
  fireEvent.change(input,{target:{value:"99999"}});
  expect(save.disabled).toBe(true);
  fireEvent.change(input,{target:{value:"12"}});
  expect(input.getAttribute("aria-invalid")).toBe("false");
  expect(save.disabled).toBe(false);
  expect(update).not.toHaveBeenCalled();
 });
 it("keeps the draft and shows save failure inside the dialog before a successful retry",async()=>{
  const update=vi.fn()
   .mockRejectedValueOnce(new Error("Runtime limits could not be saved"))
   .mockImplementation((budget:RuntimeBudget)=>Promise.resolve({...status,budget}));
  render(<RuntimePage client={makeClient({updateRuntimeBudget:update})}/>);
  const dialog=await openBudget();
  const input=within(dialog).getByRole<HTMLInputElement>("spinbutton",{name:"Global invocations"});
  fireEvent.change(input,{target:{value:"48"}});
  fireEvent.click(within(dialog).getByRole("button",{name:"Save budget"}));
  expect(await within(dialog).findByRole("alert")).toHaveProperty("textContent","Runtime limits could not be saved");
  expect(input.value).toBe("48");
  expect(screen.getByRole("dialog",{name:"Runtime budget"})).toBeTruthy();
  fireEvent.click(within(dialog).getByRole("button",{name:"Save budget"}));
  await waitFor(()=>expect(update).toHaveBeenCalledTimes(2));
  expect(update).toHaveBeenLastCalledWith({...initialBudget,maxInvocations:48});
  await waitFor(()=>expect(screen.queryByRole("dialog",{name:"Runtime budget"})).toBeNull());
 });
 it("discards an unsaved edit when canceled and starts from the persisted value next time",async()=>{
  const update=vi.fn();
  render(<RuntimePage client={makeClient({updateRuntimeBudget:update})}/>);
  const dialog=await openBudget();
  fireEvent.change(within(dialog).getByRole("spinbutton",{name:"Global invocations"}),{target:{value:"99"}});
  fireEvent.click(within(dialog).getByRole("button",{name:"Cancel"}));
  const reopened=await openBudget();
  expect(within(reopened).getByRole<HTMLInputElement>("spinbutton",{name:"Global invocations"}).value).toBe("32");
  expect(update).not.toHaveBeenCalled();
 });
 it("shows isolation save errors within its pinned dialog rather than behind the overlay",async()=>{
  const updateIsolation=vi.fn().mockRejectedValue(new Error("Isolation update denied"));
  render(<RuntimePage client={makeClient({updateRuntimeIsolation:updateIsolation})}/>);
  fireEvent.click(await screen.findByRole("button",{name:"Process isolation"}));
  fireEvent.click(screen.getByRole("button",{name:"Edit"}));
  const dialog=screen.getByRole("dialog",{name:"Process isolation"});
  expect(dialog.className).toContain("grid-rows-[auto_minmax(0,1fr)_auto]");
  expect(dialog.querySelector('[data-slot="dialog-body"]')).not.toBeNull();
  fireEvent.click(within(dialog).getByRole("button",{name:"Save isolation"}));
  expect(await within(dialog).findByRole("alert")).toHaveProperty("textContent","Isolation update denied");
  expect(updateIsolation).toHaveBeenCalledWith(status.isolation);
  expect(screen.getByRole("dialog",{name:"Process isolation"})).toBeTruthy();
 });
});
