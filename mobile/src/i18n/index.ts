import * as SecureStore from "expo-secure-store";
import { create } from "zustand";

import { en } from "@/i18n/en";
import { tr } from "@/i18n/tr";
import { deviceLocale } from "@/lib/locale";

export type Locale = "en" | "tr";

const KEY = "kinocut.locale";

/**
 * The app's language: the phone's (Turkish or English) until the user
 * picks one in Profile; the pick is kept on the device.
 */
export const useLocale = create<{ locale: Locale; setLocale: (l: Locale) => void; restore: () => Promise<void> }>((set) => ({
  locale: deviceLocale(),
  setLocale: (locale) => {
    set({ locale });
    void SecureStore.setItemAsync(KEY, locale).catch(() => {});
  },
  restore: async () => {
    try {
      const saved = await SecureStore.getItemAsync(KEY);
      if (saved === "en" || saved === "tr") set({ locale: saved });
    } catch {
      // Keep the phone's language.
    }
  },
}));

/** The dictionary for the current language, and the language itself. */
export function useT() {
  const locale = useLocale((s) => s.locale);
  return { t: locale === "tr" ? tr : en, locale };
}
