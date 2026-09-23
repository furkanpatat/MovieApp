"use client";

import { create } from "zustand";

export type AuthMode = "sign-in" | "register";

/**
 * Drives the single app-wide AuthDialog (mounted once in Providers), so any
 * gated action — rate, save, comment — can ask the user to sign in without
 * each button owning its own dialog instance.
 */
interface AuthPromptState {
  open: boolean;
  mode: AuthMode;
  openAuth: (mode?: AuthMode) => void;
  setOpen: (open: boolean) => void;
  setMode: (mode: AuthMode) => void;
}

export const useAuthPrompt = create<AuthPromptState>()((set) => ({
  open: false,
  mode: "sign-in",
  openAuth: (mode = "sign-in") => set({ open: true, mode }),
  setOpen: (open) => set({ open }),
  setMode: (mode) => set({ mode }),
}));
