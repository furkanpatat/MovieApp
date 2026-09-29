"use client";

import { useCallback } from "react";

import { en, type Dictionary } from "@/i18n/dictionaries/en";
import { tr } from "@/i18n/dictionaries/tr";
import { useLocaleStore, type Locale } from "@/i18n/locale-store";

export { useLocaleStore, LOCALES, type Locale } from "@/i18n/locale-store";

const DICTIONARIES: Record<Locale, Dictionary> = { en, tr };

// "nav.discover" | "discover.rail.like" | ...: every leaf of the dictionary.
type Leaves<T, P extends string = ""> = {
  [K in keyof T & string]: T[K] extends string ? `${P}${K}` : Leaves<T[K], `${P}${K}.`>;
}[keyof T & string];
export type MessageKey = Leaves<Dictionary>;
export type Vars = Record<string, string | number>;

function lookup(dict: Dictionary, key: string): string {
  let node: unknown = dict;
  for (const part of key.split(".")) node = (node as Record<string, unknown>)?.[part];
  return typeof node === "string" ? node : key;
}

/** "{n} comments" + { n: 3 } -> "3 comments". */
function format(template: string, vars?: Vars): string {
  return vars ? template.replace(/\{(\w+)\}/g, (m, name) => (name in vars ? String(vars[name]) : m)) : template;
}

/** Translates `key` in `locale`, outside React (toasts, formatters). */
export function translate(locale: Locale, key: MessageKey, vars?: Vars): string {
  return format(lookup(DICTIONARIES[locale], key), vars);
}

/** The current locale's `t`. Components using it re-render on a switch. */
export function useT() {
  const locale = useLocaleStore((s) => s.locale);
  const t = useCallback((key: MessageKey, vars?: Vars) => translate(locale, key, vars), [locale]);
  return { t, locale };
}

/** The locale outside React (for non-component helpers). */
export const currentLocale = (): Locale => useLocaleStore.getState().locale;

/** Pick "one" or "other" (English plurals; Turkish uses "other" forms that
 *  don't inflect after numbers, so its dictionary has both the same). */
export function plural(n: number, one: MessageKey, other: MessageKey): MessageKey {
  return n === 1 ? one : other;
}

/** A TMDB genre's name in `locale`, by id (movie and TV ids); TMDB's own
 *  English name when we don't know the id. */
export function genreName(locale: Locale, genre: { id: number; name: string }): string {
  const names = DICTIONARIES[locale].genres as Record<string, string>;
  return names[String(genre.id)] ?? genre.name;
}
