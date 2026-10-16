import type { Provider } from "@mender/sdk";
import { afterEach, beforeEach, expect, it } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { toolSourceLabel } from "./tool-source-label";

beforeEach(() => { resetLocaleForTesting(); setLocale("en"); });
afterEach(() => resetLocaleForTesting());

const provider = { providerId:"prov_1",displayName:"Opaque internal name",namespace:"opaque" } as Provider;
it("renders the verified owner instead of internal Provider facade naming", () => {
  expect(toolSourceLabel({...provider,toolSourceKind:"agent",toolSourceId:"agent_1",toolSourceName:"Research Agent"}))
    .toBe("Agent · Research Agent");
  expect(toolSourceLabel({...provider,toolSourceKind:"sandbox",toolSourceId:"sbx_1",toolSourceName:"Development Sandbox",pluginId:"io.mender.provider.workspace"}))
    .toBe("Sandbox · Development Sandbox · Workspace Tool Suite");
  expect(toolSourceLabel({...provider,toolSourceKind:"sandbox",toolSourceId:"sbx_1",toolSourceName:"SSH",pluginId:"io.mender.provider.sandbox-jobs"}))
    .toBe("Sandbox · SSH · Jobs Tool Suite");
});
it("marks unclaimed technical adapters and preserves direct Provider names",()=>{
  expect(toolSourceLabel({...provider,toolSourceKind:"unclaimed"})).toBe("Unclaimed tool source");
  expect(toolSourceLabel({...provider,toolSourceKind:"provider",toolSourceName:"GitHub MCP"})).toBe("GitHub MCP");
  expect(toolSourceLabel(undefined)).toBe("Unknown tool source");
});
