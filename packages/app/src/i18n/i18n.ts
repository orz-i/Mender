import { useSyncExternalStore } from "react";

import { zhCNMessages } from "./zh-cn";

export const supportedLocales = ["en", "zh-CN"] as const;

export type Locale = (typeof supportedLocales)[number];
export type TranslationValues = Readonly<Record<string, string | number>>;

const STORAGE_KEY = "mender.locale";
const listeners = new Set<() => void>();

let currentLocale: Locale | undefined;

function localeCandidate(value: string | null | undefined): Locale | undefined {
  const normalized = value?.trim().toLowerCase();

  if (!normalized) {
    return undefined;
  }

  if (
    normalized === "zh" ||
    normalized === "zh-cn" ||
    normalized.startsWith("zh-cn-") ||
    normalized === "zh-sg" ||
    normalized.startsWith("zh-sg-") ||
    normalized.includes("-hans")
  ) {
    return "zh-CN";
  }

  if (normalized === "en" || normalized.startsWith("en-")) {
    return "en";
  }

  return undefined;
}

export function resolveLocale(value: string | null | undefined): Locale {
  return localeCandidate(value) ?? "en";
}

function resolveInitialLocale(): Locale {
  if (typeof window === "undefined") {
    return "en";
  }

  const stored = window.localStorage.getItem(STORAGE_KEY);
  if (stored === "en" || stored === "zh-CN") {
    return stored;
  }

  for (const language of window.navigator.languages ?? []) {
    const candidate = localeCandidate(language);
    if (candidate !== undefined) {
      return candidate;
    }
  }

  return resolveLocale(window.navigator.language);
}

function syncDocumentLanguage(locale: Locale) {
  if (typeof document !== "undefined") {
    document.documentElement.lang = locale;
  }
}

export function getLocale(): Locale {
  currentLocale ??= resolveInitialLocale();
  syncDocumentLanguage(currentLocale);
  return currentLocale;
}

export function setLocale(locale: Locale) {
  if (getLocale() === locale) {
    syncDocumentLanguage(locale);
    if (typeof window !== "undefined") {
      window.localStorage.setItem(STORAGE_KEY, locale);
    }
    return;
  }

  currentLocale = locale;
  syncDocumentLanguage(locale);

  if (typeof window !== "undefined") {
    window.localStorage.setItem(STORAGE_KEY, locale);
  }

  for (const listener of listeners) {
    listener();
  }
}

export function useLocale(): Locale {
  return useSyncExternalStore(
    (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    getLocale,
    () => "en",
  );
}

export function t(message: string, values: TranslationValues = {}): string {
  const translated =
    getLocale() === "zh-CN" ? (zhCNMessages[message] ?? message) : message;

  return translated.replace(/\{\{(\w+)\}\}/g, (token, name: string) => {
    const value = values[name];
    return value === undefined ? token : String(value);
  });
}

export function formatNumber(value: number): string {
  return new Intl.NumberFormat(getLocale()).format(value);
}

export function formatDateTime(
  value: string | number | Date,
): string {
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) {
    return String(value);
  }

  return new Intl.DateTimeFormat(getLocale(), {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(date);
}

export function tCount(
  count: number,
  one: string,
  other: string,
  values: TranslationValues = {},
): string {
  const category = new Intl.PluralRules(getLocale()).select(count);
  return t(category === "one" ? one : other, {
    ...values,
    count: formatNumber(count),
  });
}

/** @internal Test-only reset so locale resolution can be verified deterministically. */
export function resetLocaleForTesting() {
  currentLocale = undefined;
  listeners.clear();
}
