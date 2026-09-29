"use client";

import { create } from "zustand";

/** Whether a title modal (@modal intercepting route) is open, for what sits
 *  behind it: the Discover video pauses, the assistant steps aside. */
export const useTitleModalStore = create<{ open: boolean; setOpen: (open: boolean) => void }>()((set) => ({
  open: false,
  setOpen: (open) => set({ open }),
}));
