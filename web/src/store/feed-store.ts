import { create } from "zustand";
import { persist } from "zustand/middleware";

interface FeedState {
  isMuted: boolean;
  toggleMute: () => void;
}

export const useFeedStore = create<FeedState>()(
  persist(
    (set) => ({
      isMuted: true,
      toggleMute: () => set((state) => ({ isMuted: !state.isMuted })),
    }),
    {
      name: "movieapp-feed-storage",
    }
  )
);
