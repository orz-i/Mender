import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  formatDateTime,
  formatNumber,
  getLocale,
  resetLocaleForTesting,
  resolveLocale,
  setLocale,
  t,
  tCount,
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
    vi.restoreAllMocks();
  });

  it("normalizes supported locales without treating Traditional Chinese as Simplified Chinese", () => {
    expect(resolveLocale("zh-CN")).toBe("zh-CN");
    expect(resolveLocale("zh-SG")).toBe("zh-CN");
    expect(resolveLocale("zh-Hans")).toBe("zh-CN");
    expect(resolveLocale("zh-TW")).toBe("en");
    expect(resolveLocale("zh-HK")).toBe("en");
    expect(resolveLocale("zh-Hant")).toBe("en");
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

  it("uses the first supported browser language when no explicit locale is stored", () => {
    vi.spyOn(window.navigator, "languages", "get").mockReturnValue([
      "zh-TW",
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
    expect(
      t("Rows {{from}}–{{to}} of {{total}}", {
        from: 1,
        to: 10,
        total: 24,
      }),
    ).toBe("第 1–10 行，共 24 行");

    setLocale("en");
    expect(t("Missing translation fallback")).toBe("Missing translation fallback");
  });

  it("formats dates and numbers with the active locale", () => {
    const date = new Date("2026-09-28T20:05:06Z");

    setLocale("en");
    expect(formatNumber(12345.6)).toBe(
      new Intl.NumberFormat("en").format(12345.6),
    );
    expect(formatDateTime(date)).toBe(
      new Intl.DateTimeFormat("en", {
        dateStyle: "medium",
        timeStyle: "medium",
      }).format(date),
    );

    setLocale("zh-CN");
    expect(formatNumber(12345.6)).toBe(
      new Intl.NumberFormat("zh-CN").format(12345.6),
    );
    expect(formatDateTime(date)).toBe(
      new Intl.DateTimeFormat("zh-CN", {
        dateStyle: "medium",
        timeStyle: "medium",
      }).format(date),
    );
  });

  it("selects English singular/plural messages and keeps Chinese count rendering localized", () => {
    setLocale("en");
    expect(
      tCount(1, "{{count}} tool", "{{count}} tools"),
    ).toBe("1 tool");
    expect(
      tCount(2, "{{count}} tool", "{{count}} tools"),
    ).toBe("2 tools");

    setLocale("zh-CN");
    expect(
      tCount(2, "{{count}} tool", "{{count}} tools"),
    ).toBe("2 个工具");
  });
});
