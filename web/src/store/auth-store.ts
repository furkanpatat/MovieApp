"use client";

import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";

/**
 * Who is signed in, for display only. The credential itself is the HttpOnly
 * session cookie the Auth service sets at login: JS can't read it (so XSS
 * can't steal it), the browser attaches it to API calls, and the Gateway
 * verifies it on every request. Nothing here is trusted for authorization.
 *
 * `expiresAt` mirrors the token's expiry so the UI stops claiming a session
 * the Gateway will reject: the session is dropped on reload once past it, and
 * a timer ends it the moment it lapses while the app is open.
 */
interface AuthState {
  userId: string | null;
  username: string | null;
  email: string | null;
  /** Epoch ms when the session cookie's token expires. */
  expiresAt: number | null;
  /** True once the persisted store has been read back from localStorage, so
   *  components can avoid rendering a guess (e.g. "Sign in") before we
   *  actually know. */
  hasHydrated: boolean;
  setSession: (session: { userId: string; username: string; email: string | null; expiresAt: number }) => void;
  clearSession: () => void;
}

const signedOut = { userId: null, username: null, email: null, expiresAt: null };

// setTimeout's delay is a 32-bit int; longer sessions are re-checked on reload.
const MAX_TIMER_MS = 2 ** 31 - 1;
let expiryTimer: ReturnType<typeof setTimeout> | undefined;

function scheduleExpiry(expiresAt: number | null) {
  clearTimeout(expiryTimer);
  if (expiresAt === null) return;
  const ms = expiresAt - Date.now();
  if (ms <= MAX_TIMER_MS) {
    expiryTimer = setTimeout(() => useAuthStore.getState().clearSession(), Math.max(0, ms));
  }
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      ...signedOut,
      hasHydrated: false,
      setSession: (session) => {
        set(session);
        scheduleExpiry(session.expiresAt);
      },
      clearSession: () => {
        set(signedOut);
        scheduleExpiry(null);
      },
    }),
    {
      name: "movieapp-auth",
      // v1 persisted the raw JWT in localStorage; migrating to v2 discards
      // it (and with it any session: the user signs in once more, which is
      // what sets the new cookie).
      version: 2,
      migrate: () => signedOut,
      storage: createJSONStorage(() => localStorage),
      partialize: ({ userId, username, email, expiresAt }) => ({ userId, username, email, expiresAt }),
      // Rehydrating from localStorage happens after the initial render, so
      // the very first paint (server and client) always shows "logged out".
      // Without this, the two can briefly disagree and React complains about
      // a hydration mismatch. onRehydrateStorage flips the flag once real
      // data (or its absence) is known.
      skipHydration: true,
      onRehydrateStorage: () => (state) => {
        const expiresAt = state?.expiresAt ?? null;
        if (expiresAt === null || expiresAt <= Date.now()) {
          useAuthStore.setState({ ...signedOut, hasHydrated: true });
        } else {
          useAuthStore.setState({ hasHydrated: true });
          scheduleExpiry(expiresAt);
        }
      },
    },
  ),
);

/** Call once on the client (see providers.tsx) to trigger the rehydration
 *  that `skipHydration: true` deferred. */
export function hydrateAuthStore() {
  void useAuthStore.persist.rehydrate();
}

export function isAuthenticated(state: AuthState): boolean {
  return state.username !== null;
}
