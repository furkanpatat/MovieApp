import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { apiFetch } from "@/lib/api-client";
import { titleApiPath, titleKey, type TitleRef } from "@/lib/media";
import { useAuthStore } from "@/store/auth-store";
import { useLibraryStore } from "@/store/library-store";
import type { Comment, Interactions, MediaType, Movie, MoviePage, Person } from "@/types/movie";

/**
 * Popular movies, paginated by the Catalog service (GET /api/v1/movies/popular
 * — the Gateway routes /api/v1/movies/* straight through, no auth required).
 *
 * Backed by useInfiniteQuery rather than a plain per-page query: the Home
 * page wants "load more" to append to what's already rendered, not replace
 * it, and useInfiniteQuery is what gives us that (`data.pages`) plus
 * `fetchNextPage`/`hasNextPage` for free.
 */
export function usePopularMovies() {
  return useInfiniteQuery({
    queryKey: ["movies", "popular"],
    queryFn: ({ pageParam }) =>
      apiFetch<MoviePage>(`/api/v1/movies/popular?page=${pageParam}`, {
        auth: false,
      }),
    initialPageParam: 1,
    getNextPageParam: (lastPage) =>
      lastPage.page < lastPage.total_pages ? lastPage.page + 1 : undefined,
    // Catalog itself keeps a warm Redis cache (1h TTL, refreshed in the
    // background every 30m) — the whole point being sub-millisecond reads,
    // so there's no reason for the client to treat this as long-lived.
    staleTime: 60_000,
  });
}

/** Popular movies or series (media type `mode`), paged like
 *  usePopularMovies: GET /api/v1/{movies,tv}/popular. */
export function usePopularTitles(mode: MediaType, enabled: boolean = true) {
  return useInfiniteQuery({
    queryKey: [mode === "tv" ? "tv" : "movies", "popular"],
    queryFn: ({ pageParam }) =>
      apiFetch<MoviePage>(`/api/v1/${mode === "tv" ? "tv" : "movies"}/popular?page=${pageParam}`, { auth: false }),
    initialPageParam: 1,
    getNextPageParam: (lastPage) => (lastPage.page < lastPage.total_pages ? lastPage.page + 1 : undefined),
    staleTime: 60_000,
    enabled,
  });
}

/** An actor (or director...) and their movies: GET /api/v1/people/{id},
 *  served by the Catalog through the same Redis/Postgres caches as movies. */
export function usePerson(personId: number) {
  return useQuery({
    queryKey: ["person", personId],
    queryFn: () => apiFetch<Person>(`/api/v1/people/${personId}`, { auth: false }),
    staleTime: 5 * 60_000,
  });
}

/** Normalised so "Dune", " dune " and "DUNE" share one cache entry. */
export function normalizeQuery(q: string) {
  return q.trim().replace(/\s+/g, " ");
}

/** TMDB title search of movies or series (GET /api/v1/search/{movies,tv}),
 *  paged like popular. */
export function useSearchTitles(query: string, mode: MediaType = "movie", enabled: boolean = true) {
  const q = normalizeQuery(query);
  const path = mode === "tv" ? "tv" : "movies";
  return useInfiniteQuery({
    queryKey: ["search", path, q.toLowerCase()],
    queryFn: ({ pageParam, signal }) =>
      apiFetch<MoviePage>(`/api/v1/search/${path}?q=${encodeURIComponent(q)}&page=${pageParam}`, {
        auth: false,
        signal,
      }),
    initialPageParam: 1,
    getNextPageParam: (last) => (last.page < last.total_pages ? last.page + 1 : undefined),
    enabled: enabled && q.length >= 2,
    staleTime: 5 * 60_000,
  });
}

// --- Discover feed ----------------------------------------------------------

/** TMDB caps /discover at 500 pages. */
const DISCOVER_MAX_PAGE = 500;
/** A fresh feed starts on one of the first pages (every genre has at least
 *  this many: the least populated has about a dozen). */
const DISCOVER_START_PAGES = 8;

/**
 * The Discover feed (GET /api/v1/discover/{movies,tv}): popular movies or
 * series, one genre or all (genre 0; TV has its own genre ids). It starts on a random page and walks on from there,
 * wrapping at the end, and each page is shuffled, so every visit feels new
 * while the catalog still serves cached pages. The start is fixed per mount
 * (the `seed`), so refetches and "load more" stay consistent.
 */
export function useDiscover(mode: MediaType, genreId: number, seed: number, enabled: boolean = true) {
  const start = 1 + (seed % DISCOVER_START_PAGES);
  const path = mode === "tv" ? "tv" : "movies";
  return useInfiniteQuery({
    queryKey: ["discover", path, genreId, seed],
    queryFn: async ({ pageParam, signal }) => {
      const page = await apiFetch<MoviePage>(`/api/v1/discover/${path}?genre=${genreId}&page=${pageParam}`, {
        auth: false,
        signal,
      });
      return { ...page, results: shuffle(page.results, seed + page.page) };
    },
    initialPageParam: start,
    enabled,
    getNextPageParam: (last, pages) => {
      const total = Math.min(last.total_pages, DISCOVER_MAX_PAGE);
      if (pages.length >= total) return undefined; // seen every page
      return last.page >= total ? 1 : last.page + 1;
    },
    staleTime: 10 * 60_000,
  });
}

/** Deterministic Fisher-Yates (mulberry32), so a refetch keeps the order. */
function shuffle<T>(items: T[], seed: number): T[] {
  const out = [...items];
  let a = seed >>> 0;
  const rand = () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
  for (let i = out.length - 1; i > 0; i--) {
    const j = Math.floor(rand() * (i + 1));
    [out[i], out[j]] = [out[j], out[i]];
  }
  return out;
}

// --- TV series ----------------------------------------------------------------

export function useTVDetails(showId: number, enabled: boolean = true) {
  return useQuery({
    queryKey: ["tv", showId],
    queryFn: () => apiFetch<Movie>(`/api/v1/tv/${showId}`, { auth: false }),
    enabled,
  });
}

// --- Movie details + CQRS interactions -------------------------------------

export function useMovieDetails(movieId: number, enabled: boolean = true) {
  return useQuery({
    queryKey: ["movie", movieId],
    queryFn: () => apiFetch<Movie>(`/api/v1/movies/${movieId}`, { auth: false }),
    enabled,
  });
}

/** Details of a movie or a series, whichever `title` is. */
export function useTitleDetails(title: Pick<Movie, "id" | "media_type">, enabled: boolean = true) {
  const tv = title.media_type === "tv";
  return useQuery({
    queryKey: [tv ? "tv" : "movie", title.id],
    queryFn: () => apiFetch<Movie>(tv ? `/api/v1/tv/${title.id}` : `/api/v1/movies/${title.id}`, { auth: false }),
    enabled,
  });
}

function interactionsKey(title: TitleRef) {
  return ["interactions", titleKey(title)] as const;
}

/** GET /api/v1/{movies,tv}/{id}/interactions — the CQRS read model
 *  (Redis-backed, public), for a movie or a series. Ratings and comments posted through useRateMovie/useComment are
 *  applied to this cache optimistically; see those hooks for why. */
export function useInteractions(title: TitleRef, enabled: boolean = true) {
  return useQuery({
    queryKey: interactionsKey(title),
    queryFn: () => apiFetch<Interactions>(`${titleApiPath(title)}/interactions`, { auth: false }),
    enabled,
    // The read model itself updates within ~1s of a write (outbox relay poll
    // + RabbitMQ delivery); no need to poll faster than that client-side.
    staleTime: 5_000,
  });
}

/**
 * The CQRS write path: POST /rate returns 202 the instant the event is
 * durably queued (the outbox), well before RabbitMQ delivers it to the
 * projector and the Redis read model catches up. onMutate applies the vote
 * to the local cache immediately so the UI never shows that gap; onSettled
 * reconciles with the real value once the pipeline has plausibly caught up,
 * and onError rolls the optimistic change back if the write was rejected
 * outright (e.g. the outbox insert itself failed).
 */
export function useRateMovie(movie: Movie) {
  const queryClient = useQueryClient();
  const saveMyRating = useLibraryStore((s) => s.rate);
  return useMutation({
    // Two writes, both idempotent (so a retry is safe): the user's own
    // rating (library, Postgres, read back as "You rated 8/10" and on their
    // profile) and the vote in the community average (Interaction, CQRS).
    mutationFn: async (score: number) => {
      const [, accepted] = await Promise.all([
        saveMyRating(movie, score),
        apiFetch<{ event_id: string }>(`${titleApiPath(movie)}/rate`, {
          method: "POST",
          body: { score },
        }),
      ]);
      return accepted;
    },
    onMutate: async (score) => {
      await queryClient.cancelQueries({ queryKey: interactionsKey(movie) });
      const previous = queryClient.getQueryData<Interactions>(interactionsKey(movie));
      queryClient.setQueryData<Interactions>(interactionsKey(movie), (old) => {
        if (!old) return old;
        // We can't know client-side whether this replaces an earlier vote
        // from the same user (that dedup happens server-side) — optimistic
        // display treats it as a new vote; the reconcile below corrects the
        // count if it was actually a re-rate.
        const total = old.total_votes + 1;
        const average = (old.average_rating * old.total_votes + score) / total;
        return { ...old, average_rating: average, total_votes: total };
      });
      return { previous };
    },
    onError: (_err, _score, context) => {
      if (context?.previous) queryClient.setQueryData(interactionsKey(movie), context.previous);
    },
    onSettled: () => {
      setTimeout(() => queryClient.invalidateQueries({ queryKey: interactionsKey(movie) }), 1500);
    },
  });
}

/** An optimistic comment carries a client-generated id and a flag the UI can
 *  use to show a "sending…" treatment until the write model confirms it. */
export interface OptimisticComment extends Comment {
  pending?: boolean;
}

export function useComment(title: TitleRef) {
  const queryClient = useQueryClient();
  const userId = useAuthStore((s) => s.userId);

  return useMutation({
    mutationFn: (text: string) =>
      apiFetch<{ event_id: string }>(`${titleApiPath(title)}/comment`, {
        method: "POST",
        body: { text },
      }),
    onMutate: async (text) => {
      await queryClient.cancelQueries({ queryKey: interactionsKey(title) });
      const previous = queryClient.getQueryData<Interactions>(interactionsKey(title));
      const optimistic: OptimisticComment = {
        id: `optimistic-${Date.now()}`,
        user_id: userId ?? "you",
        text,
        created_at: new Date().toISOString(),
        pending: true,
      };
      queryClient.setQueryData<Interactions>(interactionsKey(title), (old) =>
        old ? { ...old, recent_comments: [optimistic, ...old.recent_comments] } : old,
      );
      return { previous };
    },
    onError: (_err, _text, context) => {
      if (context?.previous) queryClient.setQueryData(interactionsKey(title), context.previous);
    },
    onSettled: () => {
      setTimeout(() => queryClient.invalidateQueries({ queryKey: interactionsKey(title) }), 1500);
    },
  });
}
