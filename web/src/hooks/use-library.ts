"use client";

import { useCallback } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";

import { useAuthPrompt, type AuthMode } from "@/store/auth-prompt-store";
import { useAuthStore } from "@/store/auth-store";
import { ApiError } from "@/lib/api-client";
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
  const saved = useUserLibrary().list.some((e) => e.movie.id === movie.id);
  const toggleSaved = useLibraryStore((s) => s.toggleSaved);
  const requireAuth = useRequireAuth();

  // The store flips `saved` at once; the toast waits for the server.
  const toggle = () =>
    requireAuth(async () => {
      try {
        const nowSaved = await toggleSaved(movie);
        toast(nowSaved ? `Added “${movie.title}” to My List` : `Removed “${movie.title}” from My List`, {
          id: `list-${movie.id}`,
          action: nowSaved ? { label: "View", onClick: () => router.push("/my-list") } : undefined,
        });
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) return; // api-client already said the session ended
        toast.error(`Couldn’t update My List. ${err instanceof ApiError ? err.message : ""}`.trim(), {
          id: `list-${movie.id}`,
        });
      }
    });

  return { saved, toggle };
}

/** The score the current user gave this movie, if they have rated it. */
export function useMyRating(movieId: number): number | undefined {
  return useLibraryStore((s) => s.ratings[movieId]?.score);
}
