import { afterEach, beforeEach, describe, expect, it } from "vitest";

import {
  MenderApiError,
  MenderClientInvariantError,
} from "@mender/sdk";

import { resetLocaleForTesting, setLocale } from "../i18n/i18n";
import { errorMessage } from "./errors";

describe("errorMessage", () => {
  beforeEach(() => {
    window.localStorage.clear();
    resetLocaleForTesting();
    setLocale("en");
  });

  afterEach(() => {
    setLocale("en");
    window.localStorage.clear();
    resetLocaleForTesting();
  });

  it("preserves detailed diagnostics in English", () => {
    expect(errorMessage(new Error("socket exploded"))).toBe("socket exploded");
    expect(
      errorMessage(
        new MenderClientInvariantError("Unsupported desktop policy mode: legacy"),
      ),
    ).toBe("Unsupported desktop policy mode: legacy");
    expect(
      errorMessage(new MenderApiError(404, "not_found", "tool not found")),
    ).toBe("tool not found");
  });

  it("localizes API and desktop invariant errors in Simplified Chinese", () => {
    setLocale("zh-CN");

    expect(
      errorMessage(new MenderApiError(404, "not_found", "tool not found")),
    ).toBe("未找到请求的项目。");
    expect(
      errorMessage(
        new MenderClientInvariantError("Unsupported desktop policy mode: legacy"),
      ),
    ).toBe("桌面端返回的数据与当前 Mender 版本不兼容。");
  });

  it("falls back to a localized generic error instead of leaking unknown English messages", () => {
    setLocale("zh-CN");

    expect(errorMessage(new Error("socket exploded"))).toBe("发生意外错误");
    expect(errorMessage(new Error("发生意外错误"))).toBe("发生意外错误");
    expect(errorMessage(null)).toBe("发生意外错误");
  });
});
