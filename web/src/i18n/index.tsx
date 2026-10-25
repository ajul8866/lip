"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import type { Locale } from "@/lib/types";
import en from "./locales/en.json";
import ko from "./locales/ko.json";

type Dictionary = Record<string, string>;

const dictionaries: Record<Locale, Dictionary> = { en, ko };

interface I18nContextValue {
  locale: Locale;
  setLocale: (locale: Locale, persistCookie?: boolean) => void;
  t: (key: string, vars?: Record<string, string | number>) => string;
  formatDate: (value: string | Date, options?: Intl.DateTimeFormatOptions) => string;
  formatNumber: (value: number) => string;
  localeTag: string;
}

const I18nContext = createContext<I18nContextValue | null>(null);

function readCookie(name: string): string | undefined {
  if (typeof document === "undefined") return undefined;
  const prefix = `${name}=`;
  for (const part of document.cookie.split(";")) {
    const trimmed = part.trim();
    if (trimmed.startsWith(prefix)) {
      return decodeURIComponent(trimmed.slice(prefix.length));
    }
  }
  return undefined;
}

function persistLocale(locale: Locale) {
  if (typeof document === "undefined") return;
  document.cookie = `lip_locale=${locale}; Path=/; Max-Age=31536000; SameSite=Lax`;
}

export function I18nProvider({ children }: { children: React.ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>("en");

  useEffect(() => {
    const fromCookie = readCookie("lip_locale");
    const browser = navigator.language.toLowerCase().startsWith("ko") ? "ko" : "en";
    const initial = fromCookie === "ko" || fromCookie === "en" ? fromCookie : browser;
    setLocaleState(initial);
  }, []);

  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);

  const setLocale = useCallback((next: Locale, persistCookie = true) => {
    setLocaleState(next);
    if (persistCookie) persistLocale(next);
  }, []);

  const t = useCallback(
    (key: string, vars?: Record<string, string | number>) => {
      const raw = dictionaries[locale][key] ?? dictionaries.en[key] ?? key;
      if (!vars) return raw;
      return raw.replace(/\{(\w+)\}/g, (match, name: string) =>
        Object.prototype.hasOwnProperty.call(vars, name) ? String(vars[name]) : match,
      );
    },
    [locale],
  );

  const value = useMemo<I18nContextValue>(() => {
    const tag = locale === "ko" ? "ko-KR" : "en-US";
    return {
      locale,
      setLocale,
      t,
      localeTag: tag,
      formatDate: (input, options) => {
        const date = typeof input === "string" ? new Date(input) : input;
        if (Number.isNaN(date.getTime())) return "—";
        return new Intl.DateTimeFormat(tag, options ?? { dateStyle: "medium", timeStyle: "short" }).format(date);
      },
      formatNumber: (number) => new Intl.NumberFormat(tag).format(number),
    };
  }, [locale, setLocale, t]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18nContextValue {
  const value = useContext(I18nContext);
  if (!value) throw new Error("useI18n must be used inside I18nProvider");
  return value;
}
