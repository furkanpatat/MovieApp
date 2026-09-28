import { create } from "zustand";

/** Discover's sound: one setting for the whole feed, on by default (the
 *  app, unlike a browser, may autoplay with sound). */
export const useFeed = create<{ muted: boolean; toggleMuted: () => void }>((set) => ({
  muted: false,
  toggleMuted: () => set((s) => ({ muted: !s.muted })),
}));
