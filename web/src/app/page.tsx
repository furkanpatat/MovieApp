"use client";

import { useEffect, useMemo } from "react";
import { TriangleAlert } from "lucide-react";

import { Button } from "@/components/ui/button";
import { MovieHero } from "@/components/movies/movie-hero";
import { MovieRow, MovieRowSkeleton } from "@/components/movies/movie-row";
import { usePopularMovies } from "@/hooks/queries";
import type { Movie } from "@/types/movie";

const HERO_COUNT = 5;
const ROW_LENGTH = 12;
// Enough titles to fill every row; the catalog serves 20 per page.
const PAGES_NEEDED = 2;

/**
 * Builds the home rails from the popular list. The catalog only exposes
 * /movies/popular today, so rows are derived client-side: "Top Rated" and
 * "New Releases" are real re-sorts of that list; "Action & Adventure" is a
 * placeholder slice until the catalog can filter by genre.
 */
function buildRows(movies: Movie[]) {
  const byRating = [...movies]
    .filter((m) => m.vote_count >= 100) // a 10/10 from 3 votes isn't "top rated"
    .sort((a, b) => b.vote_average - a.vote_average);
  const byRelease = [...movies]
    .filter((m) => m.release_date)
    .sort((a, b) => b.release_date!.localeCompare(a.release_date!));

  return [
    { title: "Trending Now", movies: movies.slice(0, ROW_LENGTH) },
    { title: "Top Rated", movies: byRating.slice(0, ROW_LENGTH) },
    { title: "New Releases", movies: byRelease.slice(0, ROW_LENGTH) },
    // TODO: replace with a genre query once the catalog supports one.
    { title: "Action & Adventure", movies: movies.slice(ROW_LENGTH, ROW_LENGTH * 2) },
    { title: "More to Explore", movies: movies.slice(ROW_LENGTH * 2, ROW_LENGTH * 3) },
  ];
}

export default function Home() {
  const { data, status, error, refetch, fetchNextPage, hasNextPage, isFetchingNextPage } = usePopularMovies();

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

  const rows = useMemo(() => buildRows(movies), [movies]);

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
          <h2 className="text-lg font-semibold">Couldn&apos;t load movies</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {error instanceof Error ? error.message : "The catalog is unreachable."}
          </p>
        </div>
        <Button onClick={() => refetch()} variant="secondary">
          Try again
        </Button>
      </div>
    );
  }

  return (
    <div className="flex flex-1 flex-col">
      <MovieHero movies={movies.slice(0, HERO_COUNT)} />

      <div className="flex flex-col gap-4 py-8 sm:gap-6">
        {rows.map((row) => (
          <MovieRow key={row.title} title={row.title} movies={row.movies} />
        ))}
        {loadedPages < PAGES_NEEDED && isFetchingNextPage && <MovieRowSkeleton />}
      </div>
    </div>
  );
}
