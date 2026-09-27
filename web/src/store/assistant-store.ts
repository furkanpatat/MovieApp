"use client";

import { create } from "zustand";

import type { ChatTurn } from "@/types/assistant";

/** The assistant panel: open state and the conversation (in memory only;
 *  it resets on reload and whenever the signed-in user changes). */
interface AssistantState {
  open: boolean;
  turns: ChatTurn[];
  /** Bumped by reset, so a reply to an earlier conversation is dropped. */
  conversation: number;
  /** The server answered with canned replies (no LLM configured). */
  demo: boolean;
  setDemo: (demo: boolean) => void;
  setOpen: (open: boolean) => void;
  toggle: () => void;
  add: (turn: ChatTurn) => void;
  remove: (id: string) => void;
  reset: () => void;
}

export const useAssistantStore = create<AssistantState>()((set) => ({
  open: false,
  turns: [],
  conversation: 0,
  demo: false,
  setDemo: (demo) => set({ demo }),
  setOpen: (open) => set({ open }),
  toggle: () => set((s) => ({ open: !s.open })),
  add: (turn) => set((s) => ({ turns: [...s.turns, turn] })),
  remove: (id) => set((s) => ({ turns: s.turns.filter((t) => t.id !== id) })),
  reset: () => set((s) => ({ turns: [], conversation: s.conversation + 1 })),
}));
