"use client";

import { useCallback } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";

import { useAuthPrompt, type AuthMode } from "@/store/auth-prompt-store";
import { useAuthStore } from "@/store/auth-store";
import { currentLocale, translate } from "@/i18n";
import { ApiError } from "@/lib/api-client";
import { titleKey, type TitleRef } from "@/lib/media";
import { useLibraryStore, useUserLibrary } from "@/store/library-store";
import type { Movie } from "@/types/movie";

/**
 * Gate for anything that needs an account: runs `action` when signed in,
 * otherwise opens the app-wide sign-in dialog instead.
 *
 *   const requireAuth = useRequireAuth();
 *   <button onClick={() => requireAuth(() => rate.mutate(10))}>
 */
export function useRequireAuth() {
  const isAuthed = useAuthStore((s) => s.hasHydrated && s.username !== null);
  const openAuth = useAuthPrompt((s) => s.openAuth);
  return useCallback(
    (action: () => void, mode: AuthMode = "sign-in") => {
      if (isAuthed) action();
      else openAuth(mode);
    },
    [isAuthed, openAuth],
  );
}

/** "Add to My List" state and a gated toggle for one movie. */
export function useSavedToggle(movie: Movie) {
  const router = useRouter();
  const key = titleKey(movie);
  const saved = useUserLibrary().list.some((e) => titleKey(e.movie) === key);
  const toggleSaved = useLibraryStore((s) => s.toggleSaved);
  const requireAuth = useRequireAuth();

  // The store flips `saved` at once; the toast waits for the server.
  const toggle = () =>
    requireAuth(async () => {
      try {
        const nowSaved = await toggleSaved(movie);
        const locale = currentLocale();
        toast(translate(locale, nowSaved ? "common.addedToList" : "common.removedFromList", { title: movie.title }), {
          id: `list-${movie.id}`,
          action: nowSaved ? { label: translate(locale, "common.view"), onClick: () => router.push("/my-list") } : undefined,
        });
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) return; // api-client already said the session ended
        toast.error(`${translate(currentLocale(), "common.listUpdateFailed")} ${err instanceof ApiError ? err.message : ""}`.trim(), {
          id: `list-${movie.id}`,
        });
      }
    });

  return { saved, toggle };
}

/** "Watched" state and a gated toggle for one title (movie or series). */
export function useWatchedToggle(movie: Movie) {
  const key = titleKey(movie);
  const watched = useUserLibrary().watched.some((e) => titleKey(e.movie) === key);
  const toggleWatched = useLibraryStore((s) => s.toggleWatched);
  const requireAuth = useRequireAuth();

  const toggle = () =>
    requireAuth(async () => {
      try {
        const now = await toggleWatched(movie);
        toast(translate(currentLocale(), now ? "common.markedWatched" : "common.unmarkedWatched", { title: movie.title }), {
          id: `watched-${key}`,
        });
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) return;
        toast.error(translate(currentLocale(), "common.watchedUpdateFailed"), { id: `watched-${key}` });
      }
    });

  return { watched, toggle };
}

/** The score the current user gave this movie, if they have rated it. */
export function useMyRating(title: TitleRef): number | undefined {
  const key = titleKey(title);
  return useLibraryStore((s) => s.ratings[key]?.score);
}
