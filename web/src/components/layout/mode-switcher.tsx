"use client";

import { usePathname, useRouter } from "next/navigation";
import { AnimatePresence, motion } from "framer-motion";
import { ArrowLeftRight, Clapperboard, Tv } from "lucide-react";

import { useT } from "@/i18n";
import { cn } from "@/lib/utils";
import { useMediaMode, useMediaModeStore } from "@/store/media-mode-store";

// Pages whose content follows the mode: switching there transforms them in
// place; anywhere else (a person, My List...) it takes you home.
const MODE_PAGES = ["/", "/discover", "/search"];

const BRAND = {
  movie: { icon: Clapperboard, word: "Movie", next: "mode.series" },
  tv: { icon: Tv, word: "Series", next: "mode.movies" },
} as const;

const FLIP = {
  initial: { rotateX: -90, opacity: 0, y: 6 },
  animate: { rotateX: 0, opacity: 1, y: 0 },
  exit: { rotateX: 90, opacity: 0, y: -6 },
  transition: { type: "spring", stiffness: 380, damping: 28 },
} as const;

/**
 * The brand logo, which is also the app's Movies <-> Series switch:
 * MovieApp (clapperboard) and SeriesApp (TV) flip into each other. On hover
 * a pill outline and a swap badge show it can be clicked; two dots mark the
 * active mode.
 */
export function ModeSwitcher({ className }: { className?: string }) {
  const { mode, ready } = useMediaMode();
  const toggle = useMediaModeStore((s) => s.toggle);
  const pathname = usePathname();
  const router = useRouter();
  const { t } = useT();
  const brand = BRAND[ready ? mode : "movie"];
  const next = t(brand.next);
  const Icon = brand.icon;

  return (
    <button
      type="button"
      onClick={() => {
        toggle();
        if (!MODE_PAGES.includes(pathname)) router.push("/");
      }}
      aria-label={t("mode.switchTo", { brand: brand.word, next })}
      aria-pressed={mode === "tv"}
      title={t("mode.switchTitle", { next })}
      className={cn(
        "group relative flex shrink-0 items-center gap-2 rounded-full py-1 pr-3 pl-1 ring-1 ring-transparent transition-[background-color,box-shadow] duration-300 outline-none hover:bg-white/[0.06] hover:ring-white/15 focus-visible:ring-2 focus-visible:ring-primary",
        className,
      )}
      style={{ perspective: 600 }}
    >
      <span className="relative flex size-8 items-center justify-center">
        <AnimatePresence mode="popLayout" initial={false}>
          <motion.span
            key={mode + "-icon"}
            {...FLIP}
            className="absolute inset-0 flex items-center justify-center rounded-md bg-primary text-primary-foreground shadow-md shadow-primary/20"
          >
            <Icon className="size-5" strokeWidth={2.25} />
          </motion.span>
        </AnimatePresence>
        {/* The swap hint, on hover. */}
        <span className="absolute -right-1.5 -bottom-1.5 flex size-4 scale-50 items-center justify-center rounded-full bg-zinc-950 text-white opacity-0 ring-1 ring-white/20 transition-all duration-200 group-hover:scale-100 group-hover:opacity-100 group-focus-visible:scale-100 group-focus-visible:opacity-100">
          <ArrowLeftRight className="size-2.5" strokeWidth={2.5} />
        </span>
      </span>

      <span className="relative flex items-baseline text-lg font-bold tracking-tight">
        <AnimatePresence mode="popLayout" initial={false}>
          <motion.span key={mode + "-word"} {...FLIP} className="inline-block">
            {brand.word}
          </motion.span>
        </AnimatePresence>
        <motion.span layout="position" className="text-primary">
          App
        </motion.span>
      </span>

      {/* Which mode is on: movie dot, series dot. */}
      <span className="flex flex-col gap-0.5" aria-hidden>
        {(["movie", "tv"] as const).map((m) => (
          <span
            key={m}
            className={cn(
              "size-1 rounded-full transition-colors duration-300",
              (ready ? mode : "movie") === m ? "bg-primary" : "bg-white/20",
            )}
          />
        ))}
      </span>
    </button>
  );
}
