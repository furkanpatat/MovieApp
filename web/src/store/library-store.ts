"use client";

import { useEffect } from "react";
import { create } from "zustand";
import { useShallow } from "zustand/react/shallow";

import { ApiError, apiFetch } from "@/lib/api-client";
import { mediaTypeOf, titleKey, type TitleRef } from "@/lib/media";
import { useAuthStore } from "@/store/auth-store";
import type { LibraryMovie, ListResponse, UserRating, WatchlistItem } from "@/types/library";
import type { Movie } from "@/types/movie";

/**
 * The signed-in user's library: their list ("My List") and the ratings
 * they've given. Postgres (via the Catalog service) is the source of truth;
 * this store is an in-memory mirror of it for the current session.
 *
 * - Loaded once per sign-in (useLibrarySync, mounted in providers.tsx) and
 *   cleared on sign-out, so accounts sharing a browser never see each other's.
 * - Mutations are optimistic: the UI updates at once, the API call follows,
 *   and a failed call rolls that one change back and rethrows.
 * - The server derives the user from the HttpOnly session cookie; no call
 *   here ever names a user id.
 */

/** The slice of a Movie a card needs. */
export type SavedMovie = LibraryMovie;

export interface ListEntry {
  movie: SavedMovie;
  addedAt: number;
}

export interface RatingEntry {
  movie: SavedMovie;
  score: number;
  ratedAt: number;
}

export type LibraryStatus = "idle" | "loading" | "ready" | "error";

interface LibraryState {
  /** Whose library is loaded (null when signed out). */
  userId: string | null;
  status: LibraryStatus;
  list: ListEntry[];
  /** By titleKey ("movie-27205", "tv-1399"): ids repeat across media types. */
  ratings: Record<string, RatingEntry>;
  load: (userId: string) => Promise<void>;
  reset: () => void;
  /** Adds the title (movie or series) to the list, or removes it;
   *  resolves to the new state. */
  toggleSaved: (movie: Movie) => Promise<boolean>;
  /** Creates or replaces the user's rating for the title. */
  rate: (movie: Movie, score: number) => Promise<void>;
}

const empty = { userId: null, status: "idle" as const, list: [], ratings: {} };

export function toSaved(m: Movie | SavedMovie): SavedMovie {
  const { id, title, overview, poster_path, backdrop_path, release_date, vote_average, vote_count, imdb_rating } = m;
  return { id, media_type: mediaTypeOf(m), title, overview, poster_path, backdrop_path, release_date, vote_average, vote_count, imdb_rating };
}

const fromWatchlist = (w: WatchlistItem): ListEntry => ({ movie: w.movie, addedAt: Date.parse(w.added_at) });
const fromRating = (r: UserRating): RatingEntry => ({
  movie: r.movie,
  score: r.rating,
  ratedAt: Date.parse(r.updated_at),
});

// Bumped on every load/reset so a slow response for a previous session is
// dropped instead of overwriting the current one.
let generation = 0;
// Titles (titleKey) with a list toggle in flight: a double click must not
// race an add against a remove on the server.
const toggling = new Set<string>();

/** What the library API takes to name a title. */
const titleBody = (m: Movie) => ({ media_type: mediaTypeOf(m), movie_id: m.id });
const sameTitle = (a: TitleRef) => (b: TitleRef) => titleKey(a) === titleKey(b);

export const useLibraryStore = create<LibraryState>()((set, get) => ({
  ...empty,

  load: async (userId) => {
    const gen = ++generation;
    set({ ...empty, userId, status: "loading" });
    await importLegacyLibrary(userId);
    try {
      const [list, ratings] = await Promise.all([
        apiFetch<ListResponse<WatchlistItem>>("/api/v1/watchlist"),
        apiFetch<ListResponse<UserRating>>("/api/v1/ratings"),
      ]);
      if (gen !== generation) return;
      set({
        status: "ready",
        list: list.items.map(fromWatchlist),
        ratings: Object.fromEntries(ratings.items.map((r) => [titleKey(r.movie), fromRating(r)])),
      });
    } catch {
      if (gen === generation) set({ status: "error" });
    }
  },

  reset: () => {
    generation++;
    toggling.clear();
    set(empty);
  },

  toggleSaved: async (movie) => {
    const { userId, list } = get();
    const key = titleKey(movie);
    const existing = list.find((e) => sameTitle(movie)(e.movie));
    const saved = existing !== undefined;
    if (!userId || toggling.has(key)) return saved;

    toggling.add(key);
    const entry: ListEntry = existing ?? { movie: toSaved(movie), addedAt: Date.now() };
    const without = (l: ListEntry[]) => l.filter((e) => !sameTitle(movie)(e.movie));
    const current = () => get().userId === userId;

    set({ list: saved ? without(list) : [entry, ...list] });
    try {
      if (saved) {
        await apiFetch<void>(`/api/v1/watchlist/${movie.id}?media_type=${mediaTypeOf(movie)}`, { method: "DELETE" });
      } else {
        const item = await apiFetch<WatchlistItem>("/api/v1/watchlist", { method: "POST", body: titleBody(movie) });
        if (current()) set({ list: get().list.map((e) => (sameTitle(movie)(e.movie) ? fromWatchlist(item) : e)) });
      }
      return !saved;
    } catch (err) {
      if (current()) set({ list: saved ? [entry, ...without(get().list)] : without(get().list) });
      throw err;
    } finally {
      toggling.delete(key);
    }
  },

  rate: async (movie, score) => {
    const { userId } = get();
    if (!userId) return;
    const key = titleKey(movie);
    const previous = get().ratings[key];
    const current = () => get().userId === userId;
    const put = (entry: RatingEntry | undefined) => {
      const ratings = { ...get().ratings };
      if (entry) ratings[key] = entry;
      else delete ratings[key];
      set({ ratings });
    };

    put({ movie: toSaved(movie), score, ratedAt: Date.now() });
    try {
      const saved = await apiFetch<UserRating>("/api/v1/ratings", {
        method: "PUT",
        body: { ...titleBody(movie), rating: score },
      });
      if (current()) put(fromRating(saved));
    } catch (err) {
      if (current()) put(previous);
      throw err;
    }
  },
}));

/** The current user's library (empty while signed out or loading). */
export function useUserLibrary() {
  return useLibraryStore(useShallow(({ list, ratings, status }) => ({ list, ratings, status })));
}

/** Loads the library when someone signs in and clears it when they sign out
 *  (including when a 401 ends the session). Mount once, in providers.tsx. */
export function useLibrarySync() {
  const hasHydrated = useAuthStore((s) => s.hasHydrated);
  const userId = useAuthStore((s) => s.userId);
  useEffect(() => {
    if (!hasHydrated) return;
    const { load, reset, userId: loaded } = useLibraryStore.getState();
    if (!userId) reset();
    else if (userId !== loaded) void load(userId);
  }, [hasHydrated, userId]);
}

// --- one-time import of the pre-backend, browser-only library -------------

const LEGACY_KEY = "movieapp-library";

interface LegacyLibrary {
  state?: {
    byUser?: Record<
      string,
      { list?: { movie: { id: number } }[]; ratings?: Record<string, { movie: { id: number }; score: number }> }
    >;
  };
}

/**
 * Before the API existed the library lived in localStorage. Uploads this
 * user's part of it once, then deletes it from the browser. Failures that
 * a retry won't fix (e.g. a movie TMDB no longer has) don't block that;
 * network or server errors leave it for the next sign-in.
 */
async function importLegacyLibrary(userId: string) {
  let legacy: LegacyLibrary;
  try {
    const raw = localStorage.getItem(LEGACY_KEY);
    if (!raw) return;
    legacy = JSON.parse(raw) as LegacyLibrary;
  } catch {
    return;
  }
  const mine = legacy.state?.byUser?.[userId];
  if (!mine) return;

  const failures: unknown[] = [];
  const attempt = (p: Promise<unknown>) => p.catch((err: unknown) => void failures.push(err));
  // One at a time, oldest first, so the server's added_at order matches the old list.
  for (const e of [...(mine.list ?? [])].reverse()) {
    await attempt(apiFetch("/api/v1/watchlist", { method: "POST", body: { movie_id: e.movie.id } }));
  }
  await Promise.all(
    Object.values(mine.ratings ?? {}).map((r) =>
      attempt(apiFetch("/api/v1/ratings", { method: "PUT", body: { movie_id: r.movie.id, rating: r.score } })),
    ),
  );
  const retryLater = failures.some((err) => !(err instanceof ApiError) || err.status === 0 || err.status >= 500);
  if (retryLater) return;

  try {
    delete legacy.state!.byUser![userId];
    if (Object.keys(legacy.state!.byUser!).length === 0) localStorage.removeItem(LEGACY_KEY);
    else localStorage.setItem(LEGACY_KEY, JSON.stringify(legacy));
  } catch {
    // Storage unavailable: the import is idempotent, so a repeat is harmless.
  }
}
