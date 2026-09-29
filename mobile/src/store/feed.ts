import { create } from "zustand";

import type { MediaType } from "@/types/movie";

interface FeedState {
  /** Discover's sound: one setting for the whole feed, on by default (the
   *  app, unlike a browser, may autoplay with sound). */
  muted: boolean;
  toggleMuted: () => void;
  /** Discover's genre filter per mode (movie and TV genre ids differ; 0 = all). */
  genres: Record<MediaType, number>;
  setGenre: (mode: MediaType, id: number) => void;
}

export const useFeed = create<FeedState>((set) => ({
  muted: false,
  toggleMuted: () => set((s) => ({ muted: !s.muted })),
  genres: { movie: 0, tv: 0 },
  setGenre: (mode, id) => set((s) => ({ genres: { ...s.genres, [mode]: id } })),
}));
