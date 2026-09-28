import { useSyncExternalStore } from "react";

import { zhCNMessages } from "./zh-cn";

export const supportedLocales = ["en", "zh-CN"] as const;

export type Locale = (typeof supportedLocales)[number];
export type TranslationValues = Readonly<Record<string, string | number>>;

const STORAGE_KEY = "mender.locale";
const listeners = new Set<() => void>();

let currentLocale: Locale | undefined;

export function resolveLocale(value: string | null | undefined): Locale {
  const normalized = value?.trim().toLowerCase();

  if (normalized?.startsWith("zh") === true) {
    return "zh-CN";
  }
  return "en";
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
    if (language.toLowerCase().startsWith("zh")) {
      return "zh-CN";
    }
    if (language.toLowerCase().startsWith("en")) {
      return "en";
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

/** @internal Test-only reset so locale resolution can be verified deterministically. */
export function resetLocaleForTesting() {
  currentLocale = undefined;
  listeners.clear();
}
