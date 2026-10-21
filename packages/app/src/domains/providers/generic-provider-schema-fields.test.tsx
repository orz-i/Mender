import {cleanup,fireEvent,render,screen} from "@testing-library/react";
import type {ProviderPlugin} from "@mender/sdk";
import {useState} from "react";
import {afterEach,beforeEach,describe,expect,it} from "vitest";

import {resetLocaleForTesting,setLocale} from "../../i18n/i18n";
import {genericConfigValidationError} from "./generic-provider-config-schema";
import {GenericProviderConfigForm} from "./generic-provider-config-form";

function Scenario({schema,initial}:{
 schema:ProviderPlugin["configSchema"];
 initial:Record<string,unknown>;
}){
 const [value,setValue]=useState(initial);
 const [draftError,setDraftError]=useState<string|null>(null);
 const validation=genericConfigValidationError(schema,value);
 return <>
  <GenericProviderConfigForm schema={schema} uiSchema={undefined}
   value={value} credentials={[]} onChange={setValue}
   onDraftErrorChange={setDraftError}/>
  <button type="button" disabled={!!(draftError||validation)}>Save</button>
  <output data-testid="model">{JSON.stringify(value)}</output>
  <output data-testid="validation">{validation??""}</output>
 </>;
}
const schema={
 type:"object",
 properties:{
  attempts:{type:"integer",minimum:1,maximum:10,description:"Maximum safe attempts"},
  ratio:{type:"number",minimum:0,maximum:1},
 }
} as const;
beforeEach(()=>{resetLocaleForTesting();setLocale("en");});
afterEach(()=>{cleanup();resetLocaleForTesting();});

describe("dynamic Schema numeric drafts and contextual field validation",()=>{
 it("keeps incomplete optional integer tokens locally without deleting the previous value",()=>{
  render(<Scenario schema={schema} initial={{attempts:3}}/>);
  const attempts=screen.getByRole<HTMLInputElement>("textbox",{name:"Attempts"});
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Save"}).disabled).toBe(false);
  fireEvent.change(attempts,{target:{value:"1e"}});
  expect(attempts.value).toBe("1e");
  expect(attempts.getAttribute("aria-invalid")).toBe("true");
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Save"}).disabled).toBe(true);
  expect(screen.getByTestId("model").textContent).toBe('{"attempts":3}');
  fireEvent.change(attempts,{target:{value:"8"}});
  expect(attempts.value).toBe("8");
  expect(attempts.getAttribute("aria-invalid")).toBe("false");
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Save"}).disabled).toBe(false);
  expect(screen.getByTestId("model").textContent).toBe('{"attempts":8}');
 });
 it("shows integer and range errors beside fields, not only in a remote summary",()=>{
  render(<Scenario schema={schema} initial={{attempts:3,ratio:0.5}}/>);
  const attempts=screen.getByLabelText<HTMLInputElement>("Attempts");
  const ratio=screen.getByLabelText<HTMLInputElement>("Ratio");
  fireEvent.change(attempts,{target:{value:"2.5"}});
  expect(attempts.value).toBe("2.5");
  expect(attempts.getAttribute("aria-invalid")).toBe("true");
  const inlineError=screen.getByRole("alert");
  expect(inlineError.textContent?.toLowerCase()).toMatch(/integer|whole number/);
  expect(attempts.getAttribute("aria-describedby")).toContain(inlineError.id);
  fireEvent.change(attempts,{target:{value:"5"}});
  expect(attempts.getAttribute("aria-invalid")).toBe("false");
  fireEvent.change(ratio,{target:{value:"1.5"}});
  expect(ratio.getAttribute("aria-invalid")).toBe("true");
  expect(screen.getByRole("alert").textContent).toMatch(/1|maximum|at most/i);
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Save"}).disabled).toBe(true);
 });
 it("associates nested field instructions and validation errors, and clicking labels focuses the field",()=>{
  const nestedSchema={
   type:"object",
   required:["connection"],
   properties:{
    connection:{
     type:"object",
     required:["transport"],
     properties:{
      transport:{
       type:"object",
       required:["endpoint"],
       properties:{
        endpoint:{type:"string",format:"uri",description:"Full endpoint URL"},
        retries:{type:"integer",minimum:1,maximum:5},
       },
      },
     },
    },
   },
  };
  render(<Scenario schema={nestedSchema} initial={{connection:{transport:{endpoint:"not-a-uri",retries:2}}}}/>);
  expect(screen.getByRole("group",{name:"Connection"})).toBeTruthy();
  expect(screen.getByRole("group",{name:"Transport"})).toBeTruthy();
  const endpoint=screen.getByRole<HTMLInputElement>("textbox",{name:"Endpoint"});
  const error=screen.getByRole("alert");
  expect(error.textContent?.toLowerCase()).toMatch(/uri|url/);
  expect(endpoint.getAttribute("aria-invalid")).toBe("true");
  expect(endpoint.getAttribute("aria-describedby")).toContain(error.id);
  expect(endpoint.getAttribute("aria-describedby")).toContain(screen.getByText("Full endpoint URL").id);
  const label=screen.getByText<HTMLLabelElement>("Endpoint");
  expect(label.tagName).toBe("LABEL");
  expect(label.control).toBe(endpoint);
  endpoint.focus();
  expect(document.activeElement).toBe(endpoint);
  fireEvent.change(endpoint,{target:{value:"https://example.test/mcp"}});
  expect(endpoint.getAttribute("aria-invalid")).toBe("false");
  expect(screen.queryByRole("alert")).toBeNull();
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Save"}).disabled).toBe(false);
 });
 it("does not mark an absent optional group as invalid before it is configured",()=>{
  render(<Scenario schema={{
   type:"object",
   properties:{
    advanced:{type:"object",required:["token"],properties:{
     token:{type:"string"},
     enabled:{type:"boolean"},
    }},
   },
  }} initial={{}}/>);
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Save"}).disabled).toBe(false);
  expect(screen.queryByRole("alert")).toBeNull();
  const token=screen.getByRole<HTMLInputElement>("textbox",{name:"Token"});
  expect(token.getAttribute("aria-invalid")).toBe("false");
  fireEvent.change(token,{target:{value:"configured"}});
  expect(screen.getByTestId("model").textContent).toContain('{"advanced":{"token":"configured"}}');
  expect(screen.queryByRole("alert")).toBeNull();
 });

 it("identifies missing nested required fields at the control itself",()=>{
  render(<Scenario schema={{
   type:"object",
   properties:{settings:{type:"object",required:["name"],properties:{name:{type:"string"}}}},
  }} initial={{settings:{}}}/>);
  const name=screen.getByRole<HTMLInputElement>("textbox",{name:"Name"});
  expect(name.getAttribute("aria-required")).toBe("true");
  expect(name.getAttribute("aria-invalid")).toBe("true");
  expect(screen.getByRole("alert").textContent?.toLowerCase()).toContain("required");
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Save"}).disabled).toBe(true);
  fireEvent.change(name,{target:{value:"demo"}});
  expect(name.getAttribute("aria-invalid")).toBe("false");
  expect(screen.queryByRole("alert")).toBeNull();
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Save"}).disabled).toBe(false);
 });
});
