import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { apiFetch } from "@/lib/api-client";
import { useAuthStore } from "@/store/auth-store";
import { useLibraryStore } from "@/store/library-store";
import type { Comment, Interactions, Movie, MoviePage, Person } from "@/types/movie";

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

/** TMDB title search (GET /api/v1/search/movies), paged like popular. */
export function useSearchMovies(query: string) {
  const q = normalizeQuery(query);
  return useInfiniteQuery({
    queryKey: ["search", q.toLowerCase()],
    queryFn: ({ pageParam, signal }) =>
      apiFetch<MoviePage>(`/api/v1/search/movies?q=${encodeURIComponent(q)}&page=${pageParam}`, {
        auth: false,
        signal,
      }),
    initialPageParam: 1,
    getNextPageParam: (last) => (last.page < last.total_pages ? last.page + 1 : undefined),
    enabled: q.length >= 2,
    staleTime: 5 * 60_000,
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

function interactionsKey(movieId: number) {
  return ["interactions", movieId] as const;
}

/** GET /api/v1/movies/{id}/interactions — the CQRS read model (Redis-backed,
 *  public). Ratings and comments posted through useRateMovie/useComment are
 *  applied to this cache optimistically; see those hooks for why. */
export function useInteractions(movieId: number) {
  return useQuery({
    queryKey: interactionsKey(movieId),
    queryFn: () => apiFetch<Interactions>(`/api/v1/movies/${movieId}/interactions`, { auth: false }),
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
export function useRateMovie(movieId: number, movie?: Movie) {
  const queryClient = useQueryClient();
  const saveMyRating = useLibraryStore((s) => s.rate);
  return useMutation({
    // Two writes, both idempotent (so a retry is safe): the user's own
    // rating (library, Postgres, read back as "You rated 8/10" and on their
    // profile) and the vote in the community average (Interaction, CQRS).
    mutationFn: async (score: number) => {
      const [, accepted] = await Promise.all([
        movie ? saveMyRating(movie, score) : undefined,
        apiFetch<{ event_id: string }>(`/api/v1/movies/${movieId}/rate`, {
          method: "POST",
          body: { score },
        }),
      ]);
      return accepted;
    },
    onMutate: async (score) => {
      await queryClient.cancelQueries({ queryKey: interactionsKey(movieId) });
      const previous = queryClient.getQueryData<Interactions>(interactionsKey(movieId));
      queryClient.setQueryData<Interactions>(interactionsKey(movieId), (old) => {
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
      if (context?.previous) queryClient.setQueryData(interactionsKey(movieId), context.previous);
    },
    onSettled: () => {
      setTimeout(() => queryClient.invalidateQueries({ queryKey: interactionsKey(movieId) }), 1500);
    },
  });
}

/** An optimistic comment carries a client-generated id and a flag the UI can
 *  use to show a "sending…" treatment until the write model confirms it. */
export interface OptimisticComment extends Comment {
  pending?: boolean;
}

export function useComment(movieId: number) {
  const queryClient = useQueryClient();
  const userId = useAuthStore((s) => s.userId);

  return useMutation({
    mutationFn: (text: string) =>
      apiFetch<{ event_id: string }>(`/api/v1/movies/${movieId}/comment`, {
        method: "POST",
        body: { text },
      }),
    onMutate: async (text) => {
      await queryClient.cancelQueries({ queryKey: interactionsKey(movieId) });
      const previous = queryClient.getQueryData<Interactions>(interactionsKey(movieId));
      const optimistic: OptimisticComment = {
        id: `optimistic-${Date.now()}`,
        user_id: userId ?? "you",
        text,
        created_at: new Date().toISOString(),
        pending: true,
      };
      queryClient.setQueryData<Interactions>(interactionsKey(movieId), (old) =>
        old ? { ...old, recent_comments: [optimistic, ...old.recent_comments] } : old,
      );
      return { previous };
    },
    onError: (_err, _text, context) => {
      if (context?.previous) queryClient.setQueryData(interactionsKey(movieId), context.previous);
    },
    onSettled: () => {
      setTimeout(() => queryClient.invalidateQueries({ queryKey: interactionsKey(movieId) }), 1500);
    },
  });
}
