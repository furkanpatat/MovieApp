"use client";

import { memo, useCallback, useEffect, useRef, useState } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";

import { MovieCard } from "@/components/movies/movie-card";
import { MovieCardSkeleton } from "@/components/movies/movie-card-skeleton";
import { Skeleton } from "@/components/ui/skeleton";
import type { Movie } from "@/types/movie";

// Card width per breakpoint: ~2.3 cards visible on phones up to ~6.5 on wide
// screens, so a partially visible card always hints that the row scrolls.
const ITEM = "w-[42vw] shrink-0 snap-start sm:w-[29vw] md:w-[21vw] lg:w-[17vw] xl:w-[14.5vw] 2xl:w-[12.5vw]";

/**
 * A titled, horizontally scrolling rail of MovieCards (Netflix-style).
 *
 * The scroller has vertical padding so the cards' hover scale-up isn't
 * clipped: overflow-x:auto forces overflow-y to clip as well. The big
 * expanded previews don't need that room; MovieCard renders them
 * position:fixed, outside the row's clipping. For the same reason nothing
 * here may use transform / will-change: transform / filter, which would
 * trap those fixed panels inside the row again.
 */
export const MovieRow = memo(function MovieRow({ title, movies }: { title: string; movies: Movie[] }) {
  const scrollerRef = useRef<HTMLDivElement>(null);
  const [canScroll, setCanScroll] = useState({ left: false, right: false });

  const updateArrows = useCallback(() => {
    const el = scrollerRef.current;
    if (!el) return;
    setCanScroll({
      left: el.scrollLeft > 4,
      right: el.scrollLeft + el.clientWidth < el.scrollWidth - 4,
    });
  }, []);

  useEffect(() => {
    const el = scrollerRef.current;
    if (!el) return;
    const ro = new ResizeObserver(updateArrows);
    ro.observe(el);
    return () => ro.disconnect();
  }, [updateArrows]);

  const page = (dir: -1 | 1) => {
    const el = scrollerRef.current;
    el?.scrollBy({ left: dir * el.clientWidth * 0.85, behavior: "smooth" });
  };

  if (movies.length === 0) return null;

  return (
    <section className="group/row relative" aria-label={title}>
      <h2 className="mb-1 px-4 text-lg font-bold tracking-tight sm:px-8 sm:text-xl">{title}</h2>

      <div
        ref={scrollerRef}
        onScroll={updateArrows}
        className="hide-scrollbar flex snap-x snap-mandatory scroll-px-4 gap-4 overflow-x-auto px-4 py-4 sm:scroll-px-8 sm:px-8"
      >
        {movies.map((movie) => (
          <div key={movie.id} className={ITEM}>
            <MovieCard movie={movie} />
          </div>
        ))}
      </div>

      {/* Paging arrows for mouse users (touch just swipes). Shown on row hover. */}
      {(["left", "right"] as const).map((side) =>
        canScroll[side] ? (
          <button
            key={side}
            type="button"
            aria-label={`Scroll ${title} ${side}`}
            onClick={() => page(side === "left" ? -1 : 1)}
            className={`absolute bottom-12 top-11 z-20 hidden w-8 items-center justify-center bg-background/70 text-white opacity-0 transition-opacity hover:bg-background/90 focus-visible:opacity-100 group-hover/row:opacity-100 md:flex ${
              side === "left" ? "left-0 rounded-r-md" : "right-0 rounded-l-md"
            }`}
          >
            {side === "left" ? <ChevronLeft className="size-6" /> : <ChevronRight className="size-6" />}
          </button>
        ) : null,
      )}
    </section>
  );
});

export function MovieRowSkeleton() {
  return (
    <div>
      <Skeleton className="mx-4 mb-1 h-6 w-48 rounded sm:mx-8" />
      <div className="flex gap-4 overflow-hidden px-4 py-4 sm:px-8">
        {Array.from({ length: 8 }).map((_, i) => (
          <div key={i} className={ITEM}>
            <MovieCardSkeleton />
          </div>
        ))}
      </div>
    </div>
  );
}
