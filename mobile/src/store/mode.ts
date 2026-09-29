import { create } from "zustand";

import type { MediaType } from "@/types/movie";

/** Movies or series: the logo switch (KinoCut ⇄ KinoShow), app-wide. */
export const useMode = create<{ mode: MediaType; toggle: () => void }>((set) => ({
  mode: "movie",
  toggle: () => set((s) => ({ mode: s.mode === "movie" ? "tv" : "movie" })),
}));
