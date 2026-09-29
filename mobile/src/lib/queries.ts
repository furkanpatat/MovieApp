import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api } from "@/lib/api";
import { toSession, type SessionResponse } from "@/lib/session";
import { useAuth } from "@/store/auth";
import type { ListResponse, UserRating, WatchedItem, WatchlistItem } from "@/types/library";
import type { MediaType, Movie, MoviePage } from "@/types/movie";

const seg = (m: MediaType) => (m === "tv" ? "tv" : "movies");

/** Popular movies or series: GET /api/v1/{movies,tv}/popular. */
export function usePopular(mode: MediaType) {
  return useQuery({
    queryKey: [seg(mode), "popular"],
    queryFn: () => api<MoviePage>(`/api/v1/${seg(mode)}/popular?page=1`, { auth: false }),
    staleTime: 60_000,
  });
}

/** A genre's feed: GET /api/v1/discover/{movies,tv}?genre=. */
export function useGenre(mode: MediaType, genre: number) {
  return useQuery({
    queryKey: ["discover", mode, genre],
    queryFn: () => api<MoviePage>(`/api/v1/discover/${seg(mode)}?genre=${genre}&page=1`, { auth: false }),
    staleTime: 5 * 60_000,
  });
}

/** One title's details (cast, trailer, IMDb): GET /api/v1/{movies,tv}/{id}. */
export function useTitle(mode: MediaType, id: number, enabled = true) {
  return useQuery({
    queryKey: [seg(mode), id],
    queryFn: () => api<Movie>(`/api/v1/${seg(mode)}/${id}`, { auth: false }),
    enabled: enabled && Number.isInteger(id) && id > 0,
    staleTime: 10 * 60_000,
  });
}

/** Search: GET /api/v1/search/{movies,tv}?q=. */
export function useSearch(mode: MediaType, q: string) {
  const query = q.trim();
  return useQuery({
    queryKey: ["search", mode, query],
    queryFn: () => api<MoviePage>(`/api/v1/search/${seg(mode)}?q=${encodeURIComponent(query)}&page=1`, { auth: false }),
    enabled: query.length >= 2,
    staleTime: 60_000,
  });
}


/** Sign in (or sign up, then in): the token goes to the Keychain. */
export function useSignIn() {
  const signIn = useAuth((s) => s.signIn);
  return useMutation({
    mutationFn: async (input: { login: string; password: string; register?: { username: string; email: string } }) => {
      if (input.register) {
        await api("/api/v1/auth/register", { method: "POST", auth: false, body: { ...input.register, password: input.password } });
      }
      // refresh: a ~30-day refresh token too, so the phone stays signed in.
      return api<SessionResponse>("/api/v1/auth/login", {
        method: "POST",
        auth: false,
        body: { login: input.register?.username ?? input.login, password: input.password, refresh: true },
      });
    },
    onSuccess: (r) => signIn(toSession(r)),
  });
}

/** TMDB caps /discover at 500 pages. */
const DISCOVER_MAX_PAGE = 500;
/** A feed starts on one of the first pages (every genre has this many). */
const DISCOVER_START_PAGES = 8;

/**
 * The Discover feed (GET /api/v1/discover/{movies,tv}; genre 0 = popular),
 * as on the web: it starts on a random page and walks on from there,
 * wrapping at the end, and each page is shuffled, so the feed isn't the
 * same every time while the catalog still serves cached pages. `seed`
 * fixes the start and the order until the user asks for a new feed.
 */
export function useDiscoverFeed(mode: MediaType, seed: number, genre = 0) {
  return useInfiniteQuery({
    queryKey: ["feed", mode, genre, seed],
    queryFn: async ({ pageParam }) => {
      const page = await api<MoviePage>(`/api/v1/discover/${seg(mode)}?genre=${genre}&page=${pageParam}`, { auth: false });
      return { ...page, results: shuffle(page.results, seed + page.page) };
    },
    initialPageParam: 1 + (seed % DISCOVER_START_PAGES),
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

/** The signed-in user's ratings (GET /api/v1/ratings): what they liked. */
export function useMyRatings() {
  const token = useAuth((s) => s.token);
  return useQuery({
    queryKey: ["me", "ratings", token],
    queryFn: () => api<ListResponse<UserRating>>("/api/v1/ratings"),
    enabled: !!token,
    staleTime: 60_000,
  });
}

/**
 * Like = rate 10, as on the web: the user's own rating (their library) and
 * the public vote (the CQRS write path, 202 at once).
 */
export function useLike() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (m: Movie & { media_type: MediaType }) => {
      await Promise.all([
        api("/api/v1/ratings", { method: "PUT", body: { media_type: m.media_type, movie_id: m.id, rating: 10 } }),
        api(`/api/v1/${seg(m.media_type)}/${m.id}/rate`, { method: "POST", body: { score: 10 } }),
      ]);
    },
    onSettled: () => client.invalidateQueries({ queryKey: ["me", "ratings"] }),
  });
}

/** The signed-in user's library: My List and Watched. */
export function useLibrary() {
  const token = useAuth((s) => s.token);
  const list = useQuery({
    queryKey: ["me", "watchlist", token],
    queryFn: () => api<ListResponse<WatchlistItem>>("/api/v1/watchlist"),
    enabled: !!token,
  });
  const watched = useQuery({
    queryKey: ["me", "watched", token],
    queryFn: () => api<ListResponse<WatchedItem>>("/api/v1/watched"),
    enabled: !!token,
  });
  return { list, watched };
}

/** Add to or remove from My List / Watched (same endpoints as the web). */
export function useToggleLibrary(kind: "watchlist" | "watched") {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ movie, media, on }: { movie: Movie; media: MediaType; on: boolean }) =>
      on
        ? api(`/api/v1/${kind}`, { method: "POST", body: { media_type: media, movie_id: movie.id } })
        : api(`/api/v1/${kind}/${movie.id}?media_type=${media}`, { method: "DELETE" }),
    onSettled: () => client.invalidateQueries({ queryKey: ["me", kind] }),
  });
}

export interface ChatMessage {
  role: "user" | "assistant";
  content: string;
}

export interface ChatReply {
  message: string;
  movies: Movie[];
  /** The model was busy: picks from keywords instead. */
  fallback?: boolean;
  demo?: boolean;
}

/** The AI concierge: POST /api/v1/chat (signed in), in the user's language. */
export function useChat() {
  return useMutation({
    mutationFn: (body: { messages: ChatMessage[]; locale: "en" | "tr" }) =>
      api<ChatReply>("/api/v1/chat", { method: "POST", body }),
    retry: false,
  });
}
