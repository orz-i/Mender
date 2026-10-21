import {cleanup,fireEvent,render,screen,waitFor,within} from "@testing-library/react";
import {useState} from "react";
import type {ProviderPlugin} from "@mender/sdk";
import {afterEach,beforeEach,describe,expect,it} from "vitest";

import {resetLocaleForTesting,setLocale} from "../../i18n/i18n";
import {GenericProviderConfigForm} from "./generic-provider-config-form";
import {genericConfigValidationError} from "./generic-provider-config-schema";

const schema={
 type:"object",additionalProperties:false,
 properties:{
  advanced:{
   type:"object",description:"Configure advanced connection features",
   required:["token"],additionalProperties:false,
   properties:{
    token:{type:"string"},
    retries:{type:"integer",minimum:1,maximum:5,default:2},
    preferences:{type:"object",required:["path"],additionalProperties:false,
     properties:{path:{type:"string"}}},
   },
  },
 },
} as const;

function Fixture({initial={},configSchema=schema,uiSchema}:{
 initial?:Record<string,unknown>;
 configSchema?:ProviderPlugin["configSchema"];
 uiSchema?:ProviderPlugin["uiSchema"];
}) {
 const [value,setValue]=useState(initial);
 const [draftError,setDraftError]=useState<string|null>(null);
 const validation=genericConfigValidationError(configSchema,value);
 return <>
  <GenericProviderConfigForm schema={configSchema} uiSchema={uiSchema} value={value}
   credentials={[]} onChange={setValue} onDraftErrorChange={setDraftError}/>
  <button type="button" disabled={draftError!==null||validation!==null}>Save configuration</button>
  <output data-testid="value">{JSON.stringify(value)}</output>
 </>;
}
beforeEach(()=>{resetLocaleForTesting();setLocale("en");});
afterEach(()=>{cleanup();resetLocaleForTesting();});

describe("optional nested JSON Schema object lifecycle",()=>{
 it("does not present unused child inputs until explicit add, and removes the key rather than saving {}",async()=>{
  render(<Fixture uiSchema={{properties:{advanced:{label:"Advanced options",description:"Configure only when needed"}}}}/>);
  const save=screen.getByRole<HTMLButtonElement>("button",{name:"Save configuration"});
  expect(save.disabled).toBe(false);
  expect(screen.queryByRole("group",{name:"Advanced options"})).toBeNull();
  expect(screen.queryByRole("textbox",{name:"Token"})).toBeNull();
  expect(screen.getByText("Configure only when needed")).toBeTruthy();
  expect(screen.getByTestId("value").textContent).toBe("{}");
  fireEvent.click(screen.getByRole("button",{name:"Add Advanced options"}));
  const group=screen.getByRole("group",{name:"Advanced options"});
  const token=within(group).getByRole<HTMLInputElement>("textbox",{name:"Token"});
  expect(token.getAttribute("aria-invalid")).toBe("true");
  expect(save.disabled).toBe(true);
  expect(screen.getByTestId("value").textContent).toBe('{"advanced":{"retries":2}}');
  expect(screen.getByRole<HTMLInputElement>("textbox",{name:"Retries"}).value).toBe("2");
  fireEvent.change(token,{target:{value:"configured"}});
  expect(save.disabled).toBe(false);
  fireEvent.click(screen.getByRole("button",{name:"Remove Advanced options"}));
  await waitFor(()=>expect(save.disabled).toBe(false));
  expect(screen.getByTestId("value").textContent).toBe("{}");
  expect(screen.queryByRole("textbox",{name:"Token"})).toBeNull();
  expect(screen.getByRole("button",{name:"Add Advanced options"})).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"Add Advanced options"}));
  expect(screen.getByRole<HTMLInputElement>("textbox",{name:"Token"}).value).toBe("");
  expect(save.disabled).toBe(true);
 });
 it("preserves existing nested objects until a deliberate remove, and can remove only a nested child",()=>{
  render(<Fixture initial={{advanced:{token:"existing",preferences:{path:"/tmp/example"}}}}/>);
  const save=screen.getByRole<HTMLButtonElement>("button",{name:"Save configuration"});
  expect(screen.queryByRole("button",{name:"Add Advanced"})).toBeNull();
  expect(screen.getByRole("group",{name:"Advanced"})).toBeTruthy();
  expect(screen.getByRole<HTMLInputElement>("textbox",{name:"Token"}).value).toBe("existing");
  expect(screen.getByRole<HTMLInputElement>("textbox",{name:"Path"}).value).toBe("/tmp/example");
  fireEvent.click(screen.getByRole("button",{name:"Remove Preferences"}));
  expect(screen.getByTestId("value").textContent).toBe('{"advanced":{"token":"existing"}}');
  expect(screen.queryByRole("textbox",{name:"Path"})).toBeNull();
  expect(save.disabled).toBe(false);
  fireEvent.click(screen.getByRole("button",{name:"Add Preferences"}));
  expect(screen.getByRole("textbox",{name:"Path"})).toBeTruthy();
  expect(save.disabled).toBe(true);
  fireEvent.change(screen.getByRole("textbox",{name:"Path"}),{target:{value:"/another"}});
  expect(save.disabled).toBe(false);
  expect(screen.getByTestId("value").textContent).toBe('{"advanced":{"token":"existing","preferences":{"path":"/another"}}}');
  fireEvent.click(screen.getByRole("button",{name:"Remove Advanced"}));
  expect(screen.getByTestId("value").textContent).toBe("{}");
  expect(save.disabled).toBe(false);
 });
 it("clears invalid descendant JSON drafts on removal without clearing unrelated parent input",async()=>{
  render(<Fixture initial={{title:"keep-me",advanced:{records:[1]}}}
   configSchema={{type:"object",properties:{
    title:{type:"string"},
    advanced:{type:"object",properties:{records:{type:"array"}}},
   }}}/>);
  const save=screen.getByRole<HTMLButtonElement>("button",{name:"Save configuration"});
  const records=screen.getByRole<HTMLTextAreaElement>("textbox",{name:"Records"});
  fireEvent.change(records,{target:{value:"[1,"}});
  expect(records.value).toBe("[1,");
  expect(save.disabled).toBe(true);
  expect(screen.getByRole("alert").textContent).toContain("Enter valid JSON.");
  fireEvent.click(screen.getByRole("button",{name:"Remove Advanced"}));
  await waitFor(()=>expect(save.disabled).toBe(false));
  expect(screen.queryByRole("alert")).toBeNull();
  expect(screen.getByTestId("value").textContent).toBe('{"title":"keep-me"}');
  fireEvent.click(screen.getByRole("button",{name:"Add Advanced"}));
  expect(screen.getByRole<HTMLTextAreaElement>("textbox",{name:"Records"}).value).toBe("");
  expect(save.disabled).toBe(false);
 });
 it("never lets a required object be removed or hidden",()=>{
  render(<Fixture configSchema={{type:"object",required:["connection"],properties:{
   connection:{type:"object",required:["endpoint"],properties:{endpoint:{type:"string"}}},
  }}}/>);
  expect(screen.getByRole("group",{name:"Connection"})).toBeTruthy();
  expect(screen.queryByRole("button",{name:/Remove Connection/})).toBeNull();
  expect(screen.queryByRole("button",{name:/Add Connection/})).toBeNull();
  expect(screen.getByRole("textbox",{name:"Endpoint"})).toBeTruthy();
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Save configuration"}).disabled).toBe(true);
  fireEvent.change(screen.getByRole("textbox",{name:"Endpoint"}),{target:{value:"valid"}});
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Save configuration"}).disabled).toBe(false);
 });
 it("shows localized Add/Remove controls while preserving group name in the accessibility tree",()=>{
  setLocale("zh-CN");
  render(<Fixture/>);
  fireEvent.click(screen.getByRole("button",{name:"添加 Advanced"}));
  expect(screen.getByRole("group",{name:"Advanced"})).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"移除 Advanced"}));
  expect(screen.queryByRole("group",{name:"Advanced"})).toBeNull();
 });
});
