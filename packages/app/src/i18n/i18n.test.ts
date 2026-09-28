import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  getLocale,
  resetLocaleForTesting,
  resolveLocale,
  setLocale,
  t,
} from "./i18n";

describe("i18n", () => {
  beforeEach(() => {
    window.localStorage.clear();
    document.documentElement.removeAttribute("lang");
    resetLocaleForTesting();
  });

  afterEach(() => {
    setLocale("en");
    window.localStorage.clear();
    resetLocaleForTesting();
  });

  it("normalizes Chinese locales and falls back to English", () => {
    expect(resolveLocale("zh-CN")).toBe("zh-CN");
    expect(resolveLocale("zh-TW")).toBe("zh-CN");
    expect(resolveLocale("en-GB")).toBe("en");
    expect(resolveLocale("fr-FR")).toBe("en");
    expect(resolveLocale(undefined)).toBe("en");
  });

  it("restores an explicit persisted locale", () => {
    window.localStorage.setItem("mender.locale", "zh-CN");
    resetLocaleForTesting();

    expect(getLocale()).toBe("zh-CN");
    expect(document.documentElement.lang).toBe("zh-CN");
  });

  it("uses the browser language when no explicit locale is stored", () => {
    vi.spyOn(window.navigator, "languages", "get").mockReturnValue([
      "zh-CN",
      "en-US",
    ]);
    resetLocaleForTesting();

    expect(getLocale()).toBe("zh-CN");
    expect(document.documentElement.lang).toBe("zh-CN");
  });

  it("persists locale changes, updates html lang, and interpolates messages", () => {
    setLocale("zh-CN");

    expect(window.localStorage.getItem("mender.locale")).toBe("zh-CN");
    expect(document.documentElement.lang).toBe("zh-CN");
    expect(t("Rows {{from}}–{{to}} of {{total}}", {
      from: 1,
      to: 10,
      total: 24,
    })).toBe("第 1–10 行，共 24 行");

    setLocale("en");
    expect(t("Missing translation fallback")).toBe("Missing translation fallback");
  });
});
