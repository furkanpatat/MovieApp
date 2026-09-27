"use client";

import { useSyncExternalStore } from "react";
import { create } from "zustand";
import { persist } from "zustand/middleware";

import type { MediaType } from "@/types/movie";

/**
 * The app-wide mode, switched by the logo: KinoCut ("movie") or KinoShow
 * ("tv"). Home, Discover and Search follow it; a title's own page doesn't
 * (its URL says which it is). Kept in localStorage.
 */
interface MediaModeState {
  mode: MediaType;
  toggle: () => void;
  setMode: (mode: MediaType) => void;
}

export const useMediaModeStore = create<MediaModeState>()(
  persist(
    (set) => ({
      mode: "movie",
      toggle: () => set((s) => ({ mode: s.mode === "movie" ? "tv" : "movie" })),
      setMode: (mode) => set({ mode }),
    }),
    { name: "movieapp-media-mode" },
  ),
);

const subscribeHydration = (onChange: () => void) => useMediaModeStore.persist.onFinishHydration(onChange);
const hydrated = () => useMediaModeStore.persist.hasHydrated();

/**
 * The mode, and whether it has been read from storage yet. Queries wait for
 * `ready` rather than fetching movies for a returning series fan first (the
 * server render, and the first client render, only know the default).
 */
export function useMediaMode() {
  const mode = useMediaModeStore((s) => s.mode);
  const ready = useSyncExternalStore(subscribeHydration, hydrated, () => false);
  return { mode, ready };
}
