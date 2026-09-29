import { create } from "zustand";

/** Immersive viewing (Discover on its side): the tab bar and status bar
 *  hide until the screen is tapped. */
export const useChrome = create<{ immersive: boolean; setImmersive: (on: boolean) => void }>((set) => ({
  immersive: false,
  setImmersive: (immersive) => set({ immersive }),
}));
