import type { Provider } from "@mender/sdk";
import { afterEach, beforeEach, expect, it } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { toolSourceLabel } from "./tool-source-label";

beforeEach(() => { resetLocaleForTesting(); setLocale("en"); });
afterEach(() => resetLocaleForTesting());

const provider = { providerId:"prov_1",displayName:"Opaque internal name",namespace:"opaque" } as Provider;
it("renders only independently configured Provider names", () => {
  expect(toolSourceLabel({...provider,toolSourceKind:"provider",toolSourceName:"GitHub MCP"})).toBe("GitHub MCP");
  expect(toolSourceLabel({...provider,displayName:"Owner selected name"})).toBe("Owner selected name");
});
it("marks unclaimed technical adapters and preserves direct Provider names",()=>{
  expect(toolSourceLabel({...provider,toolSourceKind:"unclaimed"})).toBe("Opaque internal name");
  expect(toolSourceLabel({...provider,toolSourceKind:"provider",toolSourceName:"GitHub MCP"})).toBe("GitHub MCP");
  expect(toolSourceLabel(undefined)).toBe("Service unavailable");
});
