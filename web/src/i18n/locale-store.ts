"use client";

import { create } from "zustand";
import { persist } from "zustand/middleware";

export type Locale = "en" | "tr";
export const LOCALES: Locale[] = ["en", "tr"];

/**
 * The UI language (TR | EN in the header), kept in localStorage. Not in the
 * URL: route prefixes would fight the @modal intercepting routes, and the
 * language is a per-person preference rather than part of a shareable link.
 */
export const useLocaleStore = create<{ locale: Locale; setLocale: (l: Locale) => void }>()(
  persist(
    (set) => ({
      locale: "en",
      setLocale: (locale) => set({ locale }),
    }),
    { name: "movieapp-locale" },
  ),
);
