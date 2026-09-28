import * as SecureStore from "expo-secure-store";
import { create } from "zustand";

const KEY = "kinocut.session";

interface Session {
  token: string;
  userId: string;
  username: string;
  /** Unix ms; the gateway's JWTs are short-lived. */
  expiresAt: number;
}

interface AuthState {
  token: string | null;
  userId: string | null;
  username: string | null;
  /** The saved session has been read (don't show "signed out" before). */
  ready: boolean;
  restore: () => Promise<void>;
  signIn: (s: Session) => Promise<void>;
  signOut: () => Promise<void>;
}

/**
 * The session. The token lives in the Keychain (iOS) / Keystore (Android)
 * via expo-secure-store, never in plain storage.
 */
export const useAuth = create<AuthState>((set) => ({
  token: null,
  userId: null,
  username: null,
  ready: false,
  restore: async () => {
    try {
      const raw = await SecureStore.getItemAsync(KEY);
      const s = raw ? (JSON.parse(raw) as Session) : null;
      if (s && s.expiresAt > Date.now()) set({ token: s.token, userId: s.userId, username: s.username });
      else if (s) await SecureStore.deleteItemAsync(KEY);
    } catch {
      // Unreadable: start signed out.
    }
    set({ ready: true });
  },
  signIn: async (s) => {
    await SecureStore.setItemAsync(KEY, JSON.stringify(s));
    set({ token: s.token, userId: s.userId, username: s.username });
  },
  signOut: async () => {
    await SecureStore.deleteItemAsync(KEY);
    set({ token: null, userId: null, username: null });
  },
}));
