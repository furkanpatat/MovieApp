"use client";

import { useEffect, useMemo } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { TriangleAlert } from "lucide-react";

import { Button } from "@/components/ui/button";
import { MovieHero } from "@/components/movies/movie-hero";
import { MovieRow, MovieRowSkeleton } from "@/components/movies/movie-row";
import { usePopularTitles } from "@/hooks/queries";
import { useT } from "@/i18n";
import { useMediaMode } from "@/store/media-mode-store";
import type { MediaType, Movie } from "@/types/movie";

const HERO_COUNT = 5;
const ROW_LENGTH = 12;
// Enough titles to fill every row; the catalog serves 20 per page.
const PAGES_NEEDED = 2;


/**
 * Builds the home rails from the popular list (movies or series). Rows are
 * derived client-side: "Top Rated" and "New Releases" are real re-sorts of
 * that list; the fourth row is a placeholder slice until the catalog can
 * filter by genre.
 */
function buildRows(movies: Movie[], mode: MediaType, t: ReturnType<typeof useT>["t"]) {
  // Row titles per mode (the logo switches between movies and series).
  const row = (name: "trending" | "topRated" | "fresh" | "fourth" | "more") => t(`home.rows.${mode}.${name}`);
  const [trending, topRated, fresh, fourth, more] = [row("trending"), row("topRated"), row("fresh"), row("fourth"), row("more")];
  const byRating = [...movies]
    .filter((m) => m.vote_count >= 100) // a 10/10 from 3 votes isn't "top rated"
    .sort((a, b) => b.vote_average - a.vote_average);
  const byRelease = [...movies]
    .filter((m) => m.release_date)
    .sort((a, b) => b.release_date!.localeCompare(a.release_date!));

  return [
    { title: trending, movies: movies.slice(0, ROW_LENGTH) },
    { title: topRated, movies: byRating.slice(0, ROW_LENGTH) },
    { title: fresh, movies: byRelease.slice(0, ROW_LENGTH) },
    // TODO: replace with a genre query (the catalog supports /discover now).
    { title: fourth, movies: movies.slice(ROW_LENGTH, ROW_LENGTH * 2) },
    { title: more, movies: movies.slice(ROW_LENGTH * 2, ROW_LENGTH * 3) },
  ];
}

export default function Home() {
  // Movies or series, per the logo switch; wait for the saved mode.
  const { mode, ready } = useMediaMode();
  const { data, status, error, refetch, fetchNextPage, hasNextPage, isFetchingNextPage } = usePopularTitles(mode, ready);

  const loadedPages = data?.pages.length ?? 0;
  useEffect(() => {
    if (status === "success" && loadedPages < PAGES_NEEDED && hasNextPage && !isFetchingNextPage) {
      fetchNextPage();
    }
  }, [status, loadedPages, hasNextPage, isFetchingNextPage, fetchNextPage]);

  // Popularity shifts between page requests, so the same title can appear
  // on two pages; rows key cards by id, so drop the repeats.
  const movies = useMemo(() => {
    const seen = new Set<number>();
    return (data?.pages ?? [])
      .flatMap((p) => p.results)
      .filter((m) => !seen.has(m.id) && seen.add(m.id));
  }, [data]);

  const { t } = useT();
  const rows = useMemo(() => buildRows(movies, mode, t), [movies, mode, t]);

  if (status === "pending") {
    return (
      <div className="flex flex-1 flex-col">
        <div className="h-[56vh] min-h-[420px] w-full animate-pulse bg-secondary/40 sm:h-[64vh]" />
        <div className="flex flex-col gap-6 py-8">
          <MovieRowSkeleton />
          <MovieRowSkeleton />
        </div>
      </div>
    );
  }

  if (status === "error") {
    return (
      <div className="flex flex-1 flex-col items-center justify-center gap-4 px-6 text-center">
        <TriangleAlert className="size-10 text-destructive" strokeWidth={1.5} />
        <div>
          <h2 className="text-lg font-semibold">{t(mode === "tv" ? "home.loadFailedSeries" : "home.loadFailedMovies")}</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {error instanceof Error ? error.message : t("common.catalogUnreachable")}
          </p>
        </div>
        <Button onClick={() => refetch()} variant="secondary">
          {t("common.tryAgain")}
        </Button>
      </div>
    );
  }

  // Switching modes crossfades the whole page into the other catalog.
  return (
    <AnimatePresence mode="wait" initial={false}>
      <motion.div
        key={mode}
        className="flex flex-1 flex-col"
        initial={{ opacity: 0, y: 12 }}
        animate={{ opacity: 1, y: 0 }}
        exit={{ opacity: 0, y: -8 }}
        transition={{ duration: 0.3, ease: "easeOut" }}
      >
        <MovieHero movies={movies.slice(0, HERO_COUNT)} />

        <div className="flex flex-col gap-4 py-8 sm:gap-6">
          {rows.map((row) => (
            <MovieRow key={row.title} title={row.title} movies={row.movies} />
          ))}
          {loadedPages < PAGES_NEEDED && isFetchingNextPage && <MovieRowSkeleton />}
        </div>
      </motion.div>
    </AnimatePresence>
  );
}
