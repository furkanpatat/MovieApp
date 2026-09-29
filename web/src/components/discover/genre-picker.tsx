"use client";

import { useState } from "react";
import { motion } from "framer-motion";
import { Check, ChevronDown } from "lucide-react";
import { Popover as PopoverPrimitive } from "radix-ui";

import { genreName, useT } from "@/i18n";
import { cn } from "@/lib/utils";

import type { MediaType } from "@/types/movie";

/** TMDB's movie genres (their ids are stable), in feed-friendly order. */
const MOVIE_GENRES = [
  { id: 0, name: "For You" },
  { id: 28, name: "Action" },
  { id: 878, name: "Sci-Fi" },
  { id: 18, name: "Drama" },
  { id: 35, name: "Comedy" },
  { id: 53, name: "Thriller" },
  { id: 27, name: "Horror" },
  { id: 12, name: "Adventure" },
  { id: 16, name: "Animation" },
  { id: 80, name: "Crime" },
  { id: 14, name: "Fantasy" },
  { id: 10749, name: "Romance" },
  { id: 9648, name: "Mystery" },
  { id: 10751, name: "Family" },
  { id: 36, name: "History" },
  { id: 10752, name: "War" },
  { id: 37, name: "Western" },
  { id: 99, name: "Documentary" },
];

/** TMDB's TV genres: their own ids (and fewer, broader ones). Talk, news and
 *  reality aren't offered: the catalog keeps them out of the TV feed. */
const TV_GENRES = [
  { id: 0, name: "For You" },
  { id: 18, name: "Drama" },
  { id: 10765, name: "Sci-Fi & Fantasy" },
  { id: 10759, name: "Action & Adventure" },
  { id: 35, name: "Comedy" },
  { id: 80, name: "Crime" },
  { id: 9648, name: "Mystery" },
  { id: 16, name: "Animation" },
  { id: 10768, name: "War & Politics" },
  { id: 10751, name: "Family" },
  { id: 10762, name: "Kids" },
  { id: 99, name: "Documentary" },
  { id: 37, name: "Western" },
];

export const GENRES: Record<MediaType, { id: number; name: string }[]> = { movie: MOVIE_GENRES, tv: TV_GENRES };

/**
 * The Discover feed's filter, kept out of the video's way: one small glass
 * pill (top centre, under the nav) naming the active genre. It opens a glass
 * popover with every genre as a grid of pills; picking one switches the feed
 * and closes it.
 */
export function GenrePicker({ mode, value, onChange }: { mode: MediaType; value: number; onChange: (id: number) => void }) {
  const [open, setOpen] = useState(false);
  const genres = GENRES[mode];
  const { t, locale } = useT();
  // Names from the dictionary by TMDB id ("For You" is ours: id 0).
  const label = (g: { id: number; name: string }) => (g.id === 0 ? t("discover.forYou") : genreName(locale, g));
  const current = genres.find((g) => g.id === value) ?? genres[0];

  return (
    <div className="pointer-events-none fixed inset-x-0 top-16 z-40 mt-4 flex justify-center">
      <PopoverPrimitive.Root open={open} onOpenChange={setOpen}>
        <PopoverPrimitive.Trigger asChild>
          <motion.button
            type="button"
            aria-label={t("discover.genre", { name: label(current) })}
            initial={{ opacity: 0, y: -6 }}
            animate={{ opacity: 1, y: 0 }}
            whileTap={{ scale: 0.95 }}
            transition={{ type: "spring", stiffness: 400, damping: 28 }}
            className="pointer-events-auto flex items-center gap-1.5 rounded-full border border-white/15 bg-black/40 px-4 py-1.5 text-sm font-medium text-white shadow-lg shadow-black/20 backdrop-blur-md transition-colors outline-none hover:bg-black/60 focus-visible:ring-2 focus-visible:ring-white/60"
          >
            {label(current)}
            <ChevronDown className={cn("size-4 text-white/70 transition-transform duration-200", open && "rotate-180")} />
          </motion.button>
        </PopoverPrimitive.Trigger>

        <PopoverPrimitive.Portal>
          <PopoverPrimitive.Content
            side="bottom"
            align="center"
            sideOffset={10}
            collisionPadding={12}
            className="z-50 w-72 origin-(--radix-popover-content-transform-origin) rounded-2xl border border-white/10 bg-zinc-950/90 p-3 shadow-2xl shadow-black/60 backdrop-blur-2xl outline-none data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95"
          >
            <p className="px-1 pb-2 text-[11px] font-semibold tracking-[0.2em] text-white/40 uppercase">
              {t(mode === "tv" ? "discover.seriesGenre" : "discover.movieGenre")}
            </p>
            <div role="listbox" aria-label={t("discover.genres")} className="grid grid-cols-2 gap-1.5">
              {genres.map((g) => {
                const selected = g.id === value;
                return (
                  <button
                    key={g.id}
                    type="button"
                    role="option"
                    aria-selected={selected}
                    onClick={() => {
                      onChange(g.id);
                      setOpen(false);
                    }}
                    className={cn(
                      "flex items-center justify-between rounded-full px-3 py-1.5 text-left text-sm transition-colors outline-none focus-visible:ring-2 focus-visible:ring-white/60",
                      selected
                        ? "bg-white font-semibold text-black"
                        : "bg-white/[0.04] font-medium text-white/70 hover:bg-white/10 hover:text-white",
                    )}
                  >
                    <span className="truncate">{label(g)}</span>
                    {selected && <Check className="size-3.5 shrink-0" strokeWidth={2.5} />}
                  </button>
                );
              })}
            </div>
          </PopoverPrimitive.Content>
        </PopoverPrimitive.Portal>
      </PopoverPrimitive.Root>
    </div>
  );
}
