import { create } from "zustand";
import { persist } from "zustand/middleware";

import type { MediaType } from "@/types/movie";

interface FeedState {
  isMuted: boolean;
  toggleMute: () => void;
  setMuted: (muted: boolean) => void;
  /** The Discover feed's genre filter per mode (TMDB genre ids differ
   *  between movies and TV; 0 = all). */
  genres: Record<MediaType, number>;
  setGenre: (mode: MediaType, id: number) => void;
}

export const useFeedStore = create<FeedState>()(
  persist(
    (set) => ({
      isMuted: true,
      toggleMute: () => set((state) => ({ isMuted: !state.isMuted })),
      setMuted: (isMuted) => set({ isMuted }),
      genres: { movie: 0, tv: 0 },
      setGenre: (mode, id) => set((s) => ({ genres: { ...s.genres, [mode]: id } })),
    }),
    {
      name: "movieapp-feed-storage",
      // v1: one genre per mode (movie and TV genre ids differ) replaced the
      // single genreId; keep the mute preference, start genres fresh.
      version: 1,
      migrate: (persisted, version) => {
        const old = (persisted ?? {}) as { isMuted?: boolean };
        return version < 1 ? { isMuted: old.isMuted ?? true, genres: { movie: 0, tv: 0 } } : (persisted as FeedState);
      },
    }
  )
);
