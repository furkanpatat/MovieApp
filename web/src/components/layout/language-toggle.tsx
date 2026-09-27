"use client";

import { useEffect } from "react";
import { motion } from "framer-motion";

import { LOCALES, useLocaleStore, useT, type Locale } from "@/i18n";
import { cn } from "@/lib/utils";

const LABEL: Record<Locale, string> = { en: "EN", tr: "TR" };
const NAME: Record<Locale, string> = { en: "English", tr: "Türkçe" };

/**
 * TR | EN: a compact segmented switch for the UI language. The active side
 * is a sliding pill (shared layoutId). Instant: strings come from in-memory
 * dictionaries, no navigation.
 */
export function LanguageToggle({ className }: { className?: string }) {
  const { t, locale } = useT();
  const setLocale = useLocaleStore((s) => s.setLocale);
  return (
    <div
      role="radiogroup"
      aria-label={t("nav.language")}
      className={cn("flex items-center rounded-full bg-white/5 p-0.5 text-[11px] font-bold ring-1 ring-white/10", className)}
    >
      {LOCALES.map((l) => (
        <button
          key={l}
          type="button"
          role="radio"
          aria-checked={locale === l}
          aria-label={t("nav.switchLanguage", { lang: NAME[l] })}
          lang={l}
          onClick={() => setLocale(l)}
          className={cn(
            "relative rounded-full px-2 py-1 tracking-wide transition-colors outline-none focus-visible:ring-2 focus-visible:ring-primary",
            locale === l ? "text-primary-foreground" : "text-white/60 hover:text-white",
          )}
        >
          {locale === l && (
            <motion.span layoutId="locale-pill" className="absolute inset-0 rounded-full bg-primary" transition={{ type: "spring", stiffness: 500, damping: 34 }} />
          )}
          <span className="relative">{LABEL[l]}</span>
        </button>
      ))}
    </div>
  );
}

/** Keeps <html lang> on the UI language (screen readers, hyphenation), and
 *  on a first visit (nothing chosen yet) starts from the browser's language:
 *  the switch lives in the account menu and profile, out of a visitor's reach. */
export function HtmlLang() {
  const locale = useLocaleStore((s) => s.locale);
  useEffect(() => {
    let chosen = true;
    try {
      chosen = localStorage.getItem("movieapp-locale") !== null;
    } catch {
      // Storage blocked: keep the default.
    }
    if (!chosen && navigator.language.toLowerCase().startsWith("tr")) useLocaleStore.getState().setLocale("tr");
  }, []);
  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);
  return null;
}
